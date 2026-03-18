package check

import (
	"fmt"
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
// For example, Just :: \ a. a -> Maybe a with scrutinee Maybe (Maybe Int)
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

	// Column 0 of q is a wildcard — delegate to wildcard-specific logic.
	return ch.isUsefulWildcard(mx, q, ty, restTys, depth)
}

// isUsefulWildcard handles the wildcard-in-column-0 case of isUsefulAt.
// Invariant: constructors and literals do not mix in column 0 for well-typed
// programs (Int/String/Rune have no constructors; ADTs have no literals).
// The branches below rely on this mutual exclusion.
func (ch *Checker) isUsefulWildcard(mx patMatrix, q patVec, ty types.Type, restTys []types.Type, depth int) (bool, pat) {
	headCons := columnHeadCons(mx)
	headLits := columnHeadLits(mx)

	// All wildcards: usefulness depends on remaining columns.
	if len(headCons) == 0 && len(headLits) == 0 && !hasRecordPats(mx) {
		dmx := defaultMatrix(mx)
		return ch.isUsefulAt(dmx, q[1:], restTys, depth+1)
	}

	// Record patterns: specialize by label set.
	if hasRecordPats(mx) {
		labels := allRecordLabels(mx)
		smx := specializeRecord(mx, labels)
		sq := makeWildcardVec(len(labels), q[1:])
		newTys := nilTypesWithTail(len(labels), restTys)
		return ch.isUsefulAt(smx, sq, newTys, depth+1)
	}

	// Literals only (no constructors): signature is always incomplete.
	if len(headCons) == 0 && len(headLits) > 0 {
		dmx := defaultMatrix(mx)
		return ch.isUsefulAt(dmx, q[1:], restTys, depth+1)
	}

	// Constructor patterns: check against the complete signature.
	sigs := ch.constructorSigs(ty)
	if sigs != nil {
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

	// Opaque type with constructors: conservatively assume covered.
	if len(headCons) > 0 {
		return false, nil
	}

	// No patterns at all: use default matrix.
	dmx := defaultMatrix(mx)
	return ch.isUsefulAt(dmx, q[1:], restTys, depth+1)
}

// ---- Public API ----

// checkExhaustive verifies that a set of case alternatives covers every
// constructor of the scrutinee's data type and reports redundant patterns.
func (ch *Checker) checkExhaustive(scrutTy types.Type, alts []core.Alt, s span.Span) {
	scrutTy = ch.unifier.Zonk(scrutTy)
	// Reduce type family applications in the scrutinee type so that
	// data family instances are resolved to their mangled concrete types.
	scrutTy = ch.reduceFamilyInType(scrutTy)

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
	return ch.dataTypeByName[tyName]
}
