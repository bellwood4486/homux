package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/bellwood4486/homux/internal/config"
)

// initFixture は .homux.toml を持つ repository と、まだ local config を持たない
// HOME を用意する。init の出発点はここである。
func initFixture(t *testing.T) (home, repo, localPath string) {
	t.Helper()
	home = evalTempDir(t)
	repo = evalTempDir(t)
	xdg := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = [\"work\", \"personal\"]\n")
	writeFile(t, filepath.Join(repo, ".vimrc@@work"), "work vimrc\n")
	writeFile(t, filepath.Join(repo, ".zshrc"), "zshrc\n")

	return home, repo, config.LocalPath(xdg, home)
}

// runInitInteractive は stdin / stdout が端末であるかのように runInit を動かす。
// go test では os.Stdin が TTY ではないため、CLI 全体を通す経路からは対話分岐に
// 到達できない（apply の runApplyInteractive と同じ理由）。
func runInitInteractive(t *testing.T, repoFlag, stdin string, opts initOptions) (stdout string, err error) {
	t.Helper()
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err = runInit(cmd, &globalFlags{repo: repoFlag}, opts, true)
	return out.String(), err
}

func runInitCmd(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := NewRootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	code = runRoot(cmd)
	return out.String(), errOut.String(), code
}

func loadLocal(t *testing.T, path string) *config.Local {
	t.Helper()
	local, err := config.LoadLocal(path)
	if err != nil {
		t.Fatalf("LoadLocal(%s): %v", path, err)
	}
	return local
}

// spec §12.1 の全経路: repo path 入力 → profile 選択 → apply。
func TestInitCmd_InteractiveFlowSavesConfigAndApplies(t *testing.T) {
	home, repo, localPath := initFixture(t)

	stdout, err := runInitInteractive(t, "", repo+"\n1\n", initOptions{})
	if err != nil {
		t.Fatalf("runInit: %v", err)
	}

	local := loadLocal(t, localPath)
	if local.Repo != repo {
		t.Errorf("local repo = %q, want %q", local.Repo, repo)
	}
	if local.Profile != "work" {
		t.Errorf("local profile = %q, want %q", local.Profile, "work")
	}
	assertSymlink(t, filepath.Join(home, ".vimrc"), filepath.Join(repo, ".vimrc@@work"))
	assertSymlink(t, filepath.Join(home, ".zshrc"), filepath.Join(repo, ".zshrc"))
	if !strings.Contains(stdout, "Applied 2 changes.") {
		t.Errorf("stdout:\n%s", stdout)
	}
}

// (none) を選んだときは profile-specific source が一切選ばれない（INV-09）。
func TestInitCmd_InteractiveNoneProfile(t *testing.T) {
	home, repo, localPath := initFixture(t)

	if _, err := runInitInteractive(t, "", repo+"\n3\n", initOptions{}); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	if got := loadLocal(t, localPath).Profile; got != "" {
		t.Errorf("local profile = %q, want empty", got)
	}
	assertNotExist(t, filepath.Join(home, ".vimrc"))
	assertSymlink(t, filepath.Join(home, ".zshrc"), filepath.Join(repo, ".zshrc"))
}

// 既定候補はカレントディレクトリである（spec §12.1）。
func TestInitCmd_InteractiveDefaultsToWorkingDirectory(t *testing.T) {
	_, repo, localPath := initFixture(t)
	t.Chdir(repo)

	stdout, err := runInitInteractive(t, "", "\n3\n", initOptions{})
	if err != nil {
		t.Fatalf("runInit: %v", err)
	}

	if got := loadLocal(t, localPath).Repo; got != repo {
		t.Errorf("local repo = %q, want %q", got, repo)
	}
	if !strings.Contains(stdout, "Repository path [") {
		t.Errorf("stdout has no default candidate:\n%s", stdout)
	}
}

// 対話入力が不正なパスなら、終了せずに聞き直す。
func TestInitCmd_InteractiveReAsksOnInvalidPath(t *testing.T) {
	_, repo, localPath := initFixture(t)
	missing := filepath.Join(repo, "does-not-exist")
	notDir := filepath.Join(repo, ".zshrc")

	stdout, err := runInitInteractive(t, "", missing+"\n"+notDir+"\n"+repo+"\n3\n", initOptions{})
	if err != nil {
		t.Fatalf("runInit: %v", err)
	}

	if got := loadLocal(t, localPath).Repo; got != repo {
		t.Errorf("local repo = %q, want %q", got, repo)
	}
	if n := strings.Count(stdout, "Repository path"); n != 3 {
		t.Errorf("asked %d times, want 3:\n%s", n, stdout)
	}
	if !strings.Contains(stdout, "not a directory") {
		t.Errorf("stdout does not explain the second rejection:\n%s", stdout)
	}
}

// --repo で渡されたパスは聞き直さず、その場で失敗する。
func TestInitCmd_RepoFlagInvalidPathFails(t *testing.T) {
	_, repo, localPath := initFixture(t)

	_, _, code := runInitCmd(t, "--repo", filepath.Join(repo, "does-not-exist"), "init", "--profile", "work")

	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	assertNotExist(t, localPath)
}

