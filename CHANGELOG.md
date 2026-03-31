# Changelog

## v0.25.0 — 2026-03-31

### Quick Look Impredicativity

- Multi-argument constructors now support impredicative instantiation: `Cons (\x. x) Nil :: List (\a. a -> a)` works.
- `bidir_ql.go`: `qlUnify` (shallow structural matching permitting meta → polytype), `checkAppQL` (spine-aware checking), `collectSpine` (App tree flattening).

### Universe Polymorphism Phase B-C

- **Phase B**: Explicit level quantification `\(l: Level) (a: Type l). a -> a`. `SubstLevel` for replacing `LevelVar` inside `TyCon.Level`. Parser extended for `Type l` kind application.
- **Phase C**: `LevelMax` result kind inference for multi-level forms. `ZonkLevelDefault` normalization. Dual level/type substitution at all instantiation sites.

### SMC Phase 4 — Complete

- **Step 1**: `joinGrades` switched from hardcoded `LUB` to `GradeJoin` via `resolveGradeAlgebra`.
- **Step 2**: `UsageSemiring` class with `Trivial` and `Mult` instances. Value-level `multPlus`/`multMult` functions.

### Result e Monad

- Added `Monad (Result e)` and `GIMonad g (Lift (Result e))` instances. do-notation for Result types.

### List map/foldr Fusion

- `_listMap` and `_listFoldr` escalated to Go PrimOps. Fusion rules R15 (map∘map) and R16 (foldr∘map) registered.

### Checker Decomposition (Step A-B)

- `freshMeta` moved from `*Checker` to `*CheckState` via `solverLevel` callback injection.
- Removed 6 unused delegation wrappers from `resolve_bridge.go`.

### IR Verifier

- Structural invariant checker: V1 (no Error nodes), V2 (auto-force Bind structure), V3 (no double-thunk).
- Inserted into `postCheck` pipeline behind `EnableVerifyIR()` debug flag.

## v0.24.0 — 2026-03-31

### Optimizer Phase 2-3

- **`caseOfKnownLit`**: Case elimination when scrutinee is a known literal. Symmetric with `caseOfKnownCtor` for constructor scrutinees.
- **`bindOfCase`**: Push monadic bind into case branches (commuting conversion). Exposes bind-pure elimination inside each branch.
- **List roundtrip fusion**: `_listFromSlice (_listToSlice x) → x` and reverse. Registered as rewrite rules in Prelude.

### Nested Let-Generalization (Phase 2-3)

- **Block expressions** (`{ x := e; body }`) and **do-block pure binds** (`do { x := e; ... }`) now generalize polymorphic bindings.
- Watermark-based meta filtering: only metas born during a binding's inference are candidates for generalization.
- Ambiguity-aware deferral: constraints with metas not appearing in the result type are deferred to the enclosing scope, preventing premature instance resolution.

### Stream `+>` Operator

- Added `infixr 5 +>` (prepend/cons) to `Data.Stream`. Alias for `LCons`.

### PolyKinds Phase D

- **LevelMeta activation**: unified the separate LevelMeta and concrete-L1 paths in `checkTypeAppKind`. Type parameters at any universe level now resolve via `UnifyLevels`.
- Removed the `TypeOfTypes` skip heuristic. Kind inference is now theoretically justified.

### GIMonad Lift Coercion

- Verified that `GIMonad g (Lift M)` instances (Maybe, List) work transparently via type alias expansion. Added tests.

### Bug Fixes

- **`graph.gicel` Ord early resolution**: Fixed premature constraint resolution in `localLetGen` where `Ord ?k` was resolved to `Ord Bool` before annotation type information propagated.
- **`BenchmarkCheckDoBlock` failure**: Added type annotation to bench source. GIMonad do-blocks require `checkDo` (check mode), not `inferDo`.

### Tree-sitter & Editor Sync

- **`lazy` keyword** added to tree-sitter grammar, nvim, zed, and vscode highlights.
- **`@grade` unified in type_application**: removed `@` from `row_field`, parsed uniformly as optional `@` in `type_application`. 76/76 GICEL source files parse without errors.

### SMC Phase 1.5

- Added dag+merge interaction test for parallel composition with state effects.

## v0.23.1 — 2026-03-30

### Constructor Syntax Unification

- **Removed ADT implicit return type syntax.** The old `{ Nothing: (); Just: a; }` form where `()` meant "no arguments" and the return type was implicitly appended by the checker is removed. Constructor declarations now use only two forms:
  - **Pipe shorthand:** `form Bool := True | False` (parser generates full GADT types)
  - **GADT full type:** `form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }`
- **`ADTShorthand` flag removed** from `DeclForm`. Pipe shorthand is now desugared entirely in the parser; the checker sees uniform GADT-style constructor types.

### Documentation

- **Language roadmap updated.** GOAL LINE defined (GIMonad class + universe polymorphism Phase A). Design decisions documented: GradeAlgebra Compose/Join separation, `-|` type application operator, `Merge`/`***` parallel composition, no dependent/refinement types.
- **Universe roadmap added** (`docs/roadmap/language/universe.md`).

## v0.23.0 — 2026-03-30

### Streaming Parser

- **Streaming tokenization.** Replaced bulk `Lexer.Tokenize() → []Token` with on-demand `Scanner.Next()` backed by a `tokenBuffer` sliding window. Tokens are produced lazily and compacted after consumption, reducing peak token allocation for large files.
- **Deferred fixity resolution.** Infix expressions with multiple operators are now parsed as flat `ExprInfixSpine` nodes and resolved post-parse via shunting-yard algorithm. This eliminates the two-pass `collectFixity` scan and enables fixity injection between import parsing and declaration parsing.
- **Phased parse pipeline.** `ParseImports()` → fixity injection → `ParseDecls()` → `ResolveInfix()` replaces the monolithic `ParseProgram()` + `PreScanImportNames()` flow. External module fixity is now injected at the correct point in the pipeline.

### Bug Fixes

- **Non-associative fixity check.** The shunting-yard resolver now correctly rejects equal-precedence chains when _either_ operator is non-associative. Previously only the incoming operator's associativity was checked, allowing `a <| b |> c` to silently reassociate when the stack operator was `AssocNone`.
- **Invalid UTF-8 over-advance.** Two sites in the scanner used `utf8.RuneLen(ch)` to advance past unknown/rune-literal characters. For malformed input, `DecodeRuneInString` returns `(RuneError, 1)` but `RuneLen(RuneError)` returns 3, causing 2-byte over-advance and corrupted spans. Now uses the size returned by `DecodeRuneInString` directly.

### API Changes (Breaking)

- **`NewParser` signature.** `NewParser(ctx, tokens, errors)` → `NewParser(ctx, source, errors)`. The parser now takes a `*span.Source` and creates its own scanner internally.
- **`Lexer` type removed.** Replaced by `Scanner`. Use `NewScanner(source)` + `s.Next()` for token-level access.
- **`PreScanImportNames` removed.** Use `Parser.ParseImports()` instead.
- **Engine `Reader` API removed.** `RegisterModuleReader`, `ParseReader`, `CompileReader`, `NewRuntimeReader` are removed. All methods take `string` source. Callers that have `[]byte` should use `string(data)`.

### Internal

- **tokenBuffer API.** `position()`, `prevToken()`, `prevEnd()`, `save()/restore()/commit()`, `scanForward()` form the complete interface. No direct field access from parser code.
- **`CollectFixityForImports` inlined.** Replaced by `p.AddFixity(store.CollectFixityMap(names))`.
- **Dead code removed.** `parserGuard.reset()`, `continueInfix` precedence parameter, backward-compat bulk tokenization path.

## v0.22.0 — 2026-03-30

### Performance

- **Module cache.** Process-level compiled module cache keyed by SHA-256(source, env fingerprint). Warm E2E drops from 38ms to ~2ms (19x speedup). Cache is safe for concurrent Engine instances via `sync.RWMutex`.
- **Type flag propagation.** `FlagMetaFree` now correctly propagated on `TyEvidenceRow` construction (was always zero — the single largest cold-start improvement: 33ms → 22ms). New `FlagNoFamilyApp` flag skips family reduction on stable subtrees.
- **SubstMany cleanup.** Removed dead `sortedKeys` parameter (16K wasted allocs). Lazy `fvUnion` computation defers `FreeVars` calls until a `TyForall` is actually encountered. New `PreparedSubst` API for batch application of the same substitution to multiple types.
- **Family reduction.** Lazy-init cycle-detection cache (22K map allocs removed). Inlined `MapType` traversal to eliminate closure heap-escape (12K allocs). Two-phase `AppSpineHead` check avoids `UnwindApp` slice allocation on miss path (4K allocs).
- **IR optimization.** Identity-preserving `transformRec` skips node reconstruction when no child changed, making no-op optimization passes near-zero-cost.
- **Solver.** Deferred `Zonk` to error path in `processCtEq`, avoiding redundant double-zonk on the success path.
- **ForEachChild.** Inlined `CapabilityEntries` iteration in `ForEachChild` to avoid `AllChildren` slice allocation.

