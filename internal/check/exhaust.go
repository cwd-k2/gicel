package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
)

// --- Maranget exhaustiveness + redundancy ---
//
// Reference: Luc Maranget, "Warnings for Pattern Matching" (JFP 2007).
//
// Core idea: a pattern vector q is "useful" w.r.t. a pattern matrix P iff
// there exists a value vector v matched by q but not by any row of P.
//
// Exhaustiveness: the wildcard vector (_, _, ...) is NOT useful w.r.t. the
// full matrix ⟹ all cases are covered.
//
// Redundancy: row i is useful w.r.t. rows 0..i-1 ⟹ row i contributes;
// otherwise it is redundant.

// ---- Pattern representation ----

// pat is the internal pattern representation for the algorithm.
type pat interface{ patTag() }

type pWild struct{}                          // _
type pCon struct {
	con   string
	arity int
	args  []pat
}                                            // C p1 ... pn
type pRecord struct{ fields map[string]pat } // { l1 = p1, ... }
type pLit struct{ value any }                // 42, "hello", 'x'

func (pWild) patTag()   {}
func (pCon) patTag()    {}
func (pRecord) patTag() {}
func (pLit) patTag()    {}

type patVec []pat
type patMatrix []patVec

// ---- Convert core.Pattern → pat ----

func coreToPat(p core.Pattern) pat {
	switch pp := p.(type) {
	case *core.PVar:
		return pWild{}
	case *core.PWild:
		return pWild{}
	case *core.PCon:
		args := make([]pat, len(pp.Args))
		for i, a := range pp.Args {
			args[i] = coreToPat(a)
		}
		return pCon{con: pp.Con, arity: len(pp.Args), args: args}
	case *core.PRecord:
		fields := make(map[string]pat, len(pp.Fields))
		for _, f := range pp.Fields {
			fields[f.Label] = coreToPat(f.Pattern)
		}
		return pRecord{fields: fields}
	case *core.PLit:
		return pLit{value: pp.Value}
	default:
		return pWild{}
	}
}

// ---- Signature (complete set of constructors) ----

type conSig struct {
	name  string
	arity int
}

// isGADT returns true if any constructor of the scrutinee's data type
// has a refined return type (i.e., is a GADT constructor).
func (ch *Checker) isGADT(scrutTy types.Type) bool {
	tyName := headTyCon(scrutTy)
	if tyName == "" {
		return false
	}
	info := ch.lookupDataType(tyName)
	if info == nil {
		return false
	}
	for _, c := range info.Constructors {
		if c.ReturnType != nil {
			return true
		}
	}
	return false
}

// constructorArgTypes returns the argument types for a non-GADT constructor,
// refined by unifying the constructor's return type with the scrutinee type.
// For example, Just :: forall a. a -> Maybe a with scrutinee Maybe (Maybe Int)
// yields arg type Maybe Int (not a fresh meta).
func (ch *Checker) constructorArgTypes(conName string, scrutTy types.Type) []types.Type {
	info, ok := ch.conInfo[conName]
	if !ok {
		return nil
	}
	var arity int
	for _, c := range info.Constructors {
		if c.Name == conName {
			arity = c.Arity
			break
		}
	}
	conTy := ch.conTypes[conName]
	if conTy == nil {
		return nil
	}
	ty := ch.instantiateForExhaust(conTy)

	// Peel arrows to get arg types and the return type.
	var argTys []types.Type
	for range arity {
		if arr, ok := ty.(*types.TyArrow); ok {
			argTys = append(argTys, arr.From)
			ty = arr.To
		} else {
			argTys = append(argTys, nil)
		}
	}

	// Refine type variables by unifying the return type with the scrutinee.
	// Only worth doing when the scrutinee has a known head type constructor.
	if scrutTy != nil && headTyCon(ch.unifier.Zonk(scrutTy)) != "" {
		tmp := NewUnifierShared(&ch.freshID)
		if tmp.Unify(ty, ch.unifier.Zonk(scrutTy)) == nil {
			for i, a := range argTys {
				if a != nil {
					argTys[i] = tmp.Zonk(a)
				}
			}
		}
	}
	return argTys
}

