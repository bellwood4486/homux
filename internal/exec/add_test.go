package exec

import (
	"path/filepath"
	"testing"
)

// spec §12.6: 「repo 側に同名 source が既に存在する」はエラーであり、黙って
// 上書きしない。cli/add の事前検査を信用せず exec 自身でも確かめる
// （removeSymlink が INV-14 を守るのと同じ考え方）。
func TestAddAll_RefusesToOverwriteExistingRepoFile(t *testing.T) {
	dir := evalTempDir(t)
	target := filepath.Join(dir, "home", ".zshrc")
	repoPath := filepath.Join(dir, "repo", ".zshrc")
	writeFile(t, target, "new")
	writeFile(t, repoPath, "already there")

	res := AddAll([]AddItem{{Target: target, RepoPath: repoPath}})

	if res.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if got := readFile(t, repoPath); got != "already there" {
		t.Errorf("repo content = %q, want unchanged %q", got, "already there")
	}
	if got := readFile(t, target); got != "new" {
		t.Errorf("home content = %q, want unchanged %q", got, "new")
	}
}

// 複数件のうち途中で失敗したら、それ以降は手つかずのまま Pending に残す
// （apply の部分適用と同じ考え方。spec §12.4 相当）。
func TestAddAll_PartialFailureLeavesRestPending(t *testing.T) {
	dir := evalTempDir(t)
	ok := AddItem{
		Target:   filepath.Join(dir, "home", ".zshrc"),
		RepoPath: filepath.Join(dir, "repo", ".zshrc"),
	}
	writeFile(t, ok.Target, "zshrc")

	// このターゲットは存在しないため os.Rename が失敗する。
	missing := AddItem{
		Target:   filepath.Join(dir, "home", ".does-not-exist"),
		RepoPath: filepath.Join(dir, "repo", ".does-not-exist"),
	}

	untouched := AddItem{
		Target:   filepath.Join(dir, "home", ".vimrc"),
		RepoPath: filepath.Join(dir, "repo", ".vimrc"),
	}
	writeFile(t, untouched.Target, "vimrc")

	res := AddAll([]AddItem{ok, missing, untouched})

	if res.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if len(res.Applied) != 1 || res.Applied[0] != ok {
		t.Errorf("Applied = %v, want [%v]", res.Applied, ok)
	}
	if res.Failed != missing {
		t.Errorf("Failed = %v, want %v", res.Failed, missing)
	}
	if len(res.Pending) != 1 || res.Pending[0] != untouched {
		t.Errorf("Pending = %v, want [%v]", res.Pending, untouched)
	}
	if got := readFile(t, untouched.Target); got != "vimrc" {
		t.Errorf("untouched item was modified: content = %q", got)
	}
}

func TestAddAll_MovesFileAndCreatesSymlink(t *testing.T) {
	dir := evalTempDir(t)
	target := filepath.Join(dir, "home", ".zshrc")
	repoPath := filepath.Join(dir, "repo", ".zshrc")
	writeFile(t, target, "my zshrc")

	res := AddAll([]AddItem{{Target: target, RepoPath: repoPath}})

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if len(res.Applied) != 1 {
		t.Fatalf("Applied = %d, want 1", len(res.Applied))
	}
	if got := readFile(t, repoPath); got != "my zshrc" {
		t.Errorf("repo content = %q, want %q", got, "my zshrc")
	}
	if got := readLink(t, target); got != repoPath {
		t.Errorf("link = %s, want %s", got, repoPath)
	}
}