**Cold-start totals:** 130K → 88K allocs (−32%), 7.8MB → 6.5MB (−17%), 38ms → 20ms (−47%).

### Testing

- **family package.** 62 unit tests covering type family reduction, pattern matching, builtin row families (Merge/Without/Lookup), and injectivity verification (99.6% coverage).
- **modscope package.** 38 unit tests covering import dispatch, ambiguity resolution, ownership detection, and dependency graph traversal (100% coverage).
- **Scale tests.** New `//go:build scale` tagged tests verify O(N) scaling of each pipeline stage (lex+parse, check, post-check) across N=1..1000 declarations.
- **Module cache tests.** Cache hit/miss, env sensitivity, correctness, Prelude sharing, and concurrent safety.
- **Cold-start benchmark.** `BenchmarkEngineEndToEndSmallCold` with per-iteration cache reset for accurate cold-path measurement.

### Infrastructure

- `full-check.sh` now includes `scale` tests alongside unit, probe, stress, bench, examples, and smoke tests.

## v0.21.3 — 2026-03-30

### Security

- **RunSandbox DenyAssumptions.** `RunSandbox` now calls `DenyAssumptions()` automatically, preventing user code from declaring `assumption` bindings in sandbox context. Previously, assumptions passed type checking but failed at runtime with a missing-primitive error.

### Bug Fixes

- **GradeJoin arity.** Corrected `GradeAlgebra` class declaration from `type GradeJoin :: g -> g` (unary) to `type GradeJoin :: g -> g -> g` (binary). The checker already used `GradeJoin` as a binary family; only the Prelude class declaration was inconsistent.

## v0.21.2 — 2026-03-30

### Documentation

- **Keyword count.** Fixed keyword count from 12 to 14 (`as`, `assumption` were missing) in README, agent guide, and grammar reference.
- **Architecture.** Updated from v0.18.0 to v0.21.1. Added `runtime/vm` (bytecode VM) to dependency DAG and package table.
- **RunSandbox limitations.** Documented that `DenyAssumptions` is not called (user code can declare `assumption` bindings) and that timeout does not cover pack application.
- **Exhaustiveness fallbacks.** Documented conservative "covered" assumption for deep patterns (>32 levels) and opaque types, with runtime safety net.
- **Grade system.** Documented `GradeJoin` arity inconsistency between class declaration (`g → g`) and binary enforcement usage. Documented grade enforcement skip when `GradeAlgebra` instance is not found. Corrected "Multiplicity enforcement" from future extension to implemented; "Multiplicity polymorphism" is the remaining extension.
- **sandbox.go comment.** Aligned timeout scope description with actual behavior.
- **Row type family example.** Renamed `row-merge.gicel` → `row-typefamily.gicel` with examples for all three builtin row type families (`Merge`, `Without`, `Lookup`).

## v0.21.1 — 2026-03-30

### Bug Fixes

- **Type family soundness.** Stuck type family equations (`CtFunEq`) whose reduced result conflicted with an already-solved meta were silently swallowed when `OnFailure` was nil (non-grade families). Programs like `f :: P s -> P (Opp s)` could accept `P Active` where `P Inactive` was required. Now reports `ErrTypeMismatch`. Additionally, `zonkInner` tries single-node type family reduction after resolving metas inside `TyFamilyApp` (defense-in-depth).
- **Label variable (`@l`) runtime crashes.** Label erasure only handled concrete label literals (`@#foo`), leaving type variable labels (`@l`) unerased. This caused `non-exhaustive pattern match` crashes when a function with `\(l: Label)` used `@l` in its body. Fixed with context-aware label erasure that lowers `TyLam{l: Label}` → `Lam` and `TyApp{_, TyVar{l}}` → `App{_, Var}` when the body references the label variable.
- **Compile-time `@#label` detection (E0228).** Named capability functions (`*At`) called without `@#label` now produce a compile-time error instead of deferring to the runtime guard. The checker walks Core IR after type checking to find unsolved Label-kinded metas in `TyApp` nodes. Polymorphic wrappers and phantom label parameters are excluded.

## v0.21.0 — 2026-03-30

### Named Capabilities for Mutable Effects

All mutable effect packs now support label-parameterized `*At` variants, enabling multiple independent instances of the same effect type in a single computation.

```gicel
import Prelude
import Effect.Ref as Ref
import Effect.Array as Arr

main := do {
  r <- Ref.newAt @#counter 0;
  Ref.modifyAt @#counter (+ 1) r;

  arr <- Arr.newAt @#buf 3 0;
  Arr.writeAt @#buf 0 42 arr;
  Arr.readAt @#buf 0 arr           -- Just 42
}
```

- **Effect.State**: `getAt`, `putAt`, `modifyAt` (new)
- **Effect.Array**: `newAt`, `readAt`, `writeAt`, `resizeAt`, `toSliceAt`, `fromSliceAt`
- **Effect.Ref**: `newAt`, `readAt`, `writeAt`, `modifyAt`
- **Effect.Map**: `newAt`, `insertAt`, `lookupAt`, `deleteAt`, `sizeAt`, `memberAt`, `toListAt`, `fromListAt`, `foldlWithKeyAt`, `keysAt`, `valuesAt`, `adjustAt`
- **Effect.Set**: `newAt`, `insertAt`, `memberAt`, `deleteAt`, `sizeAt`, `toListAt`, `fromListAt`, `foldAt`, `unionAt`, `intersectionAt`, `differenceAt`

Effect.Map/Effect.Set named `newAt`/`fromListAt` take an explicit `compare` function instead of `Ord k =>` (the compare is stored in the handle at creation time).

### Breaking Changes

- **Effect.Array**: `readAt` → `read`, `writeAt` → `write` (the `*At` names are now used for named capability variants). Update existing code: `readAt i arr` → `read i arr`.

### Bug Fixes

- **OccursIn for label variables.** `OccursIn` now detects label variables in row field positions, fixing a Subst fast-path skip that prevented label substitution in named capability types.
- **RowField.IsLabelVar.** Row fields now track whether their label originates from a forall-bound label variable, enabling correct `FreeVars`/`OccursIn`/`Subst` behavior without false positives from concrete labels.
- **MSet binary ops crash.** `unionAt`/`intersectionAt`/`differenceAt` crashed at runtime because the compare function was not extracted from the handle. Fixed with `withLabelCmpFromHandle` wrapper.
- **`*At` without `@#label`.** Calling named capability functions without `@#label` now produces a clear runtime error instead of an internal crash.
- **`failWithAt` consolidation.** Replaced duplicate `failWithAtImpl` with `withLabel(failImpl)`.

### Documentation

- All stdlib docs updated with named capability variants (34 new functions documented).
- `put` type corrected to `Computation` (not `Effect`) showing pre/post state type change.
- Grammar reference: keyword count 12→14 (`as`, `assumption` are full keywords), add label literal `#name` syntax, add 5 missing built-in types, add 6 missing stdlib packs, fix class count 17→20.
- Language spec: keyword list and count corrected, `DoubleLit` added to grammar, stdlib pack table completed.
- MSet `union`/`intersection`/`difference` docs corrected: return new sets, not in-place mutation.
- Remove ghost operator `+>` (snoc) from grammar reference (not implemented).
- Add missing `Show` instances to Data.Slice/Map/Set/Stream docs.
- Add `range` function to Prelude function list.

## v0.20.1 — 2026-03-29

### Bug Fixes

- **Kind resolution in type checker.** Class and type family kinds are now resolved through `kindOfType`, fixing spurious kind errors for type constructors used in class contexts (P3).

### Performance

- **Lazy TyFamilyApp args allocation.** Args arrays in zonk, type family reduction, substitution, and MapType use nil-until-change pattern, avoiding allocation when no child changes.
- **Pooled strings.Builder for type keys.** `TypeKey` and `TypeListKey` reuse builders via `sync.Pool`, reducing GC pressure in hot paths (type family cache keys, instance dictionary names).
- **HasMeta flag for zonk early bailout.** Composite types carry a `FlagMetaFree` flag. When set, `zonkInner` returns immediately without walking the subtree. Flags propagate through zonk, MapType, substitution, and factory functions.
- **OccursIn guard in Subst.** Single-variable substitution skips the full walk when the variable does not occur in the target type.

