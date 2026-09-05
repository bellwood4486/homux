package inspect

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

func mkdirAll(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", d, err)
		}
	}
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
