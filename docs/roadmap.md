# GICEL Roadmap

Current state: **v0.13.** Foundation hardening — multi-module error attribution, module export ownership, Fix node, list patterns, nesting guard, sandbox diagnostics. See `CHANGELOG.md` for details, `docs/spec/language.md` for the complete specification.

---

## OutsideIn(X) Extension Path

The checker architecture now supports incremental migration toward OutsideIn(X). Current state is **L3** (worklist + inert set). See `memory/domain/outsidein_x.md` for the full design document.

| Level  | Status         | Description                                     |
| ------ | -------------- | ----------------------------------------------- |
| **L0** | done           | Ad-hoc family reduction in `normalize()`        |
| **L1** | done (v0.10.1) | `stuckFamilyIndex` + meta-indexed re-activation |
| **L2** | done (v0.11)   | `ProcessRework` loop + `OnSolve` callback       |
| **L3** | done (v0.12)   | Worklist + inert set, constraint AST            |
| **L4** | open           | Touchability, implication constraints           |

**What L4 would add**: touchability (meta level enforcement), implication constraints (local assumptions), GADT given simplification of stuck families. Also required for row-level type family stuck constraint management (see v0.14).

---

## Release Plan

### v0.13 — Foundation Hardening

既存基盤を安定化し、後続の型システム拡張（v0.14 Row-Level TF、v0.15 SMC）の前提条件を整える。

#### Solver & Performance Optimization

L3 worklist + inert set solver は構造的に正しいが、ナイーブなコンテナ実装がボトルネック候補。隣接するデータ構造の性能問題も含めて対処する。

| Item                                    | Issue                                                                | Fix                                           | Priority |
| --------------------------------------- | -------------------------------------------------------------------- | --------------------------------------------- | -------- |
| `SortBindings` per execution            | `evalBindingsCore` が毎回 `core.SortBindings` を呼ぶ; Runtime は不変 | `NewRuntime` で事前計算、ソート済み結果を保持 | High     |
| `Env.Lookup` no flattening              | 深い環境で O(depth) 探索; 設計コメントは flatten を約束              | 深度閾値で flatten をトリガー                 | High     |
| `UnwindApp` O(n²)                       | prepend ベースの引数収集が毎回反転                                   | reverse 収集→一度だけ反転                     | High     |
| `PushFront` full copy                   | `append(cts, w.items...)` が kickout 毎にワークリスト全体をコピー    | Head-index deque or ring buffer               | Medium   |
| `removeClass`/`removeFunEq` linear scan | 各 KickOut で線形探索 + スライススプライス                           | Pointer identity set or swap-remove           | Medium   |
| `constraintKey` allocation              | ホットパスで制約毎に `strings.Builder`                               | CtClass 上の lazy key、一度だけ計算           | Low      |
| `isAmbiguousInstance` repeated trial    | 同一 (class, args) のメモ化なし                                      | Per-solve-pass cache                          | Low      |

**動機**: v0.14 で Merge 型族が stuck `CtFunEq` を大量に生む。solver の性能がボトルネックになる前に最適化する。

#### Module Boundary Hardening

| Item                          | Issue                                                                                 | Priority |
| ----------------------------- | ------------------------------------------------------------------------------------- | -------- |
| Type-level import collision   | `importOpen` が Types/Classes/Aliases/Families を `checkAmbiguousName` なしで書き込む | High     |
| Private export leak           | `ExportModule` が `_` prefix を Values のみフィルタ; types/classes/aliases がリーク   | High     |
| ~~Re-export model inconsistency~~ | ~~Values = local-only; types = accumulated (imports を含む)~~ — **done (v0.13)**: ExportModule now exports only owned declarations | ~~Medium~~ |

#### Parser & Diagnostics Hardening

パーサ基盤レビュー (2026-03-19) で特定された改善項目。v0.14 以降の言語拡張（row-level TF の型エラー、`><` の型不整合診断）に先立ち、フロントエンド品質を底上げする。

**v0.13 scope:**

