package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bellwood4486/homux/internal/config"
)

// runDeleteInteractive は端末であるかのように runProfileDelete を動かす。
func runDeleteInteractive(t *testing.T, repo, name, stdin string) (stdout string, err error) {
	t.Helper()
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	err = runProfileDelete(cmd, &globalFlags{repo: repo}, name, true)
	return out.String(), err
}

// setupDeleteHome は HOME と local config を用意し、local config のパスを返す。
func setupDeleteHome(t *testing.T, repo, profile string) (localPath string) {
	t.Helper()
	home := evalTempDir(t)
	xdg := filepath.Join(home, ".config-profile-delete")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	localPath = filepath.Join(xdg, "homux", "config.toml")
	content := "repo = \"" + repo + "\"\n"
	if profile != "" {
		content += "profile = \"" + profile + "\"\n"
	}
	writeFile(t, localPath, content)
	return localPath
}

func loadProfiles(t *testing.T, repo string) []string {
	t.Helper()
	cfg, err := config.LoadRepo(filepath.Join(repo, config.RepoFileName))
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	return cfg.Profiles
}

// spec §12.11 の表そのもの。単一 profile suffix は削除し、複数 profile
// selector はファイルを消さずに rewrite する。
func TestProfileDelete_RemovesSingleAndRewritesMulti(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\", \"personal\"]\n")
	setupDeleteHome(t, repo, "personal")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "git\n")
	writeFile(t, filepath.Join(repo, ".config", "foo", "config@@work+personal"), "foo\n")
	writeFile(t, filepath.Join(repo, ".zshrc"), "zsh\n")
	writeFile(t, filepath.Join(repo, ".vimrc@@personal"), "vim\n")

	stdout, err := runDeleteInteractive(t, repo, "work", "y\n")
	if err != nil {
		t.Fatalf("runProfileDelete: %v", err)
	}

	if _, statErr := os.Lstat(filepath.Join(repo, ".gitconfig@@work")); statErr == nil {
		t.Error(".gitconfig@@work still exists, want it removed")
	}
	if _, statErr := os.Lstat(filepath.Join(repo, ".config", "foo", "config@@work+personal")); statErr == nil {
		t.Error("config@@work+personal still exists, want it rewritten")
	}
	if got := readFileContent(t, filepath.Join(repo, ".config", "foo", "config@@personal")); got != "foo\n" {
		t.Errorf("config@@personal = %q, want %q", got, "foo\n")
	}
	// 対象外のものは動かない。
	if got := readFileContent(t, filepath.Join(repo, ".zshrc")); got != "zsh\n" {
		t.Errorf(".zshrc = %q, want it untouched", got)
	}
	if got := readFileContent(t, filepath.Join(repo, ".vimrc@@personal")); got != "vim\n" {
		t.Errorf(".vimrc@@personal = %q, want it untouched", got)
	}
	if got := loadProfiles(t, repo); !equalStrings(got, []string{"personal"}) {
		t.Errorf("profiles = %v, want [personal]", got)
	}
	if !strings.Contains(stdout, "Deleted profile \"work\".") {
		t.Errorf("stdout = %q, want the delete headline", stdout)
	}
}

// n を選んだら repository も設定も 1 バイトも変わらない。
func TestProfileDelete_DeclineChangesNothing(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\"]\n")
	setupDeleteHome(t, repo, "work")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "git\n")

	stdout, err := runDeleteInteractive(t, repo, "work", "n\n")
	if err != nil {
		t.Fatalf("runProfileDelete: %v", err)
	}

	if got := readFileContent(t, filepath.Join(repo, ".gitconfig@@work")); got != "git\n" {
		t.Errorf(".gitconfig@@work = %q, want it untouched", got)
	}
	if got := loadProfiles(t, repo); !equalStrings(got, []string{"work"}) {
		t.Errorf("profiles = %v, want [work]", got)
	}
	if !strings.Contains(stdout, "Nothing changed.") {
		t.Errorf("stdout = %q, want \"Nothing changed.\"", stdout)
	}
}

