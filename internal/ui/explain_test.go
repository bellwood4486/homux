package ui

import (
	"bytes"
	"testing"

	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/plan"
	"github.com/bellwood4486/homux/internal/resolve"
	"github.com/bellwood4486/homux/internal/scan"
	"github.com/bellwood4486/homux/internal/selector"
)

func TestRenderExplain_StaleWithCandidateReasons(t *testing.T) {
	common := scan.Source{RepoPath: ".claude/settings.json"}
	work := scan.Source{RepoPath: ".claude/settings.json@@work", Selector: &selector.Selector{Profiles: []string{"work"}}}
	personal := scan.Source{RepoPath: ".claude/settings.json@@personal", Selector: &selector.Selector{Profiles: []string{"personal"}}}

	state := inspect.TargetState{
		Resolution: resolve.Resolution{
			Target:     ".claude/settings.json",
			Candidates: []scan.Source{common, work, personal},
			Selected:   &work,
			Reason:     resolve.ReasonProfileMatch,
		},
		Kind: inspect.KindStale,
		Current: inspect.Current{
			Kind:    inspect.CurrentSymlink,
			LinkAbs: "/home/u/dotfiles/.claude/settings.json",
		},
	}
	action := &plan.Action{Kind: plan.Relink, Target: "/home/u/.claude/settings.json", LinkTo: "/home/u/dotfiles/.claude/settings.json@@work"}

	var buf bytes.Buffer
	RenderExplain(&buf, "/home/u", "work", state, action)

	want := "Target:\n  ~/.claude/settings.json\n\n" +
		"Active profile:\n  work\n\n" +
		"Candidates:\n" +
		"  .claude/settings.json  (not selected: a profile-specific source matches the active profile)\n" +
		"  .claude/settings.json@@work  (selected)\n" +
		"  .claude/settings.json@@personal  (not selected: does not match active profile \"work\")\n" +
		"\n" +
		"Selected:\n  .claude/settings.json@@work\n\n" +
		"Reason:\n  profile-specific source matches the active profile\n\n" +
		"Current:\n" +
		"  ~/.claude/settings.json\n" +
		"  -> ~/dotfiles/.claude/settings.json\n" +
		"\n" +
		"State:\n  stale\n" +
		"\n" +
		"Would apply:\n  relink to ~/dotfiles/.claude/settings.json@@work\n"

	if buf.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func TestRenderExplain_NoActiveProfile_CandidateNoteSaysNoActiveProfile(t *testing.T) {
	work := scan.Source{RepoPath: ".vimrc@@work", Selector: &selector.Selector{Profiles: []string{"work"}}}

	state := inspect.TargetState{
		Resolution: resolve.Resolution{
			Target:     ".vimrc",
			Candidates: []scan.Source{work},
			Reason:     resolve.ReasonAbsent,
		},
		Kind: inspect.KindInactive,
	}

	var buf bytes.Buffer
	RenderExplain(&buf, "/home/u", "", state, nil)

	want := "Target:\n  ~/.vimrc\n\n" +
		"Active profile:\n  (none)\n\n" +
		"Candidates:\n" +
		"  .vimrc@@work  (not selected: no active profile)\n" +
		"\n" +
		"Selected:\n  (none)\n\n" +
		"Reason:\n  no source is available for this target\n\n" +
		"Current:\n  ~/.vimrc\n\n" +
		"State:\n  inactive\n"

	if buf.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

// TestRenderExplain_AmbiguousShowsSpecDiagnostic は受け入れ条件
// 「ambiguous / unknown profile / invalid selector のときに spec §10 の診断を
// 出す」を確認する。Candidates / Selected / Reason は表示せず、
// FormatResolveError と同じ診断ブロックだけを出す。
func TestRenderExplain_AmbiguousShowsSpecDiagnostic(t *testing.T) {
	err := &resolve.AmbiguousError{
		Target:   ".gitconfig",
		Profile:  "work",
		Matching: []string{".gitconfig@@work", ".gitconfig@@work+personal"},
	}
	state := inspect.TargetState{
		Resolution: resolve.Resolution{Target: ".gitconfig", Err: err},
		Kind:       inspect.KindError,
	}

	var buf bytes.Buffer
	RenderExplain(&buf, "/home/u", "work", state, nil)

	want := "Target:\n  ~/.gitconfig\n\n" +
		"ERROR ambiguous profile match\n\n" +
		"  Target:\n    ~/.gitconfig\n\n" +
		"  Matching sources:\n" +
		"    .gitconfig@@work\n" +
		"    .gitconfig@@work+personal\n"

	if buf.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}
