// Package ir defines the Core IR — a uniform intermediate representation
// consumed by the optimizer, the bytecode compiler, and the evaluator.
// Source trees from internal/lang/syntax are lowered into Core by the
// checker (internal/compiler/check) before any downstream pass runs.
//
// # Passes and their artifacts
//
// Core trees pass through several annotation stages. Each stage is
// monotonic — it only fills previously-unset fields:
//
//	AnnotateFreeVars   populates Var.Key, Lam.FV, Thunk.FV, Merge.LeftFV/RightFV
//	AssignIndices      populates Var.Index, Lam.FVIndices, Thunk.FVIndices,
//	                   Merge.LeftFVIdx/RightFVIdx
//	RefineMergeLabels  fills Merge.LeftLabels/RightLabels and clears
//	                   Merge.PreLeft/PreRight
//
// # Invariants
//
// Once a stage has run on a tree, its annotations are final for the
// tree's lifetime. Tree identity is what determines stage status; a
// freshly allocated node starts unannotated regardless of whether
// its siblings are annotated.
//
//   - Var.Key is non-empty after AnnotateFreeVars (populated from
//     Module and Name via varKey).
//   - Var.Index is -1 for global references (resolved through Key)
//     and >= 0 for local references (de Bruijn index, 0 = innermost).
//   - Lam.FV == nil signals overflow: the depth limit was reached
//     while computing free variables; the evaluator must capture the
//     entire enclosing environment instead of trimming.
//   - Lam.FVIndices == nil with Lam.FV != nil signals "no local
//     captures" (all FVs are global). When both are non-nil,
//     len(Lam.FVIndices) == len(Lam.FV).
//   - Thunk.FV / Thunk.FVIndices follow the same conventions as Lam.
//   - Merge.PreLeft and Merge.PreRight are non-nil between inferMerge
//     (where labels may still contain unresolved metas) and
//     RefineMergeLabels (which replaces tentative labels with the
//     post-constraint-solving truth). After refinement they are nil.
//   - The App/TyApp/TyLam chain is the left spine; all read-only
//     traversals descend it iteratively via unwindLeftSpine (spine.go)
//     to avoid Go stack overflow on long operator chains.
//
// # Traversal modes
//
// Walk / Transform / TransformMut cover the three modes:
//
//	Walk         visit every node; visitor returns false to prune
//	Transform    bottom-up rebuild; shares structure when unchanged
//	TransformMut bottom-up mutation; mutates parent fields in place
//	             (caller must own the tree exclusively)
//
// FreeVars, AnnotateFreeVars, and AssignIndices all follow the
// AnnotateFreeVars → AssignIndices ordering. Running them out of
// order leaves indices unresolved against stale Key values.
package ir
