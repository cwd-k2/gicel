package check

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/exhaust"
	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// checkExhaustive delegates to the exhaust subpackage.
func (ch *Checker) checkExhaustive(scrutTy types.Type, alts []ir.Alt, s span.Span) {
	env := &exhaust.CheckEnv{
		DataTypes:    ch.reg.dataTypeByName,
		ConInfoMap:   ch.reg.conInfo,
		ConTypes:     ch.reg.conTypes,
		FreshID:      &ch.freshID,
		Unifier:      ch.unifier,
		ReduceFamily: ch.reduceFamilyInType,
		CanUnifyWith: ch.canUnifyWith,
		AddError:     ch.addCodedError,
	}
	env.CheckExhaustive(scrutTy, alts, s)
}

// canUnifyWith tests whether retTy can unify with scrutTy in a temporary
// unifier. Used for GADT exhaustiveness to filter irrelevant constructors.
func (ch *Checker) canUnifyWith(retTy, scrutTy types.Type) bool {
	tmp := unify.NewUnifierShared(&ch.freshID)
	// GADT branches refine skolems via given equalities, so allow
	// skolems to unify with arbitrary types for accessibility testing.
	tmp.FlexSkolems = true
	retTy = ch.instantiateFresh(tmp, retTy)
	return tmp.Unify(retTy, scrutTy) == nil
}

func (ch *Checker) instantiateFresh(u *unify.Unifier, ty types.Type) types.Type {
	vars := make(map[string]*types.TyMeta)
	return ch.substVarsWithMetas(u, ty, vars)
}

func (ch *Checker) substVarsWithMetas(u *unify.Unifier, ty types.Type, vars map[string]*types.TyMeta) types.Type {
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
