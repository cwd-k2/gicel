# Performance Hotspots — 改善設計

Profile 基準: `BenchmarkEngineEndToEndSmall` (Prelude compile + 小プログラム check + eval)。
runtime 除外後のアプリ CPU 配分で分析。

---

## H1. Zonk の defer オーバーヘッド

### 観測

| 指標          | 値                           |
| ------------- | ---------------------------- |
| flat CPU      | 1.69s (app 内 1 位)          |
| cum CPU       | 3.95s (app 38%)              |
| うち defer    | 1.20s (Zonk flat の **71%**) |
| alloc objects | 10.7M (total 13.6%)          |

`Zonk` は再帰呼び出しの各レベルで `Budget.Nest()` + `defer Budget.Unnest()` を実行。

### 理論: なぜ defer がここまで効くか

Go の `defer` は関数フレーム上に deferred call を積む。Go 1.14 以降 open-coded defer で高速化されたが、それでもフレーム毎に:

1. `deferprocStack`: deferred call の登録（フレーム上のスロット書き込み）
2. 関数 return 時: `(*_panic).nextDefer` → `popDefer` → `(*_panic).nextFrame` のチェーン

Zonk は **型の全ノードに再帰する**。Prelude の型は平均深さ 5-8、stdlib 全体で何万ノードも走査する。N ノードなら N 回の `Nest()/defer Unnest()` が発生。1 回あたり ~110ns (deferprocStack 18ns + nextDefer 59ns + popDefer 25ns) × 10.7M 呼び出し ≈ 1.18s。観測値 1.20s と一致。

### 設計

**Nest/Unnest を手動化。** Zonk は panic しない（型走査は制御されたループ）ので、defer の panic-safety は不要。

```go
func (u *Unifier) Zonk(t types.Type) types.Type {
    if u.Budget != nil {
        if err := u.Budget.Nest(); err != nil {
            return t
        }
        // defer u.Budget.Unnest() を以下に置換
    }
    result := u.zonkInner(t)
    if u.Budget != nil {
        u.Budget.Unnest()
    }
    return result
}
```

ただし `zonkInner` が複数の `return` を持つため、全 return path で `Unnest()` が必要。
選択肢:

**(A) zonkInner 分離**: `Zonk` は budget guard + `zonkInner` 呼び出し + unnest。`zonkInner` は budget-free な純粋再帰。Budget check は Zonk のトップコールのみで実行（深さ制限は Zonk→zonkInner の再帰スタックを `Nest` で数える）。

**(B) counter ベース**: `Budget.Nest/Unnest` を呼ばず、`Zonk` 内にローカル counter を threading。制限超過で bail out。ただし Zonk は soln map を経由するため再帰深さ ≠ 型の静的深さ。

**(A) が正解。** `Zonk` のネスティングは型の構造深さを反映し、Nest/Unnest は inc/dec のみ。`zonkInner` は Budget フリーにし、Zonk の各再帰呼び出しで Nest/Unnest を budget-aware wrapper 経由で行う。

### 期待効果

defer 排除で Zonk flat -1.20s。app CPU の ~12% 改善。alloc は変わらない。

---

## H2. TyApp.Children() のスライス alloc

### 観測

| 指標          | 値                                                 |
| ------------- | -------------------------------------------------- |
| alloc objects | 5.1M (total 6.6%)                                  |
| alloc bytes   | 156MB (total 4.1%)                                 |
| callers       | containsSkolem, typeSizeRec, AnyType, CollectTypes |

### 理論: なぜ小スライスが高コストか

`Children() []Type` は `TyApp` で `[]Type{t.Fun, t.Arg}` を返す。これは 2 要素のスライスで:

- 16 bytes (slice header: ptr + len + cap)
- 16 bytes (backing array: 2 × 8 byte pointer)
- 合計 32 bytes → Go allocator の 32-byte size class

32 bytes × 5.1M = 163 MB。観測値 156 MB と一致。各 alloc は `mallocgcSmallScanNoHeader` を経由し、GC scan 対象（ポインタを含むため）。GC の scan 負荷にも寄与。

### 設計

**Children() を使う traversal を type switch に置換。**

`Children()` の呼び出し元は 4 箇所（skolem.go の `containsSkolem`/`removeSkolemIDsFrom`、type.go の `typeSizeRec`、map.go の `MapType` — ただし MapType は既に type switch）。

