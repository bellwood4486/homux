package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/bellwood4486/homux/internal/config"
)

func runProfileCmd(t *testing.T, repo string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := NewRootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	full := append([]string{"--repo", repo, "profile"}, args...)
	cmd.SetArgs(full)
	code = runRoot(cmd)
	return out.String(), errOut.String(), code
}

func TestProfileListCmd_ShowsProfilesAndActive(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	xdg := filepath.Join(home, ".config-profile-list")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = [\"work\", \"personal\"]\n")
	writeFile(t, filepath.Join(xdg, "homux", "config.toml"), "profile = \"work\"\n")

	stdout, stderr, code := runProfileCmd(t, repo, "list")

	want := "Profiles:\n\n" +
		"  personal\n" +
		"* work\n\n" +
		"Active profile: work\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
}

func TestProfileListCmd_DoesNotModifyAnything(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	xdg := filepath.Join(home, ".config-profile-list-ro")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = [\"work\"]\n")
	localPath := filepath.Join(xdg, "homux", "config.toml")
	writeFile(t, localPath, "profile = \"work\"\n")

	homeBefore := snapshotTree(t, home)
	repoBefore := snapshotTree(t, repo)

	runProfileCmd(t, repo, "list")

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

func TestProfileUseCmd_SwitchesActiveProfile(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	xdg := filepath.Join(home, ".config-profile-use")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = [\"work\", \"personal\"]\n")
	localPath := filepath.Join(xdg, "homux", "config.toml")
	writeFile(t, localPath, "repo = \""+repo+"\"\nprofile = \"personal\"\n")

	stdout, stderr, code := runProfileCmd(t, repo, "use", "work")

	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
	want := "Active profile: personal -> work\n\n" +
		"HOME already matches this profile.\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}

	local, err := config.LoadLocal(localPath)
	if err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}
	if local.Profile != "work" {
		t.Errorf("local.Profile = %q, want work", local.Profile)
	}
}

func TestProfileUseCmd_ShowsApplyHintWhenHomeDiffers(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	xdg := filepath.Join(home, ".config-profile-use-diff")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = [\"work\", \"personal\"]\n")
	writeFile(t, filepath.Join(repo, ".vimrc@@work"), "work vimrc\n")
	localPath := filepath.Join(xdg, "homux", "config.toml")
	writeFile(t, localPath, "repo = \""+repo+"\"\nprofile = \"personal\"\n")

	stdout, _, code := runProfileCmd(t, repo, "use", "work")

	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
	want := "Active profile: personal -> work\n\n" +
		"Run \"homux apply\" to update HOME.\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestProfileUseCmd_SwitchingToCurrentProfile_IsNoOp(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	xdg := filepath.Join(home, ".config-profile-use-same")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = [\"work\"]\n")
	localPath := filepath.Join(xdg, "homux", "config.toml")
	writeFile(t, localPath, "repo = \""+repo+"\"\nprofile = \"work\"\n")

	stdout, stderr, code := runProfileCmd(t, repo, "use", "work")

	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
	want := "Active profile: work -> work\n\n" +
		"HOME already matches this profile.\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}

	local, err := config.LoadLocal(localPath)
	if err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}
	if local.Profile != "work" {
		t.Errorf("local.Profile = %q, want work", local.Profile)
	}
}

func TestProfileUseCmd_UnknownProfile_ExitsOneWithDiagnostic(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	xdg := filepath.Join(home, ".config-profile-use-unknown")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = [\"work\"]\n")
	localPath := filepath.Join(xdg, "homux", "config.toml")
	writeFile(t, localPath, "repo = \""+repo+"\"\nprofile = \"work\"\n")

	stdout, stderr, code := runProfileCmd(t, repo, "use", "worc")

	if code != ExitError {
		t.Fatalf("exit code = %d, want %d", code, ExitError)
	}
	wantErr := "ERROR active profile\n\n  Unknown profile \"worc\".\n  Did you mean \"work\"?\n"
	if stderr != wantErr {
		t.Fatalf("stderr = %q, want %q", stderr, wantErr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}

	local, err := config.LoadLocal(localPath)
	if err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}
	if local.Profile != "work" {
		t.Errorf("local.Profile changed to %q, want unchanged \"work\"", local.Profile)
	}
}

func TestProfileUseCmd_DoesNotImplicitlyCreateProfile(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	xdg := filepath.Join(home, ".config-profile-use-noimplicit")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = []\n")
	localPath := filepath.Join(xdg, "homux", "config.toml")
	writeFile(t, localPath, "repo = \""+repo+"\"\n")

	_, _, code := runProfileCmd(t, repo, "use", "newprofile")

	if code != ExitError {
		t.Fatalf("exit code = %d, want %d", code, ExitError)
	}

	repoCfg, err := config.LoadRepo(filepath.Join(repo, config.RepoFileName))
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	if len(repoCfg.Profiles) != 0 {
		t.Errorf("Profiles = %v, want empty (use must not create profiles)", repoCfg.Profiles)
	}
}