### Refactoring

- Extract `typeResolver` from Checker (design debt P1).
- Resolve design debt P5 (dead code), P11 (naming), P1 (god module preparation).

## v0.20.0 — 2026-03-29

### Bytecode VM

The tree-walker evaluator has been replaced with a bytecode compiler and virtual machine. All programs are compiled to bytecode at `NewRuntime` time; the VM dispatch loop handles TCO, pattern matching, and explain-mode instrumentation.

- **Bytecode compiler**: Core IR to stack-based bytecode with closure capture (STG flat-capture model).
- **VM dispatch**: single `execute()` loop with opcode dispatch, frame management, and tail-call optimization.
- **Explain/observe parity**: `--explain` and `--explain-all` produce identical output to the former tree-walker.
- **Applier callback**: isolated sub-execution for host-called GICEL closures (no stack corruption).

### Security

- **`assumption` gated in user code.** User-written GICEL source can no longer use `assumption` declarations. This closes a capability bypass where user code could bind any registered primitive with an arbitrary type signature (e.g., declaring `put` with empty capability row). Stdlib modules are unaffected. Go API users can opt in via `eng.DenyAssumptions()`.
- **Panic recovery sanitized.** Recovered panics from primitives no longer expose Go stack traces, filesystem paths, or memory addresses in error messages.
- **Security documentation.** Trust boundary section in Go API docs now covers `--module` path validation and `assumption` gating.

### Error Messages

- **if-condition**: `if 1 then ...` now shows "expected Bool, got **Int**" instead of "expected Bool, got Bool".
- **Haskell-style case**: `case x of { ... }` now shows "case syntax does not use 'of'; write 'case expr { ... }'" instead of confusing "expected function type" + "unbound variable: of".
- **let...in**: `let x = 1 in body` now shows "unknown keyword 'let'; use { name := expr; body }" instead of opaque "expected declaration".
- **Constructor arity**: `MkBox x y` for a 1-field constructor now shows "constructor MkBox expects 1 field(s), but pattern has 2" instead of "expected function type".
- **"Did you mean?" filtering**: Variable names only suggest other variables; operators only suggest operators. Previously `x` would suggest `$`, `&`, `*`.
- **Operator hints**: Unbound operators now get "did you mean" hints (e.g., `+++` suggests `+`, `<+`).

### CLI

- **`-e` + file conflict warning.** Using both `-e` and a file argument now emits a warning.
- **`--explain-all` / `--verbose` without `--explain`** now emits a warning instead of being silently ignored.
- **`version` command** listed in usage text.
- **Global usage simplified** to command list with "Use 'gicel <command> --help' for flag details."

### Performance

- **O(n1+n2) merge-join** for Set/MSet `intersection`, `difference`, `union` (was O(n1\*log(n2))).
- **Lazy CtOrigin contexts** via closures, avoiding premature pretty-printing.
- **String concatenation** replaces 64 `fmt.Sprintf(%s)` calls in hot paths.

### Refactoring

- Remove tree-walker evaluator; VM is the sole execution engine.
- Structured `UnifyError` fields (replaces string Detail).
- Structured stdlib error types (`ErrTypeMismatch`, `ErrMalformed`).
- Consistent `errMalformed` usage across all stdlib packs.
- Extract `scanNumeric` from lexer, `inferApply` from type checker.
- Split `format.go` into `explain.go` + `json.go`.
- Remove dead `generationScope` infrastructure from solver.
- Inline redundant solver delegation wrappers.

### Docs & Examples

- New `patterns.pointfree` example: sections, composition, flip, const, on, pipeline-style.
- Go API trust boundary documentation expanded with security notes.

## v0.19.0 — 2026-03-28

### Type System

- **Type/Kind unification.** Abolished the Kind interface; types and kinds share a unified universe with stratified levels (`Type ≤ Kind ≤ Sort₀`). Kind cumulativity allows ground kinds to unify with `Sort₀`.
- **OutsideIn(X) constraint infrastructure.** Constraint emission (`emitEq`), given equalities processed by solver with kick-out and contradiction detection, `CtOrigin` tracking, solver-level scoping.
- **Grade algebra (L0–L3).** `GradeAlgebra` type class for user-defined grades, `Mult` form, `MultJoin` in Prelude, `CtFunEq` deferred LUB, dynamic resolution via instances.
- **Equality constraints (`~`).** `a ~ b` syntax in type signatures with `CtEq` solver constraint.
- **Merge type family.** `Merge` for disjoint row merging at the type level.
- **Non-nullary constructor promotion.** Promoted constructors carry kind arrows at the type level.
- **Label kind.** `Label` as a first-class kind; `Without` and `Lookup` builtin row type families.
- **Named capabilities.** `@#label` syntax for `getAt`, `putAt`, `failWithAt` — multiple independent effect slots.

### Language

- **`rec` computation-level fixpoint.** `rec` now correctly implements effectful loops via self-referential `ThunkVal` with auto-force in Bind chains. Previously broken (returned `<function>` instead of executing).
- **`#name` label syntax.** Replaced backtick label literals with `#name`.
- **Scoped evidence injection.** `value => expr` for local instance override with private instances.
- **"Did you mean?" suggestions.** Unbound variables and constructors now suggest similar names.

### Stdlib

- **Data.Stream expansion.** `filter`, `zip`, `iterate`, `takeWhile`, `repeat`.
- **Effect.Set algebra.** `union`, `intersection`, `difference`.
- **Stdlib expansion.** Additional Prelude functions, Map/Set operations, Ref, JSON encode/decode.
- **`fromJSON` null handling.** All `fromJSON` primitives now return `Nothing` for JSON `null` (previously returned zero values like `Just 0`).

### Runtime & Performance

- **RecordVal optimization.** Internal representation changed from `map` to sorted slice — eliminates per-record map allocation.
- **43% E2E speedup.** Zonk defer elimination, `ForEachChild` replacing `Children()` allocs, incremental skolem skip, context index for instance resolution, Pretty restricted to error paths.
- **Native stdlib timeout enforcement.** `--timeout` is now respected in native Go loops (`foldl`, `replicate`, `length`, `range`, `join`) via periodic context checks.
- **`--max-depth` tracks logical recursion depth** through TCO trampolining.

### Bug Fixes

- Concretize label variables in row field substitution.
- Reject user instances overlapping with stdlib (coherence).
- Reject ambiguous type variables in constraint-only position.
- Reject `force` on `Computation` at type-check time (type soundness).
- Scope fixity injection to import closure; detect duplicate fixity declarations.
- Occurs check in `SolveFreshMeta`; kind check in `TyForall` unification.
- LookupLocal bounds check; float→int NaN/Inf guard; budget underflow guard.
- Associated type family equation preservation in diamond imports.
- Normalize ADT JSON encoding — nullary constructors use object format.
- Guard runtime error formatting against nil values (no more `%!s(<nil>)` leaks).

### Docs

- Fixed broken examples: Named Capabilities bare Computation, `Elem` stdlib name conflict, `rec` example, "Int literals require Prelude" inaccuracy.
- Removed completed performance hotspots roadmap.

## v0.18.0 — 2026-03-25

### De Bruijn Index + Array Environment

Complete migration from name-based parent-chain environment to de Bruijn indexed array environment. The evaluator now uses integer indices for all variable lookups — no hash maps on the hot path.

- **Locals**: raw `[]Value` slice, indexed by de Bruijn index
- **Globals**: slot-indexed `[]Value` on `Evaluator`, assigned by `AssignGlobalSlots`
- **Closure capture**: flat capture (STG model) with pre-allocated capacity for subsequent `Push`
- **IR pass**: `AssignIndices` assigns local de Bruijn indices; `AssignGlobalSlots` assigns global slots
- **Clone fix**: shared-node index corruption in `SubstitutePlaceholders` prevented by deep clone during beta-reduction

**Measured improvements** (vs v0.17.2):

- Per-iteration: 190ms → 94ms (**-51%**)
- Mallocs/iter: 1.62M → 1.07M (**-34%**)
- Alloc/iter: 141MB → 76MB (**-46%**)

### Compile Speed Optimization

Cumulative ~15% allocation reduction across the type checker and IR pipeline.

- **ZonkEntries**: lazy allocation — only materialized when entries exist
- **subsCheck**: redundant zonk removal
- **TyCon**: singleton interning for common type constructors
- **OccursIn**: early-exit on non-meta types
- **TyFamilyApp**: lazy allocation for reduction results
- **Context evidence index**: O(1) evidence lookup by type key
- **FreshInstanceSubst**: free-variable cache to avoid redundant walks
- **ModuleCache**: cross-engine sharing of compiled module results
- **ir.Program immutability**: mutation eliminated, enabling safe sharing

