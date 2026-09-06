package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// addFixture builds a repo + home for homux add tests. The repo defines
// profile "work" and already has one managed common source (.zshrc) so that
// tests can exercise the "common source already exists" fork case.
func addFixture(t *testing.T) (home, repo string) {
	t.Helper()
	home = evalTempDir(t)
	repo = evalTempDir(t)
	xdg := filepath.Join(home, ".config-add-fixture")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(xdg, "homux", "config.toml"), "profile = \"work\"\n")
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = [\"work\", \"personal\"]\n")
	return home, repo
}

// runAddInteractive drives runAdd as if stdin and stdout were a terminal,
// mirroring runApplyInteractive in apply_test.go.
func runAddInteractive(t *testing.T, repo, stdin string, args []string, opts addOptions) (stdout string, err error) {
	t.Helper()
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err = runAdd(cmd, repo, args, opts, true)
	return out.String(), err
}

func runAddCmd(t *testing.T, repo string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := NewRootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"--repo", repo, "add"}, args...))
	code = runRoot(cmd)
	return out.String(), errOut.String(), code
}

func TestAddCmd_SingleFile_MovesAndSymlinks(t *testing.T) {
	home, repo := addFixture(t)
	target := filepath.Join(home, ".config/foo/config")
	writeFile(t, target, "foo contents")

	stdout, err := runAddInteractive(t, repo, "y\n", []string{target}, addOptions{})
	if err != nil {
		t.Fatalf("runAdd: %v (stdout=%s)", err, stdout)
	}

	repoPath := filepath.Join(repo, ".config/foo/config")
	if got := readFileContent(t, repoPath); got != "foo contents" {
		t.Errorf("repo content = %q, want %q", got, "foo contents")
	}
	assertSymlink(t, target, repoPath)
	if !strings.Contains(stdout, "Added 1 file.\n") {
		t.Errorf("stdout missing result line; got:\n%s", stdout)
	}
}

func TestAddCmd_DeclinedConfirmation_ChangesNothing(t *testing.T) {
	home, repo := addFixture(t)
	target := filepath.Join(home, ".config/foo/config")
	writeFile(t, target, "foo contents")

	before := contentSnapshot(t, home)

	_, err := runAddInteractive(t, repo, "n\n", []string{target}, addOptions{})
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	if after := contentSnapshot(t, home); !equalStrings(before, after) {
		t.Errorf("declining changed HOME:\nbefore: %v\nafter:  %v", before, after)
	}
	assertNotExist(t, filepath.Join(repo, ".config/foo/config"))
}

func TestAddCmd_WithProfile_UsesSuffixedSourceName(t *testing.T) {
	home, repo := addFixture(t)
	target := filepath.Join(home, ".claude/settings.json")
	writeFile(t, target, "{}")

	_, err := runAddInteractive(t, repo, "y\n", []string{target}, addOptions{profile: "work"})
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	repoPath := filepath.Join(repo, ".claude/settings.json@@work")
	assertSymlink(t, target, repoPath)
}

func TestAddCmd_Directory_RecursesAndAddsAllFiles(t *testing.T) {
	home, repo := addFixture(t)
	dir := filepath.Join(home, ".config/ghostty")
	writeFile(t, filepath.Join(dir, "config"), "ghostty config")
	writeFile(t, filepath.Join(dir, "nested", "extra"), "nested file")

	_, err := runAddInteractive(t, repo, "y\n", []string{dir}, addOptions{})
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	assertSymlink(t, filepath.Join(home, ".config/ghostty/config"), filepath.Join(repo, ".config/ghostty/config"))
	assertSymlink(t, filepath.Join(home, ".config/ghostty/nested/extra"), filepath.Join(repo, ".config/ghostty/nested/extra"))
}

// ADR 0001: symlink はファイル単位である。ディレクトリ配下の各ファイルは
// 個別に --profile の suffix を持つ source へ移される。
func TestAddCmd_DirectoryWithProfile_AppliesSuffixToEachFile(t *testing.T) {
	home, repo := addFixture(t)
	dir := filepath.Join(home, ".config/ghostty")
	writeFile(t, filepath.Join(dir, "config"), "ghostty config")
	writeFile(t, filepath.Join(dir, "nested", "extra"), "nested file")

	_, err := runAddInteractive(t, repo, "y\n", []string{dir}, addOptions{profile: "work"})
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	assertSymlink(t, filepath.Join(home, ".config/ghostty/config"), filepath.Join(repo, ".config/ghostty/config@@work"))
	assertSymlink(t, filepath.Join(home, ".config/ghostty/nested/extra"), filepath.Join(repo, ".config/ghostty/nested/extra@@work"))
}