// instantiateForExhaust strips foralls and evidence qualifiers by substituting
// fresh metas. Used to extract constructor argument types for exhaustiveness.
func (ch *Checker) instantiateForExhaust(ty types.Type) types.Type {
	for {
		switch t := ty.(type) {
		case *types.TyForall:
			m := &types.TyMeta{ID: ch.fresh(), Kind: t.Kind}
			ty = types.Subst(t.Body, t.Var, m)
		case *types.TyEvidence:
			ty = t.Body
		default:
			return ty
		}
	}
}

// subPatternTypes returns argument types for specialization into sub-patterns.
// For non-GADT types, returns actual argument types from the constructor signature.
// For GADT types (or unknown), returns nil types to avoid expensive canUnifyWith calls.
func (ch *Checker) subPatternTypes(conName string, scrutTy types.Type, arity int, restTys []types.Type) []types.Type {
	if scrutTy != nil && !ch.isGADT(scrutTy) {
		argTys := ch.constructorArgTypes(conName, scrutTy)
		if argTys != nil {
			return append(argTys, restTys...)
		}
	}
	newTys := make([]types.Type, arity)
	return append(newTys, restTys...)
}

// constructorSigs returns the signature for a type, filtering by GADT
// applicability. Returns nil if the type is not a known ADT.
func (ch *Checker) constructorSigs(scrutTy types.Type) []conSig {
	tyName := headTyCon(scrutTy)
	if tyName == "" {
		return nil
	}
	info := ch.lookupDataType(tyName)
	if info == nil {
		return nil
	}
	var sigs []conSig
	for _, c := range info.Constructors {
		if c.ReturnType != nil && !ch.canUnifyWith(c.ReturnType, scrutTy) {
			continue
		}
		sigs = append(sigs, conSig{name: c.Name, arity: c.Arity})
	}
	return sigs
}

// ---- Specialize S(c, P) ----
// Keep rows whose column 0 matches constructor c, expanding sub-patterns.
// Wildcard rows are expanded to c.arity wildcards.

func specialize(mx patMatrix, con string, arity int) patMatrix {
	var result patMatrix
	for _, row := range mx {
		if len(row) == 0 {
			continue
		}
		switch p := row[0].(type) {
		case pCon:
			if p.con == con {
				newRow := make(patVec, 0, arity+len(row)-1)
				newRow = append(newRow, p.args...)
				newRow = append(newRow, row[1:]...)
				result = append(result, newRow)
			}
		case pWild:
			newRow := make(patVec, 0, arity+len(row)-1)
			for range arity {
				newRow = append(newRow, pWild{})
			}
			newRow = append(newRow, row[1:]...)
			result = append(result, newRow)
		}
	}
	return result
}

// ---- Default D(P) ----
// Keep rows whose column 0 is a wildcard/variable, dropping column 0.

func defaultMatrix(mx patMatrix) patMatrix {
	var result patMatrix
	for _, row := range mx {
		if len(row) == 0 {
			continue
		}
		if _, ok := row[0].(pWild); ok {
			result = append(result, row[1:])
		}
	}
	return result
}

// ---- Literal specialize ----

// specializeLit keeps rows whose column 0 matches a specific literal value.
// Wildcard rows are included (literal always matches subset of wildcard).
func specializeLit(mx patMatrix, val any) patMatrix {
	var result patMatrix
	for _, row := range mx {
		if len(row) == 0 {
			continue
		}
		switch p := row[0].(type) {
		case pLit:
			if p.value == val {
				result = append(result, row[1:])
			}
		case pWild:
			result = append(result, row[1:])
		}
	}
	return result
}

// columnHeadLits returns the set of literal values in column 0.
func columnHeadLits(mx patMatrix) map[any]bool {
	result := map[any]bool{}
	for _, row := range mx {
		if len(row) == 0 {
			continue
		}
		if lp, ok := row[0].(pLit); ok {
			result[lp.value] = true
		}
	}
	return result
}

// ---- Record specialize/default ----

