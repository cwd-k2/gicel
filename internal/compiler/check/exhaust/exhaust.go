package exhaust

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// CheckEnv provides the checker capabilities needed for exhaustiveness analysis.
type CheckEnv struct {
	DataTypes    map[string]*DataTypeInfo // type name → data type info
	ConInfoMap   map[string]*DataTypeInfo // constructor name → owning data type
	ConTypes     map[string]types.Type    // constructor name → full type scheme
	Fresh        func() int               // fresh ID generator (delegates to Session.fresh)
	Unifier      *unify.Unifier           // main unifier (for Zonk)
	ReduceFamily func(types.Type) types.Type
	CanUnifyWith func(retTy, scrutTy types.Type) bool
	AddError     func(code diagnostic.Code, s span.Span, msg string)
}

func (e *CheckEnv) lookupDataType(tyName string) *DataTypeInfo {
	return e.DataTypes[tyName]
}

// isGADT returns true if any constructor of the scrutinee's data type
// has a refined return type (i.e., is a GADT constructor).
func (e *CheckEnv) isGADT(scrutTy types.Type) bool {
	tyName := headTyCon(scrutTy)
	if tyName == "" {
		return false
	}
	info := e.lookupDataType(tyName)
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
func (e *CheckEnv) constructorArgTypes(conName string, scrutTy types.Type) []types.Type {
	info := e.ConInfoMap[conName]
	if info == nil {
		return nil
	}
	var arity int
	for _, c := range info.Constructors {
		if c.Name == conName {
			arity = c.Arity
			break
		}
	}
	conTy := e.ConTypes[conName]
	if conTy == nil {
		return nil
	}
	ty := e.instantiateForExhaust(conTy)

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
	if scrutTy != nil && headTyCon(e.Unifier.Zonk(scrutTy)) != "" {
		tmp := unify.NewUnifier()
		if tmp.Unify(ty, e.Unifier.Zonk(scrutTy)) == nil {
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
func (e *CheckEnv) instantiateForExhaust(ty types.Type) types.Type {
	for {
		switch t := ty.(type) {
		case *types.TyForall:
			m := &types.TyMeta{ID: e.Fresh(), Kind: t.Kind}
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
func (e *CheckEnv) subPatternTypes(conName string, scrutTy types.Type, arity int, restTys []types.Type) []types.Type {
	if scrutTy != nil && !e.isGADT(scrutTy) {
		argTys := e.constructorArgTypes(conName, scrutTy)
		if argTys != nil {
			return append(argTys, restTys...)
		}
	}
	newTys := make([]types.Type, arity)
	return append(newTys, restTys...)
}

// constructorSigs returns the signature for a type, filtering by GADT
// applicability. Returns nil if the type is not a known ADT.
func (e *CheckEnv) constructorSigs(scrutTy types.Type) []conSig {
	tyName := headTyCon(scrutTy)
	if tyName == "" {
		return nil
	}
	info := e.lookupDataType(tyName)
	if info == nil {
		return nil
	}
	var sigs []conSig
	for _, c := range info.Constructors {
		if c.ReturnType != nil && !e.CanUnifyWith(c.ReturnType, scrutTy) {
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
func (e *CheckEnv) isUseful(mx patMatrix, q patVec, scrutTys []types.Type) (bool, pat) {
	return e.isUsefulAt(mx, q, scrutTys, 0)
}

func (e *CheckEnv) isUsefulAt(mx patMatrix, q patVec, scrutTys []types.Type, depth int) (bool, pat) {
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
		ty = e.Unifier.Zonk(scrutTys[0])
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

		newTys := e.subPatternTypes(cp.con, ty, cp.arity, restTys)
		useful, witness := e.isUsefulAt(smx, sq, newTys, depth+1)
		if useful {
			return true, reconstructCon(cp.con, cp.arity, witness, q[1:])
		}
		return false, nil
	}

	// If column 0 of q is a literal pattern, specialize.
	if lp, ok := q[0].(pLit); ok {
		smx := specializeLit(mx, lp.value)
		sq := q[1:]
		useful, witness := e.isUsefulAt(smx, sq, restTys, depth+1)
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
		return e.isUsefulAt(smx, sq, newTys, depth+1)
	}

	// Column 0 of q is a wildcard — delegate to wildcard-specific logic.
	return e.isUsefulWildcard(mx, q, ty, restTys, depth)
}

// isUsefulWildcard handles the wildcard-in-column-0 case of isUsefulAt.
// Invariant: constructors and literals do not mix in column 0 for well-typed
// programs (Int/String/Rune have no constructors; ADTs have no literals).
// The branches below rely on this mutual exclusion.
func (e *CheckEnv) isUsefulWildcard(mx patMatrix, q patVec, ty types.Type, restTys []types.Type, depth int) (bool, pat) {
	headCons := columnHeadCons(mx)
	headLits := columnHeadLits(mx)

	// All wildcards: usefulness depends on remaining columns.
	if len(headCons) == 0 && len(headLits) == 0 && !hasRecordPats(mx) {
		dmx := defaultMatrix(mx)
		return e.isUsefulAt(dmx, q[1:], restTys, depth+1)
	}

	// Record patterns: specialize by label set.
	if hasRecordPats(mx) {
		labels := allRecordLabels(mx)
		smx := specializeRecord(mx, labels)
		sq := makeWildcardVec(len(labels), q[1:])
		newTys := nilTypesWithTail(len(labels), restTys)
		return e.isUsefulAt(smx, sq, newTys, depth+1)
	}

	// Literals only (no constructors): signature is always incomplete.
	if len(headCons) == 0 && len(headLits) > 0 {
		dmx := defaultMatrix(mx)
		return e.isUsefulAt(dmx, q[1:], restTys, depth+1)
	}

	// Constructor patterns: check against the complete signature.
	sigs := e.constructorSigs(ty)
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
			newTys := e.subPatternTypes(sig.name, ty, sig.arity, restTys)
			useful, witness := e.isUsefulAt(smx, sq, newTys, depth+1)
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
	return e.isUsefulAt(dmx, q[1:], restTys, depth+1)
}

// ---- Public API ----

// CheckExhaustive verifies that a set of case alternatives covers every
// constructor of the scrutinee's data type and reports redundant patterns.
func (e *CheckEnv) CheckExhaustive(scrutTy types.Type, alts []ir.Alt, s span.Span) {
	scrutTy = e.Unifier.Zonk(scrutTy)
	// Reduce type family applications in the scrutinee type so that
	// data family instances are resolved to their mangled concrete types.
	if e.ReduceFamily != nil {
		scrutTy = e.ReduceFamily(scrutTy)
	}

	// Build pattern matrix.
	var mx patMatrix
	for i, alt := range alts {
		row := patVec{coreToPat(alt.Pattern)}

		// Redundancy check: is this row useful w.r.t. previous rows?
		if i > 0 {
			useful, _ := e.isUseful(mx, row, []types.Type{scrutTy})
			if !useful {
				e.AddError(diagnostic.ErrRedundantPattern, alt.S,
					"redundant pattern in case expression")
			}
		}

		mx = append(mx, row)
	}

	// Exhaustiveness check.
	// First, a quick wildcard usefulness check to avoid expensive per-constructor
	// enumeration when the matrix is already exhaustive.
	wildcard := patVec{pWild{}}
	useful, witness := e.isUseful(mx, wildcard, []types.Type{scrutTy})
	if !useful {
		return
	}

	// The match is non-exhaustive. Enumerate which constructors are missing.
	sigs := e.constructorSigs(scrutTy)
	if sigs != nil {
		var missing []string
		for _, sig := range sigs {
			q := make(patVec, 1)
			args := make([]pat, sig.arity)
			for i := range args {
				args[i] = pWild{}
			}
			q[0] = pCon{con: sig.name, arity: sig.arity, args: args}
			u, _ := e.isUseful(mx, q, []types.Type{scrutTy})
			if u {
				missing = append(missing, sig.name)
			}
		}
		if len(missing) > 0 {
			e.AddError(diagnostic.ErrNonExhaustive, s, fmt.Sprintf(
				"non-exhaustive patterns: missing %s",
				strings.Join(missing, ", "),
			))
		}
	} else {
		e.AddError(diagnostic.ErrNonExhaustive, s, fmt.Sprintf(
			"non-exhaustive patterns: missing %s", formatWitness(witness),
		))
	}
}
