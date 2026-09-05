package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// statusFixture builds a repo + home that together exercise every
// inspect.StateKind at once, with active profile "work":
//
//	.zshrc                                  -> Linked
//	.config/foo/config                      -> Missing
//	.claude/settings.json(@@work)           -> Occupied (unmanaged file in HOME)
//	.vimrc / .vimrc@@work                   -> Stale (type 1: linked to the wrong source)
//	.config/orphan (HOME-only symlink)      -> Stale (type 2: orphaned managed symlink)
//	.config/old/config@@personal            -> Inactive (active profile is "work")
//	README.md (ignored)                     -> Ignored
//	.gitconfig@@work + .gitconfig@@work+personal -> Error (ambiguous)
func statusFixture(t *testing.T) (home, repo string) {
	t.Helper()
	home = evalTempDir(t)
	repo = evalTempDir(t)
	xdg := filepath.Join(home, ".config-status-fixture")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(xdg, "homux", "config.toml"), "profile = \"work\"\n")

	writeFile(t, filepath.Join(repo, ".homux.toml"),
		"profiles = [\"work\", \"personal\"]\nignore = [\"README.md\"]\n")
	writeFile(t, filepath.Join(repo, "README.md"), "readme\n")

	// Linked
	writeFile(t, filepath.Join(repo, ".zshrc"), "zshrc\n")
	symlink(t, filepath.Join(repo, ".zshrc"), filepath.Join(home, ".zshrc"))

	// Missing: source exists, no HOME entry.
	writeFile(t, filepath.Join(repo, ".config/foo/config"), "foo\n")

	// Occupied: HOME has an unmanaged regular file.
	writeFile(t, filepath.Join(repo, ".claude/settings.json@@work"), "{}\n")
	writeFile(t, filepath.Join(home, ".claude/settings.json"), "unmanaged\n")

	// Stale (type 1): HOME links to the common source, but "work" selects .vimrc@@work.
	writeFile(t, filepath.Join(repo, ".vimrc"), "common vimrc\n")
	writeFile(t, filepath.Join(repo, ".vimrc@@work"), "work vimrc\n")
	symlink(t, filepath.Join(repo, ".vimrc"), filepath.Join(home, ".vimrc"))

	// Inactive: personal-only source, active profile is "work", no HOME entry.
	writeFile(t, filepath.Join(repo, ".config/old/config@@personal"), "old\n")

	// Stale (type 2): orphaned managed symlink under a repo top-level dir (.config).
	symlink(t, filepath.Join(repo, ".config", "does-not-exist"), filepath.Join(home, ".config/orphan"))

	// Error: ambiguous (.gitconfig@@work and .gitconfig@@work+personal both match "work").
	writeFile(t, filepath.Join(repo, ".gitconfig@@work"), "a\n")
	writeFile(t, filepath.Join(repo, ".gitconfig@@work+personal"), "b\n")

	return home, repo
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
}

func runStatusCmd(t *testing.T, repo string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := NewRootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	full := append([]string{"--repo", repo, "status"}, args...)
	cmd.SetArgs(full)
	code = runRoot(cmd)
	return out.String(), errOut.String(), code
}

func TestStatusCmd_DefaultView(t *testing.T) {
	_, repo := statusFixture(t)

	stdout, _, code := runStatusCmd(t, repo)

	want := "Profile: work\n\n" +
		"Occupied   ~/.claude/settings.json\n" +
		"Missing    ~/.config/foo/config\n" +
		"Stale      ~/.config/orphan\n" +
		"Error      ~/.gitconfig\n" +
		"Stale      ~/.vimrc\n\n" +
		"4 changes pending, 1 error\n\n" +
		"ERROR ambiguous profile match\n\n" +
		"  Target:\n    ~/.gitconfig\n\n" +
		"  Matching sources:\n" +
		"    .gitconfig@@work\n" +
		"    .gitconfig@@work+personal\n"
	if stdout != want {
		t.Fatalf("stdout mismatch.\ngot:\n%s\nwant:\n%s", stdout, want)
	}
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d (ambiguous is a structural error)", code, ExitError)
	}
}

func TestStatusCmd_All_IncludesLinkedIgnoredInactive(t *testing.T) {
	_, repo := statusFixture(t)

	stdout, _, _ := runStatusCmd(t, repo, "--all")

	for _, want := range []string{
		"Linked     ~/.zshrc\n",
		"Ignored    README.md\n",
		"Inactive   ~/.config/old/config\n",
	} {
		if !bytes.Contains([]byte(stdout), []byte(want)) {
			t.Errorf("stdout missing %q; full output:\n%s", want, stdout)
		}
	}
}

func TestStatusCmd_Verbose_ShowsSource(t *testing.T) {
	_, repo := statusFixture(t)

	stdout, _, _ := runStatusCmd(t, repo, "--verbose")

	want := "           source: .config/foo/config\n"
	if !bytes.Contains([]byte(stdout), []byte(want)) {
		t.Errorf("stdout missing %q; full output:\n%s", want, stdout)
	}
}

func TestStatusCmd_NoDrift_ExitsZeroWithNoChanges(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config-unused"))
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = []\n")
	writeFile(t, filepath.Join(repo, ".zshrc"), "zshrc\n")
	symlink(t, filepath.Join(repo, ".zshrc"), filepath.Join(home, ".zshrc"))

	stdout, _, code := runStatusCmd(t, repo)

	want := "Profile: (none)\n\nNo changes.\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
}

func TestStatusCmd_UnknownActiveProfile_ExitsOneWithDiagnostic(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	xdg := filepath.Join(home, ".config-active")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = [\"work\"]\n")
	// local config supplies the (invalid) active profile; --repo still overrides the repo path.
	writeFile(t, filepath.Join(xdg, "homux", "config.toml"), "profile = \"worc\"\n")

	cmd := NewRootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--repo", repo, "status"})

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

func TestStatusCmd_NotConfigured_ExitsOne(t *testing.T) {
	home := evalTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config-empty"))

	cmd := NewRootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"status"})

	code := runRoot(cmd)

	if code != ExitError {
		t.Fatalf("exit code = %d, want %d", code, ExitError)
	}
	if errOut.String() == "" {
		t.Fatal("expected an error message on stderr")
	}
}

// snapshotEntry is the subset of file metadata that must stay identical
// across a status run for the run to be considered read-only.
type snapshotEntry struct {
	mode    os.FileMode
	size    int64
	modTime int64
}

func snapshotTree(t *testing.T, root string) map[string]snapshotEntry {
	t.Helper()
	got := map[string]snapshotEntry{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		got[rel] = snapshotEntry{mode: info.Mode(), size: info.Size(), modTime: info.ModTime().UnixNano()}
		return nil
	})
	if err != nil {
		t.Fatalf("filepath.Walk(%s): %v", root, err)
	}
	return got
}

func TestStatusCmd_DoesNotModifyHomeOrRepo(t *testing.T) {
	home, repo := statusFixture(t)

	homeBefore := snapshotTree(t, home)
	repoBefore := snapshotTree(t, repo)

	runStatusCmd(t, repo, "--all", "--verbose")

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