### Field Test Fixes (3 rounds)

Issues discovered by zero-context field test agents:

- **Deep operator chain**: deeply nested infix expressions no longer cause stack overflow in the parser
- **Token allocation**: pre-allocated token slice reduces lexer allocation
- **Cycle detection**: `let x := x` now reports a cycle error instead of hanging
- **MaybeT example**: corrected to use proper transformer pattern
- **Docs fixes**: multiple corrections to built-in documentation accuracy

### Checker Refactoring

Major internal restructuring of the type checker for maintainability:

- **solve_bridge.go**: consolidated solve boundary (checker ↔ solver interface)
- **Type alias relay removed**: unified all references to `env` package directly
- **check_pattern.go renamed**, `unify/zonk.go` split into focused files
- **processClassLikeForm / processImplHeader**: phase extraction into discrete steps
- **Registry immutability**: documented and reduced `buildMethodSelector` parameter surface
- **Naming conventions**: dependency inversion, code deduplication across checker

### Documentation

- Architecture doc updated with `check/solve` package
- Language roadmap reorganized into `language/` directory

## v0.17.2 — 2026-03-25

### Bind-chain Tail-Call Optimization

Do-block loops via `fix` no longer consume evaluation depth. Previously, each `<-` in a do-block grew the Go call stack by one frame, so a loop of N iterations required `--max-depth >= N`. Now Bind returns a trampoline bounce (like closure application and `force`), keeping depth flat regardless of chain length.

- **`--max-depth` default raised** from 100 to 10,000. With TCO, this limit now affects only non-tail recursive closure calls, not sequential do-blocks.
- **`bounceVal.forceSpan`** — new field that defers `ForceEffectful` to the trampoline, preserving correct forcing of bare effectful PrimOps at the end of do-blocks (e.g., `do { put 42; get }`).

### Evaluator Allocation Optimization

Reduced intermediate `map[string]Value` allocations across pattern matching and environment operations.

- **Pattern matching** — `matchDepth` rewritten to collect bindings into a `[]binding` slice via `append`, materializing the final map once in `Match()`. For a 3-variable nested pattern: 13 allocs / 2064B → 5 allocs / 560B.
- **`Env.TrimTo`** — fast path (flat env) uses direct map lookup; slow path walks the chain with a name set, avoiding a full `flatten()` allocation.
- **`Env.ExtendMany`** — single-allocation merge. Reuses parent's flat cache when available, otherwise walks the chain directly into the combined map.

### Benchmarks & Scaling Tests

- **Engine benchmarks** — 500-decl compile, all-packs load, do-block compile cost, Effect.Array and Data.Map end-to-end
- **Check benchmarks** — do-block type inference scaling (5/15/30 binds), 500-decl throughput, overlap resolution scaling (50/200 instances)
- **Eval benchmarks** — `Env.TrimTo`, nested pattern match allocation
- **`scripts/scaling-test.sh`** — parameterized wall-clock scaling tests for sort, map, sieve, set, and do-block suites

### Measured Impact

Scaling tests (wall-clock, merge sort N=2000): 0.53s → 0.42s (21% improvement).
Sieve N=50000: 2.43s → 1.93s (21% improvement).
Eval stress test (recursive_data): 282KB / 3621 allocs → 251KB / 3473 allocs per iteration.

## v0.17.1 — 2026-03-25

### Examples Overhaul

All 54 CLI examples now produce visible Console output when run with `bin/gicel run`.

- **Console output** added to every example — no `--show` or `--json` needed to see results
- **Standardized headers** — all files have `-- GICEL Example: <title>` format
- **Removed 4 files**: `types/session` (check-only, requires Go host), `types/multi-module` (stub), `types/data-family-polymorphism` and `types/fundep-inference` (merged into parent examples)
- **Bug fixes**: `algorithms/prefix-sum` (stdin dependency, Data.Slice name collision with `--packs all`), `algorithms/sieve` (same collision)
- **`patterns/comonad`** trimmed from 214 to 168 lines
- **`types/fundeps`** comments corrected (no explicit fundep syntax)

### Fixes

- **Show parenthesization** — `show (Just (Just 42))` now produces `"Just (Just 42)"` instead of `"Just Just 42"`. Same fix for `Result` (`Ok`/`Err`).
- **MMap.size / MSet.size** — changed from pure to effectful. The previous pure signature read mutable state at demand time, returning stale values when bound with `:=`.
- **Constraint solver retry** — deferred constraints are re-checked after worklist completion, catching cases where later unification resolved ambiguity.

### Editor Support

- **tree-sitter**: `if_expression` node added; `let_statement` extended to accept tuple/record pattern bindings; all 54 examples parse without errors
- **nvim/zed**: `if`/`then`/`else` keywords, `type_family`, `method_signature`, `constraint`, `evidence_injection` captures added
- **vscode**: `if`/`then`/`else` keywords added, `data` → `form` rename fix in TextMate grammar

### Known Limitations

- **State + polymorphic `show`** — `do { put 42; n <- get; show n }` infers a polymorphic type instead of resolving `Show Int`. Workaround: use `showInt n` or `show (n :: Int)`. Root cause: Atkey bind elaboration doesn't propagate row-solved types to value positions.

## v0.17.0 — 2026-03-24

### Console Pack (CLI-only)

Real stdio for CLI programs. Unlike `Effect.IO` (which buffers), Console writes to stdout and reads from stdin directly.

- `putLine` — write string + newline to stdout
- `getLine` — read line from stdin
- Capability: `{ console: () | r }`
- `--json` mode: Console output captured in `capEnv.console`
- Security: not available in `RunSandbox` or Go embedding API

### CLI Output Redesign

- **Default**: stdout is Console output only. Result value not displayed.
- **`--show`**: opt-in result value display on stdout.
- **`--json`**: unchanged (structured JSON with value, stats, capEnv).

### Examples

- `basics.hello` rewritten with Console
- `basics.echo` — interactive stdin/stdout
- `algorithms.fizzbuzz` — FizzBuzz with Console and recursion

## v0.16.5 — 2026-03-24

### UX

- `fix`/`rec` without `--recursion` now hints "requires --recursion flag" instead of generic "unbound variable"
- Diagnostic source line output truncated at 200 characters (prevents 65MB+ output on pathological inputs)
- Step limit error no longer leaks internal names like `IxMonad$Computation`

### Compatibility

- UTF-8 BOM (byte order mark) stripped at lexer entry point

## v0.16.4 — 2026-03-24

### Fixes

- **Tuple/record exhaustiveness** — Record pattern specialization now propagates field types to the exhaustiveness checker. Previously `case (t1,t2) { (A,A) => ...; (B,B) => ... }` was falsely accepted as exhaustive; now correctly reports missing patterns.
- **Duplicate binding detection** — `f := 1; f := 2` now emits E0280 "duplicate binding" instead of silently using the first definition.
- **ADT shorthand with constructor arguments** — `form Nat := Zero | Succ Nat` now correctly registers `Succ` as `Nat -> Nat` (was registering with unit return type, causing "expected Nat, got Record {}" on nested use).
- Remove dead fundep implementation (never reachable from surface syntax)

## v0.16.3 — 2026-03-24

### Fixes

- **RunSandbox custom entry point** — `SetEntryPoint` now called before `NewRuntime`, fixing E0291 false positive on non-main entry bindings with bare Computation type
- **Literal pattern parse error** — returns wildcard pattern instead of zero-value match on overflow/invalid literals
- **Superclass resolution** — prevents visited set pollution when `ClassFromDict` returns unknown dict type
- **Instance body nil guard** — `processInstanceBody` guards against nil `ClassInfo` from unregistered class lookup
- **Primitive panic stack trace** — `callPrim` now preserves Go stack trace in error message
- **List literal guard** — emits "list literals require Prelude" when Nil/Cons are not registered
- **CLI `check --json` error path** — now outputs structured JSON on preflight errors (was plain text)
- **CLI timeout propagation** — `--timeout` now covers `--module` compilation (was only main source)
- Remove unused `ErrClassSyntax` error code

## v0.16.2 — 2026-03-24

### Architecture

