# Infrastructure Path

Universe 層化の施行と grade algebra のハードコード排除を、関連する solver 課題と合わせて実施する計画。[smc.md](smc.md) Phase 4 (Multiplicity Generalization) の前提基盤を整備する。

**依存チェーン**: L0-b → (L1 → L2) → L3 → L4

**クリティカルパス**: L1-b → L2-a → L3

## 動機

### 現状の問題構造

1. **Universe stratification 未施行** — `data Foo :: Sort := ...` がコンパイルを通る。Type-in-Type。
2. **Grade algebra ハードコード** — `usageJoin` が `"Zero"/"Linear"/"Affine"/"Unrestricted"` を文字列マッチ。ユーザ定義の grade は黙殺される。
3. **Grade CtFunEq 黙殺** — `processCtFunEq` の advisory unify 失敗が `ErrMultiplicity` を emit しない。
4. **solveWanteds inert set scope** — 無条件 `Reset()` が binding 跨ぎの CtFunEq を消す。
5. **Type family reduction timing** — eager global reduction が型の identity を変え、session type を壊す。

これらは独立した bug ではなく、**solver 基盤の不足 × 型レベル表現の不完全さ** という共通の根に由来する。

### なぜ同時に対処するか

- Grade algebra のユーザ定義化には、grade kind の正当性検証 (universe) と、grade constraint の正しい伝搬 (solver) が前提。
- Universe enforcement は grade kind (`KData{Mult}`) の分類を正式にする。
- Solver の inert set scope 修正は、grade CtFunEq が生存するための必要条件。
- Type family reduction 統一は、user-defined grade family が正しく reduce されるための必要条件。

個別に修正すると、後続の変更が前の修正を invalidate するリスクがある。依存関係に沿って積み上げれば、各 layer が次の layer の不変条件を保証する。

## 依存グラフ

```
                    ┌───────────────────────┐
                    │  L0-a: Set.map bug    │  (独立)
                    │  L0-c: Optimizer      │  (独立)
                    │  L0-d: Tuple Eq/Ord   │  (独立)
                    └───────────────────────┘


    ┌─────────────────────┐         ┌──────────────────────────┐
    │ L0-b: Universe      │         │ L1-a: SolverLevel        │
    │   enforcement       │         │   protocol               │
    └────────┬────────────┘         └────────┬─────────────────┘
             │                               │
             │                     ┌─────────┴──────────────────┐
             │                     │ L1-b: Inert set scope      │
             │                     └────────┬───────────────────┘
             │                              │
             │               ┌──────────────┼──────────────────┐
             │               │              │                  │
             │    ┌──────────┴──────┐  ┌────┴───────────────┐  │
             │    │ L2-a: TF reduce │  │ L2-b: Grade        │  │
             │    │   on-demand     │  │   CtFunEq callback │  │
             │    └────────┬────────┘  └────┬───────────────┘  │
             │             │                │                  │
             │             └────────┬───────┘                  │
             │                      │                          │
             │           ┌──────────┴──────────┐               │
             └──────────→│ L3: Grade algebra   │←──────────────┘
                         │   user-definable    │
                         │   = SMC Phase 4 基盤│
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

### L0-b: Universe enforcement

`KSort` にレベルを持たせ、宣言が universe を逸脱しないか検証する。

**変更**:

| ファイル                | 変更内容                                                                                    |
| ----------------------- | ------------------------------------------------------------------------------------------- |
| `types/kind.go`         | `KSort{}` → `KSort{Level int}`。`Level 0` = 現在の Kind の sort。`Equal` は Level を比較    |
| `check/resolve_kind.go` | `KindExprSort` → `KSort{Level: 0}`。未知 kind name の `KType{}` fallback を**エラーに変更** |
| `check/decl_form.go:22` | result kind `KType{}` ハードコードに、annotation との整合性検証を追加                       |
| `unify/kind_unify.go`   | `KSort` の unification に Level 比較を追加                                                  |

**検証**:

- `data Foo :: Sort := ...` がエラーになること
- 既存テスト全 pass（`KSort{Level: 0}` が従来の `KSort{}` と等価であること）
- Promoted kind の level が正しく計算されること

**設計判断**: Sort₂ 以上は「必要になったときに `Level` を増やすだけ」で対応可能にしておく。現時点で `Level > 0` を生成するコードパスは作らない。

**後続拡張への設計注記**:

L0-b の設計は以下の拡張を**排除しない**形にする。実装は本計画のスコープ外。詳細は [universe.md](universe.md)。

- **Non-nullary constructor promotion**: `decl_form.go` の promotion ロジックで、nullary チェックをガード条件として分離し、将来 non-nullary promotion (`Just :: KType -> KData{Maybe}`) を追加する際にガード除去のみで済むようにする。
- **Universe polymorphism**: `KSort{Level: int}` の `int` は固定 level。将来 level metavariable (`KSort{Level: LevelExpr}`) に拡張する余地を残す。具体的には `KSort` の `Equal` / `String` / `KindSubst` が `Level` フィールドの型変更に対して局所的に閉じていること。
- **Cumulativity**: kind unification を equality-based で実装する。将来 subkinding (`KType ≤ KSort{0}`) を追加する場合は unification とは別の subsumption パスになるため、equality パスとの干渉はない。

### L0-c: Optimization pipeline module/main asymmetry

Module は最適化なし、main は 4 pass。明示的ポリシーに変更。

### L0-d: Tuple Eq/Ord runtime support

Runtime の辞書解決で tuple をサポート。

## Layer 1: Solver 基盤修正

### L1-a: SolverLevel protocol clarification

3 つの timing pattern を不変条件としてテストで固定する。

**3 patterns**:

| Pattern     | 場所                         | SolverLevel      | 意味                                 |
| ----------- | ---------------------------- | ---------------- | ------------------------------------ |
| Baseline    | `checker.go:279`             | `0`              | 全 meta soluble                      |
| Trial       | `checker.go:391-394`         | `-1`             | touchability 無効 (candidate テスト) |
| Implication | `solve/implication.go:80-81` | `solver.Level()` | inner scope の meta は untouchable   |

**DK interleaving constraint**: `CheckWithLocalScope` で body check の**後**に SolverLevel を設定する。body check 中の eager unification が outer meta を触れるようにするため。

**変更**:

| ファイル               | 変更内容                                                  |
| ---------------------- | --------------------------------------------------------- |
| `solve/implication.go` | 不変条件のコメント追加、deferred SolverLevel 設定のテスト |
| `checker.go`           | timing pattern ごとの regression テスト                   |

**検証**: GADT context での meta 解決が壊れないこと。

### L1-b: solveWanteds inert set scope

`solver.go:115` と `solver.go:175` の無条件 `Reset()` を、scope-aware なリセットに変更。

**問題**: 同一 binding 内で複数回 `SolveWanteds` が呼ばれると、前回の stuck CtFunEq が消える。Grade の CtFunEq が blocking meta の解決前に消失し、violation が報告されない。

**アプローチ**: InertSet に scope depth を導入。

```go
type InertSet struct {
    // ... 既存フィールド
    scopeDepth int // 現在の scope depth
}

