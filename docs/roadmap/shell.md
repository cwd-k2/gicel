# Shell Pack Design

GICEL から外部プロセスを実行するための stdlib pack 設計。

## Goal

AI エージェント用途の sandbox 内で POSIX シェルの合成力を型安全に提供する。POSIX シェルの代数構造を GICEL の型システム（行多相・indexed monad・CBPV）に忠実に写像し、既存のエフェクトパックと一貫したインターフェースを持つ。

## 設計原則

議論を通じて到達した判断とその根拠。

### Cmd は純粋な値

CBPV の value/computation 区別に従う。`Cmd r` はコマンドの記述（何を実行するか）であり、副作用を持たない。`exec` だけが computation（実際に実行する）。

### 名前と引数は構築時に確定

`sh "grep" ["-r", "pattern"]` — 引数はコマンド定義の一部。実行時に組み立てるものではない。

### stdout/stderr は戻り値、state ではない

stdout/stderr はプロセスごとに生まれる局所値。session state として扱うと関数合成が壊れる（ヘルパー関数内の exec が暗黙に外の stdout を上書きする）。各 `exec` の結果を独立した束縛として返すことで合成性を保つ。

### stdin は echo で注入

stdin (fd 0) は他の fd と同列。`exec` の引数に `String` を取る非対称な設計ではなく、`echo : String -> Cmd { out: String }` で文字列をチャネルに持ち上げて `|.` で接続する。

### fd チャネルは行多相

out / err は標準提供。追加チャネルは `fd @label n` で行を拡張する。行はキャプチャされるチャネルの集合 — `exec` の結果レコードに現れるフィールドと一致する。

### exec が唯一の materialization 境界

```
String ──echo──→ Cmd ──|.──→ Cmd ──|.──→ Cmd ──exec──→ { exit, out, err, ... }
            入口 materialization              出口 materialization
                 └──── 中間は streaming ────┘
```

入口: `echo` (String → チャネル)。出口: `exec` 結果レコード (チャネル → String)。中間の `|.` はチャネル間のストリーミング接続で、データはメモリ上に materialization されない。

中間 materialization パターン（`r <- exec a; exec (echo r.#out |. b)`）はランタイムが検出して OS パイプに fusion する余地がある（stream fusion と同じ発想）。

### ガードは構文ではなくライブラリ

`unless` + `failWith`（Effect.Fail）で早期脱出をフラットに書ける。do 記法への `guard ... else` 構文追加は表現力を加えない。`setE` が `set -e` 相当のセッション設定を提供し、以降の `exec` は非零 exit code で runtime error を発生させる。

## 言語拡張

### Label kind

唯一の言語拡張。レコード/行のラベルを `Label` kind に昇格し、型パラメータとして渡せるようにする。

```gicel
-- 既存: DataKinds による昇格
form Color := Red | Green | Blue   -- Red : Color (kind)

-- 拡張: ラベルの昇格
-- { out: String, err: String } の out, err が Label kind の型になる
-- out : Label, err : Label

-- 関数の型パラメータとして使用
fd : \(l: Label) (r: Row). Int -> Cmd r -> Cmd { l: String | r }

-- 呼び出し: 既存の @ 型適用構文
fd @log 3    -- l := log (Label kind)
```

ラベル多相（ラベル変数による行パターンマッチ）は不要。`fd` は行を拡張するだけで、ラベルで分解する操作はない。

## チャネル操作の代数

### 行の意味

`Cmd r` の行 `r` は**キャプチャされるチャネルの集合**。`exec` の結果レコードに現れるフィールドと一致する。

各チャネルはデフォルトで `{result}` に流れる（exec でキャプチャ）。操作は出力先の集合を変える。

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
redir @l path mode = close @l << tap @l path mode

Cmd { l | r }  ──tap──→  Cmd { l | r }  ──close──→  Cmd r
sinks: {result}    →     {result, file}      →       {file}
```

主な derived 定義:

```
redir @l path mode  = close @l << tap @l path mode
dropErr             = close @err
mergeErr            = merge @err @out
```

### 2 × 2 の整理

|                       | 外部行先 (ファイル) | チャネル行先 (Label) |
| --------------------- | ------------------- | -------------------- |
| **行を保つ** (複製)   | `tap @l path mode`  | —（稀）              |
| **行から除去** (移動) | `close @l`          | `merge @from @to`    |

`redir` はこの表の左列の合成（tap → close）。

### パイプ `|.` の分解

```
a |. b = merge @out(a) @stdin(b) + Merge(extra(a), channels(b))
```

左の `out` が消費され（右の stdin に接続）、左のその他のチャネル（`err`, ユーザ定義）は `Merge` で結果に合流する。

## API 定義

```gicel
import Prelude

