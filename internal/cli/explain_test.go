package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runExplainCmd(t *testing.T, repo string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := NewRootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	full := append([]string{"--repo", repo, "explain"}, args...)
	cmd.SetArgs(full)
	code = runRoot(cmd)
	return out.String(), errOut.String(), code
}

func TestExplainCmd_Stale_HomeSideArg(t *testing.T) {
	home, repo := statusFixture(t)

	stdout, _, code := runExplainCmd(t, repo, filepath.Join(home, ".vimrc"))

	want := "Target:\n  ~/.vimrc\n\n" +
		"Active profile:\n  work\n\n" +
		"Candidates:\n" +
		"  .vimrc  (not selected: a profile-specific source matches the active profile)\n" +
		"  .vimrc@@work  (selected)\n" +
		"\n" +
		"Selected:\n" +
		"  .vimrc@@work\n" +
		"\n" +
		"Reason:\n  profile-specific source matches the active profile\n\n" +
		"Current:\n" +
		"  ~/.vimrc\n" +
		"  -> " + filepath.Join(repo, ".vimrc") + "\n" +
		"\n" +
		"State:\n  stale\n" +
		"\n" +
		"Would apply:\n  relink to " + filepath.Join(repo, ".vimrc@@work") + "\n"

	if stdout != want {
		t.Fatalf("stdout mismatch.\ngot:\n%s\nwant:\n%s", stdout, want)
	}
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
}

func TestExplainCmd_RepoSideArg_SameTargetAsHomeSideArg(t *testing.T) {
	home, repo := statusFixture(t)

	wantStdout, _, _ := runExplainCmd(t, repo, filepath.Join(home, ".vimrc"))
	gotStdout, _, code := runExplainCmd(t, repo, filepath.Join(repo, ".vimrc@@work"))

	if gotStdout != wantStdout {
		t.Fatalf("repo-side arg produced different output.\ngot:\n%s\nwant:\n%s", gotStdout, wantStdout)
	}
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
}

// TestExplainCmd_RepoSideArg_ThroughSymlinkedAncestor は macOS の
// /tmp -> /private/tmp のように、引数の途中の祖先が symlink であっても
// repo 側と判定できることを確認する。loadWorkspace は e.Repo を
// filepath.EvalSymlinks 済みにするが、ユーザーが打つ引数はそうとは限らない。
func TestExplainCmd_RepoSideArg_ThroughSymlinkedAncestor(t *testing.T) {
	home, repo := statusFixture(t)

	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(filepath.Dir(repo), alias); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	aliasedArg := filepath.Join(alias, filepath.Base(repo), ".vimrc@@work")

	wantStdout, _, _ := runExplainCmd(t, repo, filepath.Join(home, ".vimrc"))
	gotStdout, _, code := runExplainCmd(t, repo, aliasedArg)

	if gotStdout != wantStdout {
		t.Fatalf("aliased repo-side arg produced different output.\ngot:\n%s\nwant:\n%s", gotStdout, wantStdout)
	}
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
}

func TestExplainCmd_Missing_ShowsCreateSymlink(t *testing.T) {
	home, repo := statusFixture(t)

	stdout, _, code := runExplainCmd(t, repo, filepath.Join(home, ".config/foo/config"))

	want := "Target:\n  ~/.config/foo/config\n\n" +
		"Active profile:\n  work\n\n" +
		"Candidates:\n" +
		"  .config/foo/config  (selected)\n" +
		"\n" +
		"Selected:\n" +
		"  .config/foo/config\n" +
		"\n" +
		"Reason:\n  no profile-specific source matches; using the common source\n\n" +
		"Current:\n" +
		"  ~/.config/foo/config\n" +
		"\n" +
		"State:\n  missing\n" +
		"\n" +
		"Would apply:\n  create symlink to " + filepath.Join(repo, ".config/foo/config") + "\n"

	if stdout != want {
		t.Fatalf("stdout mismatch.\ngot:\n%s\nwant:\n%s", stdout, want)
	}
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
}

func TestExplainCmd_Linked_NoWouldApplySection(t *testing.T) {
	home, repo := statusFixture(t)

	stdout, _, code := runExplainCmd(t, repo, filepath.Join(home, ".zshrc"))

	want := "Target:\n  ~/.zshrc\n\n" +
		"Active profile:\n  work\n\n" +
		"Candidates:\n" +
		"  .zshrc  (selected)\n" +
		"\n" +
		"Selected:\n" +
		"  .zshrc\n" +
		"\n" +
		"Reason:\n  no profile-specific source matches; using the common source\n\n" +
		"Current:\n" +
		"  ~/.zshrc\n" +
		"  -> " + filepath.Join(repo, ".zshrc") + "\n" +
		"\n" +
		"State:\n  linked\n"

	if stdout != want {
		t.Fatalf("stdout mismatch.\ngot:\n%s\nwant:\n%s", stdout, want)
	}
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
}

func TestExplainCmd_Inactive_ShowsNonMatchingCandidateReason(t *testing.T) {
	home, repo := statusFixture(t)

	stdout, _, code := runExplainCmd(t, repo, filepath.Join(home, ".config/old/config"))

	want := "Target:\n  ~/.config/old/config\n\n" +
		"Active profile:\n  work\n\n" +
		"Candidates:\n" +
		"  .config/old/config@@personal  (not selected: does not match active profile \"work\")\n" +
		"\n" +
		"Selected:\n" +
		"  (none)\n" +
		"\n" +
		"Reason:\n  no source is available for this target\n\n" +
		"Current:\n" +
		"  ~/.config/old/config\n" +
		"\n" +
		"State:\n  inactive\n"

	if stdout != want {
		t.Fatalf("stdout mismatch.\ngot:\n%s\nwant:\n%s", stdout, want)
	}
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
}