func TestAddCmd_ForkNotedWhenCommonSourceExists(t *testing.T) {
	home, repo := addFixture(t)
	writeFile(t, filepath.Join(repo, ".vimrc"), "common vimrc\n")
	target := filepath.Join(home, ".vimrc")
	writeFile(t, target, "work vimrc")

	stdout, err := runAddInteractive(t, repo, "y\n", []string{target}, addOptions{profile: "work"})
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	repoPath := filepath.Join(repo, ".vimrc@@work")
	assertSymlink(t, target, repoPath)
	if got := readFileContent(t, filepath.Join(repo, ".vimrc")); got != "common vimrc\n" {
		t.Errorf("existing common source was modified: %q", got)
	}
	if !strings.Contains(stdout, "forks") {
		t.Errorf("stdout does not mention the fork; got:\n%s", stdout)
	}
}

func TestAddCmd_AlreadyManagedSymlink_Errors(t *testing.T) {
	home, repo := addFixture(t)
	writeFile(t, filepath.Join(repo, ".zshrc"), "zshrc\n")
	target := filepath.Join(home, ".zshrc")
	symlink(t, filepath.Join(repo, ".zshrc"), target)

	_, err := runAddInteractive(t, repo, "y\n", []string{target}, addOptions{})
	if err == nil {
		t.Fatal("err = nil, want error for an already-managed symlink")
	}
}

func TestAddCmd_SymlinkElsewhere_Errors(t *testing.T) {
	home, repo := addFixture(t)
	elsewhere := filepath.Join(home, "elsewhere", ".zshrc")
	writeFile(t, elsewhere, "x")
	target := filepath.Join(home, ".zshrc")
	symlink(t, elsewhere, target)

	_, err := runAddInteractive(t, repo, "y\n", []string{target}, addOptions{})
	if err == nil {
		t.Fatal("err = nil, want error for a symlink pointing elsewhere")
	}
}

func TestAddCmd_PathOutsideHome_Errors(t *testing.T) {
	_, repo := addFixture(t)
	outside := evalTempDir(t)
	target := filepath.Join(outside, "somefile")
	writeFile(t, target, "x")

	_, err := runAddInteractive(t, repo, "y\n", []string{target}, addOptions{})
	if err == nil {
		t.Fatal("err = nil, want error for a path outside HOME")
	}
}

func TestAddCmd_ExistingRepoSource_Errors(t *testing.T) {
	home, repo := addFixture(t)
	writeFile(t, filepath.Join(repo, ".config/foo/config"), "already in repo")
	target := filepath.Join(home, ".config/foo/config")
	writeFile(t, target, "new content")

	_, err := runAddInteractive(t, repo, "y\n", []string{target}, addOptions{})
	if err == nil {
		t.Fatal("err = nil, want error when the repo already has a same-named source")
	}
	if got := readFileContent(t, filepath.Join(repo, ".config/foo/config")); got != "already in repo" {
		t.Errorf("existing repo file was modified: %q", got)
	}
}

func TestAddCmd_UnknownProfile_Errors(t *testing.T) {
	home, repo := addFixture(t)
	target := filepath.Join(home, ".zshrc")
	writeFile(t, target, "x")

	_, err := runAddInteractive(t, repo, "y\n", []string{target}, addOptions{profile: "no-such-profile"})
	if err == nil {
		t.Fatal("err = nil, want error for an unknown profile")
	}
}

func TestAddCmd_NonInteractive_Errors(t *testing.T) {
	home, repo := addFixture(t)
	target := filepath.Join(home, ".zshrc")
	writeFile(t, target, "x")

	stdout, stderr, code := runAddCmd(t, repo, target)

	if code != ExitError {
		t.Fatalf("exit code = %d, want %d", code, ExitError)
	}
	if stderr == "" {
		t.Error("expected an error message on stderr")
	}
	_ = stdout
	assertNotExist(t, filepath.Join(repo, ".zshrc"))
}

func TestAddCmd_DoesNotModifyLocalConfig(t *testing.T) {
	home, repo := addFixture(t)
	target := filepath.Join(home, ".zshrc")
	writeFile(t, target, "x")
	localPath := filepath.Join(home, ".config-add-fixture", "homux", "config.toml")
	before := readFileContent(t, localPath)

	_, err := runAddInteractive(t, repo, "y\n", []string{target}, addOptions{})
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	if after := readFileContent(t, localPath); after != before {
		t.Errorf("local config changed:\nbefore: %q\nafter:  %q", before, after)
	}
}

func readFileContent(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return string(b)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
