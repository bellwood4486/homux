package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bellwood4486/homux/internal/config"
)

// runRenameInteractive は端末であるかのように runProfileRename を動かす。
// 対話は素の [y/N] なので、stdin に答えを流し込めば本番と同じ経路を通る。
func runRenameInteractive(t *testing.T, repo, from, to, stdin string) (stdout string, err error) {
	t.Helper()
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	err = runProfileRename(cmd, repo, from, to, true)
	return out.String(), err
}

// setupRenameHome は HOME と local config を用意し、local config のパスを返す。
// profile rename は local active profile を書き換えるため、既定でこの環境を使う。
func setupRenameHome(t *testing.T, repo, profile string) (localPath string) {
	t.Helper()
	home := evalTempDir(t)
	xdg := filepath.Join(home, ".config-profile-rename")
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

// loadLocalProfile は local config の active profile を読む。TOML の引用符の
// 選び方は go-toml の裁量なので、生のバイト列ではなく読み取り結果で確かめる。
func loadLocalProfile(t *testing.T, path string) string {
	t.Helper()
	local, err := config.LoadLocal(path)
	if err != nil {
		t.Fatalf("LoadLocal %s: %v", path, err)
	}
	return local.Profile
}

// 単一 suffix と複数 profile selector の両方を整合的に更新する（INV-15）。
// 複数指定のファイルは消さず、名前の一部だけを置き換える。
func TestProfileRename_RewritesFilesAndSelectors(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\", \"personal\"]\n")
	setupRenameHome(t, repo, "personal")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "git\n")
	writeFile(t, filepath.Join(repo, ".claude", "settings.json@@work+personal"), "claude\n")
	writeFile(t, filepath.Join(repo, ".zshrc"), "zsh\n")
	writeFile(t, filepath.Join(repo, ".config", "foo@@personal"), "foo\n")

	stdout, err := runRenameInteractive(t, repo, "work", "company", "y\n")
	if err != nil {
		t.Fatalf("runProfileRename: %v", err)
	}

	if got := readFileContent(t, filepath.Join(repo, ".gitconfig@@company")); got != "git\n" {
		t.Errorf(".gitconfig@@company = %q, want %q", got, "git\n")
	}
	if got := readFileContent(t, filepath.Join(repo, ".claude", "settings.json@@company+personal")); got != "claude\n" {
		t.Errorf("settings.json@@company+personal = %q, want %q", got, "claude\n")
	}
	for _, gone := range []string{".gitconfig@@work", ".claude/settings.json@@work+personal"} {
		if _, statErr := os.Lstat(filepath.Join(repo, filepath.FromSlash(gone))); statErr == nil {
			t.Errorf("%s still exists", gone)
		}
	}
	// 対象外のものは動かない。
	if got := readFileContent(t, filepath.Join(repo, ".zshrc")); got != "zsh\n" {
		t.Errorf(".zshrc = %q, want it untouched", got)
	}
	if got := readFileContent(t, filepath.Join(repo, ".config", "foo@@personal")); got != "foo\n" {
		t.Errorf("foo@@personal = %q, want it untouched", got)
	}
	if !strings.Contains(stdout, "Renamed profile \"work\" -> \"company\".") {
		t.Errorf("stdout = %q, want the rename headline", stdout)
	}
}

// .homux.toml の profiles は range replace で更新する（design §6、ADR 0008）。
// 定義順を保ち、コメントや ignore の整形には触れない。
func TestProfileRename_UpdatesRepoFileInPlace(t *testing.T) {
	repo := setupCreateRepo(t, "# my dotfiles\nprofiles = [\"work\", \"personal\"]\n\nignore = [\"README.md\"]\n")
	setupRenameHome(t, repo, "")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "git\n")

	if _, err := runRenameInteractive(t, repo, "work", "company", "y\n"); err != nil {
		t.Fatalf("runProfileRename: %v", err)
	}

	got := readFileContent(t, filepath.Join(repo, ".homux.toml"))
	want := "# my dotfiles\nprofiles = [\n  \"company\",\n  \"personal\",\n]\n\nignore = [\"README.md\"]\n"
	if got != want {
		t.Errorf(".homux.toml =\n%s\nwant\n%s", got, want)
	}
}

