# apply Occupied 解決策選択 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `apply` の Occupied（unmanaged な HOME 上の実体があり、repo 側には既に対応する source がある）状態の対話に、既存の「repo 優先（退避して symlink）」に加えて「HOME 優先（共通 source へ取り込み）」「HOME 優先（profile 専用 source へ取り込み）」を選べるようにする。

**Architecture:** `plan.Action` / `plan.ActionKind` は変更しない。`exec.Confirm` の戻り値を `bool` から `Decision{Resolution ConflictResolution; AdoptPath string}` に拡張し、`exec` 内の enum + `switch` で分岐する。repo パスの組み立て（`p` のときの `target@@<profile>` 形式）は `ui.Prompter`（`home`/`repo`/`profile` を持つ）が行い、`exec` にはただの絶対パス（`AdoptPath`）として渡す。`exec` は Home/Repo のパスを一切知らない。

**Tech Stack:** Go 1.27+、標準ライブラリのみ（`os`, `path/filepath`）。新規外部依存は追加しない。

**Spec:** `docs/spec.md` §12.4.1、`docs/design.md` §3.1、`docs/adr/0015-occupied-conflict-resolution-choices.md`。Linear: [BEL-26](https://linear.app/bellwood4486/issue/BEL-26/apply-occupied-の解決に-home-優先の取り込みを選べるようにする)

## Global Constraints

- Go 1.27 以降。`go install ...@latest` は使わない（`mise.toml` が正本）
- ファイルシステムを interface で抽象化しない。`inspect`/`exec` のテストは `t.TempDir()` 上の実ファイル・実 symlink で行う
- `resolve`/`plan`/`selector` は `os` を import しない。ファイルシステムを変更してよいのは `internal/exec` だけ
- `cobra` を知ってよいのは `internal/cli` だけ。`huh` と色ライブラリを知ってよいのは `internal/ui` だけ（今回はどちらも使わない）
- コミットメッセージは Conventional Commits、summary は日本語・命令形・現在形（`add`/`fix` であり `added`/`fixed` ではない）
- パッケージ単体のテストは `go test ./internal/<pkg>/...`。全パッケージテスト（`go test ./...` 相当の `just test`）は各タスクの最後と、計画完了時にのみ実行する
- コード変更後は `go vet ./internal/<pkg>/...` を実行する
- `git commit --no-verify` は使わない。pre-commit hook（lefthook）が落ちたら直す

---

### Task 1: `selector.BuildName`（"@@" suffix の逆変換）

**Files:**
- Modify: `internal/selector/selector.go`
- Test: `internal/selector/selector_test.go`

**Interfaces:**
- Produces: `func BuildName(base, profile string) string` — `ParseName` の逆。`base + Delimiter + profile` を返す純粋関数

- [ ] **Step 1: 失敗するテストを書く**

`internal/selector/selector_test.go` の末尾に追加:

```go
func TestBuildName(t *testing.T) {
	tests := []struct {
		base    string
		profile string
		want    string
	}{
		{".claude/settings.json", "work", ".claude/settings.json@@work"},
		{".vimrc", "personal", ".vimrc@@personal"},
		{"tunnel@.service", "work", "tunnel@.service@@work"},
	}
	for _, tt := range tests {
		t.Run(tt.base+"|"+tt.profile, func(t *testing.T) {
			if got := BuildName(tt.base, tt.profile); got != tt.want {
				t.Errorf("BuildName(%q, %q) = %q, want %q", tt.base, tt.profile, got, tt.want)
			}
		})
	}
}

// BuildName は ParseName の逆であること。
func TestBuildName_RoundTripsWithParseName(t *testing.T) {
	name := BuildName(".claude/settings.json", "work")
	base, sel, err := ParseName(name)
	if err != nil {
		t.Fatalf("ParseName(%q): %v", name, err)
	}
	if base != ".claude/settings.json" {
		t.Errorf("base = %q, want %q", base, ".claude/settings.json")
	}
	if sel == nil || len(sel.Profiles) != 1 || sel.Profiles[0] != "work" {
		t.Errorf("sel = %+v, want Profiles=[work]", sel)
	}
}
```

- [ ] **Step 2: 実行して失敗することを確認する**

Run: `go test ./internal/selector/... -run TestBuildName -v`
Expected: FAIL（`undefined: BuildName`）

- [ ] **Step 3: 最小実装を書く**

`internal/selector/selector.go` の `ParseName` の直後（56行目の後）に追加:

```go
// BuildName は base 名と profile 名から "@@" suffix 付きファイル名を組み立てる
// （ParseName の逆）。apply の Occupied で HOME 側の実体を profile 専用
// source として取り込む際に使う（spec §12.4.1、ADR 0015）。
//
// 構文検証は行わない。呼び出し側（ui）は既に .homux.toml で有効と分かって
// いる profile 名、またはユーザーが対話で入力した文字列をそのまま渡す。
func BuildName(base, profile string) string {
	return base + Delimiter + profile
}
```

- [ ] **Step 4: 実行して成功することを確認する**

Run: `go test ./internal/selector/... -v`
Expected: PASS（既存テストも含めすべて通る）

- [ ] **Step 5: commit**

```bash
git add internal/selector/selector.go internal/selector/selector_test.go
git commit -m "$(cat <<'EOF'
feat(selector): "@@" suffix を組み立てる BuildName を追加する

apply の Occupied で HOME 側の実体を profile 専用 source として
取り込む機能（ADR 0015）のために、ParseName の逆変換が要る。
EOF
)"
```

---

### Task 2: `exec.Decision` / `ConflictResolution` の導入と `adopt` の実装

**Files:**
- Modify: `internal/exec/exec.go`
- Modify: `internal/exec/exec_test.go`（既存の `confirm` 関数シグネチャをすべて更新）
- Create: `internal/exec/adopt.go`
- Create: `internal/exec/adopt_test.go`

**Interfaces:**
- Consumes: `plan.Action`（変更なし）、`inspect.CurrentKind`（`inspect.CurrentSymlink` を使う）
- Produces:
  - `type ConflictResolution int` と定数 `ResolutionSkip`, `ResolutionKeepRepo`, `ResolutionAdopt`
  - `type Decision struct { Resolution ConflictResolution; AdoptPath string }`
  - `type Confirm func(plan.Action) (Decision, error)`（旧 `func(plan.Action) (bool, error)` を置き換える）
  - `Apply(actions []plan.Action, confirm Confirm) Result`（シグネチャ変更なし、`confirm` の型だけ変わる）

この 1 タスクで型変更と `adopt` の実装をまとめて行う。型だけ変えて `ResolutionAdopt` を未実装のまま残すと、次のタスクまでコンパイルが通ってもビルドされたバイナリが不完全になる（プレースホルダ禁止）。

- [ ] **Step 1: 既存の `exec_test.go` を新しい `Confirm` シグネチャに書き換える**

`internal/exec/exec_test.go` 内の全ての `func(a plan.Action) (bool, error) { return X, nil }` を `func(a plan.Action) (Decision, error) { return Decision{Resolution: Y}, nil }` に置き換える（`X=true` → `Y=ResolutionKeepRepo`、`X=false` → `Y=ResolutionSkip`）。

対象は以下の4箇所:

`TestApplySkipsDeclinedActionAndContinues`:
```go
	res := Apply(actions, func(a plan.Action) (Decision, error) {
		return Decision{Resolution: ResolutionSkip}, nil
	})
```

`TestApplyDoesNotAskForActionsThatNeedNoConfirmation`:
```go
	res := Apply([]plan.Action{
		{Kind: plan.CreateSymlink, Target: target, LinkTo: source},
	}, func(a plan.Action) (Decision, error) {
		asked++
		return Decision{Resolution: ResolutionSkip}, nil
	})
```

`TestApplyStopsWhenConfirmFails`:
```go
	res := Apply(actions, func(a plan.Action) (Decision, error) {
		return Decision{}, errors.New("no tty")
	})
```

`TestApplyReplaceTargetLeavesBackup` ほか `confirm` に `nil` を渡している箇所（`TestApplyCreateSymlink`, `TestApplyRelink`, `TestApplyRelinkRefusesNonSymlink`, `TestApplyRemoveStaleSymlink`, `TestApplyRemoveStaleSymlinkRefusesRegularFile`, `TestApplyReplaceTargetLeavesBackup`, `TestApplyReplaceTargetRefusesExistingBackup`, `TestApplyReplaceTargetRefusesEmptyBackup`, `TestApplyReportsPartialApplication`, `TestApplyWithoutConfirmFuncAppliesEverything`）は `nil` のままでよい（変更不要）。

- [ ] **Step 2: 実行して失敗することを確認する（コンパイルエラー）**

Run: `go build ./internal/exec/...`
Expected: FAIL（`Decision`/`ResolutionSkip`/`ResolutionKeepRepo` が undefined）

- [ ] **Step 3: `exec.go` に型を導入し、`ask`/`run`/`Apply` を書き換える**

`internal/exec/exec.go` の `Confirm` 型定義を置き換える:

```go
// ConflictResolution は Occupied（ReplaceTarget）の解決策である
// （spec §12.4.1、ADR 0015）。Relink・RemoveStaleSymlink は
// ResolutionKeepRepo/ResolutionSkip の 2 値だけを使う。
type ConflictResolution int

const (
	// ResolutionSkip は何もしない（既定）。INV-12: 永続化しない。
	ResolutionSkip ConflictResolution = iota
	// ResolutionKeepRepo は repo 優先。ReplaceTarget では退避してから
	// symlink を張る（従来の唯一の動作）。Relink/RemoveStaleSymlink では
	// そのまま実行する。
	ResolutionKeepRepo
	// ResolutionAdopt は HOME 優先（共通 / profile 専用のどちらも）。
	// AdoptPath への move + symlink を行う。ReplaceTarget にのみ現れる。
	ResolutionAdopt
)

// Decision は Confirm の応答である。
type Decision struct {
	// Resolution は選ばれた解決策。
	Resolution ConflictResolution
	// AdoptPath は ResolutionAdopt のときの取り込み先 repo 絶対パス。
	// exec は Home/Repo のパスを知らないため、組み立ては呼び出し側
	// （ui）の責務である（ADR 0015）。他の Resolution では常に空。
	AdoptPath string
}

// Confirm は Action をどう解決するかを問う。nil なら確認なしで
// ResolutionKeepRepo として全件実行する。
type Confirm func(plan.Action) (Decision, error)
```

`ask` を書き換える:

```go
// ask は Action の解決策を問う。確認を要さない Action（Missing のみ。
// spec §12.4）と confirm が nil のときは問わず ResolutionKeepRepo を返す。
func ask(confirm Confirm, a plan.Action) (Decision, error) {
	if !a.Confirm || confirm == nil {
		return Decision{Resolution: ResolutionKeepRepo}, nil
	}
	return confirm(a)
}
```

`run` を書き換える（引数に `Decision` を追加）:

```go
// run は Action 1 件を実行する。
func run(a plan.Action, d Decision) error {
	switch a.Kind {
	case plan.CreateSymlink:
		return createSymlink(a)
	case plan.Relink:
		if err := removeSymlink(a.Target); err != nil {
			return err
		}
		return createSymlink(a)
	case plan.ReplaceTarget:
		if d.Resolution == ResolutionAdopt {
			return adopt(a, d.AdoptPath)
		}
		if err := backup(a); err != nil {
			return err
		}
		return createSymlink(a)
	case plan.RemoveStaleSymlink:
		return removeSymlink(a.Target)
	default:
		return fmt.Errorf("exec: unsupported action kind %v", a.Kind)
	}
}
```

`Apply` のループを書き換える（`ok bool` → `d Decision`、判定を `d.Resolution == ResolutionSkip` に、`run(a)` → `run(a, d)`）:

```go
func Apply(actions []plan.Action, confirm Confirm) Result {
	var res Result
	for i, a := range actions {
		d, err := ask(confirm, a)
		if err != nil {
			res.Failed = a
			res.Pending = actions[i+1:]
			res.Err = err
			return res
		}
		if d.Resolution == ResolutionSkip {
			res.Skipped = append(res.Skipped, a)
			continue
		}
		if err := run(a, d); err != nil {
			res.Failed = a
			res.Pending = actions[i+1:]
			res.Err = err
			return res
		}
		res.Applied = append(res.Applied, a)
	}
	return res
}
```

- [ ] **Step 4: 実行して型エラーが `adopt` の未定義だけになることを確認する**

Run: `go build ./internal/exec/...`
Expected: FAIL（`undefined: adopt` のみ）

- [ ] **Step 5: `adopt.go` を新規作成する**

`internal/exec/adopt.go`:

```go
// adopt.go は Occupied の HOME 優先の解決策（spec §12.4.1、ADR 0015）を
// 実行する。add.go の「repo へ move → 元の位置に symlink」と同じ操作だが、
// repoPath に既存の source があっても退避せず上書きする点が異なる
// （元の内容の保全は git 履歴に委ねる。INV-17）。
package exec

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/plan"
)

// adopt は a.Target（HOME 側の実体）を repoPath へ move し、symlink を
// 張り直す。
//
// a.Current が symlink（repo 外を指す）のときはエラーを返す。リンク先の
// 実体をどこまで安全に move してよいか判断できないためで、add（spec
// §12.6）が同じ状況をエラーにしているのと同じ理由である。ui が h/p を
// symlink 対象には出さない設計だが、ここでも不整合として弾く。
func adopt(a plan.Action, repoPath string) error {
	if a.Current == inspect.CurrentSymlink {
		return fmt.Errorf("exec: %s is a symlink; adopt is not supported", a.Target)
	}
	if err := os.RemoveAll(repoPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(repoPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(a.Target, repoPath); err != nil {
		return err
	}
	return os.Symlink(repoPath, a.Target)
}
```

- [ ] **Step 6: 実行して既存テストがすべて通ることを確認する**

Run: `go test ./internal/exec/... -v`
Expected: PASS（Step 1 で書き換えたテストを含め全件通る）

- [ ] **Step 7: `adopt` 自体の失敗するテストを書く**

`internal/exec/adopt_test.go` を新規作成:

```go
package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/plan"
)

// ADR 0015: HOME 優先（共通）。HOME の実体が repo へ move され、symlink が
// 張られる。
func TestApplyReplaceTargetAdoptsHomeFile(t *testing.T) {
	dir := evalTempDir(t)
	source := filepath.Join(dir, "repo", ".claude", "settings.json@@work")
	target := filepath.Join(dir, "home", ".claude", "settings.json")
	writeFile(t, source, "from repo")
	writeFile(t, target, "handwritten")

	res := Apply([]plan.Action{
		{Kind: plan.ReplaceTarget, Target: target, LinkTo: source, Current: inspect.CurrentFile, Confirm: true},
	}, func(a plan.Action) (Decision, error) {
		return Decision{Resolution: ResolutionAdopt, AdoptPath: source}, nil
	})

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if got := readFile(t, source); got != "handwritten" {
		t.Errorf("repo content = %q, want %q", got, "handwritten")
	}
	if got := readLink(t, target); got != source {
		t.Errorf("link = %s, want %s", got, source)
	}
}

// ADR 0015: 取り込み先に同名の source が既に存在しても、退避せず上書きする。
func TestApplyReplaceTargetAdoptOverwritesExistingSource(t *testing.T) {
	dir := evalTempDir(t)
	repoPath := filepath.Join(dir, "repo", ".vimrc@@work")
	target := filepath.Join(dir, "home", ".vimrc")
	writeFile(t, repoPath, "old work vimrc")
	writeFile(t, target, "new handwritten vimrc")

	res := Apply([]plan.Action{
		{Kind: plan.ReplaceTarget, Target: target, LinkTo: repoPath, Current: inspect.CurrentFile, Confirm: true},
	}, func(a plan.Action) (Decision, error) {
		return Decision{Resolution: ResolutionAdopt, AdoptPath: repoPath}, nil
	})

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if got := readFile(t, repoPath); got != "new handwritten vimrc" {
		t.Errorf("repo content = %q, want overwritten", got)
	}
	if _, err := os.Lstat(target + ".homux-bak.20260905-153000"); !os.IsNotExist(err) {
		t.Errorf("a backup was created for an adopt, want none")
	}
}

// ADR 0015: ディレクトリは中身ごと move する。
func TestApplyReplaceTargetAdoptsHomeDirectory(t *testing.T) {
	dir := evalTempDir(t)
	repoPath := filepath.Join(dir, "repo", ".config", "foo@@work")
	target := filepath.Join(dir, "home", ".config", "foo")
	writeFile(t, filepath.Join(target, "config"), "handwritten")
	writeFile(t, filepath.Join(target, "nested", "extra"), "nested file")

	res := Apply([]plan.Action{
		{Kind: plan.ReplaceTarget, Target: target, LinkTo: repoPath, Current: inspect.CurrentDir, Confirm: true},
	}, func(a plan.Action) (Decision, error) {
		return Decision{Resolution: ResolutionAdopt, AdoptPath: repoPath}, nil
	})

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if got := readFile(t, filepath.Join(repoPath, "config")); got != "handwritten" {
		t.Errorf("repo config content = %q, want %q", got, "handwritten")
	}
	if got := readFile(t, filepath.Join(repoPath, "nested", "extra")); got != "nested file" {
		t.Errorf("repo nested content = %q, want %q", got, "nested file")
	}
	if got := readLink(t, target); got != repoPath {
		t.Errorf("link = %s, want %s", got, repoPath)
	}
}

// ADR 0015: 対象が repo 外を指す symlink のときは adopt できない
// （add の spec §12.6 と同じ安全基準）。
func TestApplyReplaceTargetAdoptRefusesSymlinkTarget(t *testing.T) {
	dir := evalTempDir(t)
	repoPath := filepath.Join(dir, "repo", ".vimrc@@work")
	target := filepath.Join(dir, "home", ".vimrc")
	elsewhere := filepath.Join(dir, "elsewhere", ".vimrc")
	writeFile(t, elsewhere, "elsewhere content")
	symlink(t, elsewhere, target)

	res := Apply([]plan.Action{
		{Kind: plan.ReplaceTarget, Target: target, LinkTo: repoPath, Current: inspect.CurrentSymlink, From: elsewhere, Confirm: true},
	}, func(a plan.Action) (Decision, error) {
		return Decision{Resolution: ResolutionAdopt, AdoptPath: repoPath}, nil
	})

	if res.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if got := readLink(t, target); got != elsewhere {
		t.Errorf("target link = %s, want unchanged %s", got, elsewhere)
	}
	if _, err := os.Lstat(repoPath); !os.IsNotExist(err) {
		t.Errorf("repoPath was created despite the refusal: %v", err)
	}
}
```

- [ ] **Step 8: 実行してすべて通ることを確認する**

Run: `go test ./internal/exec/... -v`
Expected: PASS

- [ ] **Step 9: `go vet` を実行する**

Run: `go vet ./internal/exec/...`
Expected: 出力なし（エラーなし）

- [ ] **Step 10: commit**

```bash
git add internal/exec/exec.go internal/exec/exec_test.go internal/exec/adopt.go internal/exec/adopt_test.go
git commit -m "$(cat <<'EOF'
feat(exec): Occupied の解決策に HOME 優先の取り込みを追加する

Confirm の戻り値を bool から Decision{ConflictResolution, AdoptPath}
に拡張し、ResolutionAdopt のとき add と同じ move+symlink を行う
adopt を実装する（spec §12.4.1、ADR 0015）。exec は Home/Repo の
パスを知らないため、取り込み先の絶対パスは呼び出し側が組み立てて
AdoptPath として渡す。
EOF
)"
```

---

### Task 3: `ui.Prompter` に `repo`/`profile` を追加し、`ConfirmAction` を `Decision` 返却にする

このタスクでは Occupied 特有の r/h/p/n はまだ実装しない。まず `Relink`/`RemoveStaleSymlink` を含む全体を新しい戻り値型 `exec.Decision` に対応させ、Occupied は暫定的に従来通り `[y/N]` のまま `y`→`ResolutionKeepRepo`／`n`→`ResolutionSkip` を返す状態にする（次の Task 4 で r/h/p/n に置き換える）。

**Files:**
- Modify: `internal/ui/prompt.go`
- Modify: `internal/ui/prompt_test.go`
- Modify: `internal/ui/testdata` 相当のヘルパー（`testHome` 定義箇所。既存のものをそのまま使う）

**Interfaces:**
- Consumes: `exec.Decision`, `exec.ResolutionKeepRepo`, `exec.ResolutionSkip`（Task 2 で定義済み）
- Produces: `func NewPrompter(in io.Reader, out io.Writer, home, repo, profile string) *Prompter`（シグネチャ変更）、`func (p *Prompter) ConfirmAction(a plan.Action) (exec.Decision, error)`（戻り値変更）

- [ ] **Step 1: 既存テストを新シグネチャ・新戻り値型に書き換える**

`internal/ui/prompt_test.go` の `NewPrompter(...)` 呼び出しをすべて `NewPrompter(reader, &out, testHome, testRepo, "")` に変える（`testRepo` は新規定数、後述）。`ConfirmAction` の戻り値を受ける箇所を `ok` (bool) から `d exec.Decision` に変え、比較を `d.Resolution == exec.ResolutionKeepRepo` / `d.Resolution == exec.ResolutionSkip` にする。

`testRepo` は新規定義しない。`internal/ui/apply_test.go` に既に `const testRepo = testHome + "/dotfiles"`（`testHome = "/home/u"` なので `"/home/u/dotfiles"`）が定義済みで、同一パッケージ（`ui`）内なので `prompt_test.go` からそのまま参照できる。`replaceAction.LinkTo`（`testHome + "/dotfiles/.claude/settings.json@@work"`）とも整合する値である。

`TestPrompter_ReplaceTargetShowsBackupPath` を例に書き換える:

```go
func TestPrompter_ReplaceTargetShowsBackupPath(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("y\n"), &out, testHome, testRepo, "")

	d, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if d.Resolution != exec.ResolutionKeepRepo {
		t.Errorf("Resolution = %v, want ResolutionKeepRepo", d.Resolution)
	}

	want := "Existing file detected:\n" +
		"\n" +
		"  target:\n" +
		"    ~/.claude/settings.json\n" +
		"\n" +
		"  desired:\n" +
		"    ~/dotfiles/.claude/settings.json@@work\n" +
		"\n" +
		"  backup:\n" +
		"    ~/.claude/settings.json.homux-bak.20260905-153000\n" +
		"\n" +
		"Replace it? [y/N]: "
	if got := out.String(); got != want {
		t.Errorf("prompt:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
```

このタスクの時点では Occupied のプロンプト文言（`Replace it? [y/N]:` など）はまだ何も変わらない（Task 4 で r/h/p/n に変わる）。したがって残りのテストの変更点は機械的に次の2箇所だけである。

1. `NewPrompter(strings.NewReader(...), &out, testHome)` → `NewPrompter(strings.NewReader(...), &out, testHome, testRepo, "")`（引数を2つ追加するだけ）
2. `ConfirmAction` の戻り値を受ける変数名と比較を、`bool` の `ok`/`got` から `exec.Decision` の `.Resolution` 比較に変える

この2点を次のテストにそのまま適用する: `TestPrompter_RelinkPrompt`（`if _, err := p.ConfirmAction(...)` は戻り値を捨てているだけなので 1. の引数変更のみでよい）、`TestPrompter_RemoveStaleSymlinkPrompt`（同様に引数変更のみ）、`TestPrompter_ErrorsOnEOF`（同様に引数変更のみ。戻り値は見ていない）、`TestPrompter_ReplaceTargetHeadlineByCurrentKind`（同様に引数変更のみ）、`TestPrompter_ReplaceSymlinkShowsCurrentLink`（同様に引数変更のみ）。`TestPrompter_Answers` は下記の通り書き換える（1. と 2. の両方が要る）。`TestPrompter_RepromptsOnUnrecognizedAnswer` も同様に両方が要り、具体的な書き換えは Step 1 の最後に示す。

`import` に `"github.com/bellwood4486/homux/internal/exec"` を追加する。

`TestPrompter_Answers` の書き換え例:

```go
func TestPrompter_Answers(t *testing.T) {
	tests := []struct {
		input string
		want  exec.ConflictResolution
	}{
		{"y\n", exec.ResolutionKeepRepo},
		{"Y\n", exec.ResolutionKeepRepo},
		{"yes\n", exec.ResolutionKeepRepo},
		{" yes \n", exec.ResolutionKeepRepo},
		{"n\n", exec.ResolutionSkip},
		{"no\n", exec.ResolutionSkip},
		{"NO\n", exec.ResolutionSkip},
		{"\n", exec.ResolutionSkip},
		{"  \n", exec.ResolutionSkip},
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.input)+"|", func(t *testing.T) {
			var out bytes.Buffer
			p := NewPrompter(strings.NewReader(tt.input), &out, testHome, testRepo, "")

			got, err := p.ConfirmAction(replaceAction)
			if err != nil {
				t.Fatalf("ConfirmAction: %v", err)
			}
			if got.Resolution != tt.want {
				t.Errorf("ConfirmAction(%q).Resolution = %v, want %v", tt.input, got.Resolution, tt.want)
			}
		})
	}
}
```

Note: `replaceAction` はデフォルトで `Current: inspect.CurrentFile` かつテストは `profile` を `""` で呼ぶため、この時点ではまだ Occupied の r/h/p/n 選択肢は実装されておらず、既存の `[y/N]` 経路をそのまま通る（Task 4 で置き換わる）。`TestPrompter_Answers` は Task 4 の後に選択肢が増えるため、Task 4 で `y`/`n` 以外の入力ケースを追加する。

`TestPrompter_RepromptsOnUnrecognizedAnswer` の書き換え（引数変更のみ、文言は不変）:

```go
func TestPrompter_RepromptsOnUnrecognizedAnswer(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("maybe\ny\n"), &out, testHome, testRepo, "")

	got, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if got.Resolution != exec.ResolutionKeepRepo {
		t.Error("ConfirmAction returned non-KeepRepo, want ResolutionKeepRepo after reprompt")
	}

	s := out.String()
	if !strings.Contains(s, `Please answer "y" or "n".`) {
		t.Errorf("output has no reprompt notice:\n%s", s)
	}
	if n := strings.Count(s, "Replace it? [y/N]: "); n != 2 {
		t.Errorf("question asked %d times, want 2:\n%s", n, s)
	}
	// 質問の繰り返しに詳細ブロックは付け直さない。
	if n := strings.Count(s, "Existing file detected:"); n != 1 {
		t.Errorf("detail block written %d times, want 1:\n%s", n, s)
	}
}
```

- [ ] **Step 2: 実行して失敗することを確認する**

Run: `go build ./internal/ui/...`
Expected: FAIL（`NewPrompter` の引数不足、`ConfirmAction` の戻り値の型不一致など）

- [ ] **Step 3: `prompt.go` を書き換える**

`internal/ui/prompt.go` の import に `"github.com/bellwood4486/homux/internal/exec"` を追加。

`Prompter` 構造体と `NewPrompter` を書き換える:

```go
// Prompter は 1 回の apply の対話を担う。TTY 判定は呼び出し側の責務であり、
// ここは与えられた Reader / Writer だけを見る（テストは文字列を流し込む）。
type Prompter struct {
	in      *bufio.Reader
	out     io.Writer
	home    string
	repo    string
	profile string // active profile。空文字列は「profile なし」（spec §5.3）。
}

// NewPrompter は in から答えを読み、out へ問いを書く Prompter を返す。
// home はパスを "~/" 表記にするために使う。repo は Occupied の HOME 優先
// （profile 専用）で取り込み先の絶対パスを組み立てるために使う
// （spec §12.4.1、ADR 0015）。profile はそのとき有効なアクティブ profile
// で、取り込み先 profile 名の既定値になる。
func NewPrompter(in io.Reader, out io.Writer, home, repo, profile string) *Prompter {
	return &Prompter{in: bufio.NewReader(in), out: out, home: home, repo: repo, profile: profile}
}
```

`ConfirmAction` を書き換える（Occupied 以外は今まで通り `Confirm` を呼び、結果を `Decision` に詰め替えるだけ）:

```go
// ConfirmAction は Action の解決策を問う。exec.Confirm として渡す。
//
// Occupied（ReplaceTarget）は複数の解決策から選ぶ（confirmOccupied、
// Task 4 で実装）。それ以外は [y/N] の 2 値で、既定は No である。
// y を選ばなかったことは永続化されず、conflict が残る限り次回の apply
// でも再び問う（INV-12）。
func (p *Prompter) ConfirmAction(a plan.Action) (exec.Decision, error) {
	p.writeDetails(a)
	ok, err := p.Confirm(questionFor(a.Kind))
	if err != nil {
		return exec.Decision{}, err
	}
	if ok {
		return exec.Decision{Resolution: exec.ResolutionKeepRepo}, nil
	}
	return exec.Decision{Resolution: exec.ResolutionSkip}, nil
}
```

- [ ] **Step 4: 実行して成功することを確認する**

Run: `go test ./internal/ui/... -v`
Expected: PASS（`prompt_test.go` のテストがすべて通る。他の `ui` テストは無関係なので影響なし）

- [ ] **Step 5: commit**

```bash
git add internal/ui/prompt.go internal/ui/prompt_test.go internal/ui/apply_test.go
git commit -m "$(cat <<'EOF'
refactor(ui): ConfirmAction を exec.Decision 返却に揃える

Task 2 で拡張した exec.Confirm の型に Prompter を追従させる。
Occupied 専用の r/h/p/n 選択肢はまだ実装せず、既存の [y/N] のまま
Decision に詰め替えるだけに留める。
EOF
)"
```

---

### Task 4: `ui.Prompter` に Occupied の r/h/p/n 選択肢を実装する

**Files:**
- Modify: `internal/ui/prompt.go`
- Modify: `internal/ui/prompt_test.go`

**Interfaces:**
- Consumes: `selector.BuildName`（Task 1）、`exec.ResolutionAdopt`（Task 2）
- Produces: `func (p *Prompter) confirmOccupied(a plan.Action) (exec.Decision, error)`（`ConfirmAction` 内部から呼ぶ非公開メソッド）

- [ ] **Step 1: 失敗するテストを書く（symlink でも profile 有りでもない基本ケース: r/h/n の3択）**

`internal/ui/prompt_test.go` に追加。まず `replaceAction`（`Current: inspect.CurrentFile`）に対して `profile` ありの Prompter を使うケースから始める:

```go
// spec §12.4.1: Occupied は r/h/p/n の4択（アクティブ profile があるとき）。
func TestPrompter_ConfirmOccupied_KeepRepo(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("r\n"), &out, testHome, testRepo, "work")

	d, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if d.Resolution != exec.ResolutionKeepRepo {
		t.Errorf("Resolution = %v, want ResolutionKeepRepo", d.Resolution)
	}

	want := "How do you want to resolve this?\n" +
		"  [r] keep repo version, back up HOME file\n" +
		"  [h] adopt HOME file into repo (common source)\n" +
		"  [p] adopt HOME file into repo (profile-specific source)\n" +
		"  [n] skip (default)\n" +
		"\n" +
		"Choice [r/h/p/N]: "
	if got := out.String(); !strings.HasSuffix(got, want) {
		t.Errorf("prompt:\ngot:\n%s\nwant suffix:\n%s", got, want)
	}
}

func TestPrompter_ConfirmOccupied_AdoptCommon(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("h\n"), &out, testHome, testRepo, "work")

	d, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if d.Resolution != exec.ResolutionAdopt {
		t.Errorf("Resolution = %v, want ResolutionAdopt", d.Resolution)
	}
	if d.AdoptPath != replaceAction.LinkTo {
		t.Errorf("AdoptPath = %q, want %q (Selected source unchanged)", d.AdoptPath, replaceAction.LinkTo)
	}
}

func TestPrompter_ConfirmOccupied_DefaultIsSkip(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("\n"), &out, testHome, testRepo, "work")

	d, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if d.Resolution != exec.ResolutionSkip {
		t.Errorf("Resolution = %v, want ResolutionSkip", d.Resolution)
	}
}

// spec §12.4.1: 対象が repo 外を指す symlink のときは h/p を出さない。
func TestPrompter_ConfirmOccupied_HidesAdoptForSymlinkTarget(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("n\n"), &out, testHome, testRepo, "work")

	a := replaceAction
	a.Current = inspect.CurrentSymlink
	a.From = "/opt/elsewhere/.vimrc"
	if _, err := p.ConfirmAction(a); err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "[h]") || strings.Contains(got, "[p]") {
		t.Errorf("prompt should not offer h/p for a symlink target:\n%s", got)
	}
	if !strings.Contains(got, "Choice [r/N]: ") {
		t.Errorf("prompt should show a 2-way choice:\n%s", got)
	}
}

// spec §12.4.1: アクティブ profile が無いときは p を出さない。
func TestPrompter_ConfirmOccupied_HidesProfileWhenNoActiveProfile(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("n\n"), &out, testHome, testRepo, "")

	if _, err := p.ConfirmAction(replaceAction); err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "[p]") {
		t.Errorf("prompt should not offer p without an active profile:\n%s", got)
	}
	if !strings.Contains(got, "Choice [r/h/N]: ") {
		t.Errorf("prompt should show a 3-way choice:\n%s", got)
	}
}

// spec §12.4.1: p を選ぶと profile 名の入力を求め、既定値はアクティブ profile。
func TestPrompter_ConfirmOccupied_AdoptProfileDefaultsToActiveProfile(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("p\n\n"), &out, testHome, testRepo, "work")

	d, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if d.Resolution != exec.ResolutionAdopt {
		t.Fatalf("Resolution = %v, want ResolutionAdopt", d.Resolution)
	}
	want := testRepo + "/.claude/settings.json@@work"
	if d.AdoptPath != want {
		t.Errorf("AdoptPath = %q, want %q", d.AdoptPath, want)
	}
	if !strings.Contains(out.String(), "profile name [work]: ") {
		t.Errorf("prompt should ask for a profile name with the active profile as default:\n%s", out.String())
	}
}

// profile 名は対話中に変更できる。
func TestPrompter_ConfirmOccupied_AdoptProfileCanOverrideName(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("p\npersonal\n"), &out, testHome, testRepo, "work")

	d, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	want := testRepo + "/.claude/settings.json@@personal"
	if d.AdoptPath != want {
		t.Errorf("AdoptPath = %q, want %q", d.AdoptPath, want)
	}
}
```

Note: `replaceAction.Target` は `testHome + "/.claude/settings.json"`。`testRepo` からの相対パス組み立ては `filepath.Rel(p.home, a.Target)` で `".claude/settings.json"` を得る想定。

- [ ] **Step 2: 実行して失敗することを確認する**

Run: `go test ./internal/ui/... -run TestPrompter_ConfirmOccupied -v`
Expected: FAIL（出力が旧来の `Replace it? [y/N]:` のまま）

- [ ] **Step 3: `confirmOccupied` を実装する**

`internal/ui/prompt.go` は現時点で `"bufio"`, `"errors"`, `"fmt"`, `"io"`, `"golang.org/x/term"`, `"github.com/bellwood4486/homux/internal/inspect"`, `"github.com/bellwood4486/homux/internal/plan"` を import しており、`"strings"` と `"path/filepath"` は未 import である。import ブロックに次の3つを追加する: `"path/filepath"`, `"strings"`, `"github.com/bellwood4486/homux/internal/selector"`。

`ConfirmAction` を書き換えて Occupied を分岐させる:

```go
func (p *Prompter) ConfirmAction(a plan.Action) (exec.Decision, error) {
	p.writeDetails(a)
	if a.Kind == plan.ReplaceTarget {
		return p.confirmOccupied(a)
	}
	ok, err := p.Confirm(questionFor(a.Kind))
	if err != nil {
		return exec.Decision{}, err
	}
	if ok {
		return exec.Decision{Resolution: exec.ResolutionKeepRepo}, nil
	}
	return exec.Decision{Resolution: exec.ResolutionSkip}, nil
}

// confirmOccupied は Occupied の解決策を問う（spec §12.4.1、ADR 0015）。
//
// h/p は対象が repo 外を指す symlink のときは出さない（add と同じ安全
//基準）。p はアクティブ profile が無いときは出さない（取り込み先の
// profile 名が決まらないため）。
func (p *Prompter) confirmOccupied(a plan.Action) (exec.Decision, error) {
	allowAdopt := a.Current != inspect.CurrentSymlink
	allowProfile := allowAdopt && p.profile != ""

	fmt.Fprintln(p.out, "How do you want to resolve this?")
	fmt.Fprintln(p.out, "  [r] keep repo version, back up HOME file")
	if allowAdopt {
		fmt.Fprintln(p.out, "  [h] adopt HOME file into repo (common source)")
	}
	if allowProfile {
		fmt.Fprintln(p.out, "  [p] adopt HOME file into repo (profile-specific source)")
	}
	fmt.Fprintln(p.out, "  [n] skip (default)")
	fmt.Fprintln(p.out)

	choices := "r/N"
	switch {
	case allowProfile:
		choices = "r/h/p/N"
	case allowAdopt:
		choices = "r/h/N"
	}

	for {
		fmt.Fprintf(p.out, "Choice [%s]: ", choices)
		line, err := p.readLine()
		if err != nil {
			return exec.Decision{}, err
		}
		switch strings.ToLower(line) {
		case "r":
			return exec.Decision{Resolution: exec.ResolutionKeepRepo}, nil
		case "h":
			if allowAdopt {
				return exec.Decision{Resolution: exec.ResolutionAdopt, AdoptPath: a.LinkTo}, nil
			}
		case "p":
			if allowProfile {
				return p.confirmAdoptProfile(a)
			}
		case "", "n":
			return exec.Decision{Resolution: exec.ResolutionSkip}, nil
		}
		fmt.Fprintf(p.out, "Please answer one of %s.\n", choices)
	}
}

// confirmAdoptProfile は profile 名を尋ね、target@@<profile> への取り込み
// を組み立てる。既定値はアクティブ profile。
func (p *Prompter) confirmAdoptProfile(a plan.Action) (exec.Decision, error) {
	name, err := p.AskLine("profile name", p.profile)
	if err != nil {
		return exec.Decision{}, err
	}
	rel, err := filepath.Rel(p.home, a.Target)
	if err != nil {
		return exec.Decision{}, err
	}
	repoRel := selector.BuildName(filepath.ToSlash(rel), name)
	return exec.Decision{
		Resolution: exec.ResolutionAdopt,
		AdoptPath:  filepath.Join(p.repo, filepath.FromSlash(repoRel)),
	}, nil
}
```

- [ ] **Step 4: 実行して成功することを確認する**

Run: `go test ./internal/ui/... -v`
Expected: PASS（Task 3 で書いた `TestPrompter_Answers` 等の既存テストも、Occupied は今回 4 択に変わったため、`replaceAction` を使う既存テストで `y`/`n` を送っていたものは Step 1 で `r`/`n` などに書き換わっている必要がある — 次の Step で確認する）

- [ ] **Step 5: Task 3 で書いた `TestPrompter_Answers` を Occupied 用と非 Occupied 用に分離する**

Task 3 の `TestPrompter_Answers` は `replaceAction`（`ReplaceTarget`）に `"y\n"`/`"n\n"` を投げていたが、Occupied は今や `r/h/p/n` の語彙になったため `y`/`yes` は認識されない。このテストを次のように分割する:

```go
// [y/N] の 2 値語彙を使うのは Relink/RemoveStaleSymlink だけである。
func TestPrompter_Answers(t *testing.T) {
	tests := []struct {
		input string
		want  exec.ConflictResolution
	}{
		{"y\n", exec.ResolutionKeepRepo},
		{"Y\n", exec.ResolutionKeepRepo},
		{"yes\n", exec.ResolutionKeepRepo},
		{" yes \n", exec.ResolutionKeepRepo},
		{"n\n", exec.ResolutionSkip},
		{"no\n", exec.ResolutionSkip},
		{"NO\n", exec.ResolutionSkip},
		{"\n", exec.ResolutionSkip},
		{"  \n", exec.ResolutionSkip},
	}
	relinkAction := plan.Action{
		Kind:    plan.Relink,
		Target:  testHome + "/.vimrc",
		LinkTo:  testHome + "/dotfiles/.vimrc@@work",
		From:    testHome + "/dotfiles/.vimrc",
		Confirm: true,
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.input)+"|", func(t *testing.T) {
			var out bytes.Buffer
			p := NewPrompter(strings.NewReader(tt.input), &out, testHome, testRepo, "work")

			got, err := p.ConfirmAction(relinkAction)
			if err != nil {
				t.Fatalf("ConfirmAction: %v", err)
			}
			if got.Resolution != tt.want {
				t.Errorf("ConfirmAction(%q).Resolution = %v, want %v", tt.input, got.Resolution, tt.want)
			}
		})
	}
}

// Occupied は r/h/p/n の語彙を使い、y/yes は認識されない。
func TestPrompter_ConfirmOccupied_Answers(t *testing.T) {
	tests := []struct {
		input string
		want  exec.ConflictResolution
	}{
		{"r\n", exec.ResolutionKeepRepo},
		{"R\n", exec.ResolutionKeepRepo},
		{"n\n", exec.ResolutionSkip},
		{"\n", exec.ResolutionSkip},
		{"  \n", exec.ResolutionSkip},
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.input)+"|", func(t *testing.T) {
			var out bytes.Buffer
			p := NewPrompter(strings.NewReader(tt.input), &out, testHome, testRepo, "work")

			got, err := p.ConfirmAction(replaceAction)
			if err != nil {
				t.Fatalf("ConfirmAction: %v", err)
			}
			if got.Resolution != tt.want {
				t.Errorf("ConfirmAction(%q).Resolution = %v, want %v", tt.input, got.Resolution, tt.want)
			}
		})
	}
}
```

Task 3 で書いた元の `TestPrompter_Answers`（`replaceAction` に `y`/`n` を送るもの）は削除し、上記2つに置き換える。

`TestPrompter_RepromptsOnUnrecognizedAnswer`（Task 3 で書いたもの）を、Occupied の新しい語彙に合わせて書き換える:

```go
func TestPrompter_RepromptsOnUnrecognizedAnswer(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("maybe\nr\n"), &out, testHome, testRepo, "work")

	got, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if got.Resolution != exec.ResolutionKeepRepo {
		t.Error("ConfirmAction returned non-KeepRepo, want ResolutionKeepRepo after reprompt")
	}

	s := out.String()
	if !strings.Contains(s, "Please answer one of r/h/p/N.") {
		t.Errorf("output has no reprompt notice:\n%s", s)
	}
	if n := strings.Count(s, "Choice [r/h/p/N]: "); n != 2 {
		t.Errorf("question asked %d times, want 2:\n%s", n, s)
	}
	// 質問の繰り返しに詳細ブロックは付け直さない。
	if n := strings.Count(s, "Existing file detected:"); n != 1 {
		t.Errorf("detail block written %d times, want 1:\n%s", n, s)
	}
}
```

- [ ] **Step 6: 実行してすべて通ることを確認する**

Run: `go test ./internal/ui/... -v`
Expected: PASS

- [ ] **Step 7: `go vet` を実行する**

Run: `go vet ./internal/ui/...`
Expected: 出力なし

- [ ] **Step 8: commit**

```bash
git add internal/ui/prompt.go internal/ui/prompt_test.go
git commit -m "$(cat <<'EOF'
feat(ui): Occupied の対話に r/h/p/n の4択を実装する

repo 優先(r)に加え、HOME 優先の共通反映(h)・profile 専用反映(p)を
選べるようにする。symlink 対象では h/p を、アクティブ profile が
無いときは p を選択肢から外す（spec §12.4.1、ADR 0015）。
EOF
)"
```

---

### Task 5: `cli/apply.go` を `exec.Decision` と `ui.NewPrompter` の新シグネチャに追従させる

**Files:**
- Modify: `internal/cli/apply.go`
- Modify: `internal/cli/apply_test.go`

**Interfaces:**
- Consumes: `ui.NewPrompter(in, out, home, repo, profile string) *ui.Prompter`（Task 3）、`exec.Confirm`（Task 2）
- Produces: `confirmFunc` のシグネチャは `ws *workspace` を受け取るように変わる（`home string` の代わり）

- [ ] **Step 1: `apply.go` を書き換える**

`internal/cli/apply.go` の `runApply` 内の呼び出しを変更:

```go
	confirm, err := confirmFunc(cmd, ws, p.Actions, opts, interactive)
```

`confirmFunc` のシグネチャと本体を変更:

```go
// confirmFunc は exec に渡す確認関数を決める。nil は「確認なしで全件
// ResolutionKeepRepo として実行」を意味する（exec.Apply の規約）。
//
// 非 TTY で確認が必要な場合は spec §11.4 に従いエラーで止める。確認が
// 1 件も要らない plan は対話 UI を起動しないので、非 TTY でもそのまま実行
// できる。これがないとパイプ越しの再実行が永久に収束しない。
func confirmFunc(cmd *cobra.Command, ws *workspace, actions []plan.Action, opts applyOptions, interactive bool) (exec.Confirm, error) {
	if opts.yes {
		return nil, nil
	}
	if interactive {
		return ui.NewPrompter(cmd.InOrStdin(), cmd.OutOrStdout(), ws.env.Home, ws.env.Repo, ws.profile).ConfirmAction, nil
	}
	if n := countConfirm(actions); n > 0 {
		return nil, fmt.Errorf(
			"confirmation is required for %d of these changes, but this is not an interactive terminal: re-run with --yes", n)
	}
	return nil, nil
}
```

- [ ] **Step 2: 実行して失敗することを確認する**

Run: `go build ./internal/cli/...`
Expected: FAIL（`confirmFunc` の引数不一致）

- [ ] **Step 3: ビルドが通ることを確認する**

Step 1 の変更を適用済みであれば、ここでは既にビルドが通っているはず。

Run: `go build ./internal/cli/...`
Expected: PASS

- [ ] **Step 4: 既存の統合テストを新しい語彙に合わせて修正する**

`internal/cli/apply_test.go` の `TestApplyCmd_InteractiveYesAppliesConfirmedActions` を書き換える（ReplaceTarget の確認は最初に来るので、1つ目の入力を `r` に変える。他は Relink・RemoveStaleSymlink なので `y` のまま）:

```go
func TestApplyCmd_InteractiveYesAppliesConfirmedActions(t *testing.T) {
	home, repo := applyFixture(t)

	stdout, err := runApplyInteractive(t, repo, "r\ny\ny\n", applyOptions{})
	if err != nil {
		t.Fatalf("runApply: %v", err)
	}

	if !strings.Contains(stdout, "Choice [r/h/p/N]: ") {
		t.Errorf("stdout has no occupied prompt:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Applied 4 changes.") {
		t.Errorf("stdout:\n%s", stdout)
	}
	assertSymlink(t, filepath.Join(home, ".claude/settings.json"), filepath.Join(repo, ".claude/settings.json@@work"))
	assertNotExist(t, filepath.Join(home, ".config/orphan"))
}
```

`TestApplyCmd_InteractiveNoIsNotPersisted` の入力を `"n\nn\nn\n"` のまま（Occupied の既定も `n`=skip で変わらない）にし、末尾のアサーションだけ修正する:

```go
	second, err := runApplyInteractive(t, repo, "n\nn\nn\n", applyOptions{})
	if err != nil {
		t.Fatalf("second runApply: %v", err)
	}
	if n := strings.Count(second, "[y/N]: "); n != 2 {
		t.Errorf("second run asked [y/N] %d times, want 2 (Relink + RemoveStaleSymlink):\n%s", n, second)
	}
	if !strings.Contains(second, "Choice [r/h/p/N]: ") {
		t.Errorf("second run should still ask the occupied choice:\n%s", second)
	}
```

applyFixture のアクティブ profile は `"work"` で `.homux.toml` に `profiles = ["work", "personal"]` があるため、Occupied のプロンプトは `r/h/p/N` の4択になる（`p` も出る）。

- [ ] **Step 5: 実行してすべて通ることを確認する**

Run: `go test ./internal/cli/... -v`
Expected: PASS

- [ ] **Step 6: `go vet` を実行する**

Run: `go vet ./internal/cli/...`
Expected: 出力なし

- [ ] **Step 7: commit**

```bash
git add internal/cli/apply.go internal/cli/apply_test.go
git commit -m "$(cat <<'EOF'
feat(cli): apply の対話に repo/profile を渡し Occupied の新語彙に追従する

ui.NewPrompter が repo パスとアクティブ profile を必要とするように
なったため、confirmFunc に workspace を渡す。既存の統合テストの
入力語彙を r/h/p/n に更新する。
EOF
)"
```

---

### Task 6: HOME 優先の取り込みを確認する統合テストを追加する

**Files:**
- Modify: `internal/cli/apply_test.go`

**Interfaces:**
- Consumes: `applyFixture`, `runApplyInteractive`（既存ヘルパー、Task 5 まで変更なし）

- [ ] **Step 1: 失敗するテストを書く（`h`: HOME優先・共通）**

`internal/cli/apply_test.go` に追加:

```go
// ADR 0015: HOME 優先（共通）。HOME の実体が Selected の source（この
// フィクスチャでは .claude/settings.json@@work）へ move され、repo の
// 内容は上書きされる。
func TestApplyCmd_InteractiveAdoptCommonMovesHomeFileIntoRepo(t *testing.T) {
	home, repo := applyFixture(t)

	stdout, err := runApplyInteractive(t, repo, "h\ny\ny\n", applyOptions{})
	if err != nil {
		t.Fatalf("runApply: %v", err)
	}
	if !strings.Contains(stdout, "Applied 4 changes.") {
		t.Errorf("stdout:\n%s", stdout)
	}

	repoSource := filepath.Join(repo, ".claude/settings.json@@work")
	assertSymlink(t, filepath.Join(home, ".claude/settings.json"), repoSource)
	got, err := os.ReadFile(repoSource)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", repoSource, err)
	}
	if string(got) != "unmanaged\n" {
		t.Errorf("repo source content = %q, want the adopted HOME content %q", got, "unmanaged\n")
	}
}

// ADR 0015: HOME 優先（profile 専用）。profile 名は空 Enter でアクティブ
// profile（work）を使う。
func TestApplyCmd_InteractiveAdoptProfileCreatesProfileSpecificSource(t *testing.T) {
	home, repo := applyFixture(t)

	stdout, err := runApplyInteractive(t, repo, "p\n\ny\ny\n", applyOptions{})
	if err != nil {
		t.Fatalf("runApply: %v", err)
	}
	if !strings.Contains(stdout, "Applied 4 changes.") {
		t.Errorf("stdout:\n%s", stdout)
	}

	repoSource := filepath.Join(repo, ".claude/settings.json@@work")
	assertSymlink(t, filepath.Join(home, ".claude/settings.json"), repoSource)
	got, err := os.ReadFile(repoSource)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", repoSource, err)
	}
	if string(got) != "unmanaged\n" {
		t.Errorf("repo source content = %q, want the adopted HOME content %q", got, "unmanaged\n")
	}
}

// ADR 0015: profile 名は対話中に変更できる。personal@@ という新しい
// source が作られる。
func TestApplyCmd_InteractiveAdoptProfileCanUseADifferentProfile(t *testing.T) {
	home, repo := applyFixture(t)

	stdout, err := runApplyInteractive(t, repo, "p\npersonal\ny\ny\n", applyOptions{})
	if err != nil {
		t.Fatalf("runApply: %v", err)
	}
	if !strings.Contains(stdout, "Applied 4 changes.") {
		t.Errorf("stdout:\n%s", stdout)
	}

	repoSource := filepath.Join(repo, ".claude/settings.json@@personal")
	assertSymlink(t, filepath.Join(home, ".claude/settings.json"), repoSource)
	// 元の work 専用 source は変更されずに残る。
	original, err := os.ReadFile(filepath.Join(repo, ".claude/settings.json@@work"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(original) != "{}\n" {
		t.Errorf("original @@work source was modified: %q", original)
	}
}
```

- [ ] **Step 2: 実行して失敗することを確認する**

Run: `go test ./internal/cli/... -run TestApplyCmd_InteractiveAdopt -v`
Expected: FAIL（Task 4/5 の実装前であれば挙動が異なる。この計画通り Task 5 まで完了していれば、実装は既に揃っているため PASS するはずである。もし FAIL する場合は Task 3〜5 のどこかで語彙・パス組み立てに齟齬がある。原因を調べてから次に進む）

- [ ] **Step 3: 必要なら実装を修正し、PASS するまで直す**

このタスクは新しい実装を追加するものではなく、Task 2〜5 の結線を統合テストで確認するものである。失敗した場合は `internal/exec/adopt.go` の `AdoptPath` の扱い、または `internal/ui/prompt.go` の `confirmAdoptProfile` のパス組み立てを見直す。

- [ ] **Step 4: 実行してすべて通ることを確認する**

Run: `go test ./internal/cli/... -v`
Expected: PASS

- [ ] **Step 5: commit**

```bash
git add internal/cli/apply_test.go
git commit -m "$(cat <<'EOF'
test(cli): apply の HOME 優先取り込み(h/p)の統合テストを追加する

共通 source への反映、profile 専用 source の新規作成、対話中の
profile 名変更をそれぞれ確認する（ADR 0015）。
EOF
)"
```

---

### Task 7: 仕上げ — 全体テストとドキュメント整合の最終確認

**Files:** なし（確認のみ）

- [ ] **Step 1: 全パッケージのテストを実行する**

Run: `just test`
Expected: PASS（全パッケージ）

- [ ] **Step 2: lint とフォーマットを確認する**

Run: `just check`
Expected: PASS（`fmt-check` → `vet` → `lint` → `test` の全段階）

Run 失敗時: `just fix` を実行してから再度 `just check` を実行する。

- [ ] **Step 3: `docs/spec.md` §12.4.1 のプロンプト例が実装の実際の出力と一致するか目視で確認する**

以下を手元で実行し、出力を spec の例と突き合わせる:

```bash
just build
mkdir -p /tmp/homux-manual-check/home/.claude /tmp/homux-manual-check/repo
echo '{}' > /tmp/homux-manual-check/repo/.claude/settings.json@@work
echo 'unmanaged' > /tmp/homux-manual-check/home/.claude/settings.json
echo 'profiles = ["work"]' > /tmp/homux-manual-check/repo/.homux.toml
HOME=/tmp/homux-manual-check/home XDG_CONFIG_HOME=/tmp/homux-manual-check/home/.config ./homux --repo /tmp/homux-manual-check/repo init <<< $'/tmp/homux-manual-check/repo\ny\n1\n'
HOME=/tmp/homux-manual-check/home XDG_CONFIG_HOME=/tmp/homux-manual-check/home/.config ./homux --repo /tmp/homux-manual-check/repo apply
rm -rf /tmp/homux-manual-check
```

(`init` の対話手順は既存の spec §12.1 を参照して適宜合わせる。目的は実際の端末出力が spec §12.4.1 のブロックと文言・改行位置まで一致することの目視確認であり、自動テストの代わりではない。)

- [ ] **Step 4: Linear issue BEL-26 を完了状態にする**

```bash
orca linear comment add BEL-26 --body-file - --json <<'EOF'
apply の Occupied に r(repo優先)/h(HOME優先・共通)/p(HOME優先・profile専用)/n(skip)の4択を実装した。
ADR 0015、spec §12.4.1、design.md §3.1 の設計通り。
EOF
orca linear status set BEL-26 --to "In Review" --json
```

(PR が既に作られている場合は、先に `orca linear attach --json` で PR リンクを添付してから上記を実行する。)
