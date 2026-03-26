# Infrastructure Path

Grade algebra のハードコード排除と、残存する solver 課題を解消する計画。[smc.md](smc.md) Phase 4 (Multiplicity Generalization) の前提基盤を整備する。

**状態**: L3 完了。内部 `$GradeJoin`/`$GradeDrop` fallback は残存（Prelude なし環境向け）。

## 完了済み

| Layer        | 項目                           | 概要                                                 | Commit              |
| ------------ | ------------------------------ | ---------------------------------------------------- | ------------------- |
| L0-b         | Universe enforcement           | `KSort{Level int}`、`form :: Kind` 拒否              | `92e9428`           |
| L1-a         | SolverLevel protocol           | 3 timing pattern の不変条件コメント                  | `ecfa545`           |
| L1-b         | Inert set scope                | `scopeDepth`、scope-aware `Reset`                    | `ecfa545`           |
| L1-c Stage 1 | Equality constraints (parser)  | `TyExprEq`、`TokTilde` パース                        | `e32302e`           |
| L1-c Stage 2 | Equality constraints (checker) | `CtEq`、given eq via `InstallGivenEq`                | `1d002da`           |
| L2-b         | Grade CtFunEq error            | `OnFailure` callback、`ErrMultiplicity`              | `f2892e5`           |
| L2-a partial | TF on-demand (joinGrades)      | joinGrades の CtFunEq 化                             | `ba3b499`           |
| L3           | Grade algebra user-definable   | `GradeAlgebra` class、dynamic resolution、e2e テスト | `382a9ea`–`f31b88a` |

**後続拡張への設計注記** (L0-b):

- **Non-nullary constructor promotion**: `decl_form.go` の nullary ガードを除去するのみで対応。
- **Universe polymorphism**: `KSort{Level: int}` → `KSort{Level: LevelExpr}` に局所変更で対応。L1/L2 安定後。
- **Cumulativity**: equality-based kind unification とは別の subsumption パスで対応。Universe polymorphism 後。

## 依存グラフ（残存項目）

```
                    ┌───────────────────────┐
                    │  L0-a: Set.map bug    │  (独立)
                    │  L0-c: Optimizer      │  (独立)
                    │  L0-d: Tuple Eq/Ord   │  (独立)
                    └───────────────────────┘


    ┌──────────────────────┐
    │ L1-c Stage 3:        │
    │   Given eq 統合      │
    └──────────┬───────────┘
               │
    ┌──────────┴──────────┐
    │ L2-a: TF reduce     │
    │   on-demand         │
    └──────────┬──────────┘
               │
    ┌──────────┴──────────┐
    │ L3: Grade algebra   │
    │   user-definable    │
    │   = SMC Phase 4 基盤│
    └──────────┬──────────┘
               │
    ┌──────────┴──────────┐
    │ L3 後の応用層       │
    │  Named capabilities │
    │  Per-comp grade     │
    │  Row TF (Phase 1)   │
    └──────────┬──────────┘
               │
    ┌──────────┴──────────┐
    │ L4: Solver coupling │
    │   reduction (長期)  │
    └─────────────────────┘
```

## Layer 0: 独立タスク（並行可能）

### L0-a: Data.Set.map runtime bug

2 型変数の dictionary elaboration で de Bruijn index がずれる。

- **症状**: `Data.Set.map (* 2) s` が non-exhaustive pattern match で失敗
- **原因仮説**: 2 つの dict Lam の depth shift が inner lambda の FVIndices に反映されていない
- **変更先**: `index.go`, optimizer beta-reduction
- **検証**: `Data.Set.map` の probe test が pass

### L0-c: Optimization pipeline module/main asymmetry

Module は最適化なし、main は 4 pass。明示的ポリシーに変更。

### L0-d: Tuple Eq/Ord runtime support

Runtime の辞書解決で tuple をサポート。

## Layer 1: 残存

### L1-c Stage 3: Given eq 統合

GADT ブランチの given eq を `InstallGivenEq` 直接書き込みから solver 管理の CtEq given に移行する。L2-a (on-demand reduction) と連動: given eq が solver 管理下に入ることで、given eq → CtFunEq re-activation → 型族 reduction の連鎖が自然に動く。