- **Solver extraction** — Constraint solver and instance resolution moved to `check/solve/` sub-package with consumer-defined `Env` interface (23 methods). The `check` package is no longer a god module for constraint solving.
- **Layer violations fixed** — `lang/types` no longer imports `lang/syntax`; `runtime/eval` no longer imports `lang/syntax`. Both layer violations resolved by inlining trivial `TupleLabel` computation.
- **`Limits` type cleaned** — `entryPoint` and `checkTraceHook` moved to `Engine` struct. `Limits` now contains only resource limits.
- **`CompileResult` simplified** — Removed dual type representation (`values` field eliminated; `BindingTypes()` derives from `prog.Bindings`)
- **Registry encapsulation** — Direct field access in `alias.go` and `exhaust.go` replaced with accessor methods
- **Context types shared** — `CtxEntry`, `CtxVar`, `CtxEvidence` moved to `check/env/` for cross-package access

### Public API

- Re-export `NestingLimitError`, `KindRow`, `ForallKind`

### Fixes

- `ExplainObserver.EnterInternal`/`LeaveInternal` now nil-safe (latent SIGSEGV)
- `sandbox.go` uses `SetCompileContext` instead of direct field access
- Remove dead `ModuleStore.Get()` and `Order()` methods
- Rename `CapEnv.cow` → `shared` (semantic inversion fix)

## v0.16.1 — 2026-03-24

### Internal

- Purge legacy surface syntax from internal naming (`decomposeData` → `decomposeForm`, `registerClasses` → `registerClassLikeForms`, `processClassDecl` → `processClassLikeForm`)
- Remove dead code: `//go:build legacy_syntax` test files, migration scripts (`migrate-test-syntax`, `convert-tf`, `migrate-syntax.sh`)

### Documentation

- Fix grammar specification: `'data'` → `'form'` in `grammar-reference.md` and `spec/language.md`
- Remove undocumented backtick infix syntax from agent guide
- Fix thunk/force example that failed bare Computation check
- Update smoke test to current syntax (`form` keyword, `=>` case arrows, GADT constructor syntax)
- Update multi-module example files to current syntax

## v0.16.0 — 2026-03-24

### Breaking: Unified Syntax

The `class`/`instance`/`where`/`family` keywords are replaced by `form`/`impl`/`type`.

- **Type classes** — `form Eq := \a. { eq: a -> a -> Bool; }` (was `class Eq a where eq :: a -> a -> Bool`)
- **Instances** — `impl Eq Int := { eq := \x y. True; }` (was `instance Eq Int where eq = ...`)
- **Type families** — `type F :: Type := \a. case a { ... }` (was `type family F a :: Type where ...`)
- **Case alternatives** — `case x { True => 1; False => 0 }` (was `True -> 1; False -> 0`)
- **ADT shorthand** — `form Bool := True | False` for simple enums

### Scoped Evidence Injection

- **`value => expr`** — inject a dictionary into local scope for constraint resolution
- **Private instances** — `impl _name :: Type := expr` is solver-invisible; accessible only via `=>`
- No overlap with public instances; not exported across module boundaries

### Evidence System — L9/L10

- **L9 generic defaults** — `equal.go`, `key.go`, `pretty.go` have generic default cases for future evidence fiber types
- **L10 grade constraints** — `$GradeJoin`/`$GradeDrop` internal type families; `checkGradeBoundary` emits `CtFunEq` for grades containing metavariables

### DataKinds

- All constructors (including non-nullary) promoted to type level
- Enables universe decoding patterns: `type Decode :: Type := \(u: U). case u { Set => Int; (Pi a b) => Decode a -> Decode b }`

### Parser

- `parseTypeCaseBody` — `->` (function arrow) allowed in type family case bodies
- `value => expr` parsed as scoped evidence injection (right-associative, below annotation)

### Type Checker

- `matchArrow` reduces type family applications on demand for arrow decomposition
- Bare row `{}` / `{ x: Int }` in type position unifies with `Record {}` / `Record { x: Int }` from expression position
- `SubstMany` uses sorted key iteration for deterministic capture-avoidance
- `substQuantifiedConstraint` applies capture-avoidance rename before substitution
- Zonk path compression skips trail outside snapshot scopes (memory optimization)
- `--timeout` now covers the compile phase (was eval-only)

### Refactoring

- Legacy adapter layer fully eliminated — `syntax_adapt.go` contains only decomposition utilities
- `structuralKey` replaced with canonical `TypeKey` (injectivity guarantee)
- `ReduceEnv.Families` raw map removed; `LookupFamily` callback is the single path
- `processDataDeclParts` caches decomposition results in `declPipeline`
- Unnecessary `maps.Clone` removed from compile and runtime paths
- Context double-push for annotated bindings fixed

### Error Messages

- Row/evidence jargon replaced with user-friendly terms
- Metavariables rendered as `_` instead of `?N`
- Redundant "type mismatch" suffix removed
- Arrow types parenthesized in "no instance for" messages

### Documentation

- All 51 GICEL examples updated to unified syntax
- All 21 Go examples updated to unified syntax
- Agent-guide topics (expressions, patterns, ADT, type classes, type families, session types, stdlib) updated
- Grammar reference and language spec updated

### Bug Fixes

- ADT shorthand: single-field constructor now builds arrow type (was using bare field type)
- Grade-count mismatch in row unification and case join now errors (was silent truncation)
- `joinGrades` grade-count mismatch errors consistently with row unification
- `usageJoin` alphabetical sort: `Join(Zero, Unrestricted)` now correctly returns `Unrestricted`
- Double form family registration (if → else if) in processImplHeader
- `TyRowTypeDecl` and `AstBind` span calculations use byte offset (was token index)
- Multi-constructor form family diagnostic uses `ErrParseSyntax` (was `ErrClassSyntax`)
- `resolveFromSuperclasses` respects `SolverInvisible` flag

---

## v0.15.4 — 2026-03-23

### Type Checker — OutsideIn(X) L4: Touchability

Level-based touchability for type inference. No surface syntax changes; internal precision improvement for GADT branches and higher-rank forall scopes.

- **Touchability guard** — `Unifier.SolverLevel` field controls meta touchability: metas with `Level < SolverLevel` are untouchable. `UnifyUntouchable` error kind + `ErrUntouchable` diagnostic code added
- **GADT branches** — `checkCaseAlts` migrated from `withDeferredScope` to level-based inline solving via `checkWithLocalScope`. `solver.level` incremented before body check (inner-level metas); `SolverLevel` raised only during constraint solving (preserving DK eager unification). Residuals partitioned into floatable (outer scope) and stuck (error)
- **Higher-rank forall** — `solver.level` tracking added around forall body checking. `checkSkolemEscapeInSolutions` retained as belt-and-suspenders safety net
- **CtImplication type** — implication constraint for solver-dispatched scoping (future infrastructure; not yet emitted in production)
- **`withTrial`/`withProbe` exemption** — touchability disabled in trial/probe scopes (solutions not committed / always rolled back)
- **ConstraintVar shouldDefer** — normalized ConstraintVar constraints now subject to `shouldDefer` in local scopes, closing a protocol hole where instance resolution could bypass touchability

### Type Checker — Structure

- **`withDeferredScope` removed** — replaced by level-based `checkWithLocalScope`
- **`partitionResiduals` extracted** — single implementation for stuck/floatable residual classification
- **`ctPlaceholder` removed from `Ct` interface** — was semantically meaningful only for `CtClass`; `CtFunEq`/`CtImplication` returned empty string. Interface reduced to `ctMarker` + `ctSpan`
- **`localShouldDefer` helper** — shared deferral predicate for implication scopes (defers unsolved-meta constraints to prevent inner-scope resolution)

---

## v0.15.3 — 2026-03-22

### Type Checker — Import/Export Separation

- **`modscope` subpackage** — 15 import functions (464 lines) extracted from `Scope` methods to `modscope.Importer` with callback-based `ImportEnv`, following the `exhaust.CheckEnv` / `family.ReduceEnv` pattern. `Scope` reduced from 12 fields to 4 (session/reg/config references removed)
- **`ModuleExports` moved to `env/`** — pure form type relocated to `env/module_exports.go`; `checker.go` retains a type alias for compatibility
- **Registry encapsulation completed** — 7 iteration accessors added (`AllConInfo`, `AllAliases`, `AllClasses`, `AllInstances`, `AllPromotedKinds`, `AllPromotedCons`, `AllFamilies`); `export.go` migrated to zero direct field references
- **`isPrivateName`/`isOperatorName` consolidated** — moved to `env/names.go` (exported); duplicate definitions in `export.go` and `modscope/import.go` removed

### Type Checker — Bug Fix

- **Class method ambiguity bypass fixed** — when two modules export the same class name, methods of the ambiguous class are now blocked in both `importOpen` (via `ambiguousClassMethods` exclusion set) and `importSelective` (via ambiguity gate on method import block). Previously, orphaned method values remained in scope without a registered class
- **`ownedAllNames` private constructor leak fixed** — private constructors (`_`-prefixed, `$`-containing) of public types are now excluded from `OwnedNames`

