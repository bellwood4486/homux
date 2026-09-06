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

// 範囲置換の核心は「profiles 以外に触らないこと」である（ADR 0008）。
// init の雛形が持つコメントと ignore セクションがそのまま残ることを確かめる。
func TestReplaceProfilesKeepsEverythingElse(t *testing.T) {
	path := filepath.Join(t.TempDir(), RepoFileName)
	if err := CreateRepoFile(path); err != nil {
		t.Fatalf("CreateRepoFile: %v", err)
	}

	if err := ReplaceProfiles(path, []string{"work"}); err != nil {
		t.Fatalf("ReplaceProfiles: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(b)

	want := `# homux repository configuration (spec §8)

# 利用可能な profile。homux profile create で追加する。
profiles = [
  "work",
]

# 配置対象から外すパス。repo ルートからの相対 glob で、** を含められる。
ignore = [
  # "README.md",
  # "LICENSE",
  # "docs/**",
]
`
	if got != want {
		t.Errorf("content =\n%s\nwant\n%s", got, want)
	}
}

// 置換後の内容が LoadRepo で読み戻せることまで確かめる。
func TestReplaceProfilesRoundTrip(t *testing.T) {
	path := writeFile(t, t.TempDir(), RepoFileName, "profiles = [\"work\"]\n\nignore = [\"README.md\"]\n")

	if err := ReplaceProfiles(path, []string{"work", "personal"}); err != nil {
		t.Fatalf("ReplaceProfiles: %v", err)
	}

	repo, err := LoadRepo(path)
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	if len(repo.Profiles) != 2 || repo.Profiles[0] != "work" || repo.Profiles[1] != "personal" {
		t.Errorf("Profiles = %v, want [work personal]", repo.Profiles)
	}
	if len(repo.Ignore) != 1 || repo.Ignore[0] != "README.md" {
		t.Errorf("Ignore = %v, want [README.md]", repo.Ignore)
	}
}

// 空にしたときは 1 行の [] に畳む（init の雛形と同じ形）。
func TestReplaceProfilesEmpty(t *testing.T) {
	path := writeFile(t, t.TempDir(), RepoFileName, "profiles = [\n  \"work\",\n]\nignore = []\n")

	if err := ReplaceProfiles(path, nil); err != nil {
		t.Fatalf("ReplaceProfiles: %v", err)
	}

	got := readFile(t, path)
	want := "profiles = []\nignore = []\n"
	if got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

// 配列内部のコメントに含まれる "]" で閉じ括弧を誤検出しない。
// コメント自体が失われるのは既知の制限である（spec §15）。
func TestReplaceProfilesIgnoresBracketsInsideCommentsAndStrings(t *testing.T) {
	path := writeFile(t, t.TempDir(), RepoFileName, "profiles = [\n  # not the end ]\n  \"a]b\",\n]\nignore = []\n")

	if err := ReplaceProfiles(path, []string{"work"}); err != nil {
		t.Fatalf("ReplaceProfiles: %v", err)
	}

	got := readFile(t, path)
	want := "profiles = [\n  \"work\",\n]\nignore = []\n"
	if got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

// profiles キーが無いファイルは書き換えようがない。1 バイトも書かずに落とす。
func TestReplaceProfilesWithoutKey(t *testing.T) {
	const original = "ignore = [\"README.md\"]\n"
	path := writeFile(t, t.TempDir(), RepoFileName, original)

	if err := ReplaceProfiles(path, []string{"work"}); err == nil {
		t.Fatal("ReplaceProfiles: want error, got nil")
	}
	if got := readFile(t, path); got != original {
		t.Errorf("content = %q, want it untouched %q", got, original)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(b)
}
