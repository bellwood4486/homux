package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bellwood4486/homux/internal/ui"
)

// runCreateInteractive は端末であるかのように runProfileCreate を動かす。
// huh の選択画面だけを sel で差し替え、それ以外は本番と同じ経路を通る
// （ADR 0010 の帰結。選択画面は io.Writer 越しに検証できない）。
func runCreateInteractive(t *testing.T, repo, name, stdin string, sel forkSelector) (stdout string, err error) {
	t.Helper()
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	err = runProfileCreate(cmd, repo, name, sel, true)
	return out.String(), err
}

// selects は候補のうち want を選ぶ forkSelector を返す。候補そのものも記録する。
func selects(got *[]string, want ...string) forkSelector {
	return func(_ string, candidates []string) ([]string, error) {
		*got = candidates
		return want, nil
	}
}

func setupCreateRepo(t *testing.T, toml string) string {
	t.Helper()
	repo := evalTempDir(t)
	writeFile(t, filepath.Join(repo, ".homux.toml"), toml)
	return repo
}

// fork は copy である。common source が残ることが profile なしのマシンを
// 壊さない条件である（spec §17.2）。
func TestProfileCreate_ForksSelectedAndKeepsCommon(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = []\n")
	writeFile(t, filepath.Join(repo, ".zshrc"), "zsh\n")
	writeFile(t, filepath.Join(repo, ".gitconfig"), "git\n")

	var candidates []string
	stdout, err := runCreateInteractive(t, repo, "work", "y\n", selects(&candidates, ".gitconfig"))
	if err != nil {
		t.Fatalf("runProfileCreate: %v", err)
	}

	if got := readFileContent(t, filepath.Join(repo, ".gitconfig@@work")); got != "git\n" {
		t.Errorf(".gitconfig@@work = %q, want %q", got, "git\n")
	}
	if got := readFileContent(t, filepath.Join(repo, ".gitconfig")); got != "git\n" {
		t.Errorf(".gitconfig = %q, want it left in place", got)
	}
	if _, statErr := os.Lstat(filepath.Join(repo, ".zshrc@@work")); statErr == nil {
		t.Error(".zshrc@@work was created, but .zshrc was not selected")
	}
	if !strings.Contains(stdout, "Created profile \"work\".") {
		t.Errorf("stdout = %q, want it to report the new profile", stdout)
	}
}

// profiles への追加は fork より先に行う。途中で失敗しても resolve が
// unknown profile で止まらない側に倒すためである。
func TestProfileCreate_AddsProfileToRepoFile(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"personal\"]\n\nignore = [\"README.md\"]\n")
	writeFile(t, filepath.Join(repo, ".zshrc"), "zsh\n")

	var candidates []string
	if _, err := runCreateInteractive(t, repo, "work", "y\n", selects(&candidates)); err != nil {
		t.Fatalf("runProfileCreate: %v", err)
	}

	got := readFileContent(t, filepath.Join(repo, ".homux.toml"))
	want := "profiles = [\n  \"personal\",\n  \"work\",\n]\n\nignore = [\"README.md\"]\n"
	if got != want {
		t.Errorf(".homux.toml =\n%s\nwant\n%s", got, want)
	}
}

// 候補は common source を持つ target だけである。fork は common の複製なので、
// 複製元が無い target には適用しようがない。
func TestProfileCreate_CandidatesAreCommonSourcesOnly(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"personal\"]\n\nignore = [\"README.md\"]\n")
	writeFile(t, filepath.Join(repo, ".zshrc"), "zsh\n")
	writeFile(t, filepath.Join(repo, ".gitconfig@@personal"), "git\n")
	writeFile(t, filepath.Join(repo, "README.md"), "readme\n")

	var candidates []string
	stdout, err := runCreateInteractive(t, repo, "work", "y\n", selects(&candidates))
	if err != nil {
		t.Fatalf("runProfileCreate: %v", err)
	}

	if len(candidates) != 1 || candidates[0] != ".zshrc" {
		t.Errorf("candidates = %v, want [.zshrc]", candidates)
	}
	if !strings.Contains(stdout, "1 target has no common source") {
		t.Errorf("stdout = %q, want it to report the skipped target", stdout)
	}
}

// spec §11.2: profile create は repository だけを変更する。
func TestProfileCreate_DoesNotTouchHomeOrLocalConfig(t *testing.T) {
	home := evalTempDir(t)
	xdg := filepath.Join(home, ".config-profile-create")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	localPath := filepath.Join(xdg, "homux", "config.toml")
	writeFile(t, localPath, "profile = \"personal\"\n")

	repo := setupCreateRepo(t, "profiles = [\"personal\"]\n")
	writeFile(t, filepath.Join(repo, ".zshrc"), "zsh\n")
	writeFile(t, filepath.Join(home, ".zshrc"), "home zsh\n")

	before := snapshotTree(t, home)

	var candidates []string
	if _, err := runCreateInteractive(t, repo, "work", "y\n", selects(&candidates, ".zshrc")); err != nil {
		t.Fatalf("runProfileCreate: %v", err)
	}

	after := snapshotTree(t, home)
	if len(before) != len(after) {
		t.Fatalf("HOME entry count changed: %d -> %d", len(before), len(after))
	}
	if got := readFileContent(t, localPath); got != "profile = \"personal\"\n" {
		t.Errorf("local config = %q, want it untouched", got)
	}
}

