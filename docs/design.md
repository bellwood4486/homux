# homux 設計

> **この文書は「どう作るか」の Source of Truth である。**
> 常に現在の設計だけを記述する。検討過程・却下案は書かない。
> 「なぜその選択をしたか」は `docs/adr/` を参照。
> 「何を作るか」は `docs/spec.md` を参照。

---

## 1. アーキテクチャ

責務を以下のように分離する。データは一方向に流れる。

```text
repository
    │
    ▼
  scan      ── repository を walk して []Source を得る。$HOME を見ない
    │
    ▼
 resolve    ── Source + active profile → target ごとの Resolution
    │           副作用なし・決定論的
    ▼
 inspect    ── Resolution × HOME の実状態 → []TargetState
    │
    ▼
  plan      ── []TargetState → []Action
    │           status / apply --dry-run / apply が同じ Action を受け取る
    ▼
  exec      ── Action を実行する。ここだけが破壊的操作を行う
```

**`status` と `apply --dry-run` と `apply` は同じ `Plan` を消費する。** `Plan` は「実際にファイルシステムを触る `[]Action`」と「表示・件数集計の入力である `[]TargetState`」の両方を持つ。下流が状態を数え直したり解決し直したりする余地を残さないことで INV-11 を守る。

---

## 2. ディレクトリ構成

