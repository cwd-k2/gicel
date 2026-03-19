# Post-Reorganization Review

Date: 2026-03-19

Scope:

- Non-test code only
- Current codebase state after recent structural cleanup
- Focus on unresolved findings and next implementation priorities

This review is intentionally narrower than the earlier comprehensive review.
It focuses on what changed, what improved, what remains weak, and what the next implementation tasks should be.

## Executive Judgment

The codebase has materially improved.

This is no longer best described as "a powerful but messy implementation."
It is better described as:

- a strong implementation
- with real structural cleanup underway
- and with a smaller, clearer set of remaining high-value tasks

The most meaningful improvements are:

- `Checker` state is less ad hoc than before
- parser responsibilities are more legible than before
- several dense flows have been split into narrower files

The most important remaining issues are now concentrated in a few places rather than spread everywhere.

## Positive Changes

## 1. Checker ownership is clearer than before

The introduction of grouped checker state has real value.

Relevant code:

- [checker.go](/Users/cwd-k2/Projects/gicel/internal/check/checker.go#L80)
- [checker.go](/Users/cwd-k2/Projects/gicel/internal/check/checker.go#L99)
- [checker.go](/Users/cwd-k2/Projects/gicel/internal/check/checker.go#L105)

What improved:

- semantic registries now live under `checkerRegistry`
- module/name-resolution state now lives under `checkerScope`
- the `Checker` struct is still central, but less shapeless

This is not cosmetic.
It improves the reader's ability to answer:

- which state is semantic registry state
- which state is scope/import state
- which state is operational/checking control state

## 2. Checker file structure is more differentiated

Several flows that previously lived in larger buckets are now more localized.

Relevant code:

- [decl_data.go](/Users/cwd-k2/Projects/gicel/internal/check/decl_data.go)
- [instance_body.go](/Users/cwd-k2/Projects/gicel/internal/check/instance_body.go)
- [elaborate_do_monadic.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do_monadic.go)
- [elaborate_do_mult.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do_mult.go)
- [resolve_evidence.go](/Users/cwd-k2/Projects/gicel/internal/check/resolve_evidence.go)
- [resolve_kind.go](/Users/cwd-k2/Projects/gicel/internal/check/resolve_kind.go)

This is the right direction.
These are not arbitrary splits.
They correspond to real semantic families.

## 3. Parser structure is more legible

The parser is visibly better organized than before.

Relevant code:

- [parser.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parser.go)
- [parse_import.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_import.go)
- [parse_pattern.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_pattern.go)
- [parse_class.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_class.go)

Most importantly:

- `Parser` is more clearly a coordinator
- `parse_pattern.go` is now its own unit
- import/class-specific parsing is no longer buried entirely inside a single declaration bucket

This is a meaningful maintainability gain.

## Findings

## 1. Medium: `RunSandbox.Timeout` still does not bound total compile time

Severity: Medium

Status:

- unresolved
- still the most important sandbox-side implementation gap

Relevant code:

- [sandbox.go](/Users/cwd-k2/Projects/gicel/internal/engine/sandbox.go#L33)
- [sandbox.go](/Users/cwd-k2/Projects/gicel/internal/engine/sandbox.go#L77)
- [sandbox.go](/Users/cwd-k2/Projects/gicel/internal/engine/sandbox.go#L80)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L355)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L361)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L366)

Problem:

`RunSandbox` creates a timeout context before compilation, but compilation still goes through `eng.NewRuntime(source)` synchronously.
That compile path does not accept a `context.Context` and therefore cannot be interrupted by the timeout.

Why it matters:

- if `RunSandbox` is presented as the safe single-call boundary, the timeout semantics are weaker than they appear
- parse/check/opt work can still consume time beyond the configured timeout

Practical interpretation:

- runtime timeout exists
- total compile+run timeout does not

Recommended next step:

Choose explicitly between:

1. Stronger implementation
   Make compile context-aware or externally cancellable.

2. Narrower guarantee
   Re-document `RunSandbox` as a conservative convenience API whose timeout governs evaluation, not a hard bound on the full compile path.

## 2. Medium: selective class import still bypasses normal ambiguity checks

Severity: Medium

Status:

- unresolved
- localized enough to fix without broader redesign

Relevant code:

- [import.go](/Users/cwd-k2/Projects/gicel/internal/check/import.go#L190)
- [import.go](/Users/cwd-k2/Projects/gicel/internal/check/import.go#L199)
- [import.go](/Users/cwd-k2/Projects/gicel/internal/check/import.go#L224)
- [import.go](/Users/cwd-k2/Projects/gicel/internal/check/import.go#L232)
- [import.go](/Users/cwd-k2/Projects/gicel/internal/check/import.go#L282)

Problem:

Ordinary imported values go through ambiguity handling.
Class methods imported via selective class import are still pushed directly into scope.

This affects:

- bare `import M (C)` method import
- `import M (C(...))` selective method import

Why it matters:

- import semantics are inconsistent
- method imports can become order-sensitive in a way normal value imports are not

Recommended next step:

Introduce one shared helper for imported values/methods that performs:

- ambiguity check
- `importedNames` update
- context push

and route class-method import through it.

## 3. Medium: declaration orchestration is still too concentrated in `checkDecls`

Severity: Medium

Status:

- improved surroundings
- still structurally central

Relevant code:

- [decl.go](/Users/cwd-k2/Projects/gicel/internal/check/decl.go#L12)
- [decl.go](/Users/cwd-k2/Projects/gicel/internal/check/decl.go#L74)
- [decl.go](/Users/cwd-k2/Projects/gicel/internal/check/decl.go#L84)
- [decl.go](/Users/cwd-k2/Projects/gicel/internal/check/decl.go#L93)
- [decl.go](/Users/cwd-k2/Projects/gicel/internal/check/decl.go#L108)
- [decl.go](/Users/cwd-k2/Projects/gicel/internal/check/decl.go#L113)

Problem:

The supporting checker structure is better, but declaration pipeline control is still concentrated in one function that owns:

- phase order
- instance-header collection
- annotation collection
- assumption pre-pass
- annotated binding predeclare
- instance body pass
- final value pass

Why it matters:

- phase-local state is still stored as loose locals in a central coordinator
- future declaration semantics will continue to accumulate there
- the checker still lacks a dedicated declaration-phase collaboration object

Recommended next step:

Move declaration phase orchestration into a `declPipeline`-style helper object.

The minimum useful form is:

```go
type declPipeline struct {
    ch          *Checker
    decls       []syntax.Decl
    prog        *core.Program
    annotations map[string]types.Type
    instances   []*InstanceInfo
}
```

and split phase execution into explicit methods rather than extending `checkDecls` further.

## 4. Low: `do` elaboration is split, but its entry layer is still too thick

Severity: Low

Status:

- improved
- not yet fully normalized

Relevant code:

- [elaborate_do.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do.go#L44)
- [elaborate_do.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do.go#L52)
- [elaborate_do.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do.go#L103)
- [elaborate_do_monadic.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do_monadic.go)
- [elaborate_do_mult.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do_mult.go)

Problem:

The monadic and multiplicity-related subflows are now split out, which is good.
However, `elaborate_do.go` still acts as both entry coordinator and substantial implementation body.

Why it matters:

- temporary `do`-specific state and flow control are still more centralized than necessary
- the "front door" remains heavier than it should be

Recommended next step:

Thin the entry layer further so that:

- `inferDo` and `checkDo` are orchestration entry points
- statement-threading logic and CBPV pre/post threading move behind a narrower local helper

## 5. Low: parser doctrine is better, but not fully internally consistent yet

Severity: Low

Status:

- mostly improved
- remaining issues are cleanup-scale, not architecture-scale

Relevant code:

- [parser.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parser.go#L90)
- [lexer.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/lexer.go#L7)
- [parse_decl.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_decl.go#L23)

Problem:

- parser comments still describe an older file arrangement
- non-test code still contains dot import in the lexer
- declaration parsing remains denser than the rest of the parser structure

This is no longer a major structural problem.
It is now a consistency and cleanup problem.

Recommended next step:

- remove non-test dot import from lexer
- align parser file comments with the current layout
- continue reducing `parse_decl.go` into narrower declaration-family entry points

## Priority List

The next tasks are now clearer than before.

## Priority 1: resolve the sandbox guarantee line

Decision needed:

- make compile cancellation real
- or narrow the timeout guarantee explicitly

Why first:

- this affects the external trust boundary
- this is more important than internal tidiness

## Priority 2: fix selective class import ambiguity handling

Why second:

- localized fix
- language/module semantics issue
- low redesign cost

## Priority 3: extract declaration orchestration from `checkDecls`

Why third:

- this is now the main remaining checker concentration point
- the surrounding checker structure is ready enough for it

## Priority 4: thin the `do` elaboration entry layer

Why fourth:

- useful structural cleanup
- now smaller in scope than the declaration-pipeline issue

## Priority 5: finish parser consistency cleanup

Why fifth:

- worthwhile
- but no longer the main structural bottleneck

## Overall Assessment

The current state is stronger than the previous reviewed state.

The codebase has moved from:

- architecture debt spread across many fronts

to:

- architecture debt concentrated in a few identifiable fronts

That is substantial progress.

The remaining work is no longer "find structure."
It is:

- finish the most important semantics/sandbox fixes
- finish the main checker coordination cleanup
- complete parser consistency cleanup

That is a healthier place for the project to be in.