-- ═══ Types ═══

type ExitCode                              -- opaque, Int ラッパー
type Cmd (r: Row)                          -- コマンド記述子（行 r は出力チャネル集合）
form Mode := Overwrite | Append            -- ファイルの書き込みモード ( > / >> )

-- ═══ Primitives (Go 実装) ═══

-- コマンド構築
_sh   :: String -> List String -> Cmd { out: String, err: String }
_echo :: String -> Cmd { out: String }

-- チャネル操作
_fd    :: \(l: Label) (r: Row). Int -> Cmd r -> Cmd { l: String | r }
_tap   :: \(l: Label) (r: Row). String -> Mode -> Cmd { l: String | r } -> Cmd { l: String | r }
_close :: \(l: Label) (r: Row). Cmd { l: String | r } -> Cmd r
_merge :: \(from: Label) (to: Label) (r: Row).
          Cmd { from: String, to: String | r } -> Cmd { to: String | r }

-- パイプ合成
_pipe :: \(r1: Row) (r2: Row).
         Cmd { out: String, err: String | r1 }
      -> Cmd { out: String, err: String | r2 }
      -> Cmd { out: String, err: String | Merge r1 r2 }

-- 環境修飾 (per-command)
_env :: \(r: Row). String -> String -> Cmd r -> Cmd r
_dir :: \(r: Row). String -> Cmd r -> Cmd r

-- 実行 (唯一の副作用)
_exec :: \(r: Row). Cmd r
      -> Computation { shell: () | e } { shell: () | e }
         { exit: ExitCode | r }

-- セッション設定 (shell state 変更)
_setE     :: Computation { shell: () | r } { shell: () | r } ()
_setEnv   :: String -> String -> Computation { shell: () | r } { shell: () | r } ()
_unsetEnv :: String -> Computation { shell: () | r } { shell: () | r } ()
_setDir   :: String -> Computation { shell: () | r } { shell: () | r } ()

-- ExitCode 検査
_ok   :: ExitCode -> Bool
_code :: ExitCode -> Int

_sh       := assumption
_echo     := assumption
_fd       := assumption
_tap      := assumption
_close    := assumption
_merge    := assumption
_pipe     := assumption
_env      := assumption
_dir      := assumption
_exec     := assumption
_setE     := assumption
_setEnv   := assumption
_unsetEnv := assumption
_setDir   := assumption
_ok       := assumption
_code     := assumption

-- ═══ Public API ═══

sh       := _sh
echo     := _echo
fd       := _fd
tap      := _tap
close    := _close
merge    := _merge
env      := _env
dir      := _dir
exec     := _exec
setE     := _setE
setEnv   := _setEnv
unsetEnv := _unsetEnv
setDir   := _setDir
ok       := _ok
code     := _code

infixl 4 |.
(|.) := _pipe

-- ═══ Derived (GICEL 定義) ═══

-- リダイレクト: tap + close（ファイルに送ってキャプチャ除去）
redir : \(l: Label) (r: Row). String -> Mode -> Cmd { l: String | r } -> Cmd r
redir := \path mode cmd. close @l $ tap @l path mode cmd

-- stderr 便利関数
mergeErr := merge @err @out
dropErr  := close @err

-- スコープ付き環境（bracket）
withEnv : \(r: Row). String -> String
        -> Computation { shell: () | r } { shell: () | r } a
        -> Computation { shell: () | r } { shell: () | r } a
withEnv := \k v action. do { setEnv k v; r <- action; unsetEnv k; pure r }

withDir : \(r: Row). String
        -> Computation { shell: () | r } { shell: () | r } a
        -> Computation { shell: () | r } { shell: () | r } a
