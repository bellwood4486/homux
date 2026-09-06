package ui

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bellwood4486/homux/internal/exec"
)

// spec §12.11 の plan。参照の列挙がそのまま「何が起きるか」の一覧になる。
// 削除と rewrite を分けて見せるのは、複数 profile selector が「削除されない」
// ことを確認の場で示すためである（spec §12.11 の表）。
func TestRenderDeletePlan(t *testing.T) {
	var buf bytes.Buffer

	RenderDeletePlan(&buf, DeletePlan{
		Profile: "work",
		Removals: []string{
			".gitconfig@@work",
			".claude/settings.json@@work",
		},
		Rewrites: []RewriteLine{
			{From: ".config/foo/config@@work+personal", To: ".config/foo/config@@personal"},
		},
		LocalActive: true,
		HomeChanges: []HomeChange{
			{Target: ".gitconfig", From: ".gitconfig@@work", To: ".gitconfig"},
			{Target: ".claude/settings.json", From: ".claude/settings.json@@work"},
		},
	})

	want := "Profile \"work\" is referenced by 3 sources.\n\n" +
		"Remove:\n" +
		"  .gitconfig@@work\n" +
		"  .claude/settings.json@@work\n\n" +
		"Rewrite selectors:\n" +
		"  .config/foo/config@@work+personal  -> .config/foo/config@@personal\n\n" +
		"Profile definition:\n" +
		"  work -> (removed)\n\n" +
		"Local active profile:\n" +
		"  work -> (none)\n\n" +
		"HOME after this change:\n" +
		"  ~/.gitconfig             .gitconfig@@work -> .gitconfig\n" +
		"  ~/.claude/settings.json  .claude/settings.json@@work -> (unmanaged)\n\n" +
		"HOME is not touched here. Run \"homux apply\" afterwards.\n\n"
	if got := buf.String(); got != want {
		t.Errorf("plan =\n%s\nwant\n%s", got, want)
	}
}

// 参照が 1 件も無い profile。定義を消すだけだが、破壊操作の入口は 1 つに
// 保つため plan と確認は通す。
func TestRenderDeletePlan_NoReferences(t *testing.T) {
	var buf bytes.Buffer

	RenderDeletePlan(&buf, DeletePlan{Profile: "work"})

	got := buf.String()
	if !strings.Contains(got, "Profile \"work\" is not referenced by any source.") {
		t.Errorf("plan =\n%s\nwant it to say the profile is unreferenced", got)
	}
	if !strings.Contains(got, "Remove:\n  "+noneChoice+"\n") {
		t.Errorf("plan =\n%s\nwant an empty Remove section", got)
	}
	// active でなければ HOME は何も変わらないため、節ごと出さない。
	if strings.Contains(got, "HOME after this change:") {
		t.Errorf("plan =\n%s\nwant no HOME section", got)
	}
	if strings.Contains(got, "Local active profile:") {
		t.Errorf("plan =\n%s\nwant no local active profile section", got)
	}
}

// rewrite 先が既に存在する場合。この出力が出たとき repository は 1 バイトも
// 変更されていない。
func TestFormatRewriteCollision(t *testing.T) {
	got := FormatRewriteCollision(RewriteCollision{
		Line: RewriteLine{From: "foo@@work+personal", To: "foo@@personal"},
		Kind: CollisionExists,
	})

	want := "ERROR selector rewrite collision\n\n" +
		"  foo@@work+personal\n" +
		"  -> foo@@personal\n\n" +
		"Target already exists.\n"
	if got != want {
		t.Errorf("message =\n%s\nwant\n%s", got, want)
	}
}

func TestFormatRewriteCollision_Duplicate(t *testing.T) {
	got := FormatRewriteCollision(RewriteCollision{
		Line: RewriteLine{From: "foo@@work+personal", To: "foo@@personal"},
		Kind: CollisionDuplicate,
	})

	if !strings.Contains(got, "Another source is rewritten to the same name.") {
		t.Errorf("message =\n%s\nwant the duplicate reason", got)
	}
}

// 成功時。HOME は変わっていないため apply を促す。
func TestRenderDeleteResult(t *testing.T) {
	repo := filepath.FromSlash("/repo")
	var buf bytes.Buffer

	RenderDeleteResult(&buf, repo, DeletePlan{Profile: "work", LocalActive: true}, exec.DeleteResult{
		Applied: []exec.DeleteItem{
			{Path: filepath.Join(repo, ".gitconfig@@work")},
			{Path: filepath.Join(repo, "foo@@work+personal"), Rewrite: filepath.Join(repo, "foo@@personal")},
		},
	})

	got := buf.String()
	for _, want := range []string{
		"Deleted profile \"work\".",
		"Removed 1 file, rewrote 1 selector.",
		"Active profile: work -> (none)",
		"Run \"homux apply\" to update HOME.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("result =\n%s\nwant it to contain %q", got, want)
		}
	}
}

// 部分適用。ロールバックしないため、利用者が手で終わらせるための手掛かりを
// 示す。.homux.toml はファイルの後に書くため、この時点では必ず profile が
// 残っている。
func TestRenderDeleteResult_PartialFailure(t *testing.T) {
	repo := filepath.FromSlash("/repo")
	var buf bytes.Buffer

	RenderDeleteResult(&buf, repo, DeletePlan{Profile: "work"}, exec.DeleteResult{
		Applied: []exec.DeleteItem{{Path: filepath.Join(repo, "a@@work")}},
		Failed:  exec.DeleteItem{Path: filepath.Join(repo, "b@@work")},
		Pending: []exec.DeleteItem{{Path: filepath.Join(repo, "c@@work")}},
		Err:     errors.New("permission denied"),
	})

	got := buf.String()
	for _, want := range []string{
		"Failed:",
		"b@@work",
		"permission denied",
		"Not processed:",
		"c@@work",
		"still defines \"work\"",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("result =\n%s\nwant it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "Deleted profile") {
		t.Errorf("result =\n%s\nwant no success claim", got)
	}
}