| ファイル               | 変更内容                                             |
| ---------------------- | ---------------------------------------------------- |
| `check/bidir_case.go`  | GADT given eq を CtEq given として solver 経由に統合 |
| `solve/implication.go` | given CtEq を implication scope 内で正しく管理       |

**検証**: 既存 GADT テスト全 pass、given eq → TF reduction 連鎖が動作。

## Layer 2: Type family reduction + Grade error path

### L2-a: Type family reduction on-demand 統一

現在 4 つの reduction path がある:

| Path   | 場所                             | タイミング                  | 変更                        |
| ------ | -------------------------------- | --------------------------- | --------------------------- |
| Path 1 | `unify/unify.go` FamilyReducer   | eager (unify 中)            | **削除**                    |
| Path 2 | `solve/solver.go` processCtFunEq | deferred (meta 解決後)      | **主経路に昇格**            |
| Path 3 | `bidir_case.go` joinGrades       | eager                       | **CtFunEq emission に変更** |
| Path 4 | `grade.go` checkGradeBoundary    | deferred (CtFunEq emission) | 維持                        |

Path 1 (eager) を除去し、Path 3 を deferred に変更することで、全ての type family reduction が solver の CtFunEq 経由になる。

**変更**:

| ファイル           | 変更内容                                                                                               |
| ------------------ | ------------------------------------------------------------------------------------------------------ |
| `unify/unify.go`   | `normalize` 内の FamilyReducer 呼び出しを除去。FamilyReducer callback 自体は残す（CtFunEq 生成に使う） |
| `type_family.go`   | `installFamilyReducer` の動作変更: 正規化からの呼び出しを止め、stuck constraint 登録のみに             |
| `bidir_case.go`    | `joinGrades` の直接 `reduceTyFamily` を CtFunEq emission に変更                                        |
| `family/reduce.go` | eager reduce 入口の整理                                                                                |

**検証**:

- Session type probe テスト全 pass（early reduction で壊れていたケースが修正される）
- Type family 付きのテスト全 pass（on-demand でも結果が同じ）
- 性能 regression check（eager → deferred で solve 回数が増える可能性）

**L1-c との連動**: L1-c Stage 3 で given eq が solver 管理下に入ると、given eq → CtFunEq re-activation → 型族 reduction の連鎖が on-demand 統一の枠組みで自然に処理される。`(SessionDual s1 ~ s2) =>` のような制約が、s1 の meta 解決後に SessionDual を reduce し、s2 を solve する流れ。

**リスク**: Eager reduction に依存していた推論パスがあれば壊れる。特に type alias の展開と type family reduction が混在しているケース。Alias 展開は FamilyReducer とは別経路なので影響は限定的だが、慎重にテスト。

### L2-b: Grade CtFunEq error handling

`processCtFunEq` の advisory unify 失敗を domain-specific callback で捕捉する。

**変更**:

| ファイル              | 変更内容                                                                       |
| --------------------- | ------------------------------------------------------------------------------ |
| `solve/constraint.go` | `CtFunEq` に `OnFailure` callback フィールドを追加                             |
| `solve/solver.go`     | `processCtFunEq` の advisory unify 失敗時に `OnFailure` を呼ぶ                 |
| `grade.go`            | `emitGradePreserveConstraint` で `OnFailure` を設定、`ErrMultiplicity` を emit |

```go
// solve/constraint.go
type CtFunEq struct {
    FamilyName string
    Args       []types.Type
    ResultMeta types.Type
    BlockingOn []int
    OnFailure  func(span.Span, types.Type, types.Type) // (span, expected, actual)
    S          span.Span
}

// solve/solver.go - processCtFunEq 内
if reduced {
    if err := s.env.Unify(ct.ResultMeta, result); err != nil {
        if ct.OnFailure != nil {
            ct.OnFailure(ct.S, ct.ResultMeta, result)
        }
    }
    return
}
```

**検証**:

- `@Linear` field を preserve するプログラムが `ErrMultiplicity` を出すこと（deferred path 経由）
- Grade meta が solve 後に violation が検出されること
- 非 grade の CtFunEq（通常の type family）が影響を受けないこと（OnFailure = nil）

## Layer 3: Grade algebra user-definable

`usageJoin` / `gradeDrop` のハードコードを廃止し、`GradeAlgebra` type class + user-defined type family に移行。[smc.md](smc.md) Phase 4 の実装基盤。