func TestExplainCmd_OrphanStaleSymlink_ShowsRemove(t *testing.T) {
	home, repo := statusFixture(t)

	stdout, _, code := runExplainCmd(t, repo, filepath.Join(home, ".config/orphan"))

	want := "Target:\n  ~/.config/orphan\n\n" +
		"Active profile:\n  work\n\n" +
		"Candidates:\n" +
		"  (none)\n" +
		"\n" +
		"Selected:\n" +
		"  (none)\n" +
		"\n" +
		"Reason:\n  no source is available for this target\n\n" +
		"Current:\n" +
		"  ~/.config/orphan\n" +
		"  -> " + filepath.Join(repo, ".config", "does-not-exist") + "\n" +
		"\n" +
		"State:\n  stale\n" +
		"\n" +
		"Would apply:\n  remove stale symlink\n"

	if stdout != want {
		t.Fatalf("stdout mismatch.\ngot:\n%s\nwant:\n%s", stdout, want)
	}
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
}

// TestExplainCmd_Occupied_ShowsReplaceWithBackup は Backup パスに timestamp
// を含むため、exact match ではなく変化しない部分だけを確認する。
func TestExplainCmd_Occupied_ShowsReplaceWithBackup(t *testing.T) {
	home, repo := statusFixture(t)

	stdout, _, code := runExplainCmd(t, repo, filepath.Join(home, ".claude/settings.json"))

	for _, want := range []string{
		"Candidates:\n  .claude/settings.json@@work  (selected)\n\n",
		"State:\n  occupied\n",
		"Would apply:\n  replace target (backup to ~/.claude/settings.json.homux-bak.",
		"), then link to " + filepath.Join(repo, ".claude/settings.json@@work") + "\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q; full output:\n%s", want, stdout)
		}
	}
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
}

func TestExplainCmd_Ambiguous_ShowsSpecDiagnostic(t *testing.T) {
	home, repo := statusFixture(t)

	stdout, _, code := runExplainCmd(t, repo, filepath.Join(home, ".gitconfig"))

	want := "Target:\n  ~/.gitconfig\n\n" +
		"ERROR ambiguous profile match\n\n" +
		"  Target:\n    ~/.gitconfig\n\n" +
		"  Matching sources:\n" +
		"    .gitconfig@@work\n" +
		"    .gitconfig@@work+personal\n"

	if stdout != want {
		t.Fatalf("stdout mismatch.\ngot:\n%s\nwant:\n%s", stdout, want)
	}
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d", code, ExitError)
	}
}

func TestExplainCmd_IgnoredSource_ExitsOne(t *testing.T) {
	_, repo := statusFixture(t)

	stdout, stderr, code := runExplainCmd(t, repo, filepath.Join(repo, "README.md"))

	wantErr := "Error: README.md is ignored by .homux.toml and has no target\n"
	if stderr != wantErr {
		t.Fatalf("stderr = %q, want %q", stderr, wantErr)
	}
	if stdout != "" {
		t.Fatalf("expected no stdout, got %q", stdout)
	}
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d", code, ExitError)
	}
}

func TestExplainCmd_UnmanagedRepoFile_ExitsOne(t *testing.T) {
	_, repo := statusFixture(t)

	_, stderr, code := runExplainCmd(t, repo, filepath.Join(repo, ".homux.toml"))

	wantErr := "Error: .homux.toml: not a managed source in the repository\n"
	if stderr != wantErr {
		t.Fatalf("stderr = %q, want %q", stderr, wantErr)
	}
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d", code, ExitError)
	}
}

func TestExplainCmd_OutsideHomeAndRepo_ExitsUsageError(t *testing.T) {
	_, repo := statusFixture(t)

	_, stderr, code := runExplainCmd(t, repo, "/nonexistent-explain-test-root/foo")

	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
	if stderr == "" {
		t.Fatal("expected an error message on stderr")
	}
}

func TestExplainCmd_UnknownActiveProfile_ExitsOneWithDiagnostic(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	xdg := filepath.Join(home, ".config-active")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = [\"work\"]\n")
	writeFile(t, filepath.Join(xdg, "homux", "config.toml"), "profile = \"worc\"\n")

	cmd := NewRootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--repo", repo, "explain", filepath.Join(home, ".zshrc")})

	code := runRoot(cmd)

	wantErr := "ERROR active profile\n\n  Unknown profile \"worc\".\n  Did you mean \"work\"?\n"
	if errOut.String() != wantErr {
		t.Fatalf("stderr = %q, want %q", errOut.String(), wantErr)
	}
	if out.String() != "" {
		t.Fatalf("expected no stdout, got %q", out.String())
	}
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d", code, ExitError)
	}
}

func TestExplainCmd_DoesNotModifyHomeOrRepo(t *testing.T) {
	home, repo := statusFixture(t)

	homeBefore := snapshotTree(t, home)
	repoBefore := snapshotTree(t, repo)

	runExplainCmd(t, repo, filepath.Join(home, ".vimrc"))

	homeAfter := snapshotTree(t, home)
	repoAfter := snapshotTree(t, repo)

	if len(homeBefore) != len(homeAfter) || len(repoBefore) != len(repoAfter) {
		t.Fatalf("entry count changed: home %d->%d, repo %d->%d",
			len(homeBefore), len(homeAfter), len(repoBefore), len(repoAfter))
	}
	for path, before := range homeBefore {
		if after := homeAfter[path]; after != before {
			t.Errorf("home entry %q changed: before=%+v after=%+v", path, before, after)
		}
	}
	for path, before := range repoBefore {
		if after := repoAfter[path]; after != before {
			t.Errorf("repo entry %q changed: before=%+v after=%+v", path, before, after)
		}
	}
}
