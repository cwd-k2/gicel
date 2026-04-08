// Package ir defines the Core IR — a uniform intermediate representation
// consumed by the optimizer, the bytecode compiler, and the evaluator.
// Source trees from internal/lang/syntax are lowered into Core by the
// checker (internal/compiler/check) before any downstream pass runs.
//
// # Phase-invariant nodes, side-table metadata
//
// The Core node types (Lam, Thunk, Merge, ...) carry no analysis state.
// Free-variable metadata lives in a separate *FVAnnotations side table
// that the caller owns and threads through the pipeline alongside the
// Program. Two passes populate it:
//
//	AnnotateFreeVars         → *FVAnnotations (names, Var.Key caching)
//	AssignIndices(_, annots) → populates FVInfo.Indices in place
//
// VerifyAnnotations(prog, annots) checks coherence.
//
// # Invariants
//
//   - *ir.Lam / *ir.Thunk / *ir.Merge are immutable structural forms:
//     the same pointer has the same meaning regardless of which passes
//     have been run. Two trees with structurally identical nodes are
//     interchangeable — there is no hidden state to forget.
//   - Var.Key is non-empty after any traversal that calls annotateCore
//     (populated from Module and Name via varKey).
//   - Var.Index is -1 for global references (resolved through Key) and
//     >= 0 for local references (de Bruijn index, 0 = innermost).
//   - FVInfo.Overflow == true signals the FV computation was truncated
//     by the traversal depth limit. In that case Vars and Indices are
//     not meaningful and the evaluator must capture the entire
//     enclosing environment instead of trimming.
//   - FVInfo.Overflow == false ⇒ Vars is non-nil (possibly empty) after
//     AnnotateFreeVars, and Indices is non-nil (possibly empty) after
//     AssignIndices.
//   - Merge.LeftLabels and Merge.RightLabels are final by the time the
//     IR leaves the checker. The transient pre-state types needed to
//     re-extract them after constraint resolution live in a
//     checker-local side table, not on the IR node, so the node has
//     no hidden phase distinction. See compiler/check/checker.go for
//     the side table; downstream passes can treat the labels as
//     immutable inputs.
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
// Transforms run BEFORE AnnotateFreeVars in the pipeline, so they do
// not need to propagate FV annotations — the post-transform annotate
// pass regenerates them over the rewritten tree.
package ir
