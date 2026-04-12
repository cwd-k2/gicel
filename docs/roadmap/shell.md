# Shell Pack Design

GICEL から外部プロセスを実行するための stdlib pack 設計。

## Goal

POSIX シェルの合成力を GICEL の型システム（行多相・indexed monad・CBPV）に忠実に写像し、既存のエフェクトパックと一貫したインターフェースを持つ型安全なシェル操作を提供する。

### デプロイメントモデル: `gicel` と `gishell`

Shell Pack は `shell: ()` capability を要求する。この capability の付与を CLI レベルで分離する:

|              | `gicel`                         | `gishell`                  |
| ------------ | ------------------------------- | -------------------------- |
| 対象         | AI エージェント（非信頼コード） | 人間の開発者（信頼コード） |
| Shell        | **不可**（capability 未付与）   | **可**                     |
| 再帰         | off                             | on                         |
| timeout      | 5s                              | なし or 長い               |
| リソース制限 | 厳格（100 MiB）                 | 緩和（1 GiB）              |
| 出力形式     | `--json` 対応                   | カラー、対話的             |

`gishell` は `gicel` の別エントリポイント（`cmd/gishell/`）であり、同じランタイム・同じ型検査器を共有する。違いはデフォルト設定と利用可能な capability セットのみ。

sandbox は capability の**不在**によって成立する — Shell Pack の API 設計自体は信頼レベルに依存しない。

## 設計原則

議論を通じて到達した判断とその根拠。

### Cmd は純粋な値

CBPV の value/computation 区別に従う。`Cmd r` はコマンドの記述（何を実行するか）であり、副作用を持たない。`run` / `capture` だけが computation（実際に実行する）。

### シェルの word splitting をしない

`cmd "grep" ["-r", "pattern"]` — 引数は `List String` で構築時に個別に確定する。POSIX シェルの word splitting・glob 展開・クォート解釈は行わない。引数の動的構築（`lines r.#out` で引数リストを作る、`"myapp:" <> version` で文字列を組み立てる等）は自由にできるが、それは GICEL の式評価であり、シェルの文字列評価ではない。

### stdout/stderr は戻り値、state ではない

stdout/stderr はプロセスごとに生まれる局所値。session state として扱うと関数合成が壊れる（ヘルパー関数内の capture が暗黙に外の stdout を上書きする）。各 `capture` の結果を独立した束縛として返すことで合成性を保つ。

### stdin は echo で注入

stdin (fd 0) は他の fd と同列。実行関数の引数に `String` を取る非対称な設計ではなく、`echo : String -> Cmd { out: String }` で文字列をチャネルに持ち上げて `|>` で接続する。

### fd チャネルは行多相

out / err は標準提供。追加チャネルは `fd @label n` で行を拡張する。行はキャプチャされるチャネルの集合 — `capture` の結果レコードに現れるフィールドと一致する。

### 三つの実行モード

コマンドの実行には三つの materialization レベルがある:

| モード                   | materialization | 戻り値                    | POSIX 対応       |
| ------------------------ | --------------- | ------------------------- | ---------------- |
| `run`                    | なし            | `ExitCode`                | `cmd args`       |
| `capture`                | 全量            | `{ exit, out, err, ... }` | `$(cmd)`         |
| `stream`→`recv`→`finish` | 段階的          | 行ごと + `{ exit, err }`  | — (POSIX にない) |

`capture` は `Cmd r` の行 `r` に含まれる全チャネルを結果レコードに返す。`run` は出力を端末に流す（行は無制約）。

### stdin 継承

POSIX シェルの `cmd` や `$(cmd)` と同様、`run` / `capture` / `stream` はパイプラインの最左コマンドに親プロセス（gishell）の stdin (fd 0) を暗黙に継承する。`echo text |> cmd` のようにパイプの左に入力源がある場合はそちらが優先され、親 stdin は接続されない。

