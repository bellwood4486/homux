package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLoadRepo(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, RepoFileName, `
profiles = [
  "work",
  "personal",
]

ignore = [
  "README.md",
  "docs/**",
]
`)

	repo, err := LoadRepo(path)
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	if got, want := repo.Profiles, []string{"work", "personal"}; !equalStrings(got, want) {
		t.Errorf("Profiles = %v, want %v", got, want)
	}
	if got, want := repo.Ignore, []string{"README.md", "docs/**"}; !equalStrings(got, want) {
		t.Errorf("Ignore = %v, want %v", got, want)
	}
}

func TestLoadRepo_Empty(t *testing.T) {
	// profiles も ignore も無い .homux.toml は妥当である（profile なしの運用）。
	dir := t.TempDir()
	path := writeFile(t, dir, RepoFileName, "")

	repo, err := LoadRepo(path)
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	if len(repo.Profiles) != 0 {
		t.Errorf("Profiles = %v, want empty", repo.Profiles)
	}
	if len(repo.Ignore) != 0 {
		t.Errorf("Ignore = %v, want empty", repo.Ignore)
	}
}

func TestLoadRepo_Missing(t *testing.T) {
	_, err := LoadRepo(filepath.Join(t.TempDir(), RepoFileName))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want fs.ErrNotExist", err)
	}
}

func TestLoadRepo_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"broken toml", `profiles = [`},
		{"profiles is not an array", `profiles = "work"`},
		{"invalid profile name", `profiles = ["Work"]`},
		{"empty profile name", `profiles = [""]`},
		{"duplicate profile", `profiles = ["work", "work"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, t.TempDir(), RepoFileName, tt.content)
			if _, err := LoadRepo(path); err == nil {
				t.Fatalf("LoadRepo(%q): expected error, got nil", tt.content)
			}
		})
	}
}

func TestRepo_HasProfile(t *testing.T) {
	repo := &Repo{Profiles: []string{"work", "personal"}}
	if !repo.HasProfile("work") {
		t.Error(`HasProfile("work") = false, want true`)
	}
	if repo.HasProfile("worq") {
		t.Error(`HasProfile("worq") = true, want false`)
	}
}

func TestLoadLocal(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, LocalFileName, `
repo = "/home/u/dotfiles"
profile = "work"
`)

	local, err := LoadLocal(path)
	if err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}
	if local.Repo != "/home/u/dotfiles" {
		t.Errorf("Repo = %q, want %q", local.Repo, "/home/u/dotfiles")
	}
	if local.Profile != "work" {
		t.Errorf("Profile = %q, want %q", local.Profile, "work")
	}
}

func TestLoadLocal_NoProfile(t *testing.T) {
	// profile キーの不在が「profile なし」を意味する。
	// profile = "" も同じ意味として寛容に読む（design.md §4.2）。
	tests := []struct {
		name    string
		content string
	}{
		{"key absent", `repo = "/home/u/dotfiles"`},
		{"empty value", "repo = \"/home/u/dotfiles\"\nprofile = \"\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, t.TempDir(), LocalFileName, tt.content)
			local, err := LoadLocal(path)
			if err != nil {
				t.Fatalf("LoadLocal: %v", err)
			}
			if local.Profile != "" {
				t.Errorf("Profile = %q, want %q", local.Profile, "")
			}
		})
	}
}

func TestLoadLocal_Missing(t *testing.T) {
	_, err := LoadLocal(filepath.Join(t.TempDir(), LocalFileName))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want fs.ErrNotExist", err)
	}
}

func TestLoadLocal_InvalidProfileName(t *testing.T) {
	path := writeFile(t, t.TempDir(), LocalFileName, `profile = "Work"`)
	if _, err := LoadLocal(path); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLocalPath(t *testing.T) {
	tests := []struct {
		name          string
		xdgConfigHome string
		home          string
		want          string
	}{
		{
			name:          "XDG_CONFIG_HOME set",
			xdgConfigHome: "/xdg",
			home:          "/home/u",
			want:          filepath.Join("/xdg", "homux", LocalFileName),
		},
		{
			name:          "XDG_CONFIG_HOME unset falls back to ~/.config",
			xdgConfigHome: "",
			home:          "/home/u",
			want:          filepath.Join("/home/u", ".config", "homux", LocalFileName),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LocalPath(tt.xdgConfigHome, tt.home); got != tt.want {
				t.Errorf("LocalPath(%q, %q) = %q, want %q", tt.xdgConfigHome, tt.home, got, tt.want)
			}
		})
	}
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
