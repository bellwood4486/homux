package ui

import (
	"bytes"
	"testing"

	"github.com/bellwood4486/homux/internal/exec"
	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/plan"
	"github.com/bellwood4486/homux/internal/resolve"
)

const testHome = "/home/u"

// spec §12.5 の出力例をそのまま検証する。
func TestRenderPlan_GroupsActionsByKind(t *testing.T) {
	actions := []plan.Action{
		{
			Kind:    plan.ReplaceTarget,
			Target:  testHome + "/.claude/settings.json",
			LinkTo:  testHome + "/dotfiles/.claude/settings.json@@work",
			Current: inspect.CurrentDir,
			Backup:  testHome + "/.claude/settings.json.homux-bak.20260905-153000",
			Confirm: true,
		},
		{
			Kind:   plan.CreateSymlink,
			Target: testHome + "/.config/foo/config",
			LinkTo: testHome + "/dotfiles/.config/foo/config@@work",
		},
		{
			Kind:    plan.RemoveStaleSymlink,
			Target:  testHome + "/.config/old/config",
			Current: inspect.CurrentSymlink,
			Confirm: true,
		},
		{
			Kind:    plan.Relink,
			Target:  testHome + "/.vimrc",
			LinkTo:  testHome + "/dotfiles/.vimrc@@work",
			From:    testHome + "/dotfiles/.vimrc",
			Current: inspect.CurrentSymlink,
			Confirm: true,
		},
	}

	var buf bytes.Buffer
	RenderPlan(&buf, ColorOff, testHome, actions)

	want := "Would create symlink:\n" +
		"  ~/.config/foo/config\n" +
		"  -> ~/dotfiles/.config/foo/config@@work\n" +
		"\n" +
		"Would ask before replacing:\n" +
		"  ~/.claude/settings.json (directory)\n" +
		"  -> ~/dotfiles/.claude/settings.json@@work\n" +
		"\n" +
		"Would relink:\n" +
		"  ~/.vimrc\n" +
		"  ~/dotfiles/.vimrc -> ~/dotfiles/.vimrc@@work\n" +
		"\n" +
		"Would remove stale symlink:\n" +
		"  ~/.config/old/config\n" +
		"\n"
	if got := buf.String(); got != want {
		t.Errorf("RenderPlan:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// spec §12.5: 注記は「驚きのある退避」を先に見せるためにあり、既定の期待である
// 通常ファイルには添えない。
func TestRenderPlan_ReplaceNoteOnlyForDirAndSymlink(t *testing.T) {
	tests := []struct {
		current inspect.CurrentKind
		want    string
	}{
		{inspect.CurrentFile, "  ~/.vimrc\n"},
		{inspect.CurrentDir, "  ~/.vimrc (directory)\n"},
		{inspect.CurrentSymlink, "  ~/.vimrc (symlink)\n"},
	}
	for _, tt := range tests {
		t.Run(tt.current.String(), func(t *testing.T) {
			var buf bytes.Buffer
			RenderPlan(&buf, ColorOff, testHome, []plan.Action{{
				Kind:    plan.ReplaceTarget,
				Target:  testHome + "/.vimrc",
				LinkTo:  testHome + "/dotfiles/.vimrc@@work",
				Current: tt.current,
				Backup:  testHome + "/.vimrc.homux-bak.20260905-153000",
				Confirm: true,
			}})

			want := "Would ask before replacing:\n" +
				tt.want +
				"  -> ~/dotfiles/.vimrc@@work\n" +
				"\n"
			if got := buf.String(); got != want {
				t.Errorf("RenderPlan:\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestRenderPlan_NoActionsWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	RenderPlan(&buf, ColorOff, testHome, nil)

	if got := buf.String(); got != "" {
		t.Errorf("RenderPlan with no actions wrote %q, want empty", got)
	}
}

func TestRenderDryRun_EndsWithNoChangesMade(t *testing.T) {
	p := plan.Plan{
		Actions: []plan.Action{
			{Kind: plan.CreateSymlink, Target: testHome + "/.zshrc", LinkTo: testHome + "/dotfiles/.zshrc"},
		},
		States: []inspect.TargetState{
			{Kind: inspect.KindMissing, Resolution: resolve.Resolution{Target: ".zshrc"}},
		},
	}

	var buf bytes.Buffer
	RenderDryRun(&buf, ColorOff, testHome, p)

	want := "Would create symlink:\n" +
		"  ~/.zshrc\n" +
		"  -> ~/dotfiles/.zshrc\n" +
		"\n" +
		"No changes made.\n"
	if got := buf.String(); got != want {
		t.Errorf("RenderDryRun:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderDryRun_NoActions(t *testing.T) {
	var buf bytes.Buffer
	RenderDryRun(&buf, ColorOff, testHome, plan.Plan{})

	if got, want := buf.String(), "No changes made.\n"; got != want {
		t.Errorf("RenderDryRun:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderDryRun_ShowsDiagnosticsForErrorStates(t *testing.T) {
	p := plan.Plan{
		States: []inspect.TargetState{
			{
				Kind:       inspect.KindError,
				Resolution: resolve.Resolution{Target: ".gitconfig"},
				Err:        errStub{},
			},
		},
	}

	var buf bytes.Buffer
	RenderDryRun(&buf, ColorOff, testHome, p)

	want := "No changes made.\n" +
		"\n" +
		"ERROR ~/.gitconfig\n" +
		"\n" +
		"  stub failure\n"
	if got := buf.String(); got != want {
		t.Errorf("RenderDryRun:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

type errStub struct{}

func (errStub) Error() string { return "stub failure" }

func TestRenderApplyResult_NoChanges(t *testing.T) {
	var buf bytes.Buffer
	RenderApplyResult(&buf, ColorOff, testHome, exec.Result{}, plan.Plan{})

	if got, want := buf.String(), "No changes.\n"; got != want {
		t.Errorf("RenderApplyResult:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderApplyResult_AppliedAndSkipped(t *testing.T) {
	res := exec.Result{
		Applied: []plan.Action{
			{Kind: plan.CreateSymlink, Target: testHome + "/.zshrc"},
			{Kind: plan.CreateSymlink, Target: testHome + "/.vimrc"},
		},
		Skipped: []plan.Action{
			{Kind: plan.ReplaceTarget, Target: testHome + "/.claude/settings.json"},
		},
	}

	var buf bytes.Buffer
	RenderApplyResult(&buf, ColorOff, testHome, res, plan.Plan{})

	want := "Applied 2 changes.\n" +
		"Skipped 1 change (answered no).\n"
	if got := buf.String(); got != want {
		t.Errorf("RenderApplyResult:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderApplyResult_PartialApplyReportsFailedAndPending(t *testing.T) {
	res := exec.Result{
		Applied: []plan.Action{{Kind: plan.CreateSymlink, Target: testHome + "/.zshrc"}},
		Failed:  plan.Action{Kind: plan.ReplaceTarget, Target: testHome + "/.claude/settings.json"},
		Pending: []plan.Action{
			{Kind: plan.Relink, Target: testHome + "/.vimrc"},
			{Kind: plan.RemoveStaleSymlink, Target: testHome + "/.config/orphan"},
		},
		Err: errStub{},
	}

	var buf bytes.Buffer
	RenderApplyResult(&buf, ColorOff, testHome, res, plan.Plan{})

	want := "Applied 1 change.\n" +
		"\n" +
		"Failed:\n" +
		"  ~/.claude/settings.json\n" +
		"  stub failure\n" +
		"\n" +
		"Not applied:\n" +
		"  ~/.vimrc\n" +
		"  ~/.config/orphan\n" +
		"\n" +
		"Nothing was rolled back. Run \"homux apply\" again to continue.\n"
	if got := buf.String(); got != want {
		t.Errorf("RenderApplyResult:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderApplyResult_ErrorTargetsAreCountedAndDiagnosed(t *testing.T) {
	states := []inspect.TargetState{
		{Kind: inspect.KindError, Resolution: resolve.Resolution{Target: ".gitconfig"}, Err: errStub{}},
	}

	var buf bytes.Buffer
	RenderApplyResult(&buf, ColorOff, testHome, exec.Result{}, plan.Plan{States: states})

	want := "No changes.\n" +
		"1 target skipped due to errors.\n" +
		"\n" +
		"ERROR ~/.gitconfig\n" +
		"\n" +
		"  stub failure\n"
	if got := buf.String(); got != want {
		t.Errorf("RenderApplyResult:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
