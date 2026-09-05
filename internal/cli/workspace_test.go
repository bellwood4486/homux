package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// evalTempDir は t.TempDir() を EvalSymlinks したものを返す。
// macOS では /tmp が /private/tmp への symlink であるため、比較の基準を
// 実パスに揃える（internal/inspect/helper_test.go と同じ理由）。
func evalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestLoadWorkspace_RepoFlagOverridesLocalConfig(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config-unused"))
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = [\"work\"]\n")

	ws, err := loadWorkspace(repo)
	if err != nil {
		t.Fatalf("loadWorkspace: %v", err)
	}
	if ws.env.Home != home {
		t.Errorf("Home = %q, want %q", ws.env.Home, home)
	}
	if ws.env.Repo != repo {
		t.Errorf("Repo = %q, want %q", ws.env.Repo, repo)
	}
	if ws.profile != "" {
		t.Errorf("profile = %q, want empty (no local config)", ws.profile)
	}
	if len(ws.repo.Profiles) != 1 || ws.repo.Profiles[0] != "work" {
		t.Errorf("repo.Profiles = %v, want [work]", ws.repo.Profiles)
	}
}

func TestLoadWorkspace_FallsBackToLocalConfig(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t)
	xdg := filepath.Join(home, ".config-xdg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(repo, ".homux.toml"), "profiles = [\"work\", \"personal\"]\n")
	writeFile(t, filepath.Join(xdg, "homux", "config.toml"),
		"repo = \""+repo+"\"\nprofile = \"work\"\n")

	ws, err := loadWorkspace("")
	if err != nil {
		t.Fatalf("loadWorkspace: %v", err)
	}
	if ws.env.Repo != repo {
		t.Errorf("Repo = %q, want %q", ws.env.Repo, repo)
	}
	if ws.profile != "work" {
		t.Errorf("profile = %q, want work", ws.profile)
	}
}

func TestLoadWorkspace_ExpandsTildeInLocalConfigRepo(t *testing.T) {
	home := evalTempDir(t)
	xdg := filepath.Join(home, ".config-xdg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(home, "dotfiles", ".homux.toml"), "profiles = []\n")
	writeFile(t, filepath.Join(xdg, "homux", "config.toml"), "repo = \"~/dotfiles\"\n")

	ws, err := loadWorkspace("")
	if err != nil {
		t.Fatalf("loadWorkspace: %v", err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(home, "dotfiles"))
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if ws.env.Repo != want {
		t.Errorf("Repo = %q, want %q", ws.env.Repo, want)
	}
}

func TestLoadWorkspace_NotConfigured(t *testing.T) {
	home := evalTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config-empty"))

	_, err := loadWorkspace("")
	if err == nil {
		t.Fatal("expected an error when no --repo and no local config repo is set")
	}
}

func TestLoadWorkspace_MissingHomuxToml(t *testing.T) {
	home := evalTempDir(t)
	repo := evalTempDir(t) // no .homux.toml written
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config-empty"))

	_, err := loadWorkspace(repo)
	if err == nil {
		t.Fatal("expected an error when repo has no .homux.toml")
	}
}
