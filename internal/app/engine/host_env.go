package engine

import (
	"github.com/cwd-k2/gicel/internal/host/registry"
	"github.com/cwd-k2/gicel/internal/infra/span"
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

func newHostEnv(ops *types.TypeOps) HostEnv {
	h := HostEnv{
		bindings:      make(map[string]types.Type),
		assumptions:   make(map[string]types.Type),
		registeredTys: make(map[string]types.Type),
		prims:         eval.NewPrimRegistry(),
		gatedBuiltins: make(map[string]bool),
	}
	// registeredTys maps type names to their kinds (Type, Type -> Type, etc.).
	// Distinct from types.builtinTyCons which provides *TyCon singletons
	// for pointer-identity optimization in zonk. This map is the kind
	// registry that the checker uses for StrictTypeNames validation.
	h.registeredTys["Int"] = types.TypeOfTypes
	h.registeredTys["Double"] = types.TypeOfTypes
	h.registeredTys["String"] = types.TypeOfTypes
	h.registeredTys["Rune"] = types.TypeOfTypes
	h.registeredTys["Byte"] = types.TypeOfTypes
	zs := span.Span{}
	h.registeredTys["Slice"] = ops.Arrow(types.TypeOfTypes, types.TypeOfTypes, zs)
	h.registeredTys["Array"] = ops.Arrow(types.TypeOfTypes, types.TypeOfTypes, zs)
	h.registeredTys["Map"] = ops.Arrow(types.TypeOfTypes, ops.Arrow(types.TypeOfTypes, types.TypeOfTypes, zs), zs)
	h.registeredTys["Set"] = ops.Arrow(types.TypeOfTypes, types.TypeOfTypes, zs)
	h.registeredTys["MMap"] = ops.Arrow(types.TypeOfTypes, ops.Arrow(types.TypeOfTypes, types.TypeOfTypes, zs), zs)
	h.registeredTys["MSet"] = ops.Arrow(types.TypeOfTypes, types.TypeOfTypes, zs)
	h.registeredTys["Ref"] = ops.Arrow(types.TypeOfTypes, types.TypeOfTypes, zs)
	h.registeredTys["Seq"] = ops.Arrow(types.TypeOfTypes, types.TypeOfTypes, zs)
	return h
}
