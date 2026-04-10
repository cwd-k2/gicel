package engine

import (
	"github.com/cwd-k2/gicel/internal/host/registry"
	"github.com/cwd-k2/gicel/internal/lang/types"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// HostEnv holds type registrations, primitive implementations, and rewrite rules
// accumulated by host code before compilation.
type HostEnv struct {
	bindings      map[string]types.Type
	assumptions   map[string]types.Type
	registeredTys map[string]types.Type
	prims         *eval.PrimRegistry
	gatedBuiltins map[string]bool
	rewriteRules  []registry.RewriteRule
}

func newHostEnv() HostEnv {
	h := HostEnv{
		bindings:      make(map[string]types.Type),
		assumptions:   make(map[string]types.Type),
		registeredTys: make(map[string]types.Type),
		prims:         eval.NewPrimRegistry(),
		gatedBuiltins: make(map[string]bool),
	}
	h.registeredTys["Int"] = types.TypeOfTypes
	h.registeredTys["Double"] = types.TypeOfTypes
	h.registeredTys["String"] = types.TypeOfTypes
	h.registeredTys["Rune"] = types.TypeOfTypes
	h.registeredTys["Byte"] = types.TypeOfTypes
	h.registeredTys["Slice"] = types.MkArrow(types.TypeOfTypes, types.TypeOfTypes)
	h.registeredTys["Array"] = types.MkArrow(types.TypeOfTypes, types.TypeOfTypes)
	h.registeredTys["Map"] = types.MkArrow(types.TypeOfTypes, types.MkArrow(types.TypeOfTypes, types.TypeOfTypes))
	h.registeredTys["Set"] = types.MkArrow(types.TypeOfTypes, types.TypeOfTypes)
	h.registeredTys["MMap"] = types.MkArrow(types.TypeOfTypes, types.MkArrow(types.TypeOfTypes, types.TypeOfTypes))
	h.registeredTys["MSet"] = types.MkArrow(types.TypeOfTypes, types.TypeOfTypes)
	h.registeredTys["Ref"] = types.MkArrow(types.TypeOfTypes, types.TypeOfTypes)
	return h
}