### 変更

| ファイル            | 変更内容                                                                                                 |
| ------------------- | -------------------------------------------------------------------------------------------------------- |
| `grade.go`          | `usageJoin`, `gradeDrop`, `gradeCanPreserve` を**廃止**                                                  |
| `grade.go`          | `registerGradeAlgebraFamilies` を**廃止** (`$GradeJoin` / `$GradeDrop` 内部 family 削除)                 |
| `grade.go`          | 新: `resolveGradeAlgebra(kind) → (joinFamily, dropValue)` — registry から `GradeAlgebra` instance を探索 |
| `grade.go`          | `checkGradeBoundary` 改修: `TyCBPV` 限定を外し、grade kind から algebra を解決                           |
| `resolve_type.go`   | row field の grade resolve 後、grade kind に `GradeAlgebra` instance があるか検証。なければエラー        |
| `builtin.go`        | `Zero`, `Linear`, `Affine`, `Unrestricted` の singleton を削除（ユーザ定義に移行）                       |
| stdlib (prelude 等) | `Mult`, `GradeAlgebra`, `MultJoin`, `impl GradeAlgebra Mult` を追加                                      |

### ユーザコード

```gicel
-- stdlib/prelude (または新 pack) に移動
form Mult := { Zero; Linear; Affine; Unrestricted; }

form GradeAlgebra := \(g: Kind). {
  Join: g -> g -> g;
  Drop: g
}

type MultJoin :: Mult := \(m1: Mult) (m2: Mult). case (m1, m2) {
  (Unrestricted, _) => Unrestricted;
  (_, Unrestricted) => Unrestricted;
  (Affine, _)       => Affine;
  (_, Affine)       => Affine;
  (Linear, Zero)    => Affine;
  (Zero, Linear)    => Affine;
  (x, _)            => x
}

impl GradeAlgebra Mult := {
  Join := MultJoin;
  Drop := Zero
}
```

### チェッカーの新しい動作

1. Row field に `@g` を検出
2. `g` の型 (zonk 後) から kind `K` を取得
3. `GradeAlgebra K` の instance を registry から探索
4. Instance の `Join` method (type family) と `Drop` value を取得
5. `checkGradeBoundary`: `Join(Drop, g) ~ g` を CtFunEq として emit (L2-b の callback 付き)
6. Instance が見つからなければ `ErrNoInstance` エラー（現在の黙殺を排除）

### 検証

- 既存の `@Linear` / `@Affine` / `@Unrestricted` テストが Prelude の `GradeAlgebra Mult` instance で pass
- `@Secret` のような未定義 grade がエラーになること（黙殺されないこと）
- ユーザ定義 grade algebra (例: `Level := Public | Secret`) が動作すること
- Session type probe テスト全 pass

### ユーザ定義 grade の例

```gicel
-- ユーザが Security grade を定義
form Level := { Public; Internal; Secret; }

type LevelJoin :: Level := \(l1: Level) (l2: Level). case (l1, l2) {
  (Secret, _)   => Secret;
  (_, Secret)   => Secret;
  (Internal, _) => Internal;
  (_, Internal) => Internal;
  (x, _)        => x
}

impl GradeAlgebra Level := {
  Join := LevelJoin;
  Drop := Public
}

-- row field で使用
sensitive :: Computation { key: String @Secret } { key: String @Secret } String
```

## L3 後の応用層

L3 完了で grade algebra がユーザ定義可能になった後、以下が並行して着手可能になる。

### Named Capabilities

現行のゲートモデル (`{ array: () | r }` = effect permission bit) から、名前付きリソースモデルへ移行する。

```gicel
-- ゲートモデル (現行)
main := do {
  a <- Arr.new 5 0;    -- { array: () | r } — 全配列が同一ゲート
  b <- Arr.new 3 0;
}

-- 名前付きリソースモデル (目標)
main := do {
  counts <- allocArray 5 0;
  -- post: { counts: Array Int @Linear | r }
  cache  <- allocMap;
  -- post: { counts: Array Int @Linear, cache: Map K V @Affine | r }
}
```

**理論的根拠**:

名前付き capability は追加機能ではなく、既存の構造の復元。Atkey indexed monad + row types + grades が本来持つ表現力を、ゲートモデルが退化させている。

