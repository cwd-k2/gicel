# GICEL Roadmap

Current state: **v0.12.** Session fidelity, multiplicity enforcement, checker/parser architecture reorganization. See `CHANGELOG.md` for details, `docs/spec/language.md` for the complete specification.

---

## OutsideIn(X) Extension Path

The checker architecture now supports incremental migration toward OutsideIn(X). Current state is **L2** (re-activation index + rework queue). See `memory/domain/outsidein_x.md` for the full design document.

| Level  | Status         | Description                                      |
| ------ | -------------- | ------------------------------------------------ |
| **L0** | done           | Ad-hoc family reduction in `normalize()`         |
| **L1** | done (v0.10.1) | `stuckFamilyIndex` + meta-indexed re-activation  |
| **L2** | done (v0.11)   | `ProcessRework` loop + `OnSolve` callback        |
| **L3** | done (v0.12)   | Worklist + inert set, constraint AST             |
| **L4** | open           | Touchability, implication constraints             |

**What L4 would add**: touchability (meta level enforcement), implication constraints (local assumptions), GADT given simplification of stuck families.

---

## Planned Work

### Solver Data Structure Optimization

The L3 worklist + inert set solver is architecturally correct but uses naive container implementations. Identified hotspots (`docs/reviews/2026-03-19-performance-rereview.md`):

| Item | Issue | Fix | Priority |
|------|-------|-----|----------|
| `SortBindings` per execution | `evalBindingsCore` calls `core.SortBindings` every `RunWith`; Runtime is immutable | Precompute in `NewRuntime`, store sorted result | High |
| `PushFront` full copy | `append(cts, w.items...)` copies entire worklist on every kickout | Head-index deque or ring buffer | Medium |
| `removeClass`/`removeFunEq` linear scan | Linear search + slice splice on each KickOut | Pointer identity set or swap-remove | Medium |
| `constraintKey` allocation | `strings.Builder` per constraint in hot path | Lazy key on CtClass, compute-once | Low |
| `isAmbiguousInstance` repeated trial | No memoization for same (class, args) | Per-solve-pass cache | Low |

### Module Boundary Hardening

Identified in `docs/reviews/2026-03-19-full-codebase-review.md`:

| Item | Issue | Priority |
|------|-------|----------|
| Type-level import collision | `importOpen` writes Types/Classes/Aliases/Families without `checkAmbiguousName` | High |
| Private export leak | `ExportModule` filters `_` prefix for Values only; types/classes/aliases leak | High |
| Re-export model inconsistency | Values = local-only; types = accumulated (includes imports) | Medium |

### Evidence Unification (Phase 5, deferred)

Multiplicity polymorphism (quantifying over `@Mult` annotations) requires evidence fiber crossing during unification. Deferred until a concrete use case triggers it.

---

## Design Fork Points

| Fork Point                                  | Current State                                   | Decision Trigger                                          |
| ------------------------------------------- | ----------------------------------------------- | --------------------------------------------------------- |
| `Row` as built-in kind vs structured-index  | Built-in kind; DataKinds reduces pressure       | Need for non-capability indexing                          |
| Algebraic effects/handlers vs indexed monad | Indexed monad (Atkey); type families compensate | Evidence that handlers better serve the AI agent use case |

---

## Intentional Capability Bounds

### Non-entry top-level bindings must be values (CBPV discipline)

Non-entry top-level bindings with bare `Computation` type are rejected (E0291). `thunk` で包んで `Thunk` 型（値）にする必要がある。エントリーポイント（デフォルト `main`）のみ免除。

```gicel
helper := thunk (do { x <- get; pure x })  -- Thunk = 値
main := do { h <- force helper; pure h }    -- entry point は bare Computation OK
```

**Thunk + 数値リテラルの注意**: `thunk` で包んだ `Num` リテラルを含む computation は、let-generalization で状態型が多相になる。`force` 時に `Num` 辞書パラメータが `main` に伝搬し、`main` が関数として評価される。明示的な型注釈で回避:

```gicel
counter :: Thunk { state: Int } { state: Int } Int
counter := thunk (do { _ <- put 0; _ <- modify (+ 1); get })
```

**適用範囲**: `NewRuntime`（実行用コンパイル）のみ。`Compile`（check-only）と `RegisterModule`（モジュール）では無効。`CheckConfig.EntryPoint` / CLI `--entry` で制御。

### Fundep improvement is advisory

Functional dependency improvement (`| a =: b`) is best-effort: when the `from` position matches an instance, the checker attempts to unify the `to` position with the instance type. If unification fails (e.g., the type is already constrained), the improvement is silently skipped.