// allRecordLabels collects all labels mentioned in column 0 of the matrix.
func allRecordLabels(mx patMatrix) []string {
	set := map[string]struct{}{}
	for _, row := range mx {
		if len(row) == 0 {
			continue
		}
		if p, ok := row[0].(pRecord); ok {
			for l := range p.fields {
				set[l] = struct{}{}
			}
		}
	}
	labels := make([]string, 0, len(set))
	for l := range set {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	return labels
}

// specializeRecord expands column 0 into per-label columns.
func specializeRecord(mx patMatrix, labels []string) patMatrix {
	var result patMatrix
	for _, row := range mx {
		if len(row) == 0 {
			continue
		}
		switch p := row[0].(type) {
		case pRecord:
			newRow := make(patVec, 0, len(labels)+len(row)-1)
			for _, l := range labels {
				if sub, ok := p.fields[l]; ok {
					newRow = append(newRow, sub)
				} else {
					newRow = append(newRow, pWild{})
				}
			}
			newRow = append(newRow, row[1:]...)
			result = append(result, newRow)
		case pWild:
			newRow := make(patVec, 0, len(labels)+len(row)-1)
			for range labels {
				newRow = append(newRow, pWild{})
			}
			newRow = append(newRow, row[1:]...)
			result = append(result, newRow)
		}
	}
	return result
}

// ---- Helpers ----

// nilTypesWithTail builds a type slice of n nil entries followed by tail.
// Used to represent unknown sub-pattern types for record fields.
func nilTypesWithTail(n int, tail []types.Type) []types.Type {
	result := make([]types.Type, n, n+len(tail))
	return append(result, tail...)
}

// makeWildcardVec builds a pattern vector of n wildcards followed by tail.
func makeWildcardVec(n int, tail patVec) patVec {
	result := make(patVec, 0, n+len(tail))
	for range n {
		result = append(result, pWild{})
	}
	return append(result, tail...)
}

// ---- isUseful ----

const maxExhaustDepth = 32

// isUseful returns true if the given pattern vector is useful w.r.t.
// the pattern matrix, along with a witness pattern for error reporting.
func (ch *Checker) isUseful(mx patMatrix, q patVec, scrutTys []types.Type) (bool, pat) {
	return ch.isUsefulAt(mx, q, scrutTys, 0)
}

func (ch *Checker) isUsefulAt(mx patMatrix, q patVec, scrutTys []types.Type, depth int) (bool, pat) {
	if depth > maxExhaustDepth {
		return false, nil // conservative: assume covered
	}
	// Base: no columns → useful iff matrix has no rows.
	if len(q) == 0 {
		return len(mx) == 0, pWild{}
	}

	// Empty matrix: every query vector is trivially useful.
	// The caller reconstructs the witness constructor around pWild{}.
	if len(mx) == 0 {
		return true, pWild{}
	}

	// Check column 0 type.
	var ty types.Type
	if len(scrutTys) > 0 {
		ty = ch.unifier.Zonk(scrutTys[0])
	}
	restTys := scrutTys
	if len(restTys) > 0 {
		restTys = restTys[1:]
	}

	// If column 0 of q is a constructor pattern, specialize.
	if cp, ok := q[0].(pCon); ok {
		smx := specialize(mx, cp.con, cp.arity)
		sq := make(patVec, 0, cp.arity+len(q)-1)
		sq = append(sq, cp.args...)
		sq = append(sq, q[1:]...)

		newTys := ch.subPatternTypes(cp.con, ty, cp.arity, restTys)
		useful, witness := ch.isUsefulAt(smx, sq, newTys, depth+1)
		if useful {
			return true, reconstructCon(cp.con, cp.arity, witness, q[1:])
		}
		return false, nil
	}

	// If column 0 of q is a literal pattern, specialize.
	if lp, ok := q[0].(pLit); ok {
		smx := specializeLit(mx, lp.value)
		sq := q[1:]
		useful, witness := ch.isUsefulAt(smx, sq, restTys, depth+1)
		if useful {
			return true, witness
		}
		return false, nil
	}

	// If column 0 of q is a record pattern, specialize by labels.
	if rp, ok := q[0].(pRecord); ok {
		labels := allRecordLabels(append(mx, q))
		smx := specializeRecord(mx, labels)
		sq := make(patVec, 0, len(labels)+len(q)-1)
		for _, l := range labels {
			if sub, ok := rp.fields[l]; ok {
				sq = append(sq, sub)
			} else {
				sq = append(sq, pWild{})
			}
		}
		sq = append(sq, q[1:]...)

		newTys := nilTypesWithTail(len(labels), restTys)
		return ch.isUsefulAt(smx, sq, newTys, depth+1)
	}

	// Column 0 of q is a wildcard.

	// If column 0 has only wildcards (no constructor, record, or literal patterns),
	// usefulness depends on the remaining columns — use the default matrix.
	headCons := columnHeadCons(mx)
	headLits := columnHeadLits(mx)
	if len(headCons) == 0 && len(headLits) == 0 && !hasRecordPats(mx) {
		dmx := defaultMatrix(mx)
		return ch.isUsefulAt(dmx, q[1:], restTys, depth+1)
	}

	// If column 0 has any record patterns, use record specialization.
	if hasRecordPats(mx) {
		labels := allRecordLabels(mx)
		smx := specializeRecord(mx, labels)
		sq := make(patVec, 0, len(labels)+len(q)-1)
		for range labels {
			sq = append(sq, pWild{})
		}
		sq = append(sq, q[1:]...)
		newTys := nilTypesWithTail(len(labels), restTys)
		return ch.isUsefulAt(smx, sq, newTys, depth+1)
	}

	// If column 0 has only literal patterns (no constructors), the signature
	// is always incomplete — we can't enumerate all Int/String values.
	// Use the default matrix: wildcard is useful iff it adds coverage.
	if len(headCons) == 0 && len(headLits) > 0 {
		dmx := defaultMatrix(mx)
		return ch.isUsefulAt(dmx, q[1:], restTys, depth+1)
	}

	// Get the complete signature for the scrutinee type.
	sigs := ch.constructorSigs(ty)

	if sigs != nil {
		// When the complete signature is covered (len(headCons) >= len(sigs)),
		// check all constructors. Otherwise, only check uncovered ones.
		complete := len(headCons) >= len(sigs)
		for _, sig := range sigs {
			if !complete {
				if _, covered := headCons[sig.name]; covered {
					continue
				}
			}
			smx := specialize(mx, sig.name, sig.arity)
			sq := makeWildcardVec(sig.arity, q[1:])

			newTys := ch.subPatternTypes(sig.name, ty, sig.arity, restTys)
			useful, witness := ch.isUsefulAt(smx, sq, newTys, depth+1)
			if useful {
				return true, reconstructCon(sig.name, sig.arity, witness, q[1:])
			}
		}
		return false, nil
	}

	// No signature (opaque type / unresolved meta): conservatively assume
	// the matrix covers all cases. This matches the old behavior of skipping
	// the check when the type cannot be determined.
	if len(headCons) > 0 {
		return false, nil
	}

	// No constructors at all: use default matrix.
	dmx := defaultMatrix(mx)
	useful, witness := ch.isUsefulAt(dmx, q[1:], restTys, depth+1)
	if useful {
		return true, witness
	}
	return false, nil
}

// hasRecordPats returns true if column 0 of the matrix has any record patterns.
func hasRecordPats(mx patMatrix) bool {
	for _, row := range mx {
		if len(row) > 0 {
			if _, ok := row[0].(pRecord); ok {
				return true
			}
		}
	}
	return false
}

// columnHeadCons returns the set of constructor names in column 0.
func columnHeadCons(mx patMatrix) map[string]int {
	result := map[string]int{}
	for _, row := range mx {
		if len(row) == 0 {
			continue
		}
		if cp, ok := row[0].(pCon); ok {
			result[cp.con] = cp.arity
		}
	}
	return result
}

// reconstructCon builds a witness pattern from the recursive useful result.
func reconstructCon(con string, arity int, inner pat, _ patVec) pat {
	// Extract sub-patterns from the witness (best-effort).
	args := make([]pat, arity)
	for i := range args {
		args[i] = pWild{}
	}
	// If the inner result gives us sub-pattern info, propagate it.
	if arity > 0 {
		if ic, ok := inner.(pCon); ok && ic.con == con {
			copy(args, ic.args)
		}
	}
	return pCon{con: con, arity: arity, args: args}
}

// ---- Public API ----

// checkExhaustive verifies that a set of case alternatives covers every
// constructor of the scrutinee's data type and reports redundant patterns.
func (ch *Checker) checkExhaustive(scrutTy types.Type, alts []core.Alt, s span.Span) {
	scrutTy = ch.unifier.Zonk(scrutTy)

	// Build pattern matrix.
	var mx patMatrix
	for i, alt := range alts {
		row := patVec{coreToPat(alt.Pattern)}

		// Redundancy check: is this row useful w.r.t. previous rows?
		if i > 0 {
			useful, _ := ch.isUseful(mx, row, []types.Type{scrutTy})
			if !useful {
				ch.addCodedError(errs.ErrRedundantPattern, alt.S,
					"redundant pattern in case expression")
			}
		}

		mx = append(mx, row)
	}

	// Exhaustiveness check.
	// First, a quick wildcard usefulness check to avoid expensive per-constructor
	// enumeration when the matrix is already exhaustive.
	wildcard := patVec{pWild{}}
	useful, witness := ch.isUseful(mx, wildcard, []types.Type{scrutTy})
	if !useful {
		return
	}

	// The match is non-exhaustive. Enumerate which constructors are missing.
	sigs := ch.constructorSigs(scrutTy)
	if sigs != nil {
		var missing []string
		for _, sig := range sigs {
			q := make(patVec, 1)
			args := make([]pat, sig.arity)
			for i := range args {
				args[i] = pWild{}
			}
			q[0] = pCon{con: sig.name, arity: sig.arity, args: args}
			u, _ := ch.isUseful(mx, q, []types.Type{scrutTy})
			if u {
				missing = append(missing, sig.name)
			}
		}
		if len(missing) > 0 {
			ch.addCodedError(errs.ErrNonExhaustive, s, fmt.Sprintf(
				"non-exhaustive patterns: missing %s",
				strings.Join(missing, ", "),
			))
		}
	} else {
		ch.addCodedError(errs.ErrNonExhaustive, s, fmt.Sprintf(
			"non-exhaustive patterns: missing %s", formatWitness(witness),
		))
	}
}

// formatWitness renders a witness pattern for error messages.
func formatWitness(p pat) string {
	switch pp := p.(type) {
	case pCon:
		if len(pp.args) == 0 {
			return pp.con
		}
		args := make([]string, len(pp.args))
		for i, a := range pp.args {
			s := formatWitness(a)
			// Wrap nested constructors with args in parens.
			if ac, ok := a.(pCon); ok && len(ac.args) > 0 {
				s = "(" + s + ")"
			}
			args[i] = s
		}
		return pp.con + " " + strings.Join(args, " ")
	case pWild:
		return "_"
	case pLit:
		return fmt.Sprintf("%v", pp.value)
	case pRecord:
		if len(pp.fields) == 0 {
			return "{}"
		}
		var parts []string
		for l, v := range pp.fields {
			parts = append(parts, l+" = "+formatWitness(v))
		}
		sort.Strings(parts)
		return "{ " + strings.Join(parts, ", ") + " }"
	default:
		return "_"
	}
}

// ---- Helpers shared with old code ----

// headTyCon extracts the outermost type constructor name from a type.
func headTyCon(ty types.Type) string {
	switch t := ty.(type) {
	case *types.TyCon:
		return t.Name
	case *types.TyApp:
		return headTyCon(t.Fun)
	default:
		return ""
	}
}

// canUnifyWith tests whether retTy can unify with scrutTy in a temporary
// unifier. Used for GADT exhaustiveness to filter irrelevant constructors.
func (ch *Checker) canUnifyWith(retTy, scrutTy types.Type) bool {
	tmp := NewUnifierShared(&ch.freshID)
	retTy = ch.instantiateFresh(tmp, retTy)
	return tmp.Unify(retTy, scrutTy) == nil
}

func (ch *Checker) instantiateFresh(u *Unifier, ty types.Type) types.Type {
	vars := make(map[string]*types.TyMeta)
	return ch.substVarsWithMetas(u, ty, vars)
}

func (ch *Checker) substVarsWithMetas(u *Unifier, ty types.Type, vars map[string]*types.TyMeta) types.Type {
	switch t := ty.(type) {
	case *types.TyVar:
		if m, ok := vars[t.Name]; ok {
			return m
		}
		m := &types.TyMeta{ID: ch.fresh(), Kind: types.KType{}}
		vars[t.Name] = m
		return m
	case *types.TyApp:
		f := ch.substVarsWithMetas(u, t.Fun, vars)
		a := ch.substVarsWithMetas(u, t.Arg, vars)
		if f == t.Fun && a == t.Arg {
			return ty
		}
		return &types.TyApp{Fun: f, Arg: a, S: t.S}
	case *types.TyCon:
		return ty
	case *types.TyArrow:
		from := ch.substVarsWithMetas(u, t.From, vars)
		to := ch.substVarsWithMetas(u, t.To, vars)
		if from == t.From && to == t.To {
			return ty
		}
		return &types.TyArrow{From: from, To: to, S: t.S}
	default:
		return ty
	}
}

func (ch *Checker) lookupDataType(tyName string) *DataTypeInfo {
	for _, info := range ch.conInfo {
		if info.Name == tyName {
			return info
		}
	}
	return nil
}