Zonk は Children() を使わない（既に type switch）。主な consumer は `containsSkolem` と `typeSizeRec`。

```go
// Before
for _, child := range ty.Children() {
    if id, found := ch.containsSkolem(child, skolemIDs); found {
        return id, true
    }
}

// After: inline visitor
func forEachChild(t types.Type, fn func(types.Type) bool) bool {
    switch ty := t.(type) {
    case *types.TyApp:
        return fn(ty.Fun) && fn(ty.Arg)
    case *types.TyArrow:
        return fn(ty.From) && fn(ty.To)
    case *types.TyForall:
        return fn(ty.Kind) && fn(ty.Body)
    case *types.TyCBPV:
        return fn(ty.Pre) && fn(ty.Post) && fn(ty.Result)
    case *types.TyEvidence:
        return fn(ty.Constraints) && fn(ty.Body)
    // TyEvidenceRow: entries.AllChildren() — keep slice for now
    default:
        return true // leaf
    }
}
```

**`Children()` インターフェースは削除しない**（テストや将来の汎用 traversal で有用）。ホットパスのみ `forEachChild` に置換。

### 期待効果

5.1M allocs 消滅。GC scan 対象 156 MB 減少。containsSkolem + typeSizeRec の実行時間 ~5-10% 改善。

---

## H3. checkSkolemEscapeInSolutions の O(solutions × depth) 走査

### 観測

| 指標    | 値                                                                 |
| ------- | ------------------------------------------------------------------ |
| cum CPU | 2.63s (app 25%)                                                    |
| 内訳    | containsSkolem 1.37s (52%) + Zonk 0.95s (36%) + mapIter 0.20s (8%) |

### 理論: なぜ全 solution を走査するのか

OutsideIn(X) の skolem escape check は「implication scope を抜けるとき、その scope の skolem が外側の meta solution に漏れていないことを検証する」もの。

GHC では touchability (level-based) で escape を **防止** し、check は belt-and-suspenders。GICEL も touchability を実装済み（SolverLevel による untouchable guard）だが、DK interleaving のため body check 中は SolverLevel を上げない — つまり touchability は「body check 後、solver 起動前」の窓でのみ有効。body check 中の eager unification で外側の meta に skolem が流入する可能性が理論的に残る。

現在の実装は `Solutions()` の全エントリを走査して zonk + containsSkolem。solution map のサイズは Prelude compile で ~数千エントリ。各エントリの zonk は型深さに比例。結果: O(|solutions| × avg_type_depth)。

### 設計

**Incremental skolem tracking.** solveMeta 時に、解に skolem が含まれるかを 1 bit でマークする。

```go
// Unifier に追加
type solnEntry struct {
    ty            types.Type
    hasSkolem     bool  // この solution に skolem が含まれるか
}

// solveMeta 内
func (u *Unifier) solveMeta(m *types.TyMeta, soln types.Type) {
    u.soln[m.ID] = solnEntry{
        ty:        soln,
        hasSkolem: typeContainsSkolem(soln),
    }
}
```

`checkSkolemEscapeInSolutions` は `hasSkolem == false` のエントリをスキップ。大多数の solution は skolem-free（具体型やメタチェーン）なので、実質的な走査対象は数エントリに減少。

ただし `soln` map の value 型を変えると全 Zonk/Solve/Snapshot に影響。**代替**: `hasSkolem` を別 map (`skolemSolns map[int]bool`) にし、`solveMeta` で同時に更新。既存の `soln map[int]types.Type` を変えない。

### 期待効果

solution の 95%+ が skolem-free と仮定すると、走査量 1/20 以下。2.63s → ~0.2s。app CPU の ~23% 改善。

---

## H4. ScanContext の線形走査

### 観測

| 指標    | 値                                                                   |
| ------- | -------------------------------------------------------------------- |
| cum CPU | 2.09s (app 20%)                                                      |
| 内訳    | resolveFromContext 1.33s (64%) + resolveFromSuperclasses 0.76s (36%) |

### 理論: なぜ線形走査が高コストか

`resolveFromContext` は「wanted constraint `C args` に対して、context 中の dict 変数 `d: C$Dict args` を見つける」操作。DK-style context はスタック（push/pop で scoping）なので、最新から oldest へ線形走査。

Prelude を import すると context に ~100-200 エントリが積まれる（各 instance の dict + 各 binding の型 + 型変数）。constraint solving 中に resolveInstance → resolveFromContext → ScanContext が繰り返し呼ばれる。