// n と答えたときは 1 バイトも変更しない。
func TestProfileCreate_DeclinedChangesNothing(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = []\n")
	writeFile(t, filepath.Join(repo, ".gitconfig"), "git\n")

	var candidates []string
	stdout, err := runCreateInteractive(t, repo, "work", "n\n", selects(&candidates, ".gitconfig"))
	if err != nil {
		t.Fatalf("runProfileCreate: %v", err)
	}

	if _, statErr := os.Lstat(filepath.Join(repo, ".gitconfig@@work")); statErr == nil {
		t.Error(".gitconfig@@work was created despite answering no")
	}
	if got := readFileContent(t, filepath.Join(repo, ".homux.toml")); got != "profiles = []\n" {
		t.Errorf(".homux.toml = %q, want it untouched", got)
	}
	if !strings.Contains(stdout, "Nothing changed.") {
		t.Errorf("stdout = %q, want it to say nothing changed", stdout)
	}
}

// 既存 profile への追加 fork は cp の仕事である（spec §16）。
func TestProfileCreate_RejectsExistingProfile(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\"]\n")

	var candidates []string
	_, err := runCreateInteractive(t, repo, "work", "y\n", selects(&candidates))
	if err == nil {
		t.Fatal("runProfileCreate: want error, got nil")
	}
	if !strings.Contains(err.Error(), "already") {
		t.Errorf("err = %v, want it to say the profile already exists", err)
	}
}

// profile 名の文法は spec §5.4 に従う。
func TestProfileCreate_RejectsInvalidProfileName(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = []\n")

	var candidates []string
	_, err := runCreateInteractive(t, repo, "Work", "y\n", selects(&candidates))
	if err == nil {
		t.Fatal("runProfileCreate: want error, got nil")
	}
	if got := readFileContent(t, filepath.Join(repo, ".homux.toml")); got != "profiles = []\n" {
		t.Errorf(".homux.toml = %q, want it untouched", got)
	}
}

// spec §11.4: profile create は非 TTY では実行できない。
func TestProfileCreate_RequiresInteractiveTerminal(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = []\n")
	writeFile(t, filepath.Join(repo, ".gitconfig"), "git\n")

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := runProfileCreate(cmd, repo, "work", nil, false)
	if err == nil {
		t.Fatal("runProfileCreate: want error, got nil")
	}
	if !strings.Contains(err.Error(), "interactive") {
		t.Errorf("err = %v, want it to mention the terminal", err)
	}
}

// 構造エラーのある repository は先に診断して止まる。壊れた状態に
// さらに profile を足しても状況が悪くなるだけである。
func TestProfileCreate_StopsOnStructuralError(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = []\n")
	writeFile(t, filepath.Join(repo, ".gitconfig@@ghost"), "git\n")

	var candidates []string
	stdout, err := runCreateInteractive(t, repo, "work", "y\n", selects(&candidates))
	if err == nil {
		t.Fatal("runProfileCreate: want error, got nil")
	}
	if !strings.Contains(stdout, "ghost") {
		t.Errorf("output = %q, want the unknown profile diagnostic", stdout)
	}
	if got := readFileContent(t, filepath.Join(repo, ".homux.toml")); got != "profiles = []\n" {
		t.Errorf(".homux.toml = %q, want it untouched", got)
	}
}

// 選択画面での中断は「何もしない」で終わる。
func TestProfileCreate_AbortedSelectionChangesNothing(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = []\n")
	writeFile(t, filepath.Join(repo, ".gitconfig"), "git\n")

	abort := func(_ string, _ []string) ([]string, error) { return nil, ui.ErrSelectionAborted }
	_, err := runCreateInteractive(t, repo, "work", "", abort)
	if !errors.Is(err, ui.ErrSelectionAborted) {
		t.Fatalf("err = %v, want %v", err, ui.ErrSelectionAborted)
	}
	if got := readFileContent(t, filepath.Join(repo, ".homux.toml")); got != "profiles = []\n" {
		t.Errorf(".homux.toml = %q, want it untouched", got)
	}
}

// 候補が 1 つも無ければ選択を問う意味がない。profile の追加だけを行う。
func TestProfileCreate_SkipsSelectionWithoutCandidates(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = []\n")

	called := false
	sel := func(_ string, _ []string) ([]string, error) {
		called = true
		return nil, nil
	}
	if _, err := runCreateInteractive(t, repo, "work", "y\n", sel); err != nil {
		t.Fatalf("runProfileCreate: %v", err)
	}
	if called {
		t.Error("the selection screen was shown with no candidates")
	}
	if got := readFileContent(t, filepath.Join(repo, ".homux.toml")); got != "profiles = [\n  \"work\",\n]\n" {
		t.Errorf(".homux.toml = %q, want work added", got)
	}
}
