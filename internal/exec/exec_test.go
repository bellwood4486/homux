package exec

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bellwood4486/homux/internal/plan"
)

func TestApplyCreateSymlink(t *testing.T) {
	dir := evalTempDir(t)
	source := filepath.Join(dir, "repo", ".claude", "settings.json")
	target := filepath.Join(dir, "home", ".claude", "settings.json")
	writeFile(t, source, "from repo")

	res := Apply([]plan.Action{
		{Kind: plan.CreateSymlink, Target: target, LinkTo: source},
	}, nil)

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if len(res.Applied) != 1 {
		t.Fatalf("Applied = %d, want 1", len(res.Applied))
	}
	if got := readLink(t, target); got != source {
		t.Errorf("link = %s, want %s", got, source)
	}
	if got := readFile(t, target); got != "from repo" {
		t.Errorf("content = %q, want %q", got, "from repo")
	}
}

func TestApplyRelink(t *testing.T) {
	dir := evalTempDir(t)
	old := filepath.Join(dir, "repo", ".gitconfig")
	newSource := filepath.Join(dir, "repo", ".gitconfig@@work")
	target := filepath.Join(dir, "home", ".gitconfig")
	writeFile(t, old, "common")
	writeFile(t, newSource, "work")
	symlink(t, old, target)

	res := Apply([]plan.Action{
		{Kind: plan.Relink, Target: target, LinkTo: newSource, Confirm: true},
	}, nil)

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if got := readLink(t, target); got != newSource {
		t.Errorf("link = %s, want %s", got, newSource)
	}
}

// INV-14: 削除するのは symlink のみである。plan が壊れていても exec が最後に止める。
func TestApplyRelinkRefusesNonSymlink(t *testing.T) {
	dir := evalTempDir(t)
	source := filepath.Join(dir, "repo", ".gitconfig")
	target := filepath.Join(dir, "home", ".gitconfig")
	writeFile(t, source, "repo")
	writeFile(t, target, "precious")

	res := Apply([]plan.Action{
		{Kind: plan.Relink, Target: target, LinkTo: source, Confirm: true},
	}, nil)

	if res.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if got := readFile(t, target); got != "precious" {
		t.Errorf("content = %q, want %q", got, "precious")
	}
}

func TestApplyRemoveStaleSymlink(t *testing.T) {
	dir := evalTempDir(t)
	target := filepath.Join(dir, "home", ".vimrc")
	symlink(t, filepath.Join(dir, "repo", ".vimrc"), target)

	res := Apply([]plan.Action{
		{Kind: plan.RemoveStaleSymlink, Target: target, Confirm: true},
	}, nil)

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("Lstat err = %v, want IsNotExist", err)
	}
	// 空になった親ディレクトリは削除しない（spec §12.4）。
	if _, err := os.Stat(filepath.Dir(target)); err != nil {
		t.Errorf("parent dir removed: %v", err)
	}
}

// INV-14: repository 内の source file はもちろん、HOME 側でも symlink 以外は削除しない。
func TestApplyRemoveStaleSymlinkRefusesRegularFile(t *testing.T) {
	dir := evalTempDir(t)
	target := filepath.Join(dir, "home", ".vimrc")
	writeFile(t, target, "precious")

	res := Apply([]plan.Action{
		{Kind: plan.RemoveStaleSymlink, Target: target, Confirm: true},
	}, nil)

	if res.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if got := readFile(t, target); got != "precious" {
		t.Errorf("content = %q, want %q", got, "precious")
	}
}

// INV-13 / docs/design.md §7.1: Occupied の置換で退避ファイルが必ず作られること。
func TestApplyReplaceTargetLeavesBackup(t *testing.T) {
	dir := evalTempDir(t)
	source := filepath.Join(dir, "repo", ".claude", "settings.json")
	target := filepath.Join(dir, "home", ".claude", "settings.json")
	backup := target + ".homux-bak.20260905-153000"
	writeFile(t, source, "from repo")
	writeFile(t, target, "handwritten")

	res := Apply([]plan.Action{
		{Kind: plan.ReplaceTarget, Target: target, LinkTo: source, Backup: backup, Confirm: true},
	}, nil)

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if got := readFile(t, backup); got != "handwritten" {
		t.Errorf("backup content = %q, want %q", got, "handwritten")
	}
	if got := readLink(t, target); got != source {
		t.Errorf("link = %s, want %s", got, source)
	}
}