しかし、既に `evidenceIndex map[string][]int` が `Context` にある（L20）。これは className → positions のインデックス。`LookupEvidence` (L95) はこれを使う。

### 設計

**resolveFromContext を LookupEvidence ベースに変更。**

現在 `resolveFromContext` は `ScanContext` を使い、全エントリを走査して `matchesDictVar` で照合する。`matchesDictVar` は dict 変数の型から className を抽出して比較 — しかし className は既知（引数で渡されている）。

```go
// Before (resolve_instance.go)
func (s *Solver) resolveFromContext(className string, args []types.Type, sp span.Span) ir.Core {
    var result ir.Core
    s.env.ScanContext(func(entry env.CtxEntry) bool {
        if v, ok := entry.(*env.CtxVar); ok && !v.SolverInvisible && s.matchesDictVar(v, className, args) {
            result = &ir.Var{Name: v.Name, Module: v.Module, S: sp}
            return false
        }
        return true
    })
    return result
}

// After: use LookupEvidence
func (s *Solver) resolveFromContext(className string, args []types.Type, sp span.Span) ir.Core {
    for _, ev := range s.env.LookupEvidence(className) {
        if s.matchEvidence(ev, args) {
            return &ir.Var{Name: ev.DictName, S: sp}
        }
    }
    return nil
}
```

ただし現在の `resolveFromContext` は `CtxVar`（dict 変数）を探しており、`CtxEvidence` とは異なる可能性がある。solver bridge で dict 変数がどう Push されるかの確認が必要。

`resolveFromSuperclasses` は `CtxVar` の型から superclass chain を辿るため、className でのフィルタリングが直接適用できない。ただし superclass の起点となる dict 変数は特定の className を持つので、`matchesDictVar` の先頭で className 不一致を早期 reject できる。

**段階的改善:**

1. `resolveFromContext` を `LookupEvidence` ベースに（CtxEvidence が dict を包含している場合）
2. `resolveFromSuperclasses` は className の先頭フィルタ追加（`classInfo.Supers` に target className が含まれない場合 skip）

### 期待効果

resolveFromContext: O(context_size) → O(evidence_for_class)。Prelude で context ~200、className あたり evidence ~5。40x 高速化。1.33s → ~0.05s。
resolveFromSuperclasses: super chain フィルタで ~50% 削減。0.76s → ~0.4s。
合計: 2.09s → ~0.45s。app CPU の ~16% 改善。

---

## H5. Pretty の不必要な呼び出し

### 観測

| 指標          | 値                  |
| ------------- | ------------------- |
| alloc objects | 5.9M (total 7.5%)   |
| alloc bytes   | 152 MB (total 4.0%) |
| flat CPU      | 0.36s               |

### 理論

`Pretty` は型の人間可読表現を構築する。`fmt.Sprintf` (2.3M allocs, 2.9%) と合わせて文字列構築がかなりの割合。

エラーメッセージ構築で `Pretty` が呼ばれるのは正当だが、profiler が 5.9M 回を記録している — これはエラー数に比べて異常に多い。エラー以外のパス（trace、debug、key 生成など）で `Pretty` が呼ばれている可能性。

### 設計

**調査優先。** `Pretty` の呼び出し元をリストアップし、エラーパス以外での使用を特定。遅延評価（`lazy.String` パターンや `Stringer` interface 経由）に変更可能な箇所を同定。

これはコスト特定が先で、設計は調査後。

---

## 優先度と依存関係

```
H1 (defer)  ←── 独立、mechanical。最も cost/benefit が高い
H2 (Children) ←── 独立、mechanical。H3 の前にやると H3 の改善幅も増す
H3 (skolem) ←── H2 に依存（containsSkolem が Children を使う）
H4 (context) ←── 独立。solver bridge の確認が必要
H5 (Pretty) ←── 調査フェーズ
```

| #   | 改善                | 推定改善     | 難度 | blast radius              |
| --- | ------------------- | ------------ | ---- | ------------------------- |
| H1  | defer 排除          | -12% app CPU | 低   | Zonk 1 関数               |
| H2  | Children alloc 排除 | -5% allocs   | 低   | 3 caller                  |
| H3  | incremental skolem  | -23% app CPU | 中   | solveMeta + checkSkolem   |
| H4  | context index       | -16% app CPU | 中   | resolve_instance + bridge |
| H5  | Pretty 調査         | TBD          | 低   | 調査のみ                  |
