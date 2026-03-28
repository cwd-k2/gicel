# Performance Hotspots — 改善設計

Profile 基準: `BenchmarkEngineEndToEndSmall` (Prelude compile + 小プログラム check + eval)。
runtime 除外後のアプリ CPU 配分で分析。

---

## H1. Zonk の defer オーバーヘッド — ✅ 完了

### 観測

| 指標          | 値                           |
| ------------- | ---------------------------- |
| flat CPU      | 1.69s (app 内 1 位)          |
| cum CPU       | 3.95s (app 38%)              |
| うち defer    | 1.20s (Zonk flat の **71%**) |
| alloc objects | 10.7M (total 13.6%)          |

### 実施内容

方式 (A) を採用: `zonkInner` 分離。`Zonk` はトップレベル wrapper として `Budget.Nest()` / `defer Budget.Unnest()` を 1 回だけ実行し、`zonkInner` は budget-free な純粋再帰として型の全ノードを走査する。defer オーバーヘッドは N 回/ノード → 1 回/呼び出しに削減。

---

## H2. TyApp.Children() のスライス alloc — ✅ 完了

### 観測

| 指標          | 値                                                 |
| ------------- | -------------------------------------------------- |
| alloc objects | 5.1M (total 6.6%)                                  |
| alloc bytes   | 156MB (total 4.1%)                                 |
| callers       | containsSkolem, typeSizeRec, AnyType, CollectTypes |

### 実施内容

`types.ForEachChild` (type switch ベースの inline visitor) を導入。ホットパスの `containsSkolem`、`removeSkolemIDsFrom` 等を `Children()` から `ForEachChild` に置換。`Children()` インターフェース自体はテスト用途で保持。

---

## H3. checkSkolemEscapeInSolutions の O(solutions × depth) 走査 — ✅ 完了

### 観測

| 指標    | 値                                                                 |
| ------- | ------------------------------------------------------------------ |
| cum CPU | 2.63s (app 25%)                                                    |
| 内訳    | containsSkolem 1.37s (52%) + Zonk 0.95s (36%) + mapIter 0.20s (8%) |

### 実施内容

`types.ContainsMetaOrSkolem` による ground solution 早期スキップを導入。メタ変数・skolem を含まない solution（大多数）は Zonk + containsSkolem をバイパスする。設計案の `hasSkolem` ビットマーク方式ではなく、走査時の前判定で実現。

---

## H4. ScanContext の線形走査 — ✅ 完了

### 観測

| 指標    | 値                                                                   |
| ------- | -------------------------------------------------------------------- |
| cum CPU | 2.09s (app 20%)                                                      |
| 内訳    | resolveFromContext 1.33s (64%) + resolveFromSuperclasses 0.76s (36%) |

### 実施内容

`resolveFromContext` に `LookupDictVar` (className → dict 変数の indexed lookup) を fast path として導入。`DictClassName` が設定されていない dict 変数のみ ScanContext にフォールバック。`resolveFromSuperclasses` は依然として ScanContext を使用するが、呼び出し頻度は resolveFromContext の fast path ヒットで大幅に減少。

---

## H5. Pretty の不必要な呼び出し — ✅ 完了

### 観測

| 指標          | 値                  |
| ------------- | ------------------- |
| alloc objects | 5.9M (total 7.5%)   |
| alloc bytes   | 152 MB (total 4.0%) |
| flat CPU      | 0.36s               |

### 実施内容

`types.Pretty` の呼び出しをエラーパスとトレースパスに限定。ホットパス（正常な型検査・unification）からの Pretty 呼び出しを排除。現在の呼び出し元はすべて `addCodedError` / `addSemanticUnifyError` / `ch.trace` / エンジン API の出力整形のみ。