// spec §12.4: 退避先が既に存在する場合はエラーで停止する。target は変更しない。
func TestApplyReplaceTargetRefusesExistingBackup(t *testing.T) {
	dir := evalTempDir(t)
	source := filepath.Join(dir, "repo", ".claude", "settings.json")
	target := filepath.Join(dir, "home", ".claude", "settings.json")
	backup := target + ".homux-bak.20260905-153000"
	writeFile(t, source, "from repo")
	writeFile(t, target, "handwritten")
	writeFile(t, backup, "older backup")

	res := Apply([]plan.Action{
		{Kind: plan.ReplaceTarget, Target: target, LinkTo: source, Backup: backup, Confirm: true},
	}, nil)

	if res.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if got := readFile(t, target); got != "handwritten" {
		t.Errorf("target content = %q, want %q", got, "handwritten")
	}
	if got := readFile(t, backup); got != "older backup" {
		t.Errorf("backup content = %q, want %q", got, "older backup")
	}
}

// INV-13: Backup が空の ReplaceTarget は plan の破れである。黙って上書きしない。
func TestApplyReplaceTargetRefusesEmptyBackup(t *testing.T) {
	dir := evalTempDir(t)
	source := filepath.Join(dir, "repo", ".claude", "settings.json")
	target := filepath.Join(dir, "home", ".claude", "settings.json")
	writeFile(t, source, "from repo")
	writeFile(t, target, "handwritten")

	res := Apply([]plan.Action{
		{Kind: plan.ReplaceTarget, Target: target, LinkTo: source, Confirm: true},
	}, nil)

	if res.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if got := readFile(t, target); got != "handwritten" {
		t.Errorf("target content = %q, want %q", got, "handwritten")
	}
}

// spec §12.4: 途中で失敗したらその時点で停止し、
// 「ここまで適用済み / ここから未適用」を報告する。ロールバックはしない。
func TestApplyReportsPartialApplication(t *testing.T) {
	dir := evalTempDir(t)
	source := filepath.Join(dir, "repo", "src")
	writeFile(t, source, "repo")

	first := filepath.Join(dir, "home", "first")
	// 2 件目は symlink 以外を消そうとするので必ず失敗する。
	second := filepath.Join(dir, "home", "second")
	writeFile(t, second, "precious")
	third := filepath.Join(dir, "home", "third")

	actions := []plan.Action{
		{Kind: plan.CreateSymlink, Target: first, LinkTo: source},
		{Kind: plan.RemoveStaleSymlink, Target: second, Confirm: true},
		{Kind: plan.CreateSymlink, Target: third, LinkTo: source},
	}
	res := Apply(actions, nil)

	if res.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if len(res.Applied) != 1 || res.Applied[0].Target != first {
		t.Errorf("Applied = %v, want [%s]", res.Applied, first)
	}
	if res.Failed.Target != second {
		t.Errorf("Failed.Target = %s, want %s", res.Failed.Target, second)
	}
	if len(res.Pending) != 1 || res.Pending[0].Target != third {
		t.Errorf("Pending = %v, want [%s]", res.Pending, third)
	}
	if _, err := os.Lstat(third); !os.IsNotExist(err) {
		t.Errorf("third was applied after the failure: %v", err)
	}
	// ロールバックしない。1 件目はそのまま残る。
	if got := readLink(t, first); got != source {
		t.Errorf("first link = %s, want %s", got, source)
	}
}