### Type Checker — Documentation

- `checkAmbiguousName` doc corrected: removed inaccurate Core exemption claim, "$-prefixed" → "$-containing", added `_`-prefix mention
- `Import` doc expanded to describe callback side effects
- `OwnedNames` comment precision: "type names" → "data type names"

---

## v0.15.2 — 2026-03-22

### Type Checker — Structure

- **tryResolveInstance worklist isolation** — `tryResolveInstance` now saves and restores the solver worklist around resolution attempts; orphan constraints from failed `emitClassConstraint` calls no longer leak into subsequent `solveWanteds` cycles
- **withProbe/withTrial separation** — new `withProbe` always rolls back unifier solutions regardless of outcome; `withTrial` retains commit-on-success semantics. Two call sites in `resolveQuantifiedConstraint` migrated to `withProbe` (pure unifiability tests that discard solutions)
- **checkWithEvidence Push/Pop hardening** — replaced fragile `len(dicts)*2` literal with a `pushed` counter that tracks each `ctx.Push` call, ensuring Pop count stays correct if Push structure changes
- **exhaust.CheckEnv callback** — `FreshID *int` raw pointer replaced with `Fresh func() int` callback, confining `freshID` mutation to `Session`

### Type Checker — Boundaries

- **qualified name injection** — `resolveTypeExpr` no longer mutates Registry when encountering qualified type references (`M.Alias`, `M.Family`); injections are cached in `Scope.injectedAliases`/`injectedFamilies` instead. Registry writes are now confined to declaration processing phases
- **Registry read accessors** — 16 read methods added (`LookupConType`, `LookupClass`, `InstancesForClass`, `ClassFromDict`, `IsKindVar`, etc.); all single-key map lookups across 14 files migrated to method calls. Internal representation is now encapsulated
- **Checker-level lookupAlias/lookupFamily** — unified lookup that searches both Registry (declaration phases) and Scope injections (qualified references), with nil-safe fallback for test Checkers

### Type Checker — Contracts

- **solver.level reservation** — documented as reserved for OutsideIn(X) L4 touchability; code assuming `level == 0` flagged for future review
- **resolveInstance recursion contract** — depth limit (budget.EnterResolve, default 64), no cycle detection, and meta solution accumulation semantics documented
- **Registry phase annotations** — `RegisterAlias` (phase 2), `RegisterFamily` (phase 3) annotated; qualified names use Scope injection instead
- **withTrial/withProbe scope contracts** — documented MUST NOT constraints (emit constraints, push/pop context, mutate inert set)

### Cleanup

- **evidence.go removed** — empty file (package declaration only)

---

## v0.15.1 — 2026-03-22

### Parser

- **speculate step budget fix** — `speculate()` now restores `guard.steps` on rollback; speculative parse failures no longer permanently consume step budget
- **progressGuard** — new loop guard type enforcing iteration limits and stagnation recovery, applied to 8 unbounded parser loops (infix chains, application chains, type application chains, instance/class constraints, row types, record literals/updates)

### Type Checker

- **ModuleExports ownership model** — `DataDecls []ir.DataDecl` replaced with precomputed `OwnedTypeNames`/`OwnedNames` maps; ownership checks are now O(1) instead of O(n) linear scans
- **withTrial comment correction** — documented that only unifier state is rolled back (not inert set or worklist)
- **declPipeline phase reference** — phase dependency overview added to pipeline coordinator

---

## v0.15.0 — 2026-03-22

### Architecture

- **Budget layer split** — `Budget` (runtime: steps, depth, nesting, alloc) and `CheckBudget` (compiler: tfSteps, solverSteps, resolveDepth) are now separate types. The compiler/runtime boundary is enforced at the type level
- **Registry extraction** — `Registry` struct and its 15 methods moved from `checker.go` to `registry.go`. Dict-to-class reverse map (`dictToClass`) replaces `isDictName`/`classFromDict` string heuristic
- **env → syntax forward reference eliminated** — `InstanceInfo.Methods` (unevaluated `syntax.Expr`) moved to a pipeline-local map, removing the `syntax` import from `check/env/types.go`
- **parse → types layer violation fixed** — `TupleLabel` canonical definition placed in `syntax`; parser no longer imports `lang/types`. All callers migrated to `syntax.TupleLabel`, removing `types.TupleLabel` delegation wrapper
- **Structural provenance flags** — `ir.Lam.Generated`, `ir.Bind.Generated`, `ir.Alt.Generated` replace the `isCompilerGenerated` string heuristic. Compiler sets flags at elaboration; evaluator reads them directly
- **tryResolveInstance** — centralizes the error-save/truncate probe pattern for instance resolution without emitting errors

### Examples

- **5 GICEL examples fixed** — continuation, nondeterminism, maybet, free-monad (renamed from ixmonad), session: corrected Monad/IxMonad usage and bare Computation wrapping
- **All 45 GICEL examples pass** — 44 run + 1 check-only (session types)

---

## v0.14.0 — 2026-03-21

### Architecture

- **Layered directory structure** — `internal/` restructured into `lang/`, `infra/`, `compiler/`, `runtime/`, `host/`, `app/` layers with explicit dependency direction
- **Checker service extraction** — `Session`, `Registry`, `Scope`, `Solver` as named types with method-based mutation contracts. All Registry writes go through named methods (`RegisterTypeKind`, `RegisterAlias`, `ImportInstance`, etc.)
- **Parser guard extraction** — safety harness (step/depth limits, halt flag) separated into `parserGuard` struct
- **Engine compile path unification** — shared `postCheck` helper; `compileModule` now accepts `context.Context` for cancellable module compilation

### Type Checker

- **TypeFamilies export boundary** — modules only export locally defined or locally enriched type families (not purely inherited ones)

### Stdlib

- **Data.Map expansion** — `keys`, `values`, `mapValues`, `filterWithKey`
- **Data.Set expansion** — `union`, `intersection`, `difference`

### CLI

- **`--use` → `--packs`** — flag renamed to convey "restrict to these packs"; `--use` kept as silent alias
- **Runtime error source locations** — text output shows `line:col:` prefix; JSON includes `line`, `col`, `message` fields
- **Preflight JSON errors** — bad flags and setup errors respect `--json` with `"phase": "preflight"`
- **Compile-error JSON hints** — `Diagnostic.Hints` field with secondary annotations
- **JSON allocation stats** — `"allocated"` field in success stats
- **Explain failure path** — `--explain` trace flushed on runtime errors (text and JSON)
- **Explain module names** — `--verbose` shows `[SourceName]` on module transitions
- **Explain internal distinction** — `--explain-all` dims stdlib steps in color mode

### Engine

- **RunSandbox enhancements** — `SandboxConfig.Context` for parent context propagation; `Explain` and `ExplainDepth` fields for trace hooks. Timeout now covers pack application + compilation + evaluation
- **`SetCompileContext`** — public method to bound module compilation with a context

### Documentation

- **README streamlined** — 346 → 212 lines; restructured around sandbox/PoC/embedding selling points
- **Agent guide** — minimal example fixed (`main := ()`), `--max-nesting` added to flag tables, `-e` semicolon note, `--explain-all` behavior note, Effect.IO behavior clarification, Map/Set qualified import tips, host API migration path, trust boundary section, observability hooks table
- **Operator docs fix** — `¦¦` → `||`, `<¦>` → `<|>` in functions.md
- **Stale `Std.*` references** — 6 example files updated to current module names
- **CLAUDE.md** — Rules moved to top, `--max-nesting` in flag table, probe test execution policy
- **Roadmap restructured** — split into `direction.md` (project principles), `language.md` (type system roadmap), `library.md` (stdlib/tooling roadmap). Version numbers removed; items ordered by dependency

### Testing

- **Parser/budget benchmarks** — `parse_bench_test.go` (5 benchmarks), `budget_bench_test.go` (5 benchmarks)
- **Stress harness split** — monolithic `stress_test.go` split into 6 domain files (types, typeclass, effect, stdlib, grammar, helpers)
- **Boundary test structured assertions** — `strings.Contains` replaced with `Diagnostic.Code` checks
- **Smoke test expansion** — JSON contract tests for runtime errors, preflight errors, allocation stats

---

## v0.13.0 — 2026-03-20

### Core IR

- **Fix node** — dedicated `core.Fix` replaces `LetRec` desugaring for recursive bindings. Self-referential closure creation is now a single node (`evalFix`), eliminating the two-pass `IndirectVal` patching overhead. Polymorphic recursion is naturally supported via TyLam peeling
- **List literal patterns** — `[x, y, z]` surface syntax desugared to `Cons`/`Nil` patterns during parsing. Pattern matching, exhaustiveness checking, and explain trace all support the new form