// この PC が対象 profile を使っているときは local active profile も追随する
// （spec §12.10）。使っていないときは触らない。
func TestProfileRename_UpdatesLocalActiveProfile(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\"]\n")
	localPath := setupRenameHome(t, repo, "work")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "git\n")

	stdout, err := runRenameInteractive(t, repo, "work", "company", "y\n")
	if err != nil {
		t.Fatalf("runProfileRename: %v", err)
	}

	if got := loadLocalProfile(t, localPath); got != "company" {
		t.Errorf("local active profile = %q, want %q", got, "company")
	}
	if !strings.Contains(stdout, "Active profile: work -> company") {
		t.Errorf("stdout = %q, want the active profile line", stdout)
	}
}

func TestProfileRename_LeavesOtherActiveProfileAlone(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\", \"personal\"]\n")
	localPath := setupRenameHome(t, repo, "personal")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "git\n")

	if _, err := runRenameInteractive(t, repo, "work", "company", "y\n"); err != nil {
		t.Fatalf("runProfileRename: %v", err)
	}

	if got := loadLocalProfile(t, localPath); got != "personal" {
		t.Errorf("local active profile = %q, want it untouched", got)
	}
}

// design §7.1 / INV-15: 衝突を検出したとき、1 ファイルも変更していない。
// 改名先は ignore されたファイルとして置く。scan の対象外なので unknown
// profile の診断には掛からず、事前の存在検査だけが唯一の防波堤である。
func TestProfileRename_CollisionChangesNothing(t *testing.T) {
	toml := "profiles = [\"work\"]\n\nignore = [\".gitconfig@@company\"]\n"
	repo := setupCreateRepo(t, toml)
	localPath := setupRenameHome(t, repo, "work")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "git\n")
	writeFile(t, filepath.Join(repo, ".gitconfig@@company"), "keep me\n")

	stdout, err := runRenameInteractive(t, repo, "work", "company", "y\n")
	if err == nil {
		t.Fatal("runProfileRename: want error, got nil")
	}
	if !strings.Contains(stdout, "ERROR rename collision") {
		t.Errorf("output = %q, want the collision diagnostic", stdout)
	}

	if got := readFileContent(t, filepath.Join(repo, ".gitconfig@@work")); got != "git\n" {
		t.Errorf(".gitconfig@@work = %q, want it untouched", got)
	}
	if got := readFileContent(t, filepath.Join(repo, ".gitconfig@@company")); got != "keep me\n" {
		t.Errorf(".gitconfig@@company = %q, want it untouched", got)
	}
	if got := readFileContent(t, filepath.Join(repo, ".homux.toml")); got != toml {
		t.Errorf(".homux.toml = %q, want it untouched", got)
	}
	if got := loadLocalProfile(t, localPath); got != "work" {
		t.Errorf("local active profile = %q, want it untouched", got)
	}
}

// 衝突の検出は確認プロンプトより前で行う。壊れた計画を見せて「はい」を
// 取ってから止まる、という順序にはしない。
func TestProfileRename_CollisionIsDetectedBeforeConfirmation(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\"]\n\nignore = [\".gitconfig@@company\"]\n")
	setupRenameHome(t, repo, "")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "git\n")
	writeFile(t, filepath.Join(repo, ".gitconfig@@company"), "keep me\n")

	// stdin は空。プロンプトまで進んでいたら読み取りエラーになる。
	stdout, err := runRenameInteractive(t, repo, "work", "company", "")
	if err == nil {
		t.Fatal("runProfileRename: want error, got nil")
	}
	if strings.Contains(stdout, "[y/N]") {
		t.Errorf("output = %q, want it to stop before asking", stdout)
	}
}

// spec §11.2: profile rename は HOME を変更しない。
func TestProfileRename_DoesNotTouchHome(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\"]\n")
	setupRenameHome(t, repo, "work")
	home := os.Getenv("HOME")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "git\n")
	writeFile(t, filepath.Join(home, ".gitconfig"), "home git\n")

	before := snapshotTree(t, home)

	stdout, err := runRenameInteractive(t, repo, "work", "company", "y\n")
	if err != nil {
		t.Fatalf("runProfileRename: %v", err)
	}

	after := snapshotTree(t, home)
	if len(before) != len(after) {
		t.Fatalf("HOME entry count changed: %d -> %d", len(before), len(after))
	}
	if got := readFileContent(t, filepath.Join(home, ".gitconfig")); got != "home git\n" {
		t.Errorf("~/.gitconfig = %q, want it untouched", got)
	}
	// 配置される内容は変わらないため apply は促さない。
	if strings.Contains(stdout, "homux apply") {
		t.Errorf("stdout = %q, want it not to ask for apply", stdout)
	}
}

