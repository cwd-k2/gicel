package check

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/exhaust"
	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// checkExhaustive delegates to the exhaust subpackage.
func (ch *Checker) checkExhaustive(scrutTy types.Type, alts []ir.Alt, s span.Span) {
	env := &exhaust.CheckEnv{
		DataTypes:    ch.reg.AllDataTypes(),
		ConInfoMap:   ch.reg.AllConInfo(),
		ConTypes:     ch.reg.AllConTypes(),
		Fresh:        ch.fresh,
		Unifier:      ch.unifier,
		TypeOps:      ch.typeOps,
		ReduceFamily: ch.reduceFamilyInType,
		CanUnifyWith: ch.canUnifyWith,
		AddError: func(code diagnostic.Code, s span.Span, msg string) {
			ch.addDiag(code, s, diagMsg(msg))
		},
	}
	env.CheckExhaustive(scrutTy, alts, s)
}

// canUnifyWith tests whether retTy can unify with scrutTy in an
// isolated probe scope on the main unifier. Used for GADT exhaustiveness
// to filter irrelevant constructors.
//
// Previously created a fresh `tmp := unify.NewUnifierShared(...)` per
// call. Now uses BeginProbeIsolated / EndProbeIsolated on the existing
// main unifier — same isolation guarantees, zero allocations.
//
// GADT branches refine skolems via given equalities, so we set
// FlexSkolems = true inside the probe scope to allow skolems to unify
// with arbitrary types for accessibility testing. The IsolationToken
// captures the surrounding FlexSkolems state and EndProbeIsolated
// restores it on exit.
func (ch *Checker) canUnifyWith(retTy, scrutTy types.Type) bool {
	tok := ch.unifier.BeginProbeIsolated()
	ch.unifier.FlexSkolems = true
	retTy = ch.instantiateFresh(ch.unifier, retTy)
	ok := ch.unifier.Unify(retTy, scrutTy) == nil
	ch.unifier.EndProbeIsolated(tok)
	return ok
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
		m := &types.TyMeta{ID: ch.fresh(), Kind: types.TypeOfTypes}
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
