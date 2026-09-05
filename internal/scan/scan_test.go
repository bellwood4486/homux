package scan

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bellwood4486/homux/internal/selector"
)

// mkRepo は relative path -> content のマップから repository を作る。
func mkRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return root
}

func repoPaths(sources []Source) []string {
	out := make([]string, len(sources))
	for i, s := range sources {
		out[i] = s.RepoPath
	}
	return out
}

func TestRepository_SourcesAndTargets(t *testing.T) {
	root := mkRepo(t, map[string]string{
		".homux.toml":                 "profiles = [\"work\"]\n",
		".zshrc":                      "",
		".claude/settings.json":       "",
		".claude/settings.json@@work": "",
		".config/ghostty/config":      "",
	})

	res, err := Repository(root, nil)
	if err != nil {
		t.Fatalf("Repository: %v", err)
	}

	// .homux.toml は常に暗黙除外される（spec §8.2）。
	want := []string{
		".claude/settings.json",
		".claude/settings.json@@work",
		".config/ghostty/config",
		".zshrc",
	}
	if got := repoPaths(res.Sources); !equalStrings(got, want) {
		t.Fatalf("RepoPaths = %v, want %v", got, want)
	}

	// suffix は target path に含まれない（spec §4、INV-16）。
	byPath := map[string]Source{}
	for _, s := range res.Sources {
		byPath[s.RepoPath] = s
	}
	if got := byPath[".claude/settings.json@@work"].Target; got != ".claude/settings.json" {
		t.Errorf("Target = %q, want %q", got, ".claude/settings.json")
	}
	if got := byPath[".claude/settings.json"].Target; got != ".claude/settings.json" {
		t.Errorf("Target = %q, want %q", got, ".claude/settings.json")
	}
	if sel := byPath[".claude/settings.json@@work"].Selector; sel == nil || len(sel.Profiles) != 1 || sel.Profiles[0] != "work" {
		t.Errorf("Selector = %v, want [work]", sel)
	}
	if sel := byPath[".zshrc"].Selector; sel != nil {
		t.Errorf(".zshrc Selector = %v, want nil (common source)", sel)
	}
}

func TestRepository_ExcludesGitDir(t *testing.T) {
	root := mkRepo(t, map[string]string{
		".git/config":     "",
		".git/objects/ab": "",
		".zshrc":          "",
	})

	res, err := Repository(root, nil)
	if err != nil {
		t.Fatalf("Repository: %v", err)
	}
	if got, want := repoPaths(res.Sources), []string{".zshrc"}; !equalStrings(got, want) {
		t.Errorf("RepoPaths = %v, want %v", got, want)
	}
	if len(res.Ignored) != 0 {
		t.Errorf("Ignored = %v, want empty (暗黙除外は ignore ではない)", res.Ignored)
	}
}

func TestRepository_AppliesIgnore(t *testing.T) {
	root := mkRepo(t, map[string]string{
		"README.md":       "",
		"LICENSE":         "",
		"docs/spec.md":    "",
		"docs/adr/001.md": "",
		".zshrc":          "",
	})

	res, err := Repository(root, []string{"README.md", "docs/**"})
	if err != nil {
		t.Fatalf("Repository: %v", err)
	}

	want := []string{"LICENSE", ".zshrc"}
	got := repoPaths(res.Sources)
	if !sameSet(got, want) {
		t.Errorf("RepoPaths = %v, want %v", got, want)
	}
	wantIgnored := []string{"README.md", "docs/adr/001.md", "docs/spec.md"}
	if !sameSet(res.Ignored, wantIgnored) {
		t.Errorf("Ignored = %v, want %v", res.Ignored, wantIgnored)
	}
}

func TestRepository_SingleAtIsCommonSource(t *testing.T) {
	// design.md §7.1: "@@" を含まない "@" 付きファイル名は common source である。
	root := mkRepo(t, map[string]string{
		".config/systemd/user/tunnel@.service": "",
		".gitconfig@2024":                      "",
	})

	res, err := Repository(root, nil)
	if err != nil {
		t.Fatalf("Repository: %v", err)
	}
	for _, s := range res.Sources {
		if s.Selector != nil {
			t.Errorf("%s: Selector = %v, want nil", s.RepoPath, s.Selector)
		}
		if s.SelectorErr != nil {
			t.Errorf("%s: SelectorErr = %v, want nil", s.RepoPath, s.SelectorErr)
		}
		if s.Target != s.RepoPath {
			t.Errorf("%s: Target = %q, want %q", s.RepoPath, s.Target, s.RepoPath)
		}
	}
}

func TestRepository_InvalidSelectorIsCarried(t *testing.T) {
	// 構文エラーは scan では落とさず、診断できる resolve まで運ぶ（spec §10.2）。
	root := mkRepo(t, map[string]string{
		"foo@@work++personal": "",
		"bar@@!server":        "",
	})

	res, err := Repository(root, nil)
	if err != nil {
		t.Fatalf("Repository: %v", err)
	}
	if len(res.Sources) != 2 {
		t.Fatalf("len(Sources) = %d, want 2", len(res.Sources))
	}
	byPath := map[string]Source{}
	for _, s := range res.Sources {
		byPath[s.RepoPath] = s
	}
	if err := byPath["foo@@work++personal"].SelectorErr; !errors.Is(err, selector.ErrEmptySelector) {
		t.Errorf("SelectorErr = %v, want ErrEmptySelector", err)
	}
	if got := byPath["foo@@work++personal"].Target; got != "foo" {
		t.Errorf("Target = %q, want %q", got, "foo")
	}
	if err := byPath["bar@@!server"].SelectorErr; !errors.Is(err, selector.ErrNegativeSelector) {
		t.Errorf("SelectorErr = %v, want ErrNegativeSelector", err)
	}
}

func TestRepository_MissingRoot(t *testing.T) {
	if _, err := Repository(filepath.Join(t.TempDir(), "nope"), nil); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepository_InvalidIgnorePattern(t *testing.T) {
	root := mkRepo(t, map[string]string{".zshrc": ""})
	if _, err := Repository(root, []string{"["}); err == nil {
		t.Fatal("expected error for malformed pattern, got nil")
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

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		m[s]--
	}
	for _, n := range m {
		if n != 0 {
			return false
		}
	}
	return true
}
