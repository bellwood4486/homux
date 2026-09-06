package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// applyFixture builds a repo + home whose plan contains one action of every
// kind, with active profile "work" and no structural errors:
//
//	.claude/settings.json  Occupied -> ReplaceTarget      (confirm)
//	.config/foo/config     Missing  -> CreateSymlink      (no confirm)
//	.config/orphan         Stale 2  -> RemoveStaleSymlink (confirm)
//	.vimrc                 Stale 1  -> Relink             (confirm)
//
// plan orders actions by target path, so that listing is also the execution order.
func applyFixture(t *testing.T) (home, repo string) {
	t.Helper()
	home = evalTempDir(t)
	repo = evalTempDir(t)
	xdg := filepath.Join(home, ".config-apply-fixture")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(xdg, "homux", "config.toml"), "profile = \"work\"\n")
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = [\"work\", \"personal\"]\n")

	writeFile(t, filepath.Join(repo, ".claude/settings.json@@work"), "{}\n")
	writeFile(t, filepath.Join(home, ".claude/settings.json"), "unmanaged\n")

	writeFile(t, filepath.Join(repo, ".config/foo/config"), "foo\n")

	symlink(t, filepath.Join(repo, ".config", "does-not-exist"), filepath.Join(home, ".config/orphan"))

	writeFile(t, filepath.Join(repo, ".vimrc"), "common vimrc\n")
	writeFile(t, filepath.Join(repo, ".vimrc@@work"), "work vimrc\n")
	symlink(t, filepath.Join(repo, ".vimrc"), filepath.Join(home, ".vimrc"))

	return home, repo
}

func wantApplyPlan(repo string) string {
	return "Would create symlink:\n" +
		"  ~/.config/foo/config\n" +
		"  -> " + repo + "/.config/foo/config\n" +
		"\n" +
		"Would ask before replacing:\n" +
		"  ~/.claude/settings.json\n" +
		"  -> " + repo + "/.claude/settings.json@@work\n" +
		"\n" +
		"Would relink:\n" +
		"  ~/.vimrc\n" +
		"  " + repo + "/.vimrc -> " + repo + "/.vimrc@@work\n" +
		"\n" +
		"Would remove stale symlink:\n" +
		"  ~/.config/orphan\n" +
		"\n"
}

func runApplyCmd(t *testing.T, repo string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := NewRootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"--repo", repo, "apply"}, args...))
	code = runRoot(cmd)
	return out.String(), errOut.String(), code
}

// runApplyInteractive drives runApply as if stdin and stdout were a terminal.
// The full CLI path can never reach the interactive branch under `go test`,
// because os.Stdin is not a TTY there.
func runApplyInteractive(t *testing.T, repo, stdin string, opts applyOptions) (stdout string, err error) {
	t.Helper()
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err = runApply(cmd, repo, opts, true)
	return out.String(), err
}

func assertSymlink(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("Readlink(%s): %v", path, err)
	}
	if got != want {
		t.Errorf("%s -> %s, want -> %s", path, got, want)
	}
}

func assertNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("%s exists, want absent (Lstat err = %v)", path, err)
	}
}