| Item                        | Issue                                               | Fix                                                                           | Priority |
| --------------------------- | --------------------------------------------------- | ----------------------------------------------------------------------------- | -------- |
| Syntax error specialization | E0100 一つに全構文エラーが集約                      | E0101–E0110 に分化 (unclosed delimiter, unexpected token, missing body, etc.) | High     |
| Expression-level recovery   | `case { + -> * }` で 20+ エラー cascade             | `ErrorExpr` ノード返却による phrase-level recovery                            | High     |
| Newline separator bugs      | `do`, class, instance body で改行区切りがエラー     | 共通 separator helper に統合                                                  | Medium   |
| ~~`->` / `<-` lexer guard~~     | ~~`->>`, `<->` が予約記号を含むのにトークン分割される~~ — **done (v0.13)** | ~~Low~~ |
| `main :=` silent acceptance | body なしバインディングがエラーなし                 | 明示エラー「バインディング本体が必要」                                        | Low      |

**Post-v0.13 diagnostics roadmap** (独立して段階的に実施可能):

| Phase | 内容                                                      | 工数 |
| ----- | --------------------------------------------------------- | ---- |
| B     | "Did you mean?" suggestion (Damerau-Levenshtein distance) | 中   |
| C     | Secondary span 充実 — 型エラーの期待型出所を hint 付加    | 中   |
| D     | 構造化 JSON 診断出力 (LSP `Diagnostic` 互換)              | 中   |
| E     | 内部用語のユーザー語彙翻訳 (skolem, occurs check 等)      | 低   |

#### Type-Level Syntax

二つの独立した拡張で型式の可読性を向上する。どちらもパーサ変更のみ — Core IR・型チェッカーへの影響なし。

**Type Operators** — infix type aliases:

```gicel
type (:>) a b := a b
-- Send :> Recv :> End = Send (Recv (End))
```

Session type DSL と後続の SMC 型レベル行操作の可読性向上が動機。

**Type Application Operator (`-<`)** — 組み込み右結合型適用:

```gicel
Map String -< List -< Maybe -< Int
= Map String (List (Maybe Int))

-- juxtaposition > -< > ->
String -> Map String -< List -< Int
= String -> (Map String (List Int))
```

`->` と視覚的対を成す（Haskell arrow notation の `-<` = arrow application が先行例）。

#### `do` Elaboration Consolidation

`inferDo` / `elaborateStmtsChecked` / `elaborateDoMonadic` / `elaborateDoMult` の 4 実装が文処理を重複している。共通 elaboration コアに統合し、意味修正の 4 箇所同時修正を解消する。

v0.14 で L4 implication constraints が checker を大幅に変更する前に実施。SMC 拡張（v0.15 `><` / `dag`）で elaboration に新しい文形式が加わる場合も、統合済みなら 1 箇所の変更で済む。

---

### v0.14 — Row-Level Type Families + OutsideIn(X) L4

**二つの拡張を同時に進める。** Row-level type families は L4 の touchability/given simplification と共依存関係にある。

#### Type Family Reduction Hardening

`reduceFamilyAppsN` が指数的に分岐する既知のバグ（`Grow a = Pair (Grow a) (Grow a)` 等）を修正する。`MaxReductionWork` がステップ数のみ制限し分岐数を制限しない。`Merge` 型族が open row tail で再帰的に展開される場面でこのパターンが直撃するため、Row-Level TF の前提条件。

**修正方針**: 共有ベースの簡約（同一 TyApp を一度だけ簡約しメモ化）、または分岐数の明示的制限。

#### OutsideIn(X) L4

Touchability (meta level enforcement), implication constraints (local assumptions), GADT given simplification of stuck families.

**SMC との接続**: Merge 型族が open row tail を含むと stuck `CtFunEq` が発生する。L4 の touchability は「この meta はどのスコープで解決可能か」を追跡し、不必要な re-activation を防ぐ。GADT パターンマッチで供給される given が stuck Merge を simplify する場面も L4 が必要。

#### Row-Level Type Families (SMC Phase 1)

型族パターンマッチに行構造を追加し、行の合併・分解を型レベルで公開する。

```
type Merge (r1: Row) (r2: Row) :: Row    -- 二つの非交和行を結合
type Without (l: Type) (r: Row) :: Row   -- ラベル除去
type Lookup (l: Type) (r: Row) :: Type   -- ラベル検索
```

`Merge` の簡約は既存の `classifyFields` (shared/onlyA/onlyB 分類) を型族として露出したもの。重複ラベルは型エラー。open row tail の場合は stuck (`CtFunEq` として worklist に入り、L4 の re-activation で解消)。

