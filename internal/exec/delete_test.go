package exec

import (
	"os"
	"path/filepath"
	"testing"
)

// 単一 profile suffix の source は削除され、複数 profile selector の source は
// 削除されず rewrite される（spec §12.11 の表）。
func TestDeleteAll_RemovesAndRewrites(t *testing.T) {
	repo := evalTempDir(t)
	remove := DeleteItem{Path: filepath.Join(repo, ".gitconfig@@work")}
	rewrite := DeleteItem{
		Path:    filepath.Join(repo, ".config", "foo", "config@@work+personal"),
		Rewrite: filepath.Join(repo, ".config", "foo", "config@@personal"),
	}
	writeFile(t, remove.Path, "git\n")
	writeFile(t, rewrite.Path, "foo\n")

	res := DeleteAll([]DeleteItem{rewrite, remove})

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if len(res.Applied) != 2 {
		t.Errorf("Applied = %d items, want 2", len(res.Applied))
	}
	if _, err := os.Lstat(remove.Path); err == nil {
		t.Errorf("%s still exists, want it removed", remove.Path)
	}
	if _, err := os.Lstat(rewrite.Path); err == nil {
		t.Errorf("%s still exists, want it rewritten", rewrite.Path)
	}
	if got := readFile(t, rewrite.Rewrite); got != "foo\n" {
		t.Errorf("content = %q, want %q", got, "foo\n")
	}
}

// cli 側が事前に全検証済みでも、検査と実行の間に割り込まれる余地は残る。
// exec 自身でも rewrite 先の存在を確かめ、黙って上書きしない（renameOne と同じ）。
func TestDeleteAll_RefusesToOverwriteExistingRewriteTarget(t *testing.T) {
	repo := evalTempDir(t)
	it := DeleteItem{
		Path:    filepath.Join(repo, "foo@@work+personal"),
		Rewrite: filepath.Join(repo, "foo@@personal"),
	}
	writeFile(t, it.Path, "both\n")
	writeFile(t, it.Rewrite, "already there\n")

	res := DeleteAll([]DeleteItem{it})

	if res.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if got := readFile(t, it.Rewrite); got != "already there\n" {
		t.Errorf("destination = %q, want it untouched", got)
	}
	if got := readFile(t, it.Path); got != "both\n" {
		t.Errorf("source = %q, want it untouched", got)
	}
}

// 途中で失敗したら、それ以降は手つかずのまま Pending に残す。
// ロールバックはしない（RenameAll / ForkAll と同じ方針）。
func TestDeleteAll_PartialFailureLeavesRestPending(t *testing.T) {
	repo := evalTempDir(t)
	first := DeleteItem{Path: filepath.Join(repo, "a@@work")}
	// 既に消えているものは削除できない。事前検証を通り抜けた状態変化に相当する。
	second := DeleteItem{Path: filepath.Join(repo, "gone@@work")}
	third := DeleteItem{Path: filepath.Join(repo, "c@@work")}
	writeFile(t, first.Path, "a\n")
	writeFile(t, third.Path, "c\n")

	res := DeleteAll([]DeleteItem{first, second, third})

	if res.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if len(res.Applied) != 1 || res.Applied[0] != first {
		t.Errorf("Applied = %v, want only %v", res.Applied, first)
	}
	if res.Failed != second {
		t.Errorf("Failed = %v, want %v", res.Failed, second)
	}
	if len(res.Pending) != 1 || res.Pending[0] != third {
		t.Errorf("Pending = %v, want only %v", res.Pending, third)
	}
	if got := readFile(t, third.Path); got != "c\n" {
		t.Errorf("pending item = %q, want it untouched", got)
	}
}

// HOME には一切触れない（spec §11.2）。repo 上の source を消しても、
// そこを指す HOME 側の symlink はそのまま（dangling）残る。
func TestDeleteAll_LeavesHomeUntouched(t *testing.T) {
	repo := evalTempDir(t)
	home := evalTempDir(t)
	source := filepath.Join(repo, ".gitconfig@@work")
	writeFile(t, source, "git\n")
	link := filepath.Join(home, ".gitconfig")
	symlink(t, source, link)

	if res := DeleteAll([]DeleteItem{{Path: source}}); res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}

	if got := readLink(t, link); got != source {
		t.Errorf("link = %q, want it still pointing at %q", got, source)
	}
}