// contentSnapshot records every path under root together with regular file
// contents and symlink targets, so a test can assert nothing changed.
func contentSnapshot(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		switch {
		case d.Type()&os.ModeSymlink != 0:
			link, lerr := os.Readlink(path)
			if lerr != nil {
				return lerr
			}
			entries = append(entries, rel+" -> "+link)
		case d.IsDir():
			entries = append(entries, rel+"/")
		default:
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			entries = append(entries, rel+" = "+string(b))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(entries)
	return entries
}

func backupsOf(t *testing.T, path string) []string {
	t.Helper()
	matches, err := filepath.Glob(path + ".homux-bak.*")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	return matches
}

func TestApplyCmd_DryRunPrintsPlanAndChangesNothing(t *testing.T) {
	home, repo := applyFixture(t)
	before := contentSnapshot(t, home)

	stdout, stderr, code := runApplyCmd(t, repo, "--dry-run")

	if want := wantApplyPlan(repo) + "No changes made.\n"; stdout != want {
		t.Errorf("stdout:\ngot:\n%s\nwant:\n%s", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	if code != ExitOK {
		t.Errorf("exit code = %d, want %d", code, ExitOK)
	}
	if after := contentSnapshot(t, home); !slices.Equal(before, after) {
		t.Errorf("--dry-run modified HOME:\nbefore: %v\nafter:  %v", before, after)
	}
}

func TestApplyCmd_DryRunShorthand(t *testing.T) {
	_, repo := applyFixture(t)

	stdout, _, code := runApplyCmd(t, repo, "-n")

	if want := wantApplyPlan(repo) + "No changes made.\n"; stdout != want {
		t.Errorf("stdout:\ngot:\n%s\nwant:\n%s", stdout, want)
	}
	if code != ExitOK {
		t.Errorf("exit code = %d, want %d", code, ExitOK)
	}
}

func TestApplyCmd_YesAppliesEveryActionKind(t *testing.T) {
	home, repo := applyFixture(t)

	stdout, stderr, code := runApplyCmd(t, repo, "--yes")

	if want := wantApplyPlan(repo) + "Applied 4 changes.\n"; stdout != want {
		t.Errorf("stdout:\ngot:\n%s\nwant:\n%s", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	if code != ExitOK {
		t.Errorf("exit code = %d, want %d", code, ExitOK)
	}

	assertSymlink(t, filepath.Join(home, ".config/foo/config"), filepath.Join(repo, ".config/foo/config"))
	assertSymlink(t, filepath.Join(home, ".claude/settings.json"), filepath.Join(repo, ".claude/settings.json@@work"))
	assertSymlink(t, filepath.Join(home, ".vimrc"), filepath.Join(repo, ".vimrc@@work"))
	assertNotExist(t, filepath.Join(home, ".config/orphan"))
}

// INV-13: the unmanaged file must survive as a backup, byte for byte.
func TestApplyCmd_OccupiedTargetIsBackedUp(t *testing.T) {
	home, repo := applyFixture(t)

	if _, _, code := runApplyCmd(t, repo, "--yes"); code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}

	backups := backupsOf(t, filepath.Join(home, ".claude/settings.json"))
	if len(backups) != 1 {
		t.Fatalf("found %d backups, want 1: %v", len(backups), backups)
	}
	got, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", backups[0], err)
	}
	if string(got) != "unmanaged\n" {
		t.Errorf("backup content = %q, want %q", got, "unmanaged\n")
	}
}

// spec §11.2: apply changes HOME only.
func TestApplyCmd_DoesNotModifyRepoOrLocalConfig(t *testing.T) {
	home, repo := applyFixture(t)
	beforeRepo := contentSnapshot(t, repo)
	xdg := filepath.Join(home, ".config-apply-fixture")
	beforeCfg := contentSnapshot(t, xdg)

	if _, _, code := runApplyCmd(t, repo, "--yes"); code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}

	if after := contentSnapshot(t, repo); !slices.Equal(beforeRepo, after) {
		t.Errorf("apply modified the repository:\nbefore: %v\nafter:  %v", beforeRepo, after)
	}
	if after := contentSnapshot(t, xdg); !slices.Equal(beforeCfg, after) {
		t.Errorf("apply modified local config:\nbefore: %v\nafter:  %v", beforeCfg, after)
	}
}

// design §7.1: apply is idempotent; the second run is a no-op.
func TestApplyCmd_IsIdempotent(t *testing.T) {
	home, repo := applyFixture(t)

	if _, _, code := runApplyCmd(t, repo, "--yes"); code != ExitOK {
		t.Fatalf("first run exit code = %d, want %d", code, ExitOK)
	}
	after := contentSnapshot(t, home)

	stdout, _, code := runApplyCmd(t, repo, "--yes")

	if want := "No changes.\n"; stdout != want {
		t.Errorf("second run stdout:\ngot:\n%s\nwant:\n%s", stdout, want)
	}
	if code != ExitOK {
		t.Errorf("second run exit code = %d, want %d", code, ExitOK)
	}
	if again := contentSnapshot(t, home); !slices.Equal(after, again) {
		t.Errorf("second run changed HOME:\nbefore: %v\nafter:  %v", after, again)
	}
}

// spec §11.4: without --yes, a non-TTY run must not start the interactive UI.
func TestApplyCmd_NonTTYWithoutYesFailsWhenConfirmationIsNeeded(t *testing.T) {
	home, repo := applyFixture(t)
	before := contentSnapshot(t, home)

	stdout, stderr, code := runApplyCmd(t, repo)

	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stdout, "Would ask before replacing:") {
		t.Errorf("stdout should still show the plan:\n%s", stdout)
	}
	if !strings.Contains(stderr, "--yes") {
		t.Errorf("stderr should point at --yes:\n%s", stderr)
	}
	if after := contentSnapshot(t, home); !slices.Equal(before, after) {
		t.Errorf("failed run modified HOME:\nbefore: %v\nafter:  %v", before, after)
	}
}

// A plan with nothing to confirm needs no interaction, so a non-TTY run of it
// must succeed — otherwise re-running apply in CI could never converge.
func TestApplyCmd_NonTTYWithoutYesAppliesWhenNothingNeedsConfirmation(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config-nonttty"))
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = []\n")
	writeFile(t, filepath.Join(repo, ".zshrc"), "zshrc\n")

	stdout, _, code := runApplyCmd(t, repo)

	if code != ExitOK {
		t.Errorf("exit code = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(stdout, "Applied 1 change.") {
		t.Errorf("stdout:\n%s", stdout)
	}
	assertSymlink(t, filepath.Join(home, ".zshrc"), filepath.Join(repo, ".zshrc"))
}

// A no-op plan must not be an error in a non-TTY either (idempotent re-run).
func TestApplyCmd_NonTTYWithoutYesSucceedsOnNoOpPlan(t *testing.T) {
	_, repo := applyFixture(t)
	if _, _, code := runApplyCmd(t, repo, "--yes"); code != ExitOK {
		t.Fatalf("setup run exit code = %d, want %d", code, ExitOK)
	}

	stdout, _, code := runApplyCmd(t, repo)

	if code != ExitOK {
		t.Errorf("exit code = %d, want %d", code, ExitOK)
	}
	if want := "No changes.\n"; stdout != want {
		t.Errorf("stdout:\ngot:\n%s\nwant:\n%s", stdout, want)
	}
}

func TestApplyCmd_InteractiveYesAppliesConfirmedActions(t *testing.T) {
	home, repo := applyFixture(t)

	stdout, err := runApplyInteractive(t, repo, "y\ny\ny\n", applyOptions{})
	if err != nil {
		t.Fatalf("runApply: %v", err)
	}

	if !strings.Contains(stdout, "Replace it? [y/N]: ") {
		t.Errorf("stdout has no replace prompt:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Applied 4 changes.") {
		t.Errorf("stdout:\n%s", stdout)
	}
	assertSymlink(t, filepath.Join(home, ".claude/settings.json"), filepath.Join(repo, ".claude/settings.json@@work"))
	assertNotExist(t, filepath.Join(home, ".config/orphan"))
}

// INV-12: answering "n" means "skip this time"; it is not persisted, so the
// next run asks again and HOME is untouched.
func TestApplyCmd_InteractiveNoIsNotPersisted(t *testing.T) {
	home, repo := applyFixture(t)

	stdout, err := runApplyInteractive(t, repo, "n\nn\nn\n", applyOptions{})
	if err != nil {
		t.Fatalf("runApply: %v", err)
	}
	if !strings.Contains(stdout, "Applied 1 change.\nSkipped 3 changes (answered no).\n") {
		t.Errorf("stdout:\n%s", stdout)
	}

	// Only the Missing target (which needs no confirmation) was applied.
	assertSymlink(t, filepath.Join(home, ".config/foo/config"), filepath.Join(repo, ".config/foo/config"))
	assertSymlink(t, filepath.Join(home, ".vimrc"), filepath.Join(repo, ".vimrc"))
	assertSymlink(t, filepath.Join(home, ".config/orphan"), filepath.Join(repo, ".config/does-not-exist"))
	if backups := backupsOf(t, filepath.Join(home, ".claude/settings.json")); len(backups) != 0 {
		t.Errorf("declined replace left backups: %v", backups)
	}

	second, err := runApplyInteractive(t, repo, "n\nn\nn\n", applyOptions{})
	if err != nil {
		t.Fatalf("second runApply: %v", err)
	}
	if n := strings.Count(second, "[y/N]: "); n != 3 {
		t.Errorf("second run asked %d times, want 3:\n%s", n, second)
	}
}

// spec §11.3: a structural error in the repository means exit 1, even for a
// command that changes nothing.
func TestApplyCmd_DryRunExitsOneOnStructuralError(t *testing.T) {
	_, repo := statusFixture(t)

	stdout, _, code := runApplyCmd(t, repo, "--dry-run")

	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stdout, "ERROR ambiguous profile match") {
		t.Errorf("stdout has no diagnostic:\n%s", stdout)
	}
	if !strings.Contains(stdout, "No changes made.") {
		t.Errorf("stdout:\n%s", stdout)
	}
}

// spec §12.4: an Error target is skipped, the rest is still applied, and the
// run reports the skipped count and exits 1.
func TestApplyCmd_ErrorTargetsAreSkippedAndTheRestIsApplied(t *testing.T) {
	home, repo := statusFixture(t)

	stdout, _, code := runApplyCmd(t, repo, "--yes")

	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stdout, "1 target skipped due to errors.") {
		t.Errorf("stdout has no skipped count:\n%s", stdout)
	}
	assertSymlink(t, filepath.Join(home, ".config/foo/config"), filepath.Join(repo, ".config/foo/config"))
	assertNotExist(t, filepath.Join(home, ".gitconfig"))
}

// spec §12.4: on failure apply stops, reports what was applied and what was
// left, rolls nothing back, and exits 1.
func TestApplyCmd_PartialApplyReportsWhatWasLeft(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory permissions do not restrain root")
	}
	home := evalTempDir(t)
	repo := evalTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config-partial"))
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = []\n")
	writeFile(t, filepath.Join(repo, ".config/foo/config"), "foo\n")
	writeFile(t, filepath.Join(repo, ".zshrc"), "zshrc\n")

	// ~/.config is readable (so inspect can walk it) but not writable, so
	// creating ~/.config/foo fails. ".config/foo/config" sorts before ".zshrc",
	// so the second action is never attempted.
	readOnly := filepath.Join(home, ".config")
	if err := os.MkdirAll(readOnly, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(readOnly, 0o555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o755) })

	stdout, _, code := runApplyCmd(t, repo, "--yes")

	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stdout, "Applied 0 changes.") {
		t.Errorf("stdout has no applied count:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Failed:\n  ~/.config/foo/config\n") {
		t.Errorf("stdout has no failed target:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Not applied:\n  ~/.zshrc\n") {
		t.Errorf("stdout has no pending target:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Nothing was rolled back.") {
		t.Errorf("stdout has no re-run hint:\n%s", stdout)
	}
	assertNotExist(t, filepath.Join(home, ".zshrc"))
}
