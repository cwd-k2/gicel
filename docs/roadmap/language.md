# Language Feature Roadmap

GICEL の言語機能拡張の方向性。バージョン番号は付けない — 各項目は依存関係で順序付ける。

**性格**: library/tooling roadmap が需要駆動・展開観察型であるのに対し、言語機能は**追求型**。型理論・圏論的構造の内在的な帰結を追い、設計空間の論理的な到達点を目指す。外部需要やユーザ friction ではなく、型システムの整合性と表現力の完成が駆動力。したがって direction.md の判断軸（§1 主目的直結、§2 収束速度）とは評価経路が異なり、ここでの優先順序は理論的依存関係で決まる。

## Row-Level Type Families (SMC Phase 1)

Row merging, splitting, lookup を型レベル操作として公開。

```gicel
type Merge (r1: Row) (r2: Row) :: Row    -- merge two disjoint rows
type Without (l: Type) (r: Row) :: Row   -- remove a label
type Lookup (l: Type) (r: Row) :: Type   -- look up a label
```

`Merge` reduction は既存の `classifyFields` アルゴリズムを型族として公開。重複ラベルは型エラー。Open row tail は stuck constraint を生成し、touchability の re-activation で解決。

`><` が使えず `bind` だけが動く根本原因を解消する。

### Type Family Reduction: Exponential Branching Fix

`reduceFamilyAppsN` が `Grow a = Pair (Grow a) (Grow a)` のようなパターンで指数的に分岐する。`Merge` が再帰的に open row tail を展開する際の前提修正。

**アプローチ**: shared-basis reduction (同一 TyApp を一度だけ reduce して memoize) または明示的分岐制限。

## Type-Level Syntax Extensions

パーサのみの変更。Core IR・型検査への影響なし。

### Type Operators

infix type alias:

```gicel
type (:>) a b := a b
-- Send :> Recv :> End = Send (Recv (End))
```

Session type DSL の可読性と SMC 型レベル row 操作のために。

### Type Application Operator (`-<`)

組み込み右結合型適用:

```gicel
Map String -< List -< Maybe -< Int
= Map String (List (Maybe Int))
```

`->` と視覚的に対をなす（Haskell arrow notation の `-<` = arrow application に先例あり）。

## Parallel Composition (SMC Phase 2)

```gicel
infixr 3 ><
(><) :: Computation pre₁ post₁ a -> Computation pre₂ post₂ b
     -> Computation (Merge pre₁ pre₂) (Merge post₁ post₂) (a, b)
```

Host-provided primitive。Runtime: capability 環境を分割し、両 computation を独立実行、結果環境をマージ。型検査は `Merge` 型族で row 合成を検証。

**前提**: Row-Level Type Families（Merge 型族）

## Dagger (SMC Phase 3)

```gicel
type Gate pre post := Computation pre post ()
dag :: Gate pre post -> Gate post pre
```

pre/post スワップ。`dag (dag f) = f` は構造的に保証。`dag (f ; g) = dag g ; dag f` は host 実装が保証。

**前提**: なし（型レベル pre/post swap のみ）

## Multiplicity Generalization (SMC Phase 4)

ハードコードの `@Linear`/`@Affine`/`@Unrestricted` を型クラスベースの半環に一般化:

```gicel
data UsageSemiring (s: Type) {
  zero: s; one: s; plus: s -> s -> s; mult: s -> s -> s
}
```

既存の `{0, 1, ω}` 半環はデフォルトインスタンスとして保存。量子リソース追跡 (probability semiring) や QTT 接続を可能にする。

**解決する問題**:

- "Double grading" — 半環形式化で State × Usage の積圏を明示
- "Evidence fiber crossing" — `@Mult` が型レベルパラメータになり fiber 間相互作用を形式化

**前提**: Row-Level Type Families + Parallel Composition

## Theoretical Status After Full SMC

| Concept            | Current                             | Target                   |
| ------------------ | ----------------------------------- | ------------------------ |
| Foundation         | Atkey indexed monad (monad in Prof) | Free †-SMC               |
| Sequential compose | `bind` (do blocks)                  | `;` — unchanged          |
| Parallel compose   | none                                | `><` (Merge type family) |
| Inversion          | none                                | `dag` (pre/post swap)    |
| Wire bundles       | Row types                           | Row types — unchanged    |
| Morphism type      | `Computation pre post a`            | same                     |

**ゼロ構文変更。** `do` blocks = sequential, `><` = parallel, `dag` = inversion。パーサ変更なし。意味論拡張のみ。

## Design Fork Points

| Fork Point                                  | Current State                              | Decision Trigger                            |
| ------------------------------------------- | ------------------------------------------ | ------------------------------------------- |
| `Row` as built-in kind vs structured index  | Built-in kind (DataKinds reduces pressure) | Non-capability row-like indexing の需要     |
| Algebraic effects/handlers vs indexed monad | Indexed monad (type families compensate)   | Handlers が AI agent use case に優る場合    |
| Tensor product kind (`QType`)               | Not present (rows cover current needs)     | Quantum entanglement or non-separable state |

### Tensor Product Kind

Row merging (separable composition) は SMC で提供されるが、quantum entanglement (inseparable composition) には真のテンソル積 `A ⊗ B` が必要。Row label は addressable (projectable)、tensor product は inseparable (non-projectable)。Classical capability = rows、quantum entanglement = tensors — kind レベルで分離。SMC 完成まではテンソル積なしで完結する。

## Intentional Capability Bounds

### Non-entry top-level bindings must be values (CBPV discipline)

非 entry の top-level binding に bare `Computation` 型は不可 (E0291)。`thunk` で `Thunk` 型に変換する。entry point (default `main`) のみ免除。

### Compiler-generated names use `$` convention

辞書コンストラクタ等は `$` を含む。Lexer はユーザ識別子の `$` を拒否し衝突を防止。

### Tuples are records with `_N` labels

`(a, b, c)` は `Record { _1: a, _2: b, _3: c }` に desugar。

### Exhaustiveness witness reconstruction is best-effort

witness formatting は best-effort shape recovery。error reporting 専用、semantic 判断には不使用。

## Session Types Maturity

Session types は check-only で正しく動作する。Runtime 実行には host primitive (send/recv/close) が必要。

課題:

- session example は check-only。CLI から学んで試す導線が閉じていない
- structuring rule (bare Computation prohibition) が session 文脈で十分説明されていない
- runtime 対応は host primitive 設計を伴う

対応方針: check-only としての完成度を先に上げ、runtime 対応は SMC Phase 2 (parallel composition) と合わせて検討。

## Far Future (assessed, not planned)

| Extension                                             | Category         | Prerequisite                |
| ----------------------------------------------------- | ---------------- | --------------------------- |
| Tensor product kind (`QType`)                         | Type system      | Full SMC + quantum use case |
| Optimizer Phase 2–3 (selective inline + case-of-case) | Optimization     | Benchmark-driven demand     |
| Refinement types                                      | Phase transition | Separate analysis           |
| Dependent types                                       | Full restructure | Far future                  |
