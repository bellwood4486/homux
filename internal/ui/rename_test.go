package ui

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bellwood4486/homux/internal/exec"
)

// spec §12.10 の plan レイアウトをそのまま出す。単一 suffix と複数 profile
// selector を分けて見せるのは、後者が「置換であって削除ではない」ことを
// 確認の場で示すためである。矢印は両方の節を通して同じ桁に揃える。
func TestRenderRenamePlan(t *testing.T) {
	var buf bytes.Buffer

	RenderRenamePlan(&buf, ColorOff, RenamePlan{
		From: "work",
		To:   "company",
		Files: []RenameLine{
			{From: ".gitconfig@@work", To: ".gitconfig@@company"},
			{From: ".claude/settings.json@@work", To: ".claude/settings.json@@company"},
		},
		Selectors: []RenameLine{
			{From: "foo@@work+personal", To: "foo@@company+personal"},
		},
		LocalActive: true,
	})

	want := "Rename profile \"work\" -> \"company\"\n\n" +
		"Profile definition:\n" +
		"  work -> company\n\n" +
		"Files:\n" +
		"  .gitconfig@@work             -> .gitconfig@@company\n" +
		"  .claude/settings.json@@work  -> .claude/settings.json@@company\n\n" +
		"Selectors:\n" +
		"  foo@@work+personal           -> foo@@company+personal\n\n" +
		"Local active profile:\n" +
		"  work -> company\n\n"
	if got := buf.String(); got != want {
		t.Errorf("output =\n%s\nwant\n%s", got, want)
	}
}

// 参照が 1 つも無い profile の rename も正当である（.homux.toml の定義だけが
// 変わる）。active でないなら local active profile の節は出さない。
func TestRenderRenamePlanWithoutReferences(t *testing.T) {
	var buf bytes.Buffer

	RenderRenamePlan(&buf, ColorOff, RenamePlan{From: "work", To: "company"})

	want := "Rename profile \"work\" -> \"company\"\n\n" +
		"Profile definition:\n" +
		"  work -> company\n\n" +
		"Files:\n" +
		"  (none)\n\n" +
		"Selectors:\n" +
		"  (none)\n\n"
	if got := buf.String(); got != want {
		t.Errorf("output =\n%s\nwant\n%s", got, want)
	}
}

// spec §12.10 の衝突エラー。1 バイトも変更していないことが前提なので、
// ここには「どこまで進んだか」は現れない（INV-15）。
func TestFormatRenameCollision(t *testing.T) {
	got := FormatRenameCollision(ColorOff, RenameCollision{
		Line: RenameLine{From: ".gitconfig@@work", To: ".gitconfig@@company"},
		Kind: CollisionExists,
	})

	want := "ERROR rename collision\n\n" +
		"  .gitconfig@@work\n" +
		"  -> .gitconfig@@company\n\n" +
		"Target already exists.\n"
	if got != want {
		t.Errorf("output =\n%s\nwant\n%s", got, want)
	}
}

// 2 つの source が同じ名前へ改名される場合も衝突である。宛先はまだ存在
// しないため、理由の文だけが異なる。
func TestFormatRenameCollisionDuplicate(t *testing.T) {
	got := FormatRenameCollision(ColorOff, RenameCollision{
		Line: RenameLine{From: "foo@@work+personal", To: "foo@@company+personal"},
		Kind: CollisionDuplicate,
	})

	if !strings.HasPrefix(got, "ERROR rename collision\n") {
		t.Errorf("output = %q, want the collision header", got)
	}
	if !strings.Contains(got, "Another source is renamed to the same name.") {
		t.Errorf("output = %q, want the duplicate reason", got)
	}
}

// rename は repo 上の名前を変えるだけで、HOME に配置される内容は変わらない。
// "homux apply" を促さないことがこの表示の要点である（spec §11.2）。
func TestRenderRenameResult(t *testing.T) {
	var buf bytes.Buffer
	repo := filepath.Join("/tmp", "repo")

	RenderRenameResult(&buf, ColorOff, repo, RenamePlan{From: "work", To: "company", LocalActive: true}, exec.RenameResult{
		Applied: []exec.RenameItem{
			{From: filepath.Join(repo, "a@@work"), To: filepath.Join(repo, "a@@company")},
			{From: filepath.Join(repo, "b@@work"), To: filepath.Join(repo, "b@@company")},
		},
	})

	got := buf.String()
	if !strings.Contains(got, "Renamed profile \"work\" -> \"company\".") {
		t.Errorf("output = %q, want the rename headline", got)
	}
	if !strings.Contains(got, "Renamed 2 files.") {
		t.Errorf("output = %q, want the file count", got)
	}
	if !strings.Contains(got, "Active profile: work -> company") {
		t.Errorf("output = %q, want the local active profile line", got)
	}
	if strings.Contains(got, "homux apply") {
		t.Errorf("output = %q, want it not to ask for apply", got)
	}
}

// 途中で失敗したときは、何が終わって何が残ったかと、続きの直し方を示す。
// ロールバックはしない（RenderAddResult / RenderMigrationResult と同じ方針）。
func TestRenderRenameResultPartialFailure(t *testing.T) {
	var buf bytes.Buffer
	repo := filepath.Join("/tmp", "repo")

	RenderRenameResult(&buf, ColorOff, repo, RenamePlan{From: "work", To: "company"}, exec.RenameResult{
		Applied: []exec.RenameItem{{From: filepath.Join(repo, "a@@work"), To: filepath.Join(repo, "a@@company")}},
		Failed:  exec.RenameItem{From: filepath.Join(repo, "b@@work"), To: filepath.Join(repo, "b@@company")},
		Pending: []exec.RenameItem{{From: filepath.Join(repo, "c@@work"), To: filepath.Join(repo, "c@@company")}},
		Err:     errors.New("permission denied"),
	})

	got := buf.String()
	if !strings.Contains(got, "Renamed 1 file before failing.") {
		t.Errorf("output = %q, want the applied count", got)
	}
	if !strings.Contains(got, "b@@work") || !strings.Contains(got, "permission denied") {
		t.Errorf("output = %q, want the failing item and its reason", got)
	}
	if !strings.Contains(got, "c@@work") {
		t.Errorf("output = %q, want the untouched items", got)
	}
	if !strings.Contains(got, "still defines \"work\"") {
		t.Errorf("output = %q, want it to say the definition was left alone", got)
	}
}
