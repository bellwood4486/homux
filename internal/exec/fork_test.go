package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fork は copy である。common source は残さなければならない。move にすると
// profile なしのマシンと他 profile のマシンが一斉に配置先を失う（spec §17.2）。
func TestForkAllCopiesAndKeepsCommon(t *testing.T) {
	repo := evalTempDir(t)
	common := filepath.Join(repo, ".gitconfig")
	fork := filepath.Join(repo, ".gitconfig@@work")
	writeFile(t, common, "[user]\n\tname = me\n")

	res := ForkAll([]ForkItem{{Common: common, Fork: fork}})

	if res.Err != nil {
		t.Fatalf("ForkAll: %v", res.Err)
	}
	if len(res.Applied) != 1 {
		t.Errorf("Applied = %d, want 1", len(res.Applied))
	}
	if got := readFile(t, fork); got != "[user]\n\tname = me\n" {
		t.Errorf("fork content = %q", got)
	}
	if got := readFile(t, common); got != "[user]\n\tname = me\n" {
		t.Errorf("common content = %q, want it left in place", got)
	}
}

// 深い階層の source でも、fork 先の親ディレクトリは common と同じである。
func TestForkAllNestedPath(t *testing.T) {
	repo := evalTempDir(t)
	common := filepath.Join(repo, ".claude", "settings.json")
	fork := filepath.Join(repo, ".claude", "settings.json@@work")
	writeFile(t, common, "{}\n")

	if res := ForkAll([]ForkItem{{Common: common, Fork: fork}}); res.Err != nil {
		t.Fatalf("ForkAll: %v", res.Err)
	}
	if got := readFile(t, fork); got != "{}\n" {
		t.Errorf("fork content = %q", got)
	}
}

// 既存の fork 先を黙って上書きしない。cli 側で事前検証しているが、
// os.Create は既存ファイルを切り詰めるため exec でも確かめる（addOne と同じ考え方）。
func TestForkAllDoesNotOverwrite(t *testing.T) {
	repo := evalTempDir(t)
	common := filepath.Join(repo, ".gitconfig")
	fork := filepath.Join(repo, ".gitconfig@@work")
	writeFile(t, common, "common\n")
	writeFile(t, fork, "precious\n")

	res := ForkAll([]ForkItem{{Common: common, Fork: fork}})

	if res.Err == nil {
		t.Fatal("ForkAll: want error, got nil")
	}
	if got := readFile(t, fork); got != "precious\n" {
		t.Errorf("fork content = %q, want it untouched", got)
	}
}

// 失敗した時点で止まり、手つかずの残りを Pending として報告する
// （AddAll と同じ部分適用の報告。ロールバックはしない）。
func TestForkAllReportsPending(t *testing.T) {
	repo := evalTempDir(t)
	writeFile(t, filepath.Join(repo, "a"), "a\n")
	writeFile(t, filepath.Join(repo, "b"), "b\n")
	writeFile(t, filepath.Join(repo, "c"), "c\n")
	writeFile(t, filepath.Join(repo, "b@@work"), "existing\n")

	items := []ForkItem{
		{Common: filepath.Join(repo, "a"), Fork: filepath.Join(repo, "a@@work")},
		{Common: filepath.Join(repo, "b"), Fork: filepath.Join(repo, "b@@work")},
		{Common: filepath.Join(repo, "c"), Fork: filepath.Join(repo, "c@@work")},
	}
	res := ForkAll(items)

	if res.Err == nil {
		t.Fatal("ForkAll: want error, got nil")
	}
	if len(res.Applied) != 1 || !strings.HasSuffix(res.Applied[0].Fork, "a@@work") {
		t.Errorf("Applied = %v, want [a@@work]", res.Applied)
	}
	if !strings.HasSuffix(res.Failed.Fork, "b@@work") {
		t.Errorf("Failed = %v, want b@@work", res.Failed)
	}
	if len(res.Pending) != 1 || !strings.HasSuffix(res.Pending[0].Fork, "c@@work") {
		t.Errorf("Pending = %v, want [c@@work]", res.Pending)
	}
	if _, statErr := os.Lstat(filepath.Join(repo, "c@@work")); statErr == nil {
		t.Error("c@@work was created after the failure")
	}
}