### Evaluator

- **Multi-module source attribution** — `Closure`, `ThunkVal`, and `bounceVal` now capture their originating `*span.Source`. The evaluator tracks source context through the trampoline (save/restore in `Eval`, propagation via `bounceVal.source`), ensuring `RuntimeError` and `ExplainStep` carry the correct source for line/column resolution across module boundaries
- **Structural nesting depth guard** — `budget.Budget` enforces a nesting limit (default 256) on structurally recursive value construction, preventing Go stack overflow from deeply nested Core IR trees

### Engine

- **Caps/Bindings defensive copy** — `RunWith` shallow-copies `Caps` and `Bindings` maps on entry, fulfilling the goroutine-safety contract without relying on caller discipline
- **Spanless diagnostic fix** — errors without source location (e.g. context cancellation) report `Line=0, Col=0` instead of the misleading `1:1`. The human-readable formatter omits the location line entirely
- **Sandbox panic stack trace** — `InternalPanicError` captures the goroutine stack via `runtime.Stack`, preserving diagnostic information while maintaining the same `Error()` message

### Module System

- **Owned-only exports** — `ExportModule` restricts `Types`, constructors, aliases, classes, and promoted kinds/cons to declarations defined by the module itself. Inherited names from imported modules are no longer transitively re-exported, eliminating ghost dependencies. `TypeFamilies` and `Instances` remain fully exported (they accumulate instances across modules)

### CLI

- **JSON output improvements** — `List` values serialized as JSON arrays; `Record` and `Tuple` as objects/arrays. `--json` output is now structurally faithful to GICEL values
- **Explain trace improvements** — `PLit` and list patterns rendered in source-level syntax. `ExplainStep` includes `SourceName` field for multi-module traces

### Lexer

- **Operator boundary guards** — `->`, `<-`, and `:=` reserved symbols inside operators now produce a diagnostic instead of silently splitting tokens. Fixes `=:=`, `->>`, `<->` handling

### Documentation

- **README timeout correction** — sandbox timeout description updated to reflect the actual behavior (timeout covers the entire pipeline including compilation)

### Testing

- **CLI smoke test suite** — 57-case `scripts/smoke-test.sh` covering normal operation, error handling, resource limits, adversarial inputs, list patterns, and malformed inputs
- **Malformed input stress tests** — `tests/stress/stress_malformed_test.go` with 294+ lines of adversarial parser inputs

---

## v0.12.1 — 2026-03-20

### Core

- **`Suspended r a` type alias** — `Suspended r a := Thunk r r a` added to Core module, mirroring `Effect r a := Computation r r a` for state-preserving suspended computations

### Examples

- **`thunk (do {` → `thunk do {`** — all examples now use the parenthesis-free `thunk do { ... }` form instead of `thunk (do { ... })`. Applies to `do-notation.gicel`, `fail-effect.gicel`, `state-effect.gicel`, `state-machine.gicel`, `thunk-force.gicel`
- **Inline single-use computation in `full-grammar.gicel`** — the `computation` binding (thunk + force) replaced with a direct `main := do { ... }`

### Documentation

- **Computation top-level restriction** — spec §2.1.1 and §2.1.3 now explain that bare `Computation` cannot appear at the top level (E0291), when `thunk` is needed, and that value-typed monads are unaffected
- **Agent guide fix** — incorrect bare `do`-block example in effects.md replaced with `thunk do` pattern

---

## v0.12.2 — 2026-03-20

### CLI

- **`docs` topic listing** — `gicel docs` now shows a categorized topic listing (mirroring `gicel example`) instead of dumping the full README. Full overview available via `gicel docs about`

---

## v0.12.0 — 2026-03-19

### Type System

- **OutsideIn(X) L3** — deferred constraint batch replaced with worklist + inert set architecture. Kicked-out constraints get priority re-processing via `OnSolve` callback. Eliminates quadratic re-scanning of the constraint queue
- **CBPV discipline (E0291)** — non-entry top-level bindings with bare `Computation` type are rejected. Entry point (default `main`) is exempt. Enforces the CBPV invariant that top-level bindings are values; computations must be wrapped with `thunk`
- **Quantified constraint resolution fix** — context-evidence search now performs full structural matching (arity + head-arg unification + context compatibility), matching the same precision as global instance search

### Performance

- **Undo-log trail** — Unifier snapshot/restore replaced map-copy with an append-only trail. `Snapshot()` returns a trail position; `Restore()` replays undo entries in reverse. Eliminates O(n) map cloning per trial unification
- **Deque worklist** — two-buffer deque (front stack + back queue with read cursor) replaces slice-based FIFO. Kicked-out constraints go to front for priority processing. All operations amortized O(1)
- **Ambiguity cache** — per-`solveWanteds` cache prevents redundant `isAmbiguousInstance` checks on the same constraint key
- **Precomputed SortBindings** — module bindings are topologically sorted once at `RegisterModule` time and stored in `compiledModule.sortedBindings`, eliminating per-execution re-sorting
- **Precomputed import maps** — import scope insertion consolidated into shared helpers, reducing repeated map construction

### Parser

- **Class head assertion guard** — unchecked `*TyExprVar` type assertions in `parseClassDecl` replaced with defensive checks. Malformed class heads like `class Foo (Maybe a)` now produce a parser error instead of panicking

### Refactoring

Checker restructuring: establish subpackage boundaries, then consolidate constraint solver architecture.

- **`internal/budget` extraction** — unified resource limiter (`Budget`) tracks steps, depth, and allocation bytes across all pipeline phases. Replaces the previous `eval.Limit` type
- **`check/exhaust` subpackage** — Maranget exhaustiveness checking extracted with `DataTypeInfo`/`ConInfo` types. Callback-based `Env` struct decouples from Checker state
- **`check/family` subpackage** — type family reduction engine and injectivity verification extracted. `ReduceEnv` uses callback injection for solver integration
- **`check/env` subpackage** — shared environment type definitions (`AliasInfo`, `ClassInfo`, `InstanceInfo`, `ConstraintInfo`) extracted as canonical home
- **`internal/engine` extraction** — Engine/Runtime/RunSandbox moved from root package. Root `gicel` package becomes a pure facade of type aliases and re-exports; external API unchanged
- **Legacy StuckIndex removal** — `StuckIndex`, `ProcessRework`, and `maxReworkIterations` removed. Inert set with `CtFunEq` constraints is the single mechanism for stuck type family re-activation
- **Injective type key serialization** — `typeNameForMangling` (lossy, head-only) replaced with `WriteTypeKey` (structural, collision-free) in form family mangling
- **`DefaultEntryPoint` constant** — scattered `"main"` literals consolidated into `engine.DefaultEntryPoint` (re-exported as `gicel.DefaultEntryPoint`)
- **Tuple label unification** — all tuple label sites consolidated to `types.TupleLabel`
- **Type key totality** — `WriteTypeKey` panics on unhandled variant instead of falling back to `Pretty`
- **Budget clamping** — negative allocation limits clamped to zero
- **Module boundary hardening** — `SortBindings` precomputation, strict module export filtering
- **File reorganization** — test files renamed to feature convention across `check/`, `engine/`, `eval/`, `parse/`

### Fixes

- **Import ambiguity provenance** — re-export check now verifies dependency chain instead of assuming any re-export suppresses ambiguity. `import B` (re-exporting A.x) + `import C` (native x) is correctly flagged
- **RunWith entry point default** — `RunWith(ctx, nil)` now uses the compile-time entry point (`Runtime.entryName`) instead of hardcoded "main"
- **Compile bare Computation check** — `Compile()` now enforces E0291 consistently with `NewRuntime()`, preventing "check passes but run fails" asymmetry
- **Class head parse guard** — unchecked type assertions in `parseClassDecl` now emit errors instead of panicking on malformed class heads
- **Quantified constraint context search** — full structural matching (arity + head-arg unification) instead of class-name-only match
- **Literal parse error sentinel** — invalid integer/double literals now produce `TyError` sentinel instead of zero-valued Core nodes
- **Unifier probe safety** — `bidir_case.go` tail comparison uses `tryUnify` (trial scope) instead of committed unification

### Safety & Sustainability

- **Panic defaults on sealed switches** — `core.Walk`, `core.Transform`, `core.annotateFV` panic on unhandled Core variants. `check_pattern`, `resolve_type`, `resolve_kind` emit errors for unknown AST forms. Prevents silent degradation when new variants are added
- **File splits** — `types/evidence.go` (569→327+248), `stdlib/list.go` (605→272+341)
- **Dead code removal** — `ErrUnterminatedStr`, `collectContextEvidence`/`classifyEvidence`, dead exports unexported
- **Named constants** — `TyConComputation`/`TyConThunk`, `DefaultEntryPoint`, `sandboxDefaultTimeout`, `prefixSec`/`prefixField`

