# Divergence Review

Date: 2026-03-19

Scope:

- Current repository state
- Cross-cutting review of duplicated pathways, overlapping APIs, and parallel implementations
- Includes both active risks and non-bug divergence that may become maintenance debt

This review is broader than a bug review.
It is concerned with places where the codebase achieves the same outcome through
multiple routes, or exposes multiple ways to express nearly the same concept.

Not every item below is a current bug.
Several are structural divergences that are still behaving acceptably today.
They are included because they increase the chance that future fixes or features
land in only one path.

## Executive Judgment

The codebase has improved structurally, but several important responsibilities
are still implemented through parallel flows.

The highest-risk divergence is in the engine compile pipeline.
Module registration and main-source compilation do not go through the same parse
setup, which means parser context can differ depending on entry path.

The second major concentration is `do` elaboration.
The same surface construct is elaborated through several specialized functions
that duplicate statement handling logic.

The remaining divergences are mostly medium- or low-severity design debt:

- import handling has several near-duplicate insertion paths
- optimization is applied differently to modules and main programs
- public type-construction helpers expose overlapping concepts through multiple APIs
- error reporting mostly goes through one helper, but a few direct paths still exist

## Findings

## 1. High: module registration and main compilation use different parser setup paths

Severity: High

Status:

- unresolved
- current structural divergence
- likely to cause semantic drift over time

Relevant code:

- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L158)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L176)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L267)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L275)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L276)

What diverges:

`RegisterModule` performs its own lex/parse flow directly.
`Compile` and `NewRuntime` go through `parseSource`.

The important difference is that `parseSource` injects parser fixity information
from all registered modules before parsing:

- `p.AddFixity(e.modules[name].fixity)`

`RegisterModule` does not.

Why it matters:

- the same language feature can parse differently depending on entry path
- fixes to parser setup can land in `parseSource` and silently miss module registration
- imported operator fixity handling is especially vulnerable to path drift

Practical consequence:

Main-source compilation and module compilation are not fully equivalent frontends.
Even when both are “just parsing GICEL”, they do not start from the same parser state.

Recommended next step:

Refactor the engine so lex/parse setup lives in one shared helper used by:

- `RegisterModule`
- `Compile`
- `NewRuntime`

and make any path-specific behavior explicit in parameters rather than implicit in
separate implementations.

## 2. Medium: optimization pipeline differs between registered modules and main source

Severity: Medium

Status:

- unresolved
- not necessarily incorrect today
- clear semantic drift risk for performance and future transforms

Relevant code:

- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L209)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L374)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L377)
- [runtime.go](/Users/cwd-k2/Projects/gicel/internal/engine/runtime.go#L51)
- [runtime.go](/Users/cwd-k2/Projects/gicel/internal/engine/runtime.go#L126)

What diverges:

Registered modules are compiled and stored after free-variable annotation only.
Main source compiled through `NewRuntime` is optimized and then free-variable annotated.

Why it matters:

- performance characteristics depend on whether code lives in a module or in the main program
- future optimizer passes may accidentally apply only to main source
- debugging and benchmarking become less predictable across code organization choices

Practical consequence:

Two equivalent programs can run through different optimization pipelines depending
on whether helper code is hoisted into registered modules.

Recommended next step:

Choose one explicit policy:

1. optimize both registered modules and main source
2. optimize neither until runtime assembly
3. keep the difference, but document it as intentional

Right now the distinction appears incidental rather than designed.

## 3. High: `do` elaboration is split across multiple parallel implementations

Severity: High

Status:

- unresolved
- currently functioning but structurally fragile

Relevant code:

- [elaborate_do.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do.go#L53)
- [elaborate_do.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do.go#L61)
- [elaborate_do_checked.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do_checked.go#L17)
- [elaborate_do_monadic.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do_monadic.go#L16)
- [elaborate_do_monadic.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do_monadic.go#L65)
- [elaborate_do_mult.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do_mult.go#L20)

What diverges:

The same surface construct, `do`, is handled by several cooperating but separate paths:

- inference path: `inferDo` / `elaborateStmts`
- checked CBPV path: `elaborateStmtsChecked`
- IxMonad/class-dispatch path: `checkDo` / `elaborateDoMonadic`
- multiplicity path layered on top of checked CBPV

Each path re-implements some combination of:

- empty-block rejection
- last-statement validation
- handling of `StmtBind`
- handling of `StmtPureBind`
- handling of `StmtExpr`
- context push/pop behavior

Why it matters:

- semantic fixes must be replicated in several places
- syntax-level behavior for `do` can drift between inference and checking paths
- feature additions to `do` are unusually expensive and error-prone

Concrete asymmetry already visible:

The monadic path special-cases `pure` / `ixpure`:

- [elaborate_do_monadic.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do_monadic.go#L69)
- [elaborate_do_monadic.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do_monadic.go#L94)

That behavior does not exist in the same form in the other elaboration paths.

Recommended next step:

Factor `do` elaboration into:

- one shared statement walker
- pluggable strategy hooks for bind construction, result checking, and state threading

The current split is reasonable as an optimization of concepts, but the duplicated
statement semantics are too high-risk to remain hand-synchronized.

## 4. Medium: import insertion logic is duplicated across several specialized helpers

Severity: Medium

Status:

- unresolved
- already associated with real inconsistency bugs

Relevant code:

- [import.go](/Users/cwd-k2/Projects/gicel/internal/check/import.go#L138)
- [import.go](/Users/cwd-k2/Projects/gicel/internal/check/import.go#L190)
- [import.go](/Users/cwd-k2/Projects/gicel/internal/check/import.go#L267)
- [import.go](/Users/cwd-k2/Projects/gicel/internal/check/import.go#L286)

What diverges:

Imported names are inserted into checker state through several local patterns:

- open value import
- open constructor import
- selective value import
- selective constructor import
- bare class import of all methods
- selective class method import

The operations are similar but repeated:

- ambiguity check
- context push
- constructor-module bookkeeping
- registry updates

Why it matters:

- changes to import semantics must be propagated manually
- subtle inconsistencies are easy to introduce
- previous review findings around selective class import are a direct symptom

Recommended next step:

Introduce shared helpers for:

- importing a value-like name into scope
- importing a constructor into scope
- importing registry-only type/class/family metadata

and make all import forms compose those helpers instead of open-coding their own versions.

## 5. Medium: parser separator handling still exists as multiple local loop patterns

Severity: Medium

Status:

- partially improving
- still visibly duplicated

Relevant code:

- [parse_expr.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_expr.go#L296)
- [parse_expr.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_expr.go#L336)
- [parse_class.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_class.go#L169)
- [parse_class.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_class.go#L325)
- [parse_decl.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_decl.go#L112)
- [parser.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parser.go#L238)

What diverges:

The parser now has shared boundary concepts such as `atStmtBoundary`, but the actual
body loops still exist in several specialized functions with their own recovery logic.

This affects:

- case alternatives
- do statements
- class bodies
- instance bodies
- GADT constructor bodies

Why it matters:

- separator fixes tend to be patched one loop at a time
- local recovery behavior remains inconsistent across body kinds
- future parser cleanup can easily leave one body form behind

Recommended next step:

If the new boundary model is intended to be shared infrastructure, complete the move
by introducing one generic “parse delimited body” helper that owns separator
consumption and stagnation recovery.

## 6. Medium: engine API exposes multiple lifecycle entry points with overlapping responsibility

Severity: Medium

Status:

- partly intentional
- currently useful
- still a source of semantic drift risk

Relevant code:

- [gicel.go](/Users/cwd-k2/Projects/gicel/gicel.go#L18)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L337)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L344)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L361)
- [sandbox.go](/Users/cwd-k2/Projects/gicel/internal/engine/sandbox.go#L33)
- [go-api.md](/Users/cwd-k2/Projects/gicel/docs/agent-guide/go-api.md#L124)

What diverges:

The public surface offers several ways to process source:

- `Parse`
- `Compile`
- `NewRuntime`
- `RunSandbox`

This is not inherently bad.
The issue is that they are not just thinner or thicker wrappers over a single pipeline.
They diverge in:

- whether exports are collected
- whether optimization runs
- whether runtime assembly happens
- whether sandbox limits and panic recovery are installed

Why it matters:

- behavior can drift as the engine evolves
- docs must explain subtle differences, not just lifecycle layers
- users may choose APIs based on convenience and get different semantics than expected

Recommended next step:

Make the internal layering more explicit:

- one shared compile plan
- clearly named flags or stages for parse-only, compile-only, runtime assembly, sandbox execution

This would preserve the public API while reducing hidden divergence underneath.

## 7. Low: public type-construction helpers expose overlapping concepts through multiple entry styles

Severity: Low

Status:

- intentional convenience surface
- not a bug today
- likely to confuse host API users over time

Relevant code:

- [typehelpers.go](/Users/cwd-k2/Projects/gicel/internal/engine/typehelpers.go#L68)
- [typehelpers.go](/Users/cwd-k2/Projects/gicel/internal/engine/typehelpers.go#L79)
- [typehelpers.go](/Users/cwd-k2/Projects/gicel/internal/engine/typehelpers.go#L99)
- [typehelpers.go](/Users/cwd-k2/Projects/gicel/internal/engine/typehelpers.go#L114)
- [typehelpers.go](/Users/cwd-k2/Projects/gicel/internal/engine/typehelpers.go#L119)
- [go-api.md](/Users/cwd-k2/Projects/gicel/docs/agent-guide/go-api.md#L101)

What diverges:

The public API exposes several ways to build closely-related row/record shapes:

- `NewRow().And(...).Closed()`
- `NewRow().And(...).Open("r")`
- `EmptyRowType()`
- `ClosedRowType(...)`
- `RecordType(...)`

These are not interchangeable.
They live at different abstraction levels:

- row type
- record type
- builder
- direct constructor helper

Why it matters:

- host integrations can accidentally construct a row where a record is needed
- the API offers flexibility without a strong “recommended default” path
- docs must teach semantic distinctions that the surface does not enforce

Recommended next step:

Document a preferred style, for example:

- use `RecordType` for closed records
- use `NewRow` only for open-row work
- treat `ClosedRowType` and `EmptyRowType` as lower-level helpers

The issue is not that these helpers exist.
The issue is that they are all first-class with little guidance on when each should be preferred.

## 8. Low: error creation is mostly centralized, but not fully

Severity: Low

Status:

- minor divergence
- worth cleaning eventually

Relevant code:

- [checker.go](/Users/cwd-k2/Projects/gicel/internal/check/checker.go#L401)
- [elaborate_do_monadic.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do_monadic.go#L183)
- [checker.go](/Users/cwd-k2/Projects/gicel/internal/check/checker.go#L152)

What diverges:

Most checker errors go through `addCodedError`.
A few paths still construct and append `errs.Error` directly.

Why it matters:

- formatting or enrichment changes may miss direct-call sites
- error metadata conventions become harder to enforce uniformly
- special-case reporting tends to accumulate over time

Recommended next step:

Keep direct `errors.Add` only for cases that genuinely need custom fields beyond
the helper’s scope.
Otherwise, route everything through one helper family.

## 9. Low: docs and examples already show signs of lifecycle drift

Severity: Low

Status:

- active documentation drift
- symptom of underlying API divergence

Relevant code:

- [README.md](/Users/cwd-k2/Projects/gicel/README.md#L105)
- [README.md](/Users/cwd-k2/Projects/gicel/README.md#L127)
- [go-api.md](/Users/cwd-k2/Projects/gicel/docs/agent-guide/go-api.md#L36)

What diverges:

The lifecycle is documented consistently at a high level, but examples and signatures
can drift as entry points evolve.

Example:

- the README lifecycle example still shows `eng.NewRuntime(...)` in a way that does not match the current context-taking signature

Why it matters:

- users infer the wrong “main path” from examples
- API duplication becomes more expensive because every path needs synchronized docs

Recommended next step:

As part of any lifecycle cleanup, choose one canonical embedding example and keep
all docs aligned with it.

## Synthesis

The codebase does not have one single “duplication problem”.
It has a few specific responsibility clusters where parallel implementations exist:

- engine frontend setup
- `do` elaboration
- import insertion
- parser body/separator handling
- public helper surface design

The first three are the most important because they directly affect semantics.
The last two are less urgent, but they are exactly the sort of divergence that
turns into semantic drift later if left unattended.

## Recommended Order

1. Unify engine parse/setup flow so modules and main source use the same frontend path.
2. Collapse import-side scope insertion into shared helpers.
3. Refactor `do` elaboration around one shared statement walker.
4. Standardize parser body-loop infrastructure around one separator/recovery helper.
5. Tighten public API guidance for row/record helper usage and lifecycle entry points.