// この PC が削除対象を使っている場合、local active profile は「なし」になる。
// HOME は変更せず、apply が必要である旨を出す（spec §12.11）。
func TestProfileDelete_ClearsLocalActiveProfile(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\", \"personal\"]\n")
	localPath := setupDeleteHome(t, repo, "work")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "git\n")
	writeFile(t, filepath.Join(repo, ".gitconfig"), "common\n")

	stdout, err := runDeleteInteractive(t, repo, "work", "y\n")
	if err != nil {
		t.Fatalf("runProfileDelete: %v", err)
	}

	if got := loadLocalProfile(t, localPath); got != "" {
		t.Errorf("local active profile = %q, want it cleared", got)
	}
	// plan には「HOME がどう変わるか」が含まれる。common source へ戻る。
	if !strings.Contains(stdout, "HOME after this change:") {
		t.Errorf("stdout = %q, want the HOME section", stdout)
	}
	if !strings.Contains(stdout, ".gitconfig@@work -> .gitconfig") {
		t.Errorf("stdout = %q, want the fallback to the common source", stdout)
	}
	if !strings.Contains(stdout, "Run \"homux apply\" to update HOME.") {
		t.Errorf("stdout = %q, want the apply hint", stdout)
	}
}

// active でない profile を消しても HOME の解決先は変わらない。
func TestProfileDelete_NoHomeChangeForInactiveProfile(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\", \"personal\"]\n")
	setupDeleteHome(t, repo, "personal")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "git\n")
	writeFile(t, filepath.Join(repo, ".gitconfig"), "common\n")

	stdout, err := runDeleteInteractive(t, repo, "work", "y\n")
	if err != nil {
		t.Fatalf("runProfileDelete: %v", err)
	}

	if strings.Contains(stdout, "HOME after this change:") {
		t.Errorf("stdout = %q, want no HOME section", stdout)
	}
}

// rewrite で active profile の解決先が変わる場合は、active を消さなくても
// HOME に影響が出る。
func TestProfileDelete_ReportsHomeChangeFromRewrite(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\", \"personal\"]\n")
	setupDeleteHome(t, repo, "personal")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work+personal"), "both\n")

	stdout, err := runDeleteInteractive(t, repo, "work", "y\n")
	if err != nil {
		t.Fatalf("runProfileDelete: %v", err)
	}

	if !strings.Contains(stdout, ".gitconfig@@work+personal -> .gitconfig@@personal") {
		t.Errorf("stdout = %q, want the HOME change caused by the rewrite", stdout)
	}
}

// rewrite 先が既に存在するなら、1 バイトも変更せずに止まる。
//
// この repository は active が server である限り解決できる（どちらの source も
// 一致しないため common へ fallback する）。rewrite して初めて 2 つが同じ名前を
// 争うことになる。
func TestProfileDelete_RewriteCollisionChangesNothing(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\", \"personal\", \"server\"]\n")
	setupDeleteHome(t, repo, "server")
	writeFile(t, filepath.Join(repo, ".gitconfig"), "common\n")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work+personal"), "both\n")
	writeFile(t, filepath.Join(repo, ".gitconfig@@personal"), "personal\n")

	stdout, err := runDeleteInteractive(t, repo, "work", "y\n")
	if err == nil {
		t.Fatal("err = nil, want the collision to stop the command")
	}
	if !strings.Contains(stdout, "ERROR selector rewrite collision") {
		t.Errorf("output = %q, want the collision diagnostic", stdout)
	}
	if got := readFileContent(t, filepath.Join(repo, ".gitconfig@@work+personal")); got != "both\n" {
		t.Errorf(".gitconfig@@work+personal = %q, want it untouched", got)
	}
	if got := readFileContent(t, filepath.Join(repo, ".gitconfig@@personal")); got != "personal\n" {
		t.Errorf(".gitconfig@@personal = %q, want it untouched", got)
	}
	if got := loadProfiles(t, repo); !equalStrings(got, []string{"work", "personal", "server"}) {
		t.Errorf("profiles = %v, want them untouched", got)
	}
}

