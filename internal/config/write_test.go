package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLocalRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "homux", LocalFileName)

	if err := SaveLocal(path, &Local{Repo: "/srv/dotfiles", Profile: "work"}); err != nil {
		t.Fatalf("SaveLocal: %v", err)
	}

	got, err := LoadLocal(path)
	if err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}
	if got.Repo != "/srv/dotfiles" {
		t.Errorf("Repo = %q, want %q", got.Repo, "/srv/dotfiles")
	}
	if got.Profile != "work" {
		t.Errorf("Profile = %q, want %q", got.Profile, "work")
	}
}

// profile なしは profile キーの不在で表す（docs/design.md §4.2）。
// profile = "" を書き残すと「予約文字列を使わない」という決定が曖昧になる。
func TestSaveLocalOmitsEmptyProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), LocalFileName)

	if err := SaveLocal(path, &Local{Repo: "/srv/dotfiles"}); err != nil {
		t.Fatalf("SaveLocal: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if want := "repo = '/srv/dotfiles'\n"; string(b) != want {
		t.Errorf("content = %q, want %q", b, want)
	}
}

func TestSaveLocalOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), LocalFileName)

	if err := SaveLocal(path, &Local{Repo: "/old", Profile: "work"}); err != nil {
		t.Fatalf("SaveLocal (first): %v", err)
	}
	if err := SaveLocal(path, &Local{Repo: "/new"}); err != nil {
		t.Fatalf("SaveLocal (second): %v", err)
	}

	got, err := LoadLocal(path)
	if err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}
	if got.Repo != "/new" {
		t.Errorf("Repo = %q, want %q", got.Repo, "/new")
	}
	if got.Profile != "" {
		t.Errorf("Profile = %q, want empty", got.Profile)
	}
}

// 雛形は「コメント込みで人間が読んで理解できる」こと（INV-10）。
// LoadRepo で読み戻せることまでを担保する。
func TestCreateRepoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), RepoFileName)

	if err := CreateRepoFile(path); err != nil {
		t.Fatalf("CreateRepoFile: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(b)
	for _, want := range []string{`# "README.md"`, `# "LICENSE"`, `# "docs/**"`, "profiles = []"} {
		if !strings.Contains(content, want) {
			t.Errorf("template does not contain %q:\n%s", want, content)
		}
	}

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

// 既存の .homux.toml を壊さないこと（受け入れ条件）。
func TestCreateRepoFileDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	const existing = "profiles = [\"work\"]\n"
	path := writeFile(t, dir, RepoFileName, existing)

	err := CreateRepoFile(path)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("CreateRepoFile err = %v, want fs.ErrExist", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != existing {
		t.Errorf("content = %q, want %q", b, existing)
	}
}