func (s *InertSet) EnterScope()  { s.scopeDepth++ }
func (s *InertSet) LeaveScope()  { /* scopeDepth 以上の constraint を消去 */ s.scopeDepth-- }
func (s *InertSet) Reset()       { /* 現在の scopeDepth の constraint のみ消去 */ }
```

`SolveWanteds` の `Reset()` は現在の scope の constraint のみ消去し、外側 scope の stuck CtFunEq は保持する。

**変更**:

| ファイル               | 変更内容                                                                   |
| ---------------------- | -------------------------------------------------------------------------- |
| `solve/inert.go`       | `scopeDepth` フィールド追加、`EnterScope`/`LeaveScope`/scope-aware `Reset` |
| `solve/solver.go`      | `SolveWanteds` の `Reset()` 呼び出しを scope-aware に                      |
| `solve/implication.go` | implication scope で `EnterScope`/`LeaveScope` を使用                      |

**検証**:

- Grade の CtFunEq が binding 跨ぎで生存すること
- Implication scope の constraint が scope 外に漏れないこと
- 既存テスト全 pass

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

type MultJoin :: Mult -> Mult -> Mult := \m1 m2. case (m1, m2) {
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

type LevelJoin :: Level -> Level -> Level := \l1 l2. case (l1, l2) {
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

## Layer 4: Solver coupling 削減（長期）

Layer 1-3 の過程で solver 境界に以下が導入される:

- InertSet scope depth (L1-b)
- CtFunEq OnFailure callback (L2-b)
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

詳細は [universe.md](universe.md)。

- **Universe polymorphism**: Level metavariable と Level constraint solver。L3 には不要。
- **Cumulativity**: `Type_i ≤ Type_{i+1}` の暗黙昇格。L3 には不要。
- **Non-nullary constructor promotion**: フィールド付きコンストラクタの promotion。L0-b で拡張余地を確保。
- **Sort₂+**: `KSort{Level: int}` で構造的に対応済み。
- **Impredicativity**: 計画外。