// 2 つの source が同じ名前へ rewrite される場合も、1 バイトも変更せずに止まる。
func TestProfileDelete_DuplicateRewriteTargetChangesNothing(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\", \"personal\", \"server\"]\n")
	setupDeleteHome(t, repo, "server")
	writeFile(t, filepath.Join(repo, ".gitconfig"), "common\n")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work+personal"), "one\n")
	writeFile(t, filepath.Join(repo, ".gitconfig@@personal+work"), "two\n")

	stdout, err := runDeleteInteractive(t, repo, "work", "y\n")
	if err == nil {
		t.Fatal("err = nil, want the collision to stop the command")
	}
	if !strings.Contains(stdout, "Another source is rewritten to the same name.") {
		t.Errorf("output = %q, want the duplicate collision diagnostic", stdout)
	}
	for _, keep := range []string{".gitconfig@@work+personal", ".gitconfig@@personal+work"} {
		if _, statErr := os.Lstat(filepath.Join(repo, keep)); statErr != nil {
			t.Errorf("%s: %v, want it untouched", keep, statErr)
		}
	}
}

// 参照が 1 件も無くても plan と確認を通す。破壊操作の入口を 1 つに保つ。
func TestProfileDelete_UnreferencedProfileStillConfirms(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\", \"personal\"]\n")
	setupDeleteHome(t, repo, "personal")
	writeFile(t, filepath.Join(repo, ".zshrc"), "zsh\n")

	stdout, err := runDeleteInteractive(t, repo, "work", "y\n")
	if err != nil {
		t.Fatalf("runProfileDelete: %v", err)
	}

	if !strings.Contains(stdout, "is not referenced by any source.") {
		t.Errorf("stdout = %q, want the unreferenced notice", stdout)
	}
	if got := loadProfiles(t, repo); !equalStrings(got, []string{"personal"}) {
		t.Errorf("profiles = %v, want [personal]", got)
	}
}

// 未定義の profile は消せない。
func TestProfileDelete_UnknownProfile(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"personal\"]\n")
	setupDeleteHome(t, repo, "personal")

	_, err := runDeleteInteractive(t, repo, "work", "y\n")
	if err == nil {
		t.Fatal("err = nil, want an unknown profile error")
	}
	if !strings.Contains(err.Error(), "unknown profile") {
		t.Errorf("err = %v, want it to mention the unknown profile", err)
	}
}

// 非対話端末では確認が取れないため実行しない（spec §11.4）。
func TestProfileDelete_RequiresInteractiveTerminal(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\"]\n")
	setupDeleteHome(t, repo, "work")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "git\n")

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("y\n"))

	err := runProfileDelete(cmd, &globalFlags{repo: repo}, "work", false)
	if err == nil {
		t.Fatal("err = nil, want a non-interactive error")
	}
	if got := readFileContent(t, filepath.Join(repo, ".gitconfig@@work")); got != "git\n" {
		t.Errorf(".gitconfig@@work = %q, want it untouched", got)
	}
}

// 壊れた repository の参照を機械的に消しても状況は良くならない。
// status / apply と同じ診断で先に止める（spec §10）。
func TestProfileDelete_StopsOnStructuralError(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\"]\n")
	setupDeleteHome(t, repo, "work")
	writeFile(t, filepath.Join(repo, ".gitconfig@@ghost"), "git\n")

	stdout, err := runDeleteInteractive(t, repo, "work", "y\n")
	if err == nil {
		t.Fatal("err = nil, want the diagnostic to stop the command")
	}
	if !strings.Contains(stdout, "ghost") {
		t.Errorf("output = %q, want the unknown profile diagnostic", stdout)
	}
	if got := loadProfiles(t, repo); !equalStrings(got, []string{"work"}) {
		t.Errorf("profiles = %v, want them untouched", got)
	}
}

// HOME には触れない（spec §11.2）。消した source を指す symlink はそのまま残り、
// その解消は apply の仕事である。
func TestProfileDelete_LeavesHomeUntouched(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\"]\n")
	setupDeleteHome(t, repo, "work")
	source := filepath.Join(repo, ".gitconfig@@work")
	writeFile(t, source, "git\n")

	home := os.Getenv("HOME")
	link := filepath.Join(home, ".gitconfig")
	if err := os.Symlink(source, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	if _, err := runDeleteInteractive(t, repo, "work", "y\n"); err != nil {
		t.Fatalf("runProfileDelete: %v", err)
	}

	got, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if got != source {
		t.Errorf("link = %q, want it still pointing at %q", got, source)
	}
}