| 構成                                   | 最左コマンドの stdin           |
| -------------------------------------- | ------------------------------ |
| `run $ cmd "sort" []`                  | 親から継承                     |
| `run $ echo "x" \|> cmd "sort" []`     | echo が提供（親 stdin 不使用） |
| `capture $ cmd "sort" []`              | 親から継承                     |
| `capture $ echo "x" \|> cmd "sort" []` | echo が提供                    |
| `stream $ cmd "sort" []`               | 親から継承                     |
| `spawn @#j cmd`                        | Closed（POSIX `&` と同じ）     |

逐次 `run a; run b` は stdin を共有する — a が先に消費し、b は残りを読む。これは POSIX の逐次実行と同じ全順序の「先読み勝ち」セマンティクスであり、非決定的ではない。

Go 実装: `run` / `capture` / `stream` は最左コマンドに `cmd.Stdin = os.Stdin` を設定する。`spawn` は `cmd.Stdin = nil`（`/dev/null` 相当）。パイプラインの内部接続（`|>`）は `os.Pipe()` で行い、最左以外のコマンドの stdin はパイプの読み取り端になる。

`stream` は indexed monad の pre/post 行で `process: ()` capability を追跡し、セッションのライフサイクル（開始 → 受信 → 終了）を型レベルで保証する。開かれたストリームは `finish` で閉じなければならず、capability が行に残るため型検査がリークを検出する。

中間 materialization パターン（`r <- capture a; capture (echo r.#out |> b)`）は、原理的にはランタイムが OS パイプに fusion する余地がある（stream fusion と同じ発想）。ただし `r <- capture a` で bind 済みの値が `echo` にしか使われないことの検出には escape analysis が必要であり、現実的には将来の最適化課題。

### ガードは構文ではなくライブラリ

`unless` + `failWith`（Effect.Fail）で早期脱出をフラットに書ける。do 記法への `guard ... else` 構文追加は表現力を加えない。`set ErrExit` が `set -e` 相当のセッション設定を提供し、以降の `run` / `capture` は非零 exit code で `ExitCode` を `Effect.Fail` 経由で throw する（`try` による回復が可能）。

## 言語拡張

### Label kind

レコード/行のラベルを `Label` kind に昇格し、型パラメータとして渡せるようにする。

```gicel
-- 既存: DataKinds による昇格
form Color := Red | Green | Blue   -- Red : Color (kind)

-- 拡張: ラベルの昇格
-- { out: String, err: String } の out, err が Label kind の型になる
-- out : Label, err : Label

-- 関数の型パラメータとして使用
fd :: \(l: Label) (r: Row). Int -> Cmd r -> Cmd { l: String | r }

-- 呼び出し: 既存の @ 型適用構文
fd @log 3    -- l := log (Label kind)
```

ラベル多相（ラベル変数による行パターンマッチ）は不要。`fd` は行を拡張するだけで、ラベルで分解する操作はない。

### Union type family

`Union :: Row -> Row -> Row` — 行の結び（join-semilattice）。既存の `Merge`（disjoint 制限: 重複ラベルはエラー）に対し、`Union` は重複ラベルの型が一致すれば合流を許す。パイプのチャネル合流に使用。

## 前提条件

Shell Pack は Effect.Fail の `try` を前提とする。

```gicel
try :: \e a r. Suspended { fail: e | r } a -> Effect r (Result e a)
```

`try` は `fail` を行から除去する。`RuntimeError`（アプリケーション層の失敗）のみを捕捉し、リソース制限（Budget 超過・タイムアウト）は素通りさせる。Go の型レベルで `*eval.RuntimeError` を判別するため、ホスト権威の侵害なしに失敗回復を可能にする。

`withOpt` / `withEnv` / `withDir` 等の derived 関数では、`try` 後の `fromResult` が外側シグネチャの `fail: e` を参照するため、`try` が `fail` を除去しても問題ない。CBPV auto-coercion が `Effect` → `Suspended` の thunk 化を自動処理する。

## チャネル操作の代数

### 行の意味

`Cmd r` の行 `r` は**キャプチャされるチャネルの集合**。`capture` の結果レコードに現れるフィールドと一致する。