withDir := \p action. do {
  prev <- exec $ sh "pwd" [];
  setDir p;
  r <- action;
  setDir (trim prev.#out);
  pure r
}
```

## 語彙一覧

### Primitive (16)

| 区分         | 語彙                                   | 数  |
| ------------ | -------------------------------------- | --- |
| 構築         | `sh`, `echo`                           | 2   |
| チャネル操作 | `fd`, `tap`, `close`, `merge`          | 4   |
| 合成         | `\|.`                                  | 1   |
| 環境         | `env`, `dir`                           | 2   |
| 実行         | `exec`                                 | 1   |
| 検査         | `ok`, `code`                           | 2   |
| 設定         | `setE`, `setEnv`, `unsetEnv`, `setDir` | 4   |

### 型 (3)

| 型             | 構成子                |
| -------------- | --------------------- |
| `Cmd (r: Row)` | — (opaque)            |
| `ExitCode`     | — (opaque)            |
| `Mode`         | `Overwrite`, `Append` |

### Derived (5)

`redir`, `mergeErr`, `dropErr`, `withEnv`, `withDir`

### Prelude 追加 (2)

`when`, `unless` — Shell 固有ではなく汎用。

### 言語拡張 (1)

`Label` kind — ラベルの型レベル昇格。

## 書き味

### 基本実行

```sh
ls -la
```

```gicel
main := do {
  r <- exec $ sh "ls" ["-la"];
  putLine r.#out
}
```

### stdin 注入

```sh
echo "banana\napple\ncherry" | sort
```

```gicel
main := do {
  r <- exec $ echo "banana\napple\ncherry" |. sh "sort" [];
  putLine r.#out
}
```

### パイプライン

```sh
cat access.log | grep "ERROR" | sort | uniq -c | head -10
```

```gicel
topErrors :=
    sh "cat" ["access.log"]
  |. sh "grep" ["ERROR"]
  |. sh "sort" []
  |. sh "uniq" ["-c"]
  |. sh "head" ["-10"]

main := do {
  r <- exec topErrors;
  putLine r.#out
}
```

### 中間検査 + 再注入

```sh
FILES=$(find . -name "*.go")
echo "found: $(echo "$FILES" | wc -l) files"
echo "$FILES" | grep "TODO"
```

```gicel
main := do {
  r1 <- exec $ sh "find" [".", "-name", "*.go"];
  putLine $ "found: " <> show (length (lines r1.#out)) <> " files";
  r2 <- exec $ echo r1.#out |. sh "grep" ["TODO"];
  putLine r2.#out
}
```

意味論的には materialization → 再注入。ランタイムが検出すれば OS パイプに fusion 可能。

### xargs 相当

```sh
find . -name "*.go" | xargs grep "TODO"
```

```gicel
main := do {
  r1 <- exec $ sh "find" [".", "-name", "*.go"];
  r2 <- exec $ sh "grep" (["TODO"] ++ lines r1.#out);
  putLine r2.#out
}
```

### 条件分岐

```sh
go build ./... && go test ./... || echo "failed"
```

```gicel
main := do {
  r1 <- exec $ sh "go" ["build", "./..."];
  if ok r1.#exit
    then do {
      r2 <- exec $ sh "go" ["test", "./..."];
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
  setE;
  exec $ sh "go" ["build", "./..."];
  exec $ sh "go" ["test", "./..."];
  putLine "all passed"
}
```

### ユーザ定義チャネル

```sh
my-tool --verbose 3>debug.log
```

```gicel
main := do {
  r <- exec $ redir @log "debug.log" Overwrite $ fd @log 3 $ sh "my-tool" ["--verbose"];
  putLine r.#out
}
```

### パイプ + ユーザチャネル保存

```sh
my-tool 3>debug.log | sort
```

```gicel
main := do {
  r <- exec $ tap @log "debug.log" Overwrite $ fd @log 3 $ sh "my-tool" [] |. sh "sort" [];
  putLine r.#out;
  putLine r.#log
}
```

### stderr 合流

```sh
make all 2>&1 | tee build.log
```

```gicel
main := do {
  r <- exec $ mergeErr $ sh "make" ["all"] |. sh "tee" ["build.log"];
  putLine r.#out
}
```

### リダイレクト

```sh
cmd > output.txt 2>> error.log
```

```gicel
main := do {
  exec $ redir @out "output.txt" Overwrite
       $ tap @err "error.log" Append
       $ sh "cmd" [];
  putLine "done"
}
```

### 環境変数 (per-command)

```sh
CGO_ENABLED=0 go build -o bin/app ./cmd/app/
```

```gicel
main := do {
  setE;
  exec $ env "CGO_ENABLED" "0" $ sh "go" ["build", "-o", "bin/app", "./cmd/app/"]
}
```

### 環境変数 (scoped)

```sh
(export CC=gcc CXX=g++; make clean; make all)
docker build .
```

```gicel
gccEnv := withEnv "CC" "gcc" << withEnv "CXX" "g++"

main := do {
  setE;
  gccEnv $ do {
    exec $ sh "make" ["clean"];
    exec $ sh "make" ["all"]
  };
  exec $ sh "docker" ["build", "."]
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
docker push "myapp:$VERSION"
echo "Deployed $VERSION"
```

```gicel
import Prelude
import Shell
import Console

staticBuild := withEnv "CGO_ENABLED" "0"

main := do {
  setE;

  r <- exec $ sh "git" ["describe", "--tags", "--always"];
  let version = trim r.#out;
  putLine $ "Building " <> version;

  staticBuild $ do {
    exec $ sh "go" ["build", "-o", "bin/app", "./cmd/app/"];
    exec $ sh "go" ["test", "./..."]
  };

  exec $ sh "docker" ["build", "-t", "myapp:" <> version, "."];
  exec $ sh "docker" ["push", "myapp:" <> version];

  putLine $ "Deployed " <> version
}
```

## POSIX 対応表

| POSIX                   | GICEL Shell                            | 層                        |
| ----------------------- | -------------------------------------- | ------------------------- |
| `cmd arg1 arg2`         | `sh "cmd" ["arg1", "arg2"]`            | Value (Cmd 構築)          |
| `echo "text"`           | `echo "text"`                          | Value (String → Cmd)      |
| `a \| b`                | `a \|. b`                              | Value (Cmd 合成)          |
| `a ; b`                 | `exec a; exec b`                       | Computation (do 記法)     |
| `a && b`                | `if ok r.#exit then ...`               | Computation               |
| `a \|\| b`              | `if ok r.#exit then ... else ...`      | Computation               |
| `set -e`                | `setE`                                 | Computation (shell state) |
| `$()`                   | `r <- exec cmd; r.#out`                | Computation (bind + 射影) |
| `tee file`              | `sh "tee" ["file"]` (パイプ内コマンド) | Value (パイプライン)      |
| —                       | `tap @l path mode` (fd レベル複製)     | Value (Cmd 修飾)          |
| `xargs`                 | 不要 — `lines r.#out` で引数に変換     | —                         |
| `> path`                | `redir @out path Overwrite`            | Value (tap + close)       |
| `>> path`               | `redir @out path Append`               | Value (tap + close)       |
| `2>&1`                  | `mergeErr` (= `merge @err @out`)       | Value                     |
| `2>/dev/null`           | `dropErr` (= `close @err`)             | Value                     |
| `FOO=bar cmd`           | `env "FOO" "bar" $ cmd`                | Value (per-command)       |
| `export FOO=bar`        | `setEnv "FOO" "bar"`                   | Computation (session)     |
| `(export FOO=bar; ...)` | `withEnv "FOO" "bar" $ do {...}`       | Computation (scoped)      |
| `cd dir`                | `setDir dir`                           | Computation (session)     |
| `make -C dir`           | `dir path $ cmd`                       | Value (per-command)       |
| `3>file`                | `fd @log 3` + `redir @log path mode`   | Value                     |

## 理論的背景

### POSIX シェルの多ソート代数

シェルの演算子は 3 つのソートにまたがる多ソート代数を成す:

- **バイトストリーム**: パイプ `|` はストリーム変換の圏（`cat` が恒等射）
- **終了コード**: `&&`, `||`, `!` はブール代数（{0, nonzero} への商）
- **シェル環境**: `;` は ShellState 上の自己準同型モノイド

本設計ではこの 3 ソートを GICEL の型体系に写像する:

| ソート              | GICEL の表現                                             |
| ------------------- | -------------------------------------------------------- |
| ストリーム (パイプ) | `\|.` — `Cmd` の Value レベル合成                        |
| 終了コード (制御)   | `if ok r.#exit` — Computation レベル分岐                 |
| 環境 (状態)         | `setE`, `setEnv`, `setDir` — shell capability の状態変更 |

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
| Kahn Process Networks (1974)                   | パイプラインは決定的データフロー。`\|.` は連続関数の合成 |
| Concurrent Kleene Algebra (Hoare et al., 2011) | `\|.` (並行) と `;` (逐次) の交換律に対応                |
| Arrows (Hughes, 2000)                          | `>>>` がパイプ合成、`first` が並行チャネル               |
| π 計算                                         | `fd @l n` はチャネル生成 (ν バインダ)                    |
| Scsh (Shivers, 1994)                           | 文字列評価排除 + 型安全なプロセス合成の先例              |
| Stream fusion                                  | 中間 materialization の最適化モデル                      |
