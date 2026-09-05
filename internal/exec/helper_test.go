package exec

import (
	"os"
	"path/filepath"
	"testing"
)

// evalTempDir は t.TempDir() を EvalSymlinks したものを返す。
// macOS の /var -> /private/var のような経路差でテストが偽陽性にならないようにする。
func evalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	return dir
}

func symlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(newname), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(newname), err)
	}
	if err := os.Symlink(oldname, newname); err != nil {
		t.Fatalf("Symlink %s -> %s: %v", newname, oldname, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

// readLink は path が symlink であることを確かめ、その生のリンク先を返す。
func readLink(t *testing.T, path string) string {
	t.Helper()
	link, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("Readlink %s: %v", path, err)
	}
	return link
}

// readFile は path の内容を返す。symlink は辿られる。
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return string(b)
}
