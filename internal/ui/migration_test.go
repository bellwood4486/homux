package ui

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/bellwood4486/homux/internal/exec"
)

// spec §12.9 の plan レイアウトをそのまま出す。common のまま残るものと
// fork するものを分けて見せることが、確認の意味を持たせる唯一の方法である。
func TestRenderMigrationPlan(t *testing.T) {
	var buf bytes.Buffer

	RenderMigrationPlan(&buf, ColorOff, MigrationPlan{
		Profile:     "work",
		KeepTargets: []string{".zshrc", ".config/ghostty/config"},
		Forks: []ForkLine{
			{Common: ".gitconfig", Fork: ".gitconfig@@work"},
			{Common: ".claude/settings.json", Fork: ".claude/settings.json@@work"},
		},
	})

	want := "Profile migration plan: work\n\n" +
		"Keep common:\n" +
		"  ~/.zshrc\n" +
		"  ~/.config/ghostty/config\n\n" +
		"Fork:\n" +
		"  .gitconfig             -> .gitconfig@@work\n" +
		"  .claude/settings.json  -> .claude/settings.json@@work\n\n"
	if got := buf.String(); got != want {
		t.Errorf("output =\n%s\nwant\n%s", got, want)
	}
}

// 何も fork しない選択も正当である（spec §12.9:「既定では common のまま」）。
func TestRenderMigrationPlanWithoutForks(t *testing.T) {
	var buf bytes.Buffer

	RenderMigrationPlan(&buf, ColorOff, MigrationPlan{Profile: "work", KeepTargets: []string{".zshrc"}})

	want := "Profile migration plan: work\n\n" +
		"Keep common:\n" +
		"  ~/.zshrc\n\n" +
		"Fork:\n" +
		"  (none)\n\n"
	if got := buf.String(); got != want {
		t.Errorf("output =\n%s\nwant\n%s", got, want)
	}
}

// common source を持たない target は fork のしようがない。黙って消すのではなく
// 件数を示す（INV-10: CLI なしでも状況が理解できること）。
func TestRenderMigrationPlanReportsSkipped(t *testing.T) {
	var buf bytes.Buffer

	RenderMigrationPlan(&buf, ColorOff, MigrationPlan{
		Profile:         "work",
		KeepTargets:     []string{".zshrc"},
		SkippedNoCommon: 2,
	})

	want := "Profile migration plan: work\n\n" +
		"Keep common:\n" +
		"  ~/.zshrc\n\n" +
		"Fork:\n" +
		"  (none)\n\n" +
		"2 targets have no common source and cannot be forked here.\n\n"
	if got := buf.String(); got != want {
		t.Errorf("output =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderMigrationResult(t *testing.T) {
	repo := filepath.FromSlash("/srv/dotfiles")
	var buf bytes.Buffer

	RenderMigrationResult(&buf, ColorOff, repo, "work", exec.ForkResult{
		Applied: []exec.ForkItem{
			{Common: filepath.Join(repo, ".gitconfig"), Fork: filepath.Join(repo, ".gitconfig@@work")},
		},
	})

	want := "Created profile \"work\".\n" +
		"Forked 1 file.\n\n" +
		"Run \"homux profile use work\" on machines that should use it, " +
		"then \"homux apply\" to update HOME.\n"
	if got := buf.String(); got != want {
		t.Errorf("output =\n%s\nwant\n%s", got, want)
	}
}

// 部分適用はロールバックしない。どこまで進んだかを repo 相対パスで示す
// （RenderAddResult と同じ方針）。
func TestRenderMigrationResultPartial(t *testing.T) {
	repo := filepath.FromSlash("/srv/dotfiles")
	var buf bytes.Buffer

	RenderMigrationResult(&buf, ColorOff, repo, "work", exec.ForkResult{
		Applied: []exec.ForkItem{
			{Common: filepath.Join(repo, "a"), Fork: filepath.Join(repo, "a@@work")},
		},
		Failed:  exec.ForkItem{Common: filepath.Join(repo, "b"), Fork: filepath.Join(repo, "b@@work")},
		Pending: []exec.ForkItem{{Common: filepath.Join(repo, "c"), Fork: filepath.Join(repo, "c@@work")}},
		Err:     errBoom,
	})

	want := "Created profile \"work\".\n" +
		"Forked 1 file before failing.\n\n" +
		"Failed:\n" +
		"  b@@work\n" +
		"  boom\n\n" +
		"Not forked:\n" +
		"  c@@work\n"
	if got := buf.String(); got != want {
		t.Errorf("output =\n%s\nwant\n%s", got, want)
	}
}
