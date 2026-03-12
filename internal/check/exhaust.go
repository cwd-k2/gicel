package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/span"
	"github.com/cwd-k2/gomputation/pkg/types"
)

// checkExhaustive verifies that a set of case alternatives covers every
// constructor of the scrutinee's data type. It implements a simplified
// Maranget-style exhaustiveness check: a pattern matrix is exhaustive
// when every constructor of the scrutinee type appears in at least one
// top-level pattern, or when a wildcard/variable pattern exists.
//
// Redundancy detection is deferred to a later phase.
func (ch *Checker) checkExhaustive(scrutTy types.Type, alts []core.Alt, s span.Span) {
	scrutTy = ch.unifier.Zonk(scrutTy)

	// Extract the head type constructor name from the scrutinee type.
	tyName := headTyCon(scrutTy)
	if tyName == "" {
		// Cannot determine the data type (meta, error, etc.) — skip.
		return
	}

	// Look up the DataTypeInfo for this type.
	info := ch.lookupDataType(tyName)
	if info == nil {
		// Not a known algebraic data type (e.g. opaque host type) — skip.
		return
	}

	// Build the set of constructors required for exhaustiveness.
	// GADT: skip constructors whose ReturnType cannot unify with the scrutinee type.
	required := make(map[string]bool, len(info.Constructors))
	for _, c := range info.Constructors {
		if c.ReturnType != nil {
			if !ch.canUnifyWith(c.ReturnType, scrutTy) {
				continue // irrelevant constructor
			}
		}
		required[c.Name] = true
	}

	// Walk the alternatives. A wildcard or variable pattern covers
	// all constructors; a constructor pattern covers exactly one.
	for _, alt := range alts {
		switch p := alt.Pattern.(type) {
		case *core.PVar, *core.PWild:
			// Covers every constructor — the match is trivially exhaustive.
			return
		case *core.PCon:
			delete(required, p.Con)
		}
	}

	if len(required) == 0 {
		return
	}

	// Collect missing constructor names in declaration order.
	var missing []string
	for _, c := range info.Constructors {
		if required[c.Name] {
			missing = append(missing, c.Name)
		}
	}

	ch.addCodedError(errs.ErrNonExhaustive, s, fmt.Sprintf(
		"non-exhaustive patterns: missing %s",
		strings.Join(missing, ", "),
	))
}

// headTyCon extracts the outermost type constructor name from a type.
// For a bare TyCon it returns the name directly; for a TyApp chain
// (e.g. Maybe a = TyApp(TyCon("Maybe"), TyVar("a"))) it peels
// applications until it reaches the head.
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
// unifier. Used for GADT exhaustiveness: if a constructor's return type
// cannot unify with the scrutinee, the constructor is irrelevant.
func (ch *Checker) canUnifyWith(retTy, scrutTy types.Type) bool {
	tmp := NewUnifierShared(&ch.freshID)
	// Instantiate any free type variables in retTy with fresh metas.
	retTy = ch.instantiateFresh(tmp, retTy)
	return tmp.Unify(retTy, scrutTy) == nil
}

// instantiateFresh replaces TyVar nodes with fresh metas, simulating
// the forall-instantiation that checkPattern performs.
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

// lookupDataType finds the DataTypeInfo for a given type name by
// scanning the conInfo map. Every constructor of the same data type
// points to the same *DataTypeInfo, so we stop at the first match.
func (ch *Checker) lookupDataType(tyName string) *DataTypeInfo {
	for _, info := range ch.conInfo {
		if info.Name == tyName {
			return info
		}
	}
	return nil
}