各チャネルはデフォルトで `{result}` に流れる（capture でキャプチャ）。操作は出力先の集合を変える。

### 基本操作

行に対する基本操作は挿入・除去・恒等の 3 種:

| 操作               | 行の効果        | 出力先への効果                  | 型                                                                 |
| ------------------ | --------------- | ------------------------------- | ------------------------------------------------------------------ |
| `fd @l n`          | 挿入 S ∪ {l}    | sinks(l) = {result}             | `Cmd r -> Cmd { l: String \| r }`                                  |
| `close @l`         | 除去 S \ {l}    | sinks(l) = {}                   | `Cmd { l: String \| r } -> Cmd r`                                  |
| `tap @l path mode` | 恒等 S          | sinks(l) ∪= {file}              | `Cmd { l: String \| r } -> Cmd { l: String \| r }`                 |
| `merge @from @to`  | 除去 S \ {from} | sinks(from) = {to のストリーム} | `Cmd { from: String, to: String \| r } -> Cmd { to: String \| r }` |

### 合成による導出

`redir`（リダイレクト）は `tap` + `close` の合成:

```
redir @l path mode = (close @l . tap @l path mode) cmd

Cmd { l | r }  ──tap──→  Cmd { l | r }  ──close──→  Cmd r
sinks: {result}    →     {result, file}      →       {file}
```

主な derived 定義:

```
redir @l path mode  = (close @l . tap @l path mode) cmd
dropErr             = close @err
mergeErr            = merge @err @out
```

### 2 × 2 の整理

|                       | 外部行先 (ファイル) | チャネル行先 (Label) |
| --------------------- | ------------------- | -------------------- |
| **行を保つ** (複製)   | `tap @l path mode`  | —（稀）              |
| **行から除去** (移動) | `close @l`          | `merge @from @to`    |

`redir` はこの表の左列の合成（tap → close）。

### 行の合成: Merge と Union

行 `r` は部分関数 `Label ⇀ Type` と見なせる。行の合成には二つの操作がある:

| 操作          | 代数的意味        | 定義条件                                | 用途                             |
| ------------- | ----------------- | --------------------------------------- | -------------------------------- |
| `Merge r₁ r₂` | 余積（coproduct） | `dom(r₁) ∩ dom(r₂) = ∅`                 | Computation のキャパビリティ合成 |
| `Union r₁ r₂` | 結び（join）      | `∀l ∈ dom(r₁) ∩ dom(r₂). r₁(l) = r₂(l)` | パイプのチャネル合流             |

`Union` は join-semilattice の結び演算で、交換律・結合律・冪等律・単位元 `{}` を満たす。`Merge` は `Union` の `dom(r₁) ∩ dom(r₂) = ∅` への制限。

キャパビリティ行には `Merge`（重複は設計ミス）、チャネル行には `Union`（共有は POSIX の fd 共有セマンティクス）を使い分ける。

### パイプ `|>` の分解

```
a |> b = connect(a.out, b.stdin) ⊕ Union(rest(a), rest(b))
```

左の `out` が消費され（右の stdin に接続）、残りのチャネルは `Union`（半束の結び）で合流する。`err` や同名のカスタムチャネルは型が一致すれば合流し、一致しなければ型エラー。

実行時の合流は時間順インターリーブ（POSIX の fd 共有セマンティクス）。これは接続チャネル（out → stdin）のデータフロー決定性とは独立な、並行プロセス固有の非決定性である（Kahn 1974: 決定性は入力の全順序読み取りに依存し、共有チャネルの合流はこの前提の外にある）。

## API 定義

