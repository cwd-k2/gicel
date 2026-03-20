package engine

import (
	"github.com/cwd-k2/gicel/internal/eval"
	"github.com/cwd-k2/gicel/internal/reg"
	"github.com/cwd-k2/gicel/internal/types"
)

// HostEnv holds type registrations, primitive implementations, and rewrite rules
// accumulated by host code before compilation.
type HostEnv struct {
	bindings      map[string]types.Type
	assumptions   map[string]types.Type
	registeredTys map[string]types.Kind
	prims         *eval.PrimRegistry
	gatedBuiltins map[string]bool
	rewriteRules  []reg.RewriteRule
}

func newHostEnv() HostEnv {
	h := HostEnv{
		bindings:      make(map[string]types.Type),
		assumptions:   make(map[string]types.Type),
		registeredTys: make(map[string]types.Kind),
		prims:         eval.NewPrimRegistry(),
		gatedBuiltins: make(map[string]bool),
	}
	h.registeredTys["Int"] = types.KType{}
	h.registeredTys["Double"] = types.KType{}
	h.registeredTys["String"] = types.KType{}
	h.registeredTys["Rune"] = types.KType{}
	h.registeredTys["Slice"] = &types.KArrow{From: types.KType{}, To: types.KType{}}
	h.registeredTys["Map"] = &types.KArrow{From: types.KType{}, To: &types.KArrow{From: types.KType{}, To: types.KType{}}}
	h.registeredTys["Set"] = &types.KArrow{From: types.KType{}, To: types.KType{}}
	return h
}
