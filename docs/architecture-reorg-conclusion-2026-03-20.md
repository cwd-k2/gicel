# Architecture Reorg Conclusion 2026-03-20

対象:

- `internal/check`
- `internal/syntax/parse`
- `internal/engine`
- `internal/eval`
- 周辺の `types`, `core`, `stdlib`

目的:

- 再編するとしたら、どのアーキテクチャ方針を採るべきかを結論ベースで示す
- 「完全 subsystem 化できるか」という問いに対する実務的な答えを残す

## 結論

このコードベースを再編するなら、採るべき方針は次です。

1. 外周は hexagonal 風に整理する
2. compiler 中核は `Session + Services` で整理する
3. `check` は完全 subsystem 化を目指さない
4. `parse` は subsystem 化を比較的強く進めてよい

短く言うと:

- **外は hexagonal**
- **中は compiler-oriented**

です。

## なぜ純粋なヘキサゴナルではないのか

ヘキサゴナルアーキテクチャは、このリポジトリの**外周**にはよく合います。

たとえば:

- CLI
- sandbox API
- host bindings
- stdlib pack registration
- module loading
- diagnostics / explain / trace 出力

は port / adapter に落としやすいです。

一方で `check` の内部は、典型的な hexagonal の「中心ドメイン + 明確な入出力境界」にそのまま乗りません。

理由:

- infer/check が unifier に密接依存する
- unifier が family reduction と solver に密接依存する
- solver が scope, instance registry, evidence に密接依存する
- elaboration が型情報と解決済み evidence の両方に依存する

つまり `check` の内部は、port 越しに独立 subsystems を呼ぶ構図ではなく、**共有途中状態を抱えたドメイン実装**です。

ここを無理に hexagonal にすると:

- interface が増える
- 状態所有者が逆に見えにくくなる
- 実装上の密な相互作用が interface の裏に隠れる

ので、設計として見た目だけがきれいになって、実質は悪化しやすいです。

## 採るべきアーキテクチャ

## 1. Outer Layer: Hexagonal-ish

外周は明確に切れます。

### Core domain

- `types`
- `core`
- surface AST / token model
- constraint / module / diagnostic model

### Application layer

- compile pipeline
- runtime assembly
- sandbox orchestration
- docs/example/CLI execution flow

### Ports

- source input
- module repository
- primitive registry
- diagnostics sink
- trace / explain hooks
- budget / cancellation provider

### Adapters

- CLI
- Go embedding API
- embedded stdlib source
- sandbox helper

この整理は特に `engine` に合います。

## 2. Compiler Core: Session + Services

compiler 中核はこれが最も現実的です。

### 中心

- `CompilationSession`

ここに:

- source
- diagnostics
- budget
- feature flags
- global type/module registry snapshot

を持たせる。

### ぶら下げる service

- `ParserCore`
- `RecoveryService`
- `ScopeService`
- `RegistryService`
- `TypeChecker`
- `ConstraintSolver`
- `FamilyReducer`
- `EvidenceBuilder`
- `Elaborator`

重要なのは、これらを**完全独立 subsystem**として扱わないことです。

方針は:

- service ごとに責務を分ける
- 共有途中状態は session に集める
- 直接 field 乱用ではなく service 境由で使う

です。

これは今の `Checker` を否定するのではなく、**巨大な `Checker` を coordinator + domain services に薄く分解する**発想です。

## `check` に関する補足結論

`check` については、DIP を前面に出すよりも **state ownership の整理** を優先すべきです。

理由:

- `check` の複雑さは interface 不足より shared mutable state の混線から来ている
- 抽象を増やしても、途中状態の所有者が曖昧なままだと改善しにくい
- infer / unify / solve / evidence / family は、抽象境界より state 境界で整理したほうが自然

したがって `check` の再編原則は:

1. まず state owner を決める
2. 次に mutation authority を絞る
3. その後で必要な箇所だけ抽象を入れる

です。

### `check` で明示すべき state owner

理想的には、少なくとも以下の ownership を固定するべきです。

#### 1. Scope / Import ownership

所有対象:

- current module
- imported names
- qualified scopes
- ambiguity cache
- ownership cache

責務:

- import 解決
- qualified/open/selective import
- ambiguity 判定

今のコードではこの state が `Checker.scope` にぶら下がりつつ、広く参照されています。ここは `ScopeService` に寄せる価値が高いです。

#### 2. Type / Class / Family registry ownership

所有対象:

- constructors
- aliases
- classes
- instances
- promoted kinds/cons
- type families

責務:

- declaration registration
- module export/import 時の registry 反映
- lookup 用の安定ビュー提供

今の `checkerRegistry` は一応まとまっていますが、誰がどこまで mutate してよいかが広いです。ここは `RegistryService` か `SemanticRegistry` として owner を明示したほうがよいです。

#### 3. Constraint solving ownership

所有対象:

- worklist
- inert set
- ambiguity cache
- resolve depth / implication level

責務:

- wanted constraint の処理
- instance resolution
- residual constraint 管理

ここは `SolverService` が owner であるべきです。`Checker` 直轄の field 群として持ち続けると、将来的にも solver の境界が曖昧なままです。

#### 4. Unification ownership

所有対象:

- meta solutions
- kind solutions
- row label context
- snapshot / restore trail

責務:

- unify
- zonk
- rollback

これは比較的すでに `Unifier` に ownership が寄っています。`check` 再編でも、この ownership は崩さないほうがよいです。