go.dev の [Organizing a Go module](https://go.dev/doc/modules/layout) の "Basic command" / "Server project" パターンに従う。単一コマンドかつエクスポート用 API を持たないため、`main.go` をルートに置き、実装をすべて `internal/` に収める。`cmd/` は使わない。

```text
homux/
├── go.mod                    module github.com/bellwood4486/homux
├── main.go                   func main のみ。internal/cli を呼ぶ
├── LICENSE                   MIT
├── README.md
├── CLAUDE.md
├── docs/
│   ├── spec.md
│   ├── design.md
│   └── adr/
└── internal/
    ├── cli/                  cobra のコマンド定義。ここだけが cobra を知る
    ├── config/               .homux.toml (repo) と config.toml (local)
    ├── selector/             @@ suffix のパーサ。純粋
    ├── scan/                 repository walk → []Source。$HOME を見ない
    ├── resolve/              Source + profile → Resolution。純粋・副作用なし
    ├── env/                  Home / Repo の絶対パス。環境を明示的に持ち回る
    ├── inspect/              desired × HOME 実状態 → []TargetState
    ├── plan/                 []TargetState → []Action。純粋
    ├── exec/                 Action の実行。破壊的操作はここだけ
    └── ui/                   対話プロンプト・出力整形。ここだけが huh と色を知る
```

### 2.1 パッケージの import 制約

以下の制約を守る。違反はレビューで指摘する。

| パッケージ | import してはいけないもの | 理由 |
|---|---|---|
| `selector` | すべての外部依存、`os` | 純粋な文字列処理 |
| `resolve` | `os`, `path/filepath` の FS アクセス関数 | 純粋・決定論的であること（INV-11 の土台） |
| `plan` | `os` | 純粋であること |
| `scan` | `$HOME` に触れる一切 | spec §9 の責務分離 |
| `inspect`, `exec` 以外 | ファイルシステムを変更する API | 破壊的操作の集約 |
| `cli` 以外 | `cobra` | フレームワーク差し替え可能性の確保 |
| `ui` 以外 | `huh`, 色ライブラリ | 同上 |

`resolve` が `os` を import した瞬間に設計が壊れたと分かる、という検知装置としてこの境界を置いている。

---

## 3. 主要な型

```go
// scan
type Source struct {
    RepoPath string    // repo ルートからの相対パス  ".claude/settings.json@@work"
    Target   string    // HOME からの相対パス        ".claude/settings.json"
    Selector *Selector // nil なら common source
}

// selector
type Selector struct {
    Profiles []string // ["work", "personal"]  (@@work+personal)
}

// resolve
type Resolution struct {
    Target     string   // HOME からの相対パス
    Candidates []Source // この target に対応する全 source
    Selected   *Source  // nil なら absent
    Reason     Reason   // なぜ選ばれた／選ばれなかったか（explain 用）
    Err        error    // ambiguous / unknown profile / invalid selector
}

// inspect
type TargetState struct {
    Resolution Resolution  // Ignored のときはゼロ値。孤児 symlink では Target だけが埋まる
    RepoPath   string      // Ignored のときの repo 相対パス。それ以外は空
    Kind       StateKind   // Linked / Missing / Occupied / Stale / Ignored / Inactive / Error
    Current    Current     // HOME 上の実状態
    Err        error       // HOME の読み取りエラー（Resolution.Err とは別物）
}

type Current struct {
    Kind    CurrentKind // Absent / File / Dir / Symlink
    Link    string      // Readlink の生の値。相対リンクなら相対のまま
    LinkAbs string      // Link を絶対化・正規化したもの
    Managed bool        // リンク先が repo 配下に解決されるか（spec §9.1）
}

// plan
type Plan struct {
    Actions []Action            // FS を触るものだけ。何もしない target は含まない
    States  []TargetState       // 表示と件数集計の唯一の入力
}

func (p Plan) Errors() int      // spec §12.4 のスキップ件数 = exit code 1 の根拠

type Action struct {
    Kind    ActionKind  // CreateSymlink / Relink / ReplaceTarget / RemoveStaleSymlink
    Target  string      // 絶対パス
    LinkTo  string      // これから張るリンク先の絶対パス。RemoveStaleSymlink では空
    From    string      // 今のリンク先の絶対パス。Relink と、symlink の ReplaceTarget でのみ非空
    Current CurrentKind // Target が今何であるか。File / Dir / Symlink
    Backup  string      // ReplaceTarget のときの退避先。ReplaceTarget では必ず非空（INV-13）
    Confirm bool        // 実行前に確認が必要か
}
```

`Reason` は `explain` の出力に直結する。resolver は「選んだ結果」だけでなく「なぜそう選んだか」を必ず返す。

`Current` を `fs.FileMode` ではなく `CurrentKind` で公開するのは、`plan` が `io/fs` を import できないためである（§2.1）。ファイルシステムを読むのは `inspect` の 1 度だけで、`plan` と `ui` はその値だけを見る。`Stale` は 1 つの `StateKind` であり、`plan` が `Resolution.Selected` の有無で relink と削除を振り分ける（spec §9.2 の種類1／種類2）。

`Action` に「何もしない」を表す `Skip` は無い。理由を持たない `Skip` を並べても `ui` は結局 `TargetState` を引き直すことになり、`Ignored` は HOME target を持たないため `Action` と 1:1 に対応させることもできない。何もしない target は `Plan.States` にのみ現れる。

`plan` は `Action` を作る直前に、`Target` が repo 配下に解決されないことを検査する。`inspect` の HOME 走査は repo が `$HOME` 配下にある場合に repo 内へ降りうるため、ここが INV-14 を構造として守る最後の関門になる。違反した状態は `Action` を生成せず `KindError` に落とす。

`From` と `Current` は `exec` が使わない。`ui` が確認プロンプトの文言（spec §12.4 の 3 種類）と dry-run の注記（spec §12.5）を決めるためだけに存在する。`ui` に `TargetState` を引き直させない — `Action` と `TargetState` の対応付けを `ui` に持たせると、同じ突き合わせが `RenderPlan` と `ConfirmAction` に二重に現れる。`exec` は `Kind` / `Target` / `LinkTo` / `Backup` だけを見る。

退避先を決めるのは `plan` である。`exec` は `Backup` が既に存在すれば停止するだけで、空いている名前を探さない（ADR 0012）。`plan` が `os` を持たない以上、退避先の実在検査は `exec` にしか書けず、`apply --dry-run` は退避先の衝突を検出できない。この非対称性は spec §12.5 に「dry-run は plan が見える範囲を示す」として現れている。

退避先の timestamp に使う時刻は `Input.Now` で受け取る。`plan` は `time.Now()` を呼ばない。`MkdirAll`（spec §4.1）は独立した `Action` にせず、`CreateSymlink` / `Relink` / `ReplaceTarget` の暗黙の一部として `exec` が行う。

---

## 4. 環境の受け渡し

グローバル変数・`os.UserHomeDir()` の直接呼び出しを禁止する。環境は明示的に持ち回る。

```go
type Env struct {
    Home string // 絶対パス
    Repo string // 絶対パス。EvalSymlinks 済み
}
```

これにより、テストで `t.TempDir()` を Home / Repo に差し替えるだけで実 `$HOME` から完全に切り離せる。

### 4.1 repo path の解決順序

```text
1. --repo フラグ
2. local config の repo
```

環境変数による上書きは提供しない。

### 4.2 local config

場所は `$XDG_CONFIG_HOME/homux/config.toml`。`XDG_CONFIG_HOME` が未設定なら `~/.config/homux/config.toml`。

```toml
repo = "~/dotfiles"
profile = "work"          # キーの不在 = profile なし
```

`profile` キーの**不在**が「profile なし」を意味する。`profile = ""` も同じ意味として寛容に読む。`"none"` のような予約文字列は使わない（`none` という名前の profile と衝突するため）。

repo path は入力時に絶対パスへ展開し、`filepath.EvalSymlinks` で実体まで解決してから保存する。これを怠ると、リンク先の実体パスと config 上の repo path が一致せず、すべての symlink が unmanaged と判定される。

---

## 5. 依存ライブラリ

| 用途 | ライブラリ | 備考 |
|---|---|---|
| CLI フレームワーク | `github.com/spf13/cobra` | サブコマンド階層と shell 補完生成のため |
| 対話 UI | `github.com/charmbracelet/huh` | `profile create` の fork 対象選択（MultiSelect）にだけ使う。`internal/ui` に閉じ込める |
| glob | `github.com/bmatcuk/doublestar/v4` | `ignore` の `**` サポート |
| TOML 読み取り | `github.com/pelletier/go-toml/v2` | 書き込みは §6 参照 |
| TTY 判定 | `golang.org/x/term` | `--color auto` の非 TTY 検出用（ADR 0009） |

Go は 1.27 以降。ライセンスは MIT。

**huh は bubbletea v1 系に依存しており、間接依存を含め約 30 パッケージを持ち込む。** 将来 huh v2 への追従が発生しうるため、`internal/ui` の外に huh の型を漏らさない。

対話は既定で `internal/ui` の素のプロンプト（`ui.Prompter`）で行う。`apply` の確認（`[y/N]`）も `init` の 3 つの問い（パス入力・`[y/N]`・profile の単一選択）も 1 問 1 答であり、spec のプロンプト例をそのまま出せて、出力が丸ごと検証できることを取る。

**huh を使うのは `profile create` の fork 対象選択だけである**（ADR 0010、ADR 0011）。`internal/ui/select.go` がその唯一の場所であり、`profile create` 自身の `Apply this migration? [y/N]` を含め、他のすべての対話は素のプロンプトのままである。

huh は自前で端末を掴むため、選択画面だけは `cmd.SetIn` / `cmd.SetOut` 越しに検証できない。`internal/cli` は選択を関数値として受け取り、テストはそれを差し替えて選択画面以外の全経路を検証する。

### 5.1 バージョン表示

`homux version` および `--version` は `<version> (<revision>)` を表示する。値の取り方は 2 系統ある。

- **`-ldflags` で埋め込まれた値を正本とする。** GoReleaser が `internal/cli` の `ldVersion` / `ldCommit` に注入する（ADR 0013）
- **埋め込みが無ければ `debug.ReadBuildInfo()` にフォールバックする。** `go install` で入れたバイナリはこちらを通り、VCS リビジョンが表示される

ldflags が要るのは、`debug.ReadBuildInfo()` の `Main.Version` にタグが入るのが module proxy 経由で取得したときだけだからである。GoReleaser はローカル checkout からビルドするため、埋め込みが無いと配布バイナリが `(devel)` を返してしまう。

---

## 6. `.homux.toml` の書き込み

読み取りは `go-toml/v2` の `Unmarshal` を使う。

**書き込みは全体の Marshal をしない。** `profiles = [` の開始位置から対応する `]` までのバイト範囲だけを特定して置換し、ファイルの他の部分（`ignore` セクション、ユーザーが書いたコメント、整形）には一切触れない。

`profile create` が `init` の生成した雛形コメントを消してしまう事故を防ぐためであり、INV-10（CLI が変更しても結果が理解可能であること）の要請でもある。

`profiles` 配列の**内部**に書かれたコメントは失われる（spec §15 の既知の制限）。

範囲置換の対象は**既に存在する** `.homux.toml` である。`init` が雛形を書き出すのは新規作成のときだけで、`O_EXCL` で開いて既存ファイルには 1 バイトも書かない。

local config（`config.toml`）は homux が全体を所有するため、丸ごと Marshal してよい。一時ファイル + rename で書き、`profile` が空のときはキーごと省く（§4.2）。

---

## 7. テスト戦略

ファイルシステムを interface で抽象化しない。symlink・dangling link・パーミッションといった**このツールの本質そのもの**を、モックで潰してしまうため。

| 層 | 手法 |
|---|---|
| `selector` / `scan` / `resolve` / `plan` | 純粋関数の table-driven テスト。ファイルシステム不要 |
| `inspect` / `exec` | `t.TempDir()` 上に実ファイル・実 symlink を作って検証 |
| `cli` | `t.TempDir()` を Home / Repo として `--repo` 経由で注入する統合テスト |

`io/fs` は symlink 作成 API を持たないため使わない。

### 7.1 必ずテストで担保する項目

以下は仕様の中核であり、テストがない状態でマージしない。

- `@@` を含まない `@` 付きファイル名（`tunnel@.service`、`.gitconfig@2024`）が common source として扱われること
- `foo@@work` と `foo@@work+personal` が同時に存在するとき ambiguous エラーになること（INV-07）
- profile なしのとき profile-specific source が一切選択されないこと（INV-09）
- dangling symlink であっても repo 配下を指していれば managed と判定されること（spec §9.1）
- Occupied の置換で退避ファイルが必ず作られること（INV-13）
- `apply` が冪等であること（2 回実行して 2 回目が no-op になる）
- `profile rename` が衝突を検出したとき、1 ファイルも変更していないこと（INV-15）

---

## 8. 出力

出力生成は `internal/ui` の 1 箇所に集約する。機械可読出力（`--json`）は V1 では実装しないが、後から追加できる形を保つ。

色は `--color auto|always|never` と `NO_COLOR` 環境変数で制御し、既定は `auto`（非 TTY では無効）。

---

## 9. 実装順序

純粋パッケージを先に横で固め、その上は縦に切って常に動くバイナリがある状態を保つ。

```text
Phase 1 — 心臓部（副作用なし。ここを固めきる）
  1. selector    @@ のパース、profile 名文法、+ による複数指定
  2. config      .homux.toml / local config の読み取り
  3. scan        repository walk、ignore 適用、Source の生成
  4. resolve     Resolution の算出、ambiguous / unknown profile の検出

Phase 2 — 読み取り専用の CLI（HOME を一切変更しない）
  5. inspect     HOME の実状態との突き合わせ
  6. plan        Action の生成
  7. cli/status  homux status（--all / --verbose）
  8. cli/explain homux explain（HOME 側・repo 側の両方の引数）

Phase 3 — 破壊的操作
  9. exec        Action の実行、退避、部分適用時の報告
 10. cli/apply   homux apply --dry-run → homux apply

Phase 4 — セットアップと取り込み
 11. cli/init    repo path 入力、雛形生成、profile 選択、apply まで
 12. cli/add     move + symlink、ディレクトリの再帰 add

Phase 5 — profile 管理
 13. profile list / use
 14. profile create（migration wizard）
 15. profile rename（事前全検証 + 一括実行）
 16. profile delete（参照の列挙と selector の rewrite）
```

Phase 2 の完了時点で「壊さないツール」として実用可能になる。Phase 3 以降は既存の Planner の出力を実行するだけであり、新しい解決ロジックを足さない。

個々のタスク分解と進捗は Linear プロジェクト [homux](https://linear.app/bellwood4486/project/homux-0dbb86b4f471) が持つ。上のフェーズが Milestone、各項目が issue に対応する。この文書はフェーズの依存順序のみを定義する。

---

## 10. CI

GitHub Actions で以下を実行する。最初から入れる。

```text
go test ./...
go vet ./...
golangci-lint run
```

配布（goreleaser / Homebrew tap）は V1 のスコープ外とする。当面は `go install github.com/bellwood4486/homux@latest` で運用する。