**Rationale**: a hard error would reject valid programs where the fundep simply provides no additional information — the type may already be determined by annotation or direct instance resolution.

**Regression test**: `TestRegressionFundepBestEffort` and `TestRegressionFundepImprovementFromMeta` in `internal/check/regression_test.go`.

**Implementation**: `resolve.go:277–283` — `_ = ch.unifier.Unify(args[toIdx], instArg)`.

### Compiler-generated names use `$` convention

Compiler-generated identifiers (dictionary constructors, instance bindings, internal binders) contain `$` in their names. The evaluator's explain mode uses `strings.Contains(name, "$")` to distinguish user-visible bindings from compiler internals.

**Rationale**: the lexer rejects `$` in user identifiers, so collision with source names is prevented at the grammar level. An explicit AST/Core flag for generatedness would be structurally cleaner but is unnecessary given the lexer guarantee.

**Invariant**: `$` must remain prohibited in user identifiers. If this changes, explain-mode filtering must switch to explicit metadata.

**Implementation**: `eval.go` `isCompilerGenerated()`.

### Tuples are encoded as records with `_1`, `_2`, ... labels

Tuples `(a, b, c)` are desugared to `Record { _1: a, _2: b, _3: c }` by the parser. Multiple subsystems detect tuple-shaped records by checking for sequential `_N` labels: parser (constraint tuple desugaring), evaluator (pretty-printing), stdlib (zip/unzip/splitAt).

**Canonical definition**: `types.TupleLabel(pos)`.

**Rationale**: first-class tuple types would add language complexity for marginal benefit. Record encoding is sufficient and composes with the existing row type system.

**Invariant**: all tuple construction and detection must use the `_N` convention. If the encoding changes, all consumers must be updated together.

### Exhaustiveness witness reconstruction is best-effort

The exhaustiveness checker's witness formatting (`exhaust/matrix.go`) uses best-effort shape recovery and sorts record fields for stable rendering. This is acceptable because witnesses are used only for error reporting, not for semantic decisions.

---

## Known Theoretical Boundaries

These are not bugs or missing features. They are consequences of GICEL's design coordinate (Atkey indexed monad × row polymorphism × CBPV × Go embedding) that existing literature does not address. Each is currently handled by a practical workaround; the notes below record when the workaround would break.

### Double grading

`Computation pre post a` is indexed by state transition (pre → post). Adding `@Mult` grading creates a second axis: how many times a capability can be used. The two axes interact inside row unification — the pre/post diff must account for both label presence and usage count.

**Current state**: multiplicity enforcement counts same-type preservations at bind sites. Row unification uses LUB for heterogeneous joins.
**Triggers**: multiplicity _polymorphism_ (quantifying over `@Mult`). At that point, row unification must solve state-transition and usage constraints simultaneously — a problem not covered by existing graded monad literature (Orchard, Petricek et al.), which treats grading on a single axis.

### Type family / row unification scheduling

Type families can return `Row` values used in `Computation pre post a` indices. This creates a dependency: row unification needs the reduced result, but reduction may need unification to resolve meta-variables first.

**Current state**: L2 re-activation index handles this — stuck families are re-reduced when blocking metas are solved, with cascading support via `ProcessRework`.
**Triggers**: programs requiring L3+ (GADT givens simplifying stuck families). No reports to date.

### Evidence fiber crossing

The evidence system separates fibers (`Type`, `Constraint`, `Row`). Type families can cross fibers (`Row → Row`, `Type → Constraint`). When a family result feeds into a different fiber's unification, the "fibers are independent" assumption breaks.

**Current state**: the single-pass reduce → unify pipeline handles current cases because family results are fully reduced before entering unification.
**Triggers**: a family whose result is another family application in a different fiber (e.g., a `Row → Constraint` family whose result enters evidence resolution). Would require interleaved reduction across fibers.

---

## Potential Extensions (assessed, not planned)

| Extension        | Classification   | Prerequisite        |
| ---------------- | ---------------- | ------------------- |
| Type operators   | Syntax           | Parser (~140 lines) |
| Refinement Types | Phase transition | Separate analysis   |
| Dependent Types  | Full restructure | Far future          |

### Type operators

Infix aliases for types: `type (:>) a b := a b` enables `Send :> Recv :> End` instead of `Send (Recv End)`. Haskell `TypeOperators` の最小サブセット（型別名のみ）。Parser 変更のみ、型システムへの影響なし。Session type DSL の可読性向上が主な動機。