```gicel
import Prelude

-- ═══ Types ═══

-- opaque types (Go 側で engine.RegisterType により登録):
--   ExitCode :: Type                      -- Int ラッパー
--   Cmd      :: Row -> Type               -- コマンド記述子（行 r は出力チャネル集合）
form Mode := Overwrite | Append            -- ファイルの書き込みモード ( > / >> )
form ShellOpt := ErrExit | PipeFail        -- シェルオプション（set -e / set -o pipefail）

-- ═══ Primitives (Go 実装) ═══

-- コマンド構築
_cmd   :: String -> List String -> Cmd { out: String, err: String }
_echo :: String -> Cmd { out: String }

-- チャネル操作
_fd    :: \(l: Label) (r: Row). Int -> Cmd r -> Cmd { l: String | r }
_tap   :: \(l: Label) (r: Row). String -> Mode -> Cmd { l: String | r } -> Cmd { l: String | r }
_close :: \(l: Label) (r: Row). Cmd { l: String | r } -> Cmd r
_merge :: \(from: Label) (to: Label) (r: Row).
          Cmd { from: String, to: String | r } -> Cmd { to: String | r }

-- パイプ合成（Union: 行の結び。重複ラベルは型一致で合流）
_pipe :: \(r1: Row) (r2: Row).
         Cmd { out: String | r1 }
      -> Cmd { out: String | r2 }
      -> Cmd { out: String | Union r1 r2 }

-- コマンド修飾 (per-command, 純粋な Cmd 変換)
_env     :: \(r: Row). String -> String -> Cmd r -> Cmd r
_dir     :: \(r: Row). String -> Cmd r -> Cmd r
_timeout :: \(r: Row). Int -> Cmd r -> Cmd r       -- 秒数。超過時 exit code 124

-- 実行
_run     :: \(r: Row). Cmd r
         -> Effect { shell: () | e } ExitCode
_capture :: \(r: Row). Cmd { out: String, err: String | r }
         -> Effect { shell: () | e }
            { exit: ExitCode, out: String, err: String | r }

-- ストリーミング（段階的 materialization。session typed via pre/post rows）
_stream  :: \(r: Row). Cmd { out: String, err: String | r }
         -> Computation { shell: () | e } { shell: (), process: () | e } ()
_recv    :: Effect { shell: (), process: () | e }
            (Maybe String)
_finish  :: Computation { shell: (), process: () | e } { shell: () | e }
            { exit: ExitCode, err: String }

-- バックグラウンド実行（session typed: spawn で capability 追加、wait で消費）
_spawn :: \(n: Label) (r: Row). Cmd r
       -> Computation { shell: () | e } { shell: (), n: () | e } ()
_wait  :: \(n: Label).
          Computation { shell: (), n: () | e } { shell: () | e } ExitCode

-- セッション設定 (shell state 変更)
_set      :: ShellOpt -> Effect { shell: () | r } ()
_unset    :: ShellOpt -> Effect { shell: () | r } ()
_setEnv   :: String -> String -> Effect { shell: () | r } ()
_unsetEnv :: String -> Effect { shell: () | r } ()
_setDir   :: String -> Effect { shell: () | r } ()

-- セッション参照 (shell state 読み取り)
_getEnv   :: String -> Effect { shell: () | r } (Maybe String)
_getDir   :: Effect { shell: () | r } String

-- ExitCode 検査
_ok   :: ExitCode -> Bool
_code :: ExitCode -> Int

_cmd       := assumption
_echo     := assumption
_fd       := assumption
_tap      := assumption
_close    := assumption
_merge    := assumption
_pipe     := assumption
_env      := assumption
_dir      := assumption
_timeout  := assumption
_run      := assumption
_capture  := assumption
_stream   := assumption
_recv     := assumption
_finish   := assumption
_spawn    := assumption
_wait     := assumption
_set      := assumption
_unset    := assumption
_setEnv   := assumption
_unsetEnv := assumption
_setDir   := assumption
_getEnv   := assumption
_getDir   := assumption
_ok       := assumption
_code     := assumption

-- ═══ Public API ═══

cmd      := _cmd
echo     := _echo
fd       := _fd
tap      := _tap
close    := _close
merge    := _merge
env      := _env
dir      := _dir
timeout  := _timeout
run      := _run
capture  := _capture
stream   := _stream
recv     := _recv
finish   := _finish
spawn    := _spawn
wait     := _wait
set      := _set
unset    := _unset
setEnv   := _setEnv
unsetEnv := _unsetEnv
setDir   := _setDir
getEnv   := _getEnv
getDir   := _getDir
ok       := _ok
code     := _code

infixl 4 |>
(|>) := _pipe

-- ═══ Derived (GICEL 定義) ═══

-- リダイレクト: tap + close（ファイルに送ってキャプチャ除去）
redir :: \(l: Label) (r: Row). String -> Mode -> Cmd { l: String | r } -> Cmd r
redir := \path mode cmd. close @l $ tap @l path mode cmd

-- ストリーミング fold（--recursion 必須）
foldLines :: \(r: Row) a. Cmd { out: String, err: String | r }
         -> a
         -> (a -> String -> Effect { shell: () | e } a)
         -> Effect { shell: () | e }
            { exit: ExitCode, err: String, acc: a }
foldLines := \cmd init step. do {
  stream cmd;
  loop := fix \self acc. do {
    line <- recv;
    case line {
      Just l  => do { acc' <- step acc l; self acc' };
      Nothing => pure acc
    }
  };
  acc <- loop init;
  r <- finish;
  pure { exit: r.#exit, err: r.#err, acc: acc }
}

-- stderr 便利関数
mergeErr := merge @err @out
dropErr  := close @err

-- スコープ付き設定（try + fromResult で例外安全。
-- action が fail しても cleanup が走り、その後 fromResult が再 throw する。）

withOpt :: \(r: Row) e. ShellOpt
       -> Effect { shell: (), fail: e | r } a
       -> Effect { shell: (), fail: e | r } a
withOpt := \opt action. do {
  set opt;
  result <- try action;
  unset opt;
  fromResult result
}

withEnv :: \(r: Row) e. String -> String
        -> Effect { shell: (), fail: e | r } a
        -> Effect { shell: (), fail: e | r } a
withEnv := \k v action. do {
  prev <- getEnv k;
  setEnv k v;
  result <- try action;
  case prev {
    Just old => setEnv k old;
    Nothing  => unsetEnv k
  };
  fromResult result
}

withDir :: \(r: Row) e. String
        -> Effect { shell: (), fail: e | r } a
        -> Effect { shell: (), fail: e | r } a
withDir := \p action. do {
  prev <- getDir;
  setDir p;
  result <- try action;
  setDir prev;
  fromResult result
}
```

