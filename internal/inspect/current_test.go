package inspect

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestReadCurrent_Absent(t *testing.T) {
	repo, home := evalTempDir(t), evalTempDir(t)

	got, err := ReadCurrent(repo, filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatalf("ReadCurrent: %v", err)
	}
	if got.Kind != CurrentAbsent {
		t.Errorf("Kind = %v, want CurrentAbsent", got.Kind)
	}
}

func TestReadCurrent_RegularFile(t *testing.T) {
	repo, home := evalTempDir(t), evalTempDir(t)
	writeFile(t, filepath.Join(home, ".zshrc"), "x")

	got, err := ReadCurrent(repo, filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatalf("ReadCurrent: %v", err)
	}
	if got.Kind != CurrentFile {
		t.Errorf("Kind = %v, want CurrentFile", got.Kind)
	}
}

func TestReadCurrent_Directory(t *testing.T) {
	repo, home := evalTempDir(t), evalTempDir(t)
	mkdirAll(t, filepath.Join(home, ".claude"))

	got, err := ReadCurrent(repo, filepath.Join(home, ".claude"))
	if err != nil {
		t.Fatalf("ReadCurrent: %v", err)
	}
	if got.Kind != CurrentDir {
		t.Errorf("Kind = %v, want CurrentDir", got.Kind)
	}
}

func TestReadCurrent_ManagedSymlinkKeepsRawLink(t *testing.T) {
	base := evalTempDir(t)
	repo := filepath.Join(base, "dotfiles")
	home := filepath.Join(base, "home")
	writeFile(t, filepath.Join(repo, ".zshrc"), "x")

	target := filepath.Join(home, ".zshrc")
	symlink(t, "../dotfiles/.zshrc", target)

	got, err := ReadCurrent(repo, target)
	if err != nil {
		t.Fatalf("ReadCurrent: %v", err)
	}
	if got.Kind != CurrentSymlink {
		t.Fatalf("Kind = %v, want CurrentSymlink", got.Kind)
	}
	if got.Link != "../dotfiles/.zshrc" {
		t.Errorf("Link = %q, want %q", got.Link, "../dotfiles/.zshrc")
	}
	if want := filepath.Join(repo, ".zshrc"); got.LinkAbs != want {
		t.Errorf("LinkAbs = %q, want %q", got.LinkAbs, want)
	}
	if !got.Managed {
		t.Error("Managed = false, want true")
	}
}

// TestReadCurrent_DanglingSymlinkIntoRepoIsManaged は design.md §7.1 の必須項目である。
func TestReadCurrent_DanglingSymlinkIntoRepoIsManaged(t *testing.T) {
	base := evalTempDir(t)
	repo := filepath.Join(base, "dotfiles")
	home := filepath.Join(base, "home")
	mkdirAll(t, repo)

	target := filepath.Join(home, ".zshrc")
	symlink(t, filepath.Join(repo, ".zshrc"), target) // リンク先は存在しない

	got, err := ReadCurrent(repo, target)
	if err != nil {
		t.Fatalf("ReadCurrent: %v", err)
	}
	if got.Kind != CurrentSymlink {
		t.Fatalf("Kind = %v, want CurrentSymlink", got.Kind)
	}
	if !got.Managed {
		t.Error("Managed = false, want true (ADR 0003: dangling でも repo 配下なら managed)")
	}
}

func TestReadCurrent_SymlinkOutsideRepoIsUnmanaged(t *testing.T) {
	base := evalTempDir(t)
	repo := filepath.Join(base, "dotfiles")
	home := filepath.Join(base, "home")
	mkdirAll(t, repo)
	writeFile(t, filepath.Join(base, "elsewhere", ".zshrc"), "x")

	target := filepath.Join(home, ".zshrc")
	symlink(t, filepath.Join(base, "elsewhere", ".zshrc"), target)

	got, err := ReadCurrent(repo, target)
	if err != nil {
		t.Fatalf("ReadCurrent: %v", err)
	}
	if got.Managed {
		t.Error("Managed = true, want false")
	}
}

// TestReadCurrent_AncestorIsRegularFile は、target の祖先が通常ファイルのとき
// 専用のエラーを返すことを確認する。Occupied の退避フローでは解決できないため、
// 判定表 #2 で Error として扱う。
func TestReadCurrent_AncestorIsRegularFile(t *testing.T) {
	repo, home := evalTempDir(t), evalTempDir(t)
	writeFile(t, filepath.Join(home, ".config"), "not a directory")

	_, err := ReadCurrent(repo, filepath.Join(home, ".config", "foo", "config"))

	var notDir *AncestorNotDirError
	if !errors.As(err, &notDir) {
		t.Fatalf("err = %v, want *AncestorNotDirError", err)
	}
	if want := filepath.Join(home, ".config"); notDir.Path != want {
		t.Errorf("Path = %q, want %q", notDir.Path, want)
	}
}