// spec §12.4 / INV-12: n を選んだ target は変更されず、その選択は保存されない。
// スキップは失敗ではないので残りの適用は続行する。
func TestApplySkipsDeclinedActionAndContinues(t *testing.T) {
	dir := evalTempDir(t)
	source := filepath.Join(dir, "repo", "src")
	writeFile(t, source, "repo")

	declined := filepath.Join(dir, "home", "declined")
	writeFile(t, declined, "handwritten")
	accepted := filepath.Join(dir, "home", "accepted")

	actions := []plan.Action{
		{
			Kind:    plan.ReplaceTarget,
			Target:  declined,
			LinkTo:  source,
			Backup:  declined + ".homux-bak.20260905-153000",
			Confirm: true,
		},
		{Kind: plan.CreateSymlink, Target: accepted, LinkTo: source},
	}
	res := Apply(actions, func(a plan.Action) (bool, error) {
		return false, nil
	})

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Target != declined {
		t.Errorf("Skipped = %v, want [%s]", res.Skipped, declined)
	}
	if got := readFile(t, declined); got != "handwritten" {
		t.Errorf("declined content = %q, want %q", got, "handwritten")
	}
	if _, err := os.Lstat(declined + ".homux-bak.20260905-153000"); !os.IsNotExist(err) {
		t.Errorf("backup created for a declined action: %v", err)
	}
	if len(res.Applied) != 1 || res.Applied[0].Target != accepted {
		t.Errorf("Applied = %v, want [%s]", res.Applied, accepted)
	}
}

// spec §12.4: Missing だけが確認不要である。Confirm が false の Action は問わない。
func TestApplyDoesNotAskForActionsThatNeedNoConfirmation(t *testing.T) {
	dir := evalTempDir(t)
	source := filepath.Join(dir, "repo", "src")
	target := filepath.Join(dir, "home", "target")
	writeFile(t, source, "repo")

	asked := 0
	res := Apply([]plan.Action{
		{Kind: plan.CreateSymlink, Target: target, LinkTo: source},
	}, func(a plan.Action) (bool, error) {
		asked++
		return false, nil
	})

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if asked != 0 {
		t.Errorf("asked = %d, want 0", asked)
	}
	if len(res.Applied) != 1 {
		t.Errorf("Applied = %d, want 1", len(res.Applied))
	}
}

// confirm が nil なら確認を要する Action もそのまま実行する（--yes 相当）。
func TestApplyWithoutConfirmFuncAppliesEverything(t *testing.T) {
	dir := evalTempDir(t)
	source := filepath.Join(dir, "repo", "src")
	target := filepath.Join(dir, "home", "target")
	backup := target + ".homux-bak.20260905-153000"
	writeFile(t, source, "repo")
	writeFile(t, target, "handwritten")

	res := Apply([]plan.Action{
		{Kind: plan.ReplaceTarget, Target: target, LinkTo: source, Backup: backup, Confirm: true},
	}, nil)

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if got := readFile(t, backup); got != "handwritten" {
		t.Errorf("backup content = %q, want %q", got, "handwritten")
	}
}

// confirm 自体の失敗（端末が閉じた等）は停止であり、スキップではない。
func TestApplyStopsWhenConfirmFails(t *testing.T) {
	dir := evalTempDir(t)
	source := filepath.Join(dir, "repo", "src")
	target := filepath.Join(dir, "home", "target")
	pending := filepath.Join(dir, "home", "pending")
	writeFile(t, source, "repo")
	symlink(t, source, target)

	actions := []plan.Action{
		{Kind: plan.RemoveStaleSymlink, Target: target, Confirm: true},
		{Kind: plan.CreateSymlink, Target: pending, LinkTo: source},
	}
	res := Apply(actions, func(a plan.Action) (bool, error) {
		return false, errors.New("no tty")
	})

	if res.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if res.Failed.Target != target {
		t.Errorf("Failed.Target = %s, want %s", res.Failed.Target, target)
	}
	if len(res.Pending) != 1 || res.Pending[0].Target != pending {
		t.Errorf("Pending = %v, want [%s]", res.Pending, pending)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Errorf("target removed after a failed confirmation: %v", err)
	}
}
