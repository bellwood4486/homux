package exec

import (
	"os"
	"path/filepath"
	"testing"
)

// 通常の rename。中身は動かさず名前だけが変わる。
func TestRenameAll_RenamesEveryItem(t *testing.T) {
	repo := evalTempDir(t)
	items := []RenameItem{
		{From: filepath.Join(repo, ".gitconfig@@work"), To: filepath.Join(repo, ".gitconfig@@company")},
		{From: filepath.Join(repo, ".claude", "settings.json@@work"), To: filepath.Join(repo, ".claude", "settings.json@@company")},
	}
	writeFile(t, items[0].From, "git\n")
	writeFile(t, items[1].From, "claude\n")

	res := RenameAll(items)

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if len(res.Applied) != 2 {
		t.Errorf("Applied = %d items, want 2", len(res.Applied))
	}
	for _, it := range items {
		if _, err := os.Lstat(it.From); err == nil {
			t.Errorf("%s still exists", it.From)
		}
	}
	if got := readFile(t, items[0].To); got != "git\n" {
		t.Errorf("content = %q, want %q", got, "git\n")
	}
	if got := readFile(t, items[1].To); got != "claude\n" {
		t.Errorf("content = %q, want %q", got, "claude\n")
	}
}

// cli 側が事前に全検証済みでも（INV-15）、検査と rename の間に割り込まれる
// 余地は残る。exec 自身でも宛先の存在を確かめ、黙って上書きしない
// （forkOne / addOne と同じ考え方）。
func TestRenameAll_RefusesToOverwriteExistingFile(t *testing.T) {
	repo := evalTempDir(t)
	it := RenameItem{
		From: filepath.Join(repo, ".gitconfig@@work"),
		To:   filepath.Join(repo, ".gitconfig@@company"),
	}
	writeFile(t, it.From, "work\n")
	writeFile(t, it.To, "already there\n")

	res := RenameAll([]RenameItem{it})

	if res.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if got := readFile(t, it.To); got != "already there\n" {
		t.Errorf("destination = %q, want it untouched", got)
	}
	if got := readFile(t, it.From); got != "work\n" {
		t.Errorf("source = %q, want it untouched", got)
	}
}

// 途中で失敗したら、それ以降は手つかずのまま Pending に残す。
// ロールバックはしない（AddAll / ForkAll と同じ方針）。
func TestRenameAll_PartialFailureLeavesRestPending(t *testing.T) {
	repo := evalTempDir(t)
	first := RenameItem{
		From: filepath.Join(repo, "a@@work"),
		To:   filepath.Join(repo, "a@@company"),
	}
	writeFile(t, first.From, "a\n")

	// From が存在しないため rename が失敗する。
	broken := RenameItem{
		From: filepath.Join(repo, "missing@@work"),
		To:   filepath.Join(repo, "missing@@company"),
	}
	last := RenameItem{
		From: filepath.Join(repo, "z@@work"),
		To:   filepath.Join(repo, "z@@company"),
	}
	writeFile(t, last.From, "z\n")

	res := RenameAll([]RenameItem{first, broken, last})

	if res.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if len(res.Applied) != 1 || res.Applied[0] != first {
		t.Errorf("Applied = %v, want only %v", res.Applied, first)
	}
	if res.Failed != broken {
		t.Errorf("Failed = %v, want %v", res.Failed, broken)
	}
	if len(res.Pending) != 1 || res.Pending[0] != last {
		t.Errorf("Pending = %v, want only %v", res.Pending, last)
	}
	if _, err := os.Lstat(last.To); err == nil {
		t.Error("the pending item was renamed after the failure")
	}
}