**実装箇所**: `internal/check/family/reduce.go` の `MatchTyPattern()` に `TyEvidenceRow` パターンを追加。既存の行単一化アルゴリズムをそのまま利用。

**解消される boundary**: "Row operations not exposed at type level" — `bind` のみ通り `><` が通らなかった根本原因が解消される。

---

### v0.15 — Parallel Composition + Dagger (SMC Phase 2-3)

v0.14 の型レベル行操作の上に、Free SMC の残り二つの合成操作を構築する。

#### Parallel Composition (SMC Phase 2)

```
infixr 3 ><
(><) :: Computation pre₁ post₁ a -> Computation pre₂ post₂ b
     -> Computation (Merge pre₁ pre₂) (Merge post₁ post₂) (a, b)
```

ホスト提供プリミティブ。実行時動作: 能力環境を分割し、両計算を独立実行し、結果環境を合成。型検査は `Merge` 型族で行の結合を検証。

**実装箇所**: `PrimOp` 登録 + `Merge` 型族の組み込み簡約。型検査器の変更は v0.14 に含まれる。

#### Dagger (SMC Phase 3)

```
type Gate pre post := Computation pre post ()
dag :: Gate pre post -> Gate post pre
```

pre/post を交換する。対合律 `dag (dag f) = f` は構造的に成立 (二重交換)。反変則 `dag (f ; g) = dag g ; dag f` はホスト実装が保証。

**実装箇所**: `PrimOp` 登録。型レベルでは pre/post の交換のみ — 型検査器への変更は不要 (関数の型が既に正しい)。

#### Theoretical status after v0.15

| 概念     | v0.12 (現在)                        | v0.15 (到達点)                             |
| -------- | ----------------------------------- | ------------------------------------------ |
| 基底構造 | Atkey indexed monad (Prof のモナド) | Free †-SMC                                 |
| 逐次合成 | `bind` (do ブロック)                | `;` — 不変                                 |
| 並列合成 | なし                                | `><` (Merge 型族)                          |
| 反転     | なし                                | `dag` (pre/post 交換)                      |
| ワイヤ束 | 行型 (Row)                          | 行型 — 不変                                |
| 射型     | `Computation pre post a`            | 同左 (= `pre ⊸ {_r: Cl a \| post}` の糖衣) |

**構文の変更はゼロ。** `do` ブロック = 逐次、ユーザ定義演算子 `><` = 並列、関数 `dag` = 反転。パーサ変更不要。意味論の拡張のみ。

---

### v0.16 — Multiplicity Generalization (SMC Phase 4 + Evidence Phase 5)

既存の Evidence Phase 5 (multiplicity polymorphism) と SMC Phase 4 (semiring generalization) を統合する。**実質的に同一の作業。**

`@Linear`/`@Affine`/`@Unrestricted` のハードコード (`elaborate_do_mult.go` の `multLimit`) を型クラスベースの半環に一般化:

```
class UsageSemiring (s: Type) {
  zero :: s; one :: s; plus :: s -> s -> s; mult :: s -> s -> s
}
```

既存の `{0, 1, ω}` 半環はデフォルトインスタンスとして保存。量子リソース追跡 (確率半環) や量的型理論 (QTT) 接続が可能になる。

**解消される boundaries**:

- "Double grading" — 半環の形式化により State × Usage の積圏構造が明示化。Triple grading (State × Usage × Probability) への拡張経路が開く。
- "Evidence fiber crossing" — `@Mult` が型レベルパラメータになることで fiber 間の相互作用が形式的に扱われる。

---

## Design Fork Points

| Fork Point                                  | Current State                                   | Decision Trigger                                          |
| ------------------------------------------- | ----------------------------------------------- | --------------------------------------------------------- |
| `Row` as built-in kind vs structured-index  | Built-in kind; DataKinds reduces pressure       | Need for non-capability indexing                          |
| Algebraic effects/handlers vs indexed monad | Indexed monad (Atkey); type families compensate | Evidence that handlers better serve the AI agent use case |
| Tensor product kind (`QType`)               | Not present; rows cover all current use cases   | Quantum entanglement or other non-separable state         |

### Tensor product kind