// n と答えたときは 1 バイトも変更しない。
func TestProfileRename_DeclinedChangesNothing(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\"]\n")
	setupRenameHome(t, repo, "work")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "git\n")

	stdout, err := runRenameInteractive(t, repo, "work", "company", "n\n")
	if err != nil {
		t.Fatalf("runProfileRename: %v", err)
	}

	if _, statErr := os.Lstat(filepath.Join(repo, ".gitconfig@@company")); statErr == nil {
		t.Error(".gitconfig@@company was created despite answering no")
	}
	if got := readFileContent(t, filepath.Join(repo, ".homux.toml")); got != "profiles = [\"work\"]\n" {
		t.Errorf(".homux.toml = %q, want it untouched", got)
	}
	if !strings.Contains(stdout, "Nothing changed.") {
		t.Errorf("stdout = %q, want it to say nothing changed", stdout)
	}
}

// 未定義の profile は rename できない。
func TestProfileRename_RejectsUnknownSource(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\"]\n")
	setupRenameHome(t, repo, "")

	_, err := runRenameInteractive(t, repo, "ghost", "company", "y\n")
	if err == nil {
		t.Fatal("runProfileRename: want error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown profile") {
		t.Errorf("err = %v, want it to say the profile is unknown", err)
	}
}

// 既存 profile への rename は統合を意味してしまう。profile rename の責務では
// ない（spec §12.10 は改名だけを定義している）。
func TestProfileRename_RejectsExistingDestination(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\", \"personal\"]\n")
	setupRenameHome(t, repo, "")

	_, err := runRenameInteractive(t, repo, "work", "personal", "y\n")
	if err == nil {
		t.Fatal("runProfileRename: want error, got nil")
	}
	if !strings.Contains(err.Error(), "already") {
		t.Errorf("err = %v, want it to say the profile already exists", err)
	}
}

// profile 名の文法は spec §5.4 に従う。使い方の誤りなので終了コードは 2。
func TestProfileRename_RejectsInvalidNewName(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\"]\n")
	setupRenameHome(t, repo, "")

	_, err := runRenameInteractive(t, repo, "work", "Company", "y\n")
	if err == nil {
		t.Fatal("runProfileRename: want error, got nil")
	}
	var ue *usageError
	if !errors.As(err, &ue) {
		t.Errorf("err = %v, want a usage error", err)
	}
	if got := readFileContent(t, filepath.Join(repo, ".homux.toml")); got != "profiles = [\"work\"]\n" {
		t.Errorf(".homux.toml = %q, want it untouched", got)
	}
}

// 構造エラーのある repository は先に診断して止まる（profile create と同じ）。
func TestProfileRename_StopsOnStructuralError(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\"]\n")
	setupRenameHome(t, repo, "")
	writeFile(t, filepath.Join(repo, ".gitconfig@@ghost"), "git\n")

	stdout, err := runRenameInteractive(t, repo, "work", "company", "y\n")
	if err == nil {
		t.Fatal("runProfileRename: want error, got nil")
	}
	if !strings.Contains(stdout, "ghost") {
		t.Errorf("output = %q, want the unknown profile diagnostic", stdout)
	}
	if got := readFileContent(t, filepath.Join(repo, ".homux.toml")); got != "profiles = [\"work\"]\n" {
		t.Errorf(".homux.toml = %q, want it untouched", got)
	}
}

// spec §11.4: 確認を要するコマンドは非 TTY では実行できない。
func TestProfileRename_RequiresInteractiveTerminal(t *testing.T) {
	repo := setupCreateRepo(t, "profiles = [\"work\"]\n")
	setupRenameHome(t, repo, "")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "git\n")

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := runProfileRename(cmd, repo, "work", "company", false)
	if err == nil {
		t.Fatal("runProfileRename: want error, got nil")
	}
	if !strings.Contains(err.Error(), "interactive") {
		t.Errorf("err = %v, want it to mention the terminal", err)
	}
}