## 語彙一覧

### Primitive (24)

| 区分             | 語彙                                           | 数  |
| ---------------- | ---------------------------------------------- | --- |
| 構築             | `cmd`, `echo`                                  | 2   |
| チャネル操作     | `fd`, `tap`, `close`, `merge`                  | 4   |
| 合成             | `\|>`                                          | 1   |
| 修飾             | `env`, `dir`, `timeout`                        | 3   |
| 実行             | `run`, `capture`                               | 2   |
| ストリーミング   | `stream`, `recv`, `finish`                     | 3   |
| バックグラウンド | `spawn`, `wait`                                | 2   |
| 検査             | `ok`, `code`                                   | 2   |
| 設定             | `set`, `unset`, `setEnv`, `unsetEnv`, `setDir` | 5   |
| 参照             | `getEnv`, `getDir`                             | 2   |

Note: `set` / `unset` は `ShellOpt` を引数に取る汎用 primitive。オプション追加時は `ShellOpt` に構成子を足すだけ。

### 型 (4)

| 型             | 構成子                |
| -------------- | --------------------- |
| `Cmd (r: Row)` | — (opaque)            |
| `ExitCode`     | — (opaque)            |
| `Mode`         | `Overwrite`, `Append` |
| `ShellOpt`     | `ErrExit`, `PipeFail` |

### Derived (7)

`redir`, `mergeErr`, `dropErr`, `foldLines`, `withOpt`, `withEnv`, `withDir`

### 前提条件 (Effect.Fail)

`try` — 失敗を `Result e a` に変換。`with*` 系の例外安全の基盤。

### Prelude (2)

`when`, `unless` — Shell 固有ではなく汎用。

### Type Family (1)

`Union :: Row -> Row -> Row` — 行の結び（join-semilattice）。パイプのチャネル合流に使用。

