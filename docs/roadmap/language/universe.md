# Universe Extensions

[infrastructure.md](infrastructure.md) L0-b (universe enforcement) 後に開かれる拡張。各項目は L0-b の `KSort{Level: int}` 設計が**排除しない**形になっていることを前提とする。

**依存**: 全項目が L0-b を前提とする。一部は solver 基盤 (L1-L2) も前提。

**実行順序**:

```
L0-b (Universe enforcement)
  │
  ├──→ Non-nullary promotion    (L0-b のみ。L1/L2 と並行可能)
  │
  └──→ L1/L2 安定後:
         │
         ├──→ Universe polymorphism  (Level metavar + constraint solver)
         │       │
         └───────┴──→ Cumulativity   (subkinding rule。Univ poly 後が望ましい)
```

## Non-nullary Constructor Promotion

現状 nullary コンストラクタのみ promote される。フィールド付きコンストラクタの promotion は Kind level の型適用を要する。

```gicel
form Maybe := \a. { Nothing; Just: a; }
-- 現状: Nothing は promoted, Just は promoted されない
-- promotion 後: Just :: KType -> KData{Maybe}
```

**必要なインフラ**: promoted constructor に kind arrow を持たせる、Kind level の型適用解決。

**前提**: L0-b。L3 (grade algebra) には不要 — grade 値は nullary。Session type の型レベルエンコード高度化で必要になる。

## Universe Polymorphism

単一の定義を複数の universe level で使う。

```gicel
-- 現状: level ごとに別定義
id   :: \a. a -> a := \x. x;
type Id :: \(k: Kind). k -> k := \a. a;

-- universe polymorphism: level を量化
id :: \(l: Level). \(a: Type l). a -> a := \x. x;
```

**必要なインフラ**:

- Level metavariable (`?ℓ`) — 型推論中に生成される level の未知数
- Level expression (`LevelExpr ::= 0 | ℓ + 1 | max(ℓ₁, ℓ₂) | ?ℓ`) — `KSort{Level: int}` を `KSort{Level: LevelExpr}` に拡張
- Level constraint solver (`ℓ₁ ≤ ℓ₂`, `max(ℓ₁, ℓ₂)`) — Harper-Pollack (1991) のアルゴリズム: constraint graph の acyclicity check + shortest-path assignment
- Level quantifier の parsing と elaboration

**前提**: L0-b + solver 基盤 (L1-L2)。L3 (grade algebra) には不要。型レベルライブラリを複数 level で共有する場面で必要になる。

**参考設計**:

| 言語         | Level 表現                            | 推論                     | ユーザ記述                                                |
| ------------ | ------------------------------------- | ------------------------ | --------------------------------------------------------- |
| Agda         | `lzero`, `lsuc`, `_⊔_`                | 明示的                   | `{ℓ : Level} → Set ℓ → ...`                               |
| Lean 4       | `0`, `u+1`, `max u v`, `imax u v`     | 自動                     | ほぼ不要                                                  |
| Coq 8.5+     | universe variable + global constraint | 自動 (typical ambiguity) | `@{u}` で明示可                                           |
| GICEL (将来) | `0`, `ℓ+1`, `max(ℓ₁, ℓ₂)`             | keyword + auto           | keyword (`type`/`form`) で base level、level param は推論 |

## Cumulativity

`Type_i ≤ Type_{i+1}` の暗黙昇格。Type level の型を Kind level で直接参照可能にする。

**必要なインフラ**: Kind unification に subkinding rule 追加、subsumption check の kind 版。

**前提**: L0-b + kind unification 安定。Universe polymorphism の後が望ましい — level metavariable が入った状態で subkinding を設計する方が、後から level polymorphism を追加するより整合的。現在の promotion 機構で level 間参照は明示的に可能であり、cumulativity は利便性の問題だが、ユーザ体験に大きく効く。

**Coq の教訓**: Cumulative inductive types (Timany-Sozeau 2017) は cumulativity と inductive types の相互作用を慎重に設計した。GICEL の `form` (inductive) に cumulativity を追加する場合、同様の設計判断が必要。

## Sort₂ 以上

`Kind` をペイロードに持つ data 型の promotion で必要。

```gicel
form KCode := { EmbedK: Kind; ArrK: KCode -> KCode; }
-- promoted level = max(2, 3) + 1 = 4 (Sort₂) — 現状表現不能
```

L0-b の `KSort{Level: int}` 設計で構造的には対応済み。`Level > 0` を生成するコードパスは需要発生時に追加。`KSort{Level: 1}` = Sort₂ に対応。

## Impredicativity

**計画外**。`Prop` 相当の impredicative universe は GICEL の設計目標と合致しない。Impredicativity は証明系における proof-irrelevance と大消去 (large elimination) の制御のために必要であり、GICEL のような計算言語では利点よりも複雑さが上回る。