#### 5. Evidence / elaboration ownership

所有対象:

- deferred evidence placeholders
- dict parameter naming
- Core への evidence 注入規則

責務:

- evidence resolution
- dict application/lambda の挿入
- typeclass / qualified type elaboration

ここは solver と密ですが、ownership は分けたほうがよいです。solver が「何を解くか」を持ち、elaborator が「Core にどう出すか」を持つ構図が望ましいです。

### `check` で DIP を使うべき場所

部分的には有効です。

特に抽象化しやすいのは:

- module repository 的な import source
- diagnostics sink
- family lookup/reduction interface
- evidence resolution interface
- export view builder

つまり **`check` の外周境界** です。

ここでは:

- high-level orchestration
- concrete storage / module loading

を切り離す意味があります。

### `check` で DIP を使いすぎるべきでない場所

以下は、無理に interface 越しにしないほうがよいです。

- infer/check と unifier の境界
- solver と unifier の境界
- solver と evidence collection の境界
- family reduction と solver の内部協調

これらは本質的に shared intermediate state を必要とします。

ここまで抽象化すると:

- interface の数だけ増える
- 実装依存は隠れるだけ
- デバッグが難しくなる

可能性が高いです。

### `check` の現実的な再編像

`check` は最終的に以下のような形が現実的です。

#### `TypeCheckSession`

所有:

- source
- diagnostics
- budget
- feature flags
- top-level config snapshot

#### `ScopeService`

所有:

- import / qualification / ambiguity state

#### `RegistryService`

所有:

- semantic registry

#### `SolverService`

所有:

- worklist / inert set / solve state

依存:

- `Unifier`
- `RegistryService`

#### `ElaborationService`

所有:

- evidence insertion rules
- Core emission policy

依存:

- `SolverService`
- `Unifier`

#### `TypeChecker`

責務:

- infer/check の coordinator
- 上記 services を束ねる

この形なら:

- state owner が見える
- mutation authority が見える
- それでも compiler 的な密な協調は維持できる

### `check` に対する最終結論

`check` で優先すべきなのは:

- **DIP の徹底**

ではなく

- **state ownership の明確化**
- **service ごとの mutation authority の固定**
- **Checker の coordinator 化**

です。

抽象化は、そのあと必要なところにだけ使うべきです。

## 3. Parser Core: 比較的強く分けてよい

`parse` は `check` より分けやすいです。

現実的には:

- `ParserCore`
  - token stream
  - cursor
  - fixity
  - generic helpers
- `ExprParser`
- `TypeParser`
- `PatternParser`
- `DeclParser`
- `ImportParser`
- `ClassParser`
- `Recovery`
- `Speculation`

くらいまでは十分切れます。

特に分けたいのは:

- speculative parse / rollback
- error recovery

です。

ここは今の parser の複雑さの大半を生んでいるので、専用 service に寄せる価値が高いです。

## 採るべきでないアーキテクチャ

## 1. `check` の完全 subsystem 化

おすすめしません。

理由:

- 相互依存が本質
- shared intermediate state が本質
- 境界を細かく切ると interface の数だけ増える
- 意味論の一貫性よりも分離を優先してしまう

特に:

- infer/check
- unify
- solve
- evidence
- family
- elaboration

を完全独立にするのは非現実的です。

## 2. すべてを hexagonal に寄せること

おすすめしません。

language implementation は、通常の業務システムほど clean な port separation で表現できません。

内部の型検査と制約解決は、むしろ:

- compiler passes
- mutable session state
- internal services

で考えたほうが実態に合います。

## 望ましい最終像

最終的な見取り図は、例えば次です。

### Domain

- `internal/domain/types`
- `internal/domain/core`
- `internal/domain/syntax`
- `internal/domain/constraints`

### Compiler

- `internal/compiler/session`
- `internal/compiler/parse`
- `internal/compiler/scope`
- `internal/compiler/check`
- `internal/compiler/solve`
- `internal/compiler/family`
- `internal/compiler/elab`
- `internal/compiler/diag`

### Runtime

- `internal/runtime/eval`
- `internal/runtime/capenv`
- `internal/runtime/trace`

### Host / App

- `internal/host/registry`
- `internal/host/stdlib`
- `internal/app/engine`
- `internal/app/sandbox`
- `cmd/gicel`

これはあくまで方向性ですが、重要なのはディレクトリ名ではなく分割原理です。

## 分割原理

分けるときの原則は以下です。

1. shared state の所有者を一つにする
2. algorithm と registry を分ける
3. diagnostics と budget は cross-cutting service にする
4. parse は文法責務と recovery 責務を分ける
5. engine は orchestration に専念させる

## 実務的な移行方針

大規模再編を一気にやるべきではありません。

順番としては:

1. `engine` 外周の port/adaptor を整理する
2. `parse` を `core + recovery + subparsers` に分ける
3. `check` で `scope/import`, `solver/evidence`, `family`, `diag` を service 化する
4. 最後に `Checker` を coordinator へ薄くする

`check` から先に全面再編すると、意味論回帰のリスクが高すぎます。

## 最後の結論

このリポジトリに最も合うのは、**ハイブリッド構成**です。

- 外周: hexagonal 風
- compiler 中核: session + services
- parser: subsystem 化を進める
- checker: 完全分離ではなく coordinator 化

これが、構造改善と実装現実性のバランスが最も良い案です。