### 言語拡張 (2)

- `Label` kind — ラベルの型レベル昇格。
- `Union` type family — 行の結び。`Merge`（disjoint 制限）に対する重複許容版。

## 書き味

### 基本実行

```sh
ls -la
```

```gicel
main := do {
  run $ cmd "ls" ["-la"]
}
```

### stdin 注入

```sh
echo "banana\napple\ncherry" | sort
```

```gicel
main := do {
  run $ echo "banana\napple\ncherry" |> cmd "sort" []
}
```

### パイプライン

```sh
cat access.log | grep "ERROR" | sort | uniq -c | head -10
```

```gicel
main := do {
  cat  := cmd "cat" ["access.log"];
  grep := cmd "grep" ["ERROR"];
  sort := cmd "sort" [];
  uniq := cmd "uniq" ["-c"];
  head := cmd "head" ["-10"];

  run $ cat |> grep |> sort |> uniq |> head
}
```

各段を `:=` で束縛し、`|>` で合成する。修飾（`env`, `timeout` 等）は各束縛に `&` で付ける。パイプラインの構造が一行で見える。

### 中間検査 + 再注入

```sh
FILES=$(find . -name "*.go")
echo "found: $(echo "$FILES" | wc -l) files"
echo "$FILES" | grep "TODO"
```

```gicel
main := do {
  r1 <- capture $ cmd "find" [".", "-name", "*.go"];
  putLine $ "found: " <> show (length (lines r1.#out)) <> " files";
  r2 <- capture $ echo r1.#out |> cmd "grep" ["TODO"];
  putLine r2.#out
}
```

意味論的には materialization → 再注入。ランタイムが検出すれば OS パイプに fusion 可能（前述の escape analysis 課題）。

### xargs 相当

```sh
find . -name "*.go" | xargs grep "TODO"
```

```gicel
main := do {
  r1 <- capture $ cmd "find" [".", "-name", "*.go"];
  r2 <- capture $ cmd "grep" (["TODO"] ++ lines r1.#out);
  putLine r2.#out
}
```

単純なケースでは `xargs` は不要 — `lines r.#out` で引数リストに変換できる。ただし空白を含むパス名や `ARG_MAX` 超過の分割は `xargs` 固有の機能であり、等価ではない。

### 条件分岐

```sh
go build ./... && go test ./... || echo "failed"
```

```gicel
main := do {
  r1 <- capture $ cmd "go" ["build", "./..."];
  if ok r1.#exit
    then do {
      r2 <- capture $ cmd "go" ["test", "./..."];
      if ok r2.#exit
        then putLine "all passed"
        else putLine $ "test failed: " <> r2.#err
    }
    else putLine $ "build failed: " <> r1.#err
}
```

### ガード (set -e)

```sh
set -e
go build ./...
go test ./...
echo "all passed"
```

```gicel
main := do {
  set ErrExit;
  run $ cmd "go" ["build", "./..."];
  run $ cmd "go" ["test", "./..."];
  putLine "all passed"
}
```

### set -e + エラー回復

```sh
set -e
make all || true
echo "continued"
```

```gicel
main := do {
  set ErrExit;
  _ <- try $ run $ cmd "make" ["all"];
  putLine "continued"
}
```

### ユーザ定義チャネル

```sh
my-tool --verbose 3>debug.log
```

```gicel
main := do {
  tool := cmd "my-tool" ["--verbose"] & fd @log 3 & redir @log "debug.log" Overwrite;
  run tool
}
```

### パイプ + ユーザチャネル保存

```sh
my-tool 3>debug.log | sort
```

```gicel
main := do {
  tool := cmd "my-tool" [] & fd @log 3 & redir @log "debug.log" Overwrite;
  sort := cmd "sort" [];
  run $ tool |> sort
}
```

### stderr 合流

```sh
make all 2>&1 | tee build.log
```

```gicel
main := do {
  make := cmd "make" ["all"] & mergeErr;
  tee  := cmd "tee" ["build.log"];
  run $ make |> tee
}
```