// .homux.toml が無い場合は確認のうえ雛形を書き出す（spec §12.1）。
func TestInitCmd_CreatesRepoFileTemplate(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	xdg := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(repo, ".zshrc"), "zshrc\n")

	stdout, err := runInitInteractive(t, "", repo+"\ny\n", initOptions{})
	if err != nil {
		t.Fatalf("runInit: %v", err)
	}

	repoFile := filepath.Join(repo, config.RepoFileName)
	b, readErr := os.ReadFile(repoFile)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if !strings.Contains(string(b), `# "docs/**"`) {
		t.Errorf("template lacks commented-out ignore entries:\n%s", b)
	}
	// profiles が空なので profile 選択は出さない。
	if strings.Contains(stdout, "Available profiles:") {
		t.Errorf("should not ask for a profile when none are defined:\n%s", stdout)
	}
	if got := loadLocal(t, config.LocalPath(xdg, home)).Profile; got != "" {
		t.Errorf("local profile = %q, want empty", got)
	}
	assertSymlink(t, filepath.Join(home, ".zshrc"), filepath.Join(repo, ".zshrc"))
}

// 初期化を断ったら何も書かずに終わる。
func TestInitCmd_DeclineRepoFileCreationAborts(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	xdg := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	if _, err := runInitInteractive(t, "", repo+"\nn\n", initOptions{}); err == nil {
		t.Fatal("runInit err = nil, want error")
	}

	assertNotExist(t, filepath.Join(repo, config.RepoFileName))
	assertNotExist(t, config.LocalPath(xdg, home))
}

// 既存の .homux.toml をコメントごと壊さない（受け入れ条件）。
func TestInitCmd_KeepsExistingRepoFileByteForByte(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	const existing = "# hand written\nprofiles = [\n  \"work\", # main machine\n]\n"
	repoFile := filepath.Join(repo, config.RepoFileName)
	writeFile(t, repoFile, existing)
	writeFile(t, filepath.Join(repo, ".zshrc"), "zshrc\n")

	if _, _, code := runInitCmd(t, "--repo", repo, "init", "--profile", "work"); code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}

	b, err := os.ReadFile(repoFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != existing {
		t.Errorf("%s was rewritten:\ngot:\n%s\nwant:\n%s", config.RepoFileName, b, existing)
	}
}

// --repo と --profile が揃っていれば非対話で完走する（spec §11.4）。
func TestInitCmd_NonInteractiveWithFlags(t *testing.T) {
	home, repo, localPath := initFixture(t)

	_, _, code := runInitCmd(t, "--repo", repo, "init", "--profile", "work")

	if code != ExitOK {
		t.Errorf("exit code = %d, want %d", code, ExitOK)
	}
	if got := loadLocal(t, localPath).Profile; got != "work" {
		t.Errorf("local profile = %q, want %q", got, "work")
	}
	assertSymlink(t, filepath.Join(home, ".vimrc"), filepath.Join(repo, ".vimrc@@work"))
}

// --profile を空文字列で明示すると profile なしになる。"none" のような予約
// 文字列は使わない（docs/design.md §4.2）。
func TestInitCmd_NonInteractiveEmptyProfileMeansNoProfile(t *testing.T) {
	home, repo, localPath := initFixture(t)

	_, _, code := runInitCmd(t, "--repo", repo, "init", "--profile=")

	if code != ExitOK {
		t.Errorf("exit code = %d, want %d", code, ExitOK)
	}
	if got := loadLocal(t, localPath).Profile; got != "" {
		t.Errorf("local profile = %q, want empty", got)
	}
	assertNotExist(t, filepath.Join(home, ".vimrc"))
}

func TestInitCmd_UnknownProfileFlagFails(t *testing.T) {
	_, repo, localPath := initFixture(t)

	_, stderr, code := runInitCmd(t, "--repo", repo, "init", "--profile", "nope")

	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, `unknown profile "nope"`) {
		t.Errorf("stderr:\n%s", stderr)
	}
	assertNotExist(t, localPath)
}

// 非 TTY で対話が必要になったらエラー終了する（spec §11.4）。
func TestInitCmd_NonInteractiveWithoutRepoFails(t *testing.T) {
	_, _, _ = initFixture(t)

	_, stderr, code := runInitCmd(t, "init", "--profile=")

	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "--repo") {
		t.Errorf("stderr should point at --repo:\n%s", stderr)
	}
}

func TestInitCmd_NonInteractiveWithoutProfileFails(t *testing.T) {
	_, repo, localPath := initFixture(t)

	_, stderr, code := runInitCmd(t, "--repo", repo, "init")

	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "--profile") {
		t.Errorf("stderr should point at --profile:\n%s", stderr)
	}
	assertNotExist(t, localPath)
}

// .homux.toml が無く、その作成確認を出せない非 TTY では止まる。
func TestInitCmd_NonInteractiveWithoutRepoFileFails(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	_, stderr, code := runInitCmd(t, "--repo", repo, "init", "--profile=")

	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, config.RepoFileName) {
		t.Errorf("stderr:\n%s", stderr)
	}
	assertNotExist(t, filepath.Join(repo, config.RepoFileName))
}