- **分離論理**: row field = `x ↦ v`、row merge = `P * Q`（分離積）、tail variable = frame rule
- **Session types**: 名前付き capability は session の一般化。Session = 名前付きチャネルの型レベル追跡、named cap = 名前付きリソースの型レベル追跡
- **†-SMC**: named resource row = wire bundle。ゲートモデルはワイヤーバンドルを 1 本に潰した退化形
- **QTT**: per-resource grade = graded context の各変数に semiring 係数が付く構造

**前提**: L3 (grade algebra)。Row 基盤 (`KRow`, `RowField`, `RowField.Grades`) は既存。

**変更範囲**: stdlib の Effect.Array/Map/State/Set の API 改修が主。型検査器の変更は限定的（row infrastructure は既に名前付き typed fields をサポート）。

### Per-Computation Grade (Double Grading)

現行の grade は row field 上の per-resource grade。これに加え、Computation 型自体に grade を付与する。

```gicel
-- per-resource grade (現行)
{ x: T @Linear | r }

-- per-computation grade (追加)
Computation pre post @cost a
```

**ユースケース**: 全て同一の機構（ユーザ定義半環 + 型検査器の合成検証）で実現される。

| Grade type   | 半環の Join           | 半環の Drop | 検出対象            |
| ------------ | --------------------- | ----------- | ------------------- |
| `Cost`       | `max(O(n), O(n²))`    | `O(1)`      | 計算量注釈          |
| `QueryBatch` | `max(Single, InLoop)` | `None`      | N+1 クエリパターン  |
| `Level`      | `max(Public, Secret)` | `Public`    | Security level 追跡 |

自動推論ではなくユーザ宣言 + 機械検証 — typed sandbox 思想と合致。

**前提**: L3 (grade algebra) + Computation 型の grade パラメータ追加。[smc.md](smc.md) Phase 4 設計ノートの "double grading" に該当。

**設計判断 (未決)**:

- Computation の grade パラメータの位置 (`Computation pre post a` → `Computation pre post g a` or `Computation g pre post a`)
- Per-computation grade と per-resource grade の相互作用規則
- `bind` での grade 合成: `bind : T_g A → (A → T_h B) → T_{g·h} B` (Katsumata graded monad)

## Layer 4: Solver coupling 削減（長期）

Layer 1-3 の過程で solver 境界に以下が導入される（✓ = 完了）:

- ✓ InertSet scope depth (L1-b)
- ✓ CtEq wanted/given (L1-c)
- ✓ CtFunEq OnFailure callback (L2-b)
- On-demand reduction (L2-a)
- GradeAlgebra instance 探索 (L3)

これらが solver と checker の結合を**実質的に削減**する。残る結合は DK interleaving に起因する本質的な部分のみ。完全な OutsideIn(X) 分離は DK を捨てない限り不要であり、目指さない。

## Open Questions

### Grade 関連

- **`@` の将来**: 型演算子への降格 (`(@) :: Type -> Grade -> Type`) は本計画のスコープ外。L3 完了後に検討。`@` が中置型演算子になれば特殊記法が消え、grade が universe 内の通常の型操作になる。
- **Grade polymorphism**: `\(π: Mult). A @π -> B` は solver に grade meta を追加する。L3 の後続課題。
- **□ modality**: Graded modality `□ g A` の導入は `@` の意味論を変える。L3 の後続課題。`□` が型構成子であれば `@` は `□` の中置糖衣に降格可能。
- **Row field 以外への grade 適用**: Arrow grade (`A @1 -> B`)、data field grade (`MkP: A @1, B @1`)、QTT-style context grade は本計画のスコープ外。

### Universe 関連

詳細は [universe.md](universe.md)。L0-b + L1/L2 安定後に着手。

- **Non-nullary constructor promotion**: L0-b 後に着手可能。L1/L2 と並行。
- **Universe polymorphism**: Level metavariable と Level constraint solver。L0-b + L1/L2 が前提。
- **Cumulativity**: `Type_i ≤ Type_{i+1}` の暗黙昇格。Universe polymorphism 後。
- **Sort₂+**: `KSort{Level: int}` で構造的に対応済み。
- **Impredicativity**: 計画外。