### リダイレクト

```sh
cmd > output.txt 2>> error.log
```

```gicel
main := do {
  c := cmd "my-tool" []
     & redir @out "output.txt" Overwrite
     & redir @err "error.log" Append;
  run c;
  putLine "done"
}
```

### 環境変数 (per-command)

```sh
CGO_ENABLED=0 go build -o bin/app ./cmd/app/
```

```gicel
main := do {
  set ErrExit;
  build := cmd "go" ["build", "-o", "bin/app", "./cmd/app/"] & env "CGO_ENABLED" "0";
  run build
}
```

### 環境変数 (scoped)

```sh
(export CC=gcc CXX=g++; make clean; make all)
docker build .
```

```gicel
gccEnv := withEnv "CC" "gcc" . withEnv "CXX" "g++"

main := do {
  set ErrExit;
  gccEnv $ do {
    run $ cmd "make" ["clean"];
    run $ cmd "make" ["all"]
  };
  run $ cmd "docker" ["build", "."]
}
```

### ビルド & デプロイ

```sh
#!/bin/sh
set -e
VERSION=$(git describe --tags --always)
echo "Building $VERSION"
CGO_ENABLED=0 go build -o bin/app ./cmd/app/
go test ./...
docker build -t "myapp:$VERSION" .
docker pucmd "myapp:$VERSION"
echo "Deployed $VERSION"
```

```gicel
import Prelude
import Shell
import Console

main := do {
  set ErrExit;

  r <- capture $ cmd "git" ["describe", "--tags", "--always"];
  version := trim r.#out;
  putLine $ "Building " <> version;

  withEnv "CGO_ENABLED" "0" $ do {
    run $ cmd "go" ["build", "-o", "bin/app", "./cmd/app/"];
    run $ cmd "go" ["test", "./..."]
  };

  run $ cmd "docker" ["build", "-t", "myapp:" <> version, "."];
  run $ cmd "docker" ["push", "myapp:" <> version];

  putLine $ "Deployed " <> version
}
```

## POSIX 対応表

| POSIX                   | GICEL Shell                               | 層                        |
| ----------------------- | ----------------------------------------- | ------------------------- |
| `cmd arg1 arg2`         | `cmd "name" ["arg1", "arg2"]`             | Value (Cmd 構築)          |
| `echo "text"`           | `echo "text"`                             | Value (String → Cmd)      |
| `a \| b`                | `a \|> b`                                 | Value (Cmd 合成)          |
| `a ; b`                 | `run a; run b`                            | Computation (do 記法)     |
| `a && b`                | `if ok r.#exit then ...`                  | Computation               |
| `a \|\| b`              | `if ok r.#exit then ... else ...`         | Computation               |
| `set -e`                | `set ErrExit`                             | Computation (shell state) |
| `set +e`                | `unset ErrExit`                           | Computation (shell state) |
| `set -o pipefail`       | `set PipeFail`                            | Computation (shell state) |
| `$()`                   | `r <- capture cmd; r.#out`                | Computation (bind + 射影) |
| `tee file`              | `cmd "tee" ["file"]` (パイプ内コマンド)   | Value (パイプライン)      |
| —                       | `tap @l path mode` (fd レベル複製)        | Value (Cmd 修飾)          |
| `xargs`                 | `lines r.#out` で引数に変換（単純ケース） | —                         |
| `> path`                | `redir @out path Overwrite`               | Value (tap + close)       |
| `>> path`               | `redir @out path Append`                  | Value (tap + close)       |
| `2>&1`                  | `mergeErr` (= `merge @err @out`)          | Value                     |
| `2>/dev/null`           | `dropErr` (= `close @err`)                | Value                     |
| `FOO=bar cmd`           | `env "FOO" "bar" $ cmd`                   | Value (per-command)       |
| `export FOO=bar`        | `setEnv "FOO" "bar"`                      | Computation (session)     |
| `(export FOO=bar; ...)` | `withEnv "FOO" "bar" $ do {...}`          | Computation (scoped)      |
| `cd dir`                | `setDir dir`                              | Computation (session)     |
| `make -C dir`           | `dir path $ cmd`                          | Value (per-command)       |
| `3>file`                | `fd @log 3` + `redir @log path mode`      | Value                     |