### Documentation

- **Trust boundary clarification** — README and agent guide now explicitly document that host-registered primitives (`RegisterPrim`) are trusted computing base code, and that `Timeout` bounds evaluation time only
- **CLAUDE.md** — unified to English; stdlib pack name → module name mapping table; package-name-as-feature test naming rule
- **Agent guide restructured** — hierarchical directory structure with dot-separated topic names (`features.records`, `stdlib.prelude`); 6 new feature docs added
- **Examples restructured** — `basics/`, `types/`, `effects/` subdirectories; directory-based CLI grouping
- **Roadmap** — documented fundep improvement as intentional bound; design conventions for tuple labels and compiler-generated names
- **Integer overflow** — specified as Go `int64` wrapping semantics

---

## v0.11.0 — 2026-03-18

### Language Features

- **Multiplicity enforcement** — `@Linear` and `@Affine` capabilities are now enforced at bind sites. Same-type preservations (re-use without state change) are counted; exceeding the limit produces error `E0290`. Type-changing preservations (protocol transitions) are unrestricted
- **LUB branch joins** — case branches with different capability multiplicities are joined via the `LUB` type family, resolving `Linear ⊔ Affine → Affine` instead of failing with a unification error
- **Session type library** — protocol states as type constructors (`Send`/`Recv`/`End`), `Dual` type family, `send`/`recv`/`close` operations with `@Linear` capability tracking

### Type System

- **Session fidelity theorem** — formal proof in `docs/spec/language.md` Chapter 18: protocol compliance, communication safety, session completion. 12 probe tests in `tests/probe/`
- **TyMeta levels** — metavariables carry implication nesting depth (`Level` field), preparing for OutsideIn(X) touchability

### Refactoring

Structural refactoring for next-version extensibility:

- **`TyCBPV` unification** — `TyComp` and `TyThunk` (structurally identical) merged into a single `TyCBPV` type with `CBPVTag`. Halves switch-case count across 23 files (-93 lines net)
- **Generic type traversals** — `types.MapType`, `types.AnyType`, `types.CollectTypes` replace 3 hand-written switch traversals. New type nodes only need updating in `MapType` and `Children()`
- **`check/unify` subpackage** — Unifier extracted into `internal/check/unify/` with compiler-enforced dependency direction. `OnSolve` callback replaces direct type family coupling
- **Stuck family decoupling** — `stuckFamilyEntry` and re-activation index moved from Unifier to `type_family.go` as `stuckFamilyIndex`. Unifier is now a pure unification engine
- **Row helpers to `types`** — `ClassifyRowFields`, `DecomposeConstraintType`, and 6 other pure helpers moved from `check` to `types/evidence.go`
- **Type family memoization** — `reduceFamilyApps` uses structural cache keys, replacing the node-budget heuristic

---

## v0.10.1 — 2026-03-18

### Language Features

- **Qualified constructor patterns** — `case x { Q.Con a -> ... }` with adjacency-based dot disambiguation, mirroring expression-level qualified names

### Type Inference

- **Type family re-activation** — stuck type family applications (blocked on unsolved metas) are now re-reduced when their blocking metas are solved, addressing the type family / row unification scheduling problem. ~190 lines, additive (OutsideIn(X) L1+L2)

### Fixes

- **`NormalizeRow` panic → error return** — duplicate labels during type checking no longer crash the host process; `RunSandbox` now wraps the entire compile+execute path in a top-level recover
- **`flatten()` form race** — `Env.Flatten()` pre-materializes the builtin environment at Runtime construction, eliminating a benign form race when sharing a Runtime across goroutines
- **`ResetFreshCounter`** — test determinism for type variable naming

### Refactoring

- **Prelude source consolidation** — `num.gicel`, `list.gicel`, `str.gicel` merged into `prelude.gicel`; Go-side string concatenation and `stripImportPrelude` removed. Core remains a separate auto-injected module
- **API parameter reduction** — `buildMethodSelector` (7→6 via `dictLayout` struct), `evalBindingsCore` (6→5 via derived `userVisible`)
- **Dead code removal** — unused `showIntImpl` in str.go
- **Selective exports excluded** from spec — `_` prefix is sufficient for GICEL's embedding-focused design

### Tests

- Regression tests for qualified constraints (`P.Num a =>`, tuple constraints, user-defined classes)
- Unit tests for `eval/env.go` (Extend, Lookup, ExtendMany, Flatten, TrimTo, shadowing) and `eval/capenv.go` (Get, Set, Delete, Labels, COW semantics)

---

## v0.10.0 — 2026-03-18

### Language Features

- **Type families** — closed type families with pattern matching and reduction, associated types in class/instance declarations, recursive type families (fuel 100), form families with constructor mangling and exhaustiveness support
- **Functional dependencies** on multi-parameter type classes (`| a =: b`)
- **Divergent post-states** — case branches may consume different capabilities; post-states are joined by intersection
- **Data families** — associated form type instances with automatic constructor mangling
- **Multiplicity annotations** — `@Mult` syntax on row types (structural foundation; enforcement added in v0.11.0)
- **Constraint tuple syntax** — `(Eq a, Ord a) => T` in class/instance declarations and type annotations
- **Literal patterns** — Int, String, Rune, Double in case expressions
- **Explicit type application** — `f @Int` syntax
- **Higher-rank record fields** — records may contain polymorphic values
- **Double type** — IEEE 754 with scientific notation (`3.14`, `1.0e-3`)
- **Numeric underscore separators** — `1_000_000`
- **`<+` / `+>` operators** and `FromList` / `ToList` type classes

### Module System

- **Selective imports** — `import M (x, T(..), Class(method))`
- **Qualified imports** — `import M as N`; `N.value`, `N.Constructor`, `N.Type` in expressions and type annotations
- **Private names** — `_` prefix excludes bindings from module exports
- **Module-qualified Core IR** — `Var.Module` and `Con.Module` carry canonical module names, eliminating name shadowing between stdlib modules
- **GHC-style ambiguity detection** — open imports of modules with overlapping names produce clear errors with disambiguation guidance
- **CLI `--module` flag** — `gicel run --module Lib=lib.gicel main.gicel` for multi-file projects
- **CLI `-e` flag** — `gicel run -e 'import Prelude; main := 1 + 2'` for inline evaluation

### Optimizer

- **Core IR optimizer** — Phase 1 algebraic simplifications (beta reduction, case-of-known-constructor, bind-pure elimination) and Phase 4 registered fusion rules

### Runtime

- **Allocation tracking** — `ChargeAlloc` infrastructure; all stdlib Go-level allocations (closures, constructors, records, AVL nodes, list cells) count against `MaxAlloc`
- **Structured explain detail** — explain events carry structured `ExplainDetail` with typed fields (bindings, scrutinee, pattern, capability diff) instead of format strings
- **`--explain-all`** — trace stdlib/module internals alongside user code

### API

- **`Div` class** — integer and double division (`div`, `mod`, `divDouble`)
- **`Computation` type alias** — `Effect` as shorthand for `Computation r r a`
- **`RunSandbox`** — single-call sandbox API with conservative defaults

### Fixes

- Fix `failImpl` silently dropping error message — `failWith` now includes the argument in the error
- Fix import-order re-registration causing unimported module names to shadow
- Fix compiler-generated `$` names leaking into module exports
- Fix class method shadowing from conflicting stdlib renames
- 7 bugs from adversarial testing (118 probe tests): unknown type name acceptance, builtin type leak through qualified imports, malformed import error recovery
- Security hardening from penetration probe findings

### Refactoring

Systematic three-round structural reorganization (net -3,200 lines):

- **8 new cohesive files** — `import.go`, `bidir_cbpv.go`, `bidir_case.go`, `exhaust_matrix.go`, `decl_generalize.go`, `eval_apply.go`, `eval_letrec.go`, `avl.go`
- **Dead code removal** — unused `group` field, `evidenceSource` type, 3 temporary probe test files
- **Content coupling fix** — `Unifier.InstallTempSolution`/`RemoveTempSolution` API replaces direct `soln` map access
- **O(1) form type lookup** — `dataTypeByName` reverse index for exhaustiveness checking
- **Naming cleanup** — `hasMeta`/`containsMeta` → `sliceHasMeta`/`typeHasMeta`; raw `"\x00"` → `core.QualifiedKey`