v0.15 は行の合併 (可分結合) を提供するが、量子もつれ (不可分結合) には真のテンソル積 `A ⊗ B` が必要。これは `QType` カインドの導入を意味し、行型との共存設計が分岐点になる。v0.14-15 は行型のみで完結し、テンソル積カインドの導入判断を遅延できる。

行型ラベルはアドレス可能 (射影可能) だが、テンソル積は不可分 (射影不可能)。古典的能力管理には行型、量子もつれにはテンソル積、という使い分けが自然。両者はカインドレベルで分離される。

理論的背景: `memory/domain/` の 6 文書 (categorical_quantum_mechanics, quantum_pl_type_systems, tensor_products_type_theory, qtt_quantum_resources, polynomial_functors, optics_quantum) を参照。

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

These are not bugs or missing features. They are consequences of GICEL's design coordinate (Atkey indexed monad × row polymorphism × CBPV × Go embedding) that existing literature does not address. Each is currently handled by a practical workaround; the notes below record when the workaround would break and which release addresses it.

### Double grading

`Computation pre post a` is indexed by state transition (pre → post). Adding `@Mult` grading creates a second axis: how many times a capability can be used. The two axes interact inside row unification — the pre/post diff must account for both label presence and usage count.

**Current state**: multiplicity enforcement counts same-type preservations at bind sites. Row unification uses LUB for heterogeneous joins.
**Triggers**: multiplicity _polymorphism_ (quantifying over `@Mult`). At that point, row unification must solve state-transition and usage constraints simultaneously — a problem not covered by existing graded monad literature (Orchard, Petricek et al.), which treats grading on a single axis.
**Addressed by**: v0.16 (semiring generalization formalizes the product category State × Usage).

### Type family / row unification scheduling

Type families can return `Row` values used in `Computation pre post a` indices. This creates a dependency: row unification needs the reduced result, but reduction may need unification to resolve meta-variables first.

**Current state**: L2 re-activation index handles this — stuck families are re-reduced when blocking metas are solved, with cascading support via `ProcessRework`.
**Triggers**: programs requiring L4+ (GADT givens simplifying stuck families, touchability for Merge on open rows). Merge type family (v0.14) will generate stuck `CtFunEq` constraints requiring L4 infrastructure.
**Addressed by**: v0.14 (L4 touchability + row-level type families, co-developed).

### Row operations not exposed at type level

Row merging, splitting, and label lookup are internal to the unifier (`unifyEvCapRows`) but not available as type-level expressions. This blocks parallel composition (`><`) — its type requires `Merge r1 r2`, which is a type-level _construction_, not unification. Sequential composition (`bind`) succeeds because it only requires _unification_ of shared indices (post₁ = pre₂).

**Current state**: row operations are unifier-internal. Type families cannot pattern-match on row structure.
**Triggers**: parallel composition, dagger, any combinator whose type requires row-level computation (not just row-level unification).
**Addressed by**: v0.14 (row-level type families expose Merge/Without/Lookup).

### Evidence fiber crossing

The evidence system separates fibers (`Type`, `Constraint`, `Row`). Type families can cross fibers (`Row → Row`, `Type → Constraint`). When a family result feeds into a different fiber's unification, the "fibers are independent" assumption breaks.

**Current state**: the single-pass reduce → unify pipeline handles current cases because family results are fully reduced before entering unification.
**Triggers**: a family whose result is another family application in a different fiber (e.g., a `Row → Constraint` family whose result enters evidence resolution). Would require interleaved reduction across fibers.
**Addressed by**: v0.16 (multiplicity generalization requires @Mult to cross the Type/Row fiber boundary).

---

## Far Future (assessed, not planned)

| Extension                                                          | Classification   | Prerequisite             |
| ------------------------------------------------------------------ | ---------------- | ------------------------ |
| Tensor product kind (`QType`)                                      | Type system      | v0.15 + quantum use case |
| Optimizer Phase 2–3 (selective inline + case-of-case)              | Optimization     | Benchmark-driven demand  |
| Tuple `Eq`/`Ord` runtime support                                   | Stdlib           | User demand              |
| Diagnostics Phase B–E (suggestion, secondary span, JSON, 用語翻訳) | DX               | Incremental, post-v0.13  |
| Refinement types                                                   | Phase transition | Separate analysis        |
| Dependent types                                                    | Full restructure | Far future               |