### 意図的な非対応

以下の POSIX 機能は設計スコープ外。GICEL の既存機構で代替するか、意図的に排除する:

| POSIX                        | 理由                                                                                              |
| ---------------------------- | ------------------------------------------------------------------------------------------------- |
| `< file`（入力リダイレクト） | `capture (cmd "cat" [path])` で文字列化し `echo \|>` で注入。IO 層を持たず shell コマンドに委ねる |
| `<< EOF`（heredoc）          | `echo` に複数行文字列を渡す                                                                       |
| Glob 展開 (`*.go`)           | シェルの文字列評価を排除する設計方針。`cmd "find" [...]` + `lines` で明示的に構築                 |
| Word splitting               | 引数は `List String` で個別に確定。シェルの暗黙分割は行わない                                     |
| `!` (否定)                   | `not (ok r.#exit)` — GICEL の式                                                                   |
| `trap` (シグナル)            | ランタイムが Go 側でシグナルを管理。GICEL レベルのハンドラは不要                                  |
| `exec`                       | `run` が stdin を暗黙継承するため、プロセス置換なしで同等の効果を得られる。sandbox 環境では不許可 |
| `source` / `.`               | GICEL のモジュールシステム (`import`) で代替                                                      |
| Job control (`bg`/`fg`)      | `spawn`/`wait` が基本ケースをカバー。対話的ジョブ制御はスコープ外                                 |

## 理論的背景

### POSIX シェルの多ソート代数

シェルの演算子は 3 つのソートにまたがる多ソート代数を成す:

- **バイトストリーム**: パイプ `|` はストリーム変換の圏（`cat` が恒等射）
- **終了コード**: `&&`, `||`, `!` はブール代数（{0, nonzero} への商）
- **シェル環境**: `;` は ShellState 上の自己準同型モノイド

本設計ではこの 3 ソートを GICEL の型体系に写像する:

| ソート              | GICEL の表現                                            |
| ------------------- | ------------------------------------------------------- |
| ストリーム (パイプ) | `\|>` — `Cmd` の Value レベル合成                       |
| 終了コード (制御)   | `if ok r.#exit` — Computation レベル分岐                |
| 環境 (状態)         | `set`, `setEnv`, `setDir` — shell capability の状態変更 |

パイプだけが Value 層にいる理由: それが唯一「exit code に依存しない」合成（データフロー）だから。`&&`/`||`/`;` は exit code（副作用の結果）に基づく制御フローであり Computation 層が自然。

### チャネル操作の分類

fd チャネルへの操作は、行（キャプチャ集合）への効果と出力先への効果で分類される:

|                       | 外部行先 (ファイル) | チャネル行先 (Label) |
| --------------------- | ------------------- | -------------------- |
| **行を保つ** (複製)   | `tap`               | —                    |
| **行から除去** (移動) | `close`             | `merge`              |

`redir` は左列の合成（tap → close）。`fd` は行の挿入。

### 先行研究との対応

| 理論                                           | 対応                                                     |
| ---------------------------------------------- | -------------------------------------------------------- |
| Kahn Process Networks (1974)                   | パイプラインは決定的データフロー。`\|>` は連続関数の合成 |
| Concurrent Kleene Algebra (Hoare et al., 2011) | `\|>` (並行) と `;` (逐次) の交換律に対応                |
| Arrows (Hughes, 2000)                          | `>>>` がパイプ合成、`first` が並行チャネル               |
| π 計算                                         | `fd @l n` はチャネル生成 (ν バインダ)                    |
| Scsh (Shivers, 1994)                           | 文字列評価排除 + 型安全なプロセス合成の先例              |
| Join-semilattice (部分関数の束)                | `Union` — 行の結び。`Merge` は disjoint 制限（余積）     |
| Stream fusion                                  | 中間 materialization の最適化モデル                      |
