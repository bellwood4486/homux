package ui

import (
	"bytes"
	"errors"
	"testing"

	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/resolve"
	"github.com/bellwood4486/homux/internal/scan"
)

var errBoom = errors.New("boom")

func TestRenderStatus_DefaultView_MatchesSpecExample(t *testing.T) {
	states := []inspect.TargetState{
		{
			Resolution: resolve.Resolution{
				Target:   ".config/foo/config",
				Selected: &scan.Source{RepoPath: ".config/foo/config"},
			},
			Kind: inspect.KindMissing,
		},
		{
			Resolution: resolve.Resolution{
				Target:   ".claude/settings.json",
				Selected: &scan.Source{RepoPath: ".claude/settings.json@@work"},
			},
			Kind:    inspect.KindOccupied,
			Current: inspect.Current{Kind: inspect.CurrentFile},
		},
		{
			Resolution: resolve.Resolution{
				Target:   ".config/old/config",
				Selected: &scan.Source{RepoPath: ".config/old/config@@work"},
			},
			Kind:    inspect.KindStale,
			Current: inspect.Current{Kind: inspect.CurrentSymlink, LinkAbs: "/home/u/dotfiles/.config/old/config@@personal"},
		},
	}

	var out bytes.Buffer
	RenderStatus(&out, "/home/u", "work", states, StatusOptions{})

	want := "Profile: work\n\n" +
		"Missing    ~/.config/foo/config\n" +
		"Occupied   ~/.claude/settings.json\n" +
		"Stale      ~/.config/old/config\n\n" +
		"3 changes pending\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestRenderStatus_NoChanges(t *testing.T) {
	states := []inspect.TargetState{
		{Resolution: resolve.Resolution{Target: ".zshrc", Selected: &scan.Source{RepoPath: ".zshrc"}}, Kind: inspect.KindLinked},
	}

	var out bytes.Buffer
	RenderStatus(&out, "/home/u", "", states, StatusOptions{})

	want := "Profile: (none)\n\nNo changes.\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestRenderStatus_All_IncludesLinkedIgnoredInactive(t *testing.T) {
	states := []inspect.TargetState{
		{Resolution: resolve.Resolution{Target: ".zshrc", Selected: &scan.Source{RepoPath: ".zshrc"}}, Kind: inspect.KindLinked},
		{RepoPath: "README.md", Kind: inspect.KindIgnored},
		{Resolution: resolve.Resolution{Target: ".config/old/config"}, Kind: inspect.KindInactive},
	}

	var out bytes.Buffer
	RenderStatus(&out, "/home/u", "work", states, StatusOptions{All: true})

	want := "Profile: work\n\n" +
		"Linked     ~/.zshrc\n" +
		"Ignored    README.md\n" +
		"Inactive   ~/.config/old/config\n\n" +
		"No changes.\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestRenderStatus_Verbose_ShowsSourceAndCurrent(t *testing.T) {
	states := []inspect.TargetState{
		{
			Resolution: resolve.Resolution{
				Target:   ".config/old/config",
				Selected: &scan.Source{RepoPath: ".config/old/config@@work"},
			},
			Kind: inspect.KindStale,
			Current: inspect.Current{
				Kind:    inspect.CurrentSymlink,
				LinkAbs: "/home/u/dotfiles/.config/old/config@@personal",
			},
		},
	}

	var out bytes.Buffer
	RenderStatus(&out, "/home/u", "work", states, StatusOptions{Verbose: true})

	want := "Profile: work\n\n" +
		"Stale      ~/.config/old/config\n" +
		"           source: .config/old/config@@work\n" +
		"           current: -> ~/dotfiles/.config/old/config@@personal\n\n" +
		"1 change pending\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestRenderStatus_Verbose_NoSourceShowsNone(t *testing.T) {
	states := []inspect.TargetState{
		{
			Resolution: resolve.Resolution{Target: ".config/old/config"},
			Kind:       inspect.KindInactive,
		},
	}

	var out bytes.Buffer
	RenderStatus(&out, "/home/u", "work", states, StatusOptions{All: true, Verbose: true})

	want := "Profile: work\n\n" +
		"Inactive   ~/.config/old/config\n" +
		"           source: (none)\n\n" +
		"No changes.\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestRenderStatus_ErrorEntry_AppendsDiagnosticBlock(t *testing.T) {
	states := []inspect.TargetState{
		{
			Resolution: resolve.Resolution{
				Target: ".gitconfig",
				Err: &resolve.AmbiguousError{
					Target:   ".gitconfig",
					Profile:  "work",
					Matching: []string{".gitconfig@@work", ".gitconfig@@work+personal"},
				},
			},
			Kind: inspect.KindError,
		},
	}

	var out bytes.Buffer
	RenderStatus(&out, "/home/u", "work", states, StatusOptions{})

	want := "Profile: work\n\n" +
		"Error      ~/.gitconfig\n\n" +
		"1 error\n\n" +
		"ERROR ambiguous profile match\n\n" +
		"  Target:\n    ~/.gitconfig\n\n" +
		"  Matching sources:\n" +
		"    .gitconfig@@work\n" +
		"    .gitconfig@@work+personal\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestRenderStatus_ErrorEntry_FallsBackToTargetStateErr(t *testing.T) {
	states := []inspect.TargetState{
		{
			Resolution: resolve.Resolution{Target: ".zshrc"},
			Kind:       inspect.KindError,
			Err:        errBoom,
		},
	}

	var out bytes.Buffer
	RenderStatus(&out, "/home/u", "work", states, StatusOptions{})

	want := "Profile: work\n\n" +
		"Error      ~/.zshrc\n\n" +
		"1 error\n\n" +
		"ERROR ~/.zshrc\n\n" +
		"  boom\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}
