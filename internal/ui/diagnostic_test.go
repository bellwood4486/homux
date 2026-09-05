package ui

import (
	"errors"
	"testing"

	"github.com/bellwood4486/homux/internal/resolve"
)

func TestFormatResolveError_UnknownProfileWithRepoPath(t *testing.T) {
	err := &resolve.UnknownProfileError{
		RepoPath:   ".claude/settings.json@@worc",
		Profile:    "worc",
		Suggestion: "work",
	}
	want := "ERROR .claude/settings.json@@worc\n\n" +
		"  Unknown profile \"worc\".\n" +
		"  Did you mean \"work\"?\n"
	if got := FormatResolveError(err); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatResolveError_UnknownProfileNoSuggestion(t *testing.T) {
	err := &resolve.UnknownProfileError{RepoPath: "foo@@zzz", Profile: "zzz"}
	want := "ERROR foo@@zzz\n\n  Unknown profile \"zzz\".\n"
	if got := FormatResolveError(err); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatResolveError_UnknownActiveProfile(t *testing.T) {
	err := &resolve.UnknownProfileError{Profile: "worc", Suggestion: "work"}
	want := "ERROR active profile\n\n" +
		"  Unknown profile \"worc\".\n" +
		"  Did you mean \"work\"?\n"
	if got := FormatResolveError(err); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatResolveError_InvalidSelector(t *testing.T) {
	err := &resolve.InvalidSelectorError{RepoPath: "foo@@work++personal"}
	want := "ERROR foo@@work++personal\n\n  Invalid selector syntax.\n"
	if got := FormatResolveError(err); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatResolveError_Ambiguous(t *testing.T) {
	err := &resolve.AmbiguousError{
		Target:   "foo",
		Profile:  "work",
		Matching: []string{"foo@@work", "foo@@work+personal"},
	}
	want := "ERROR ambiguous profile match\n\n" +
		"  Target:\n" +
		"    ~/foo\n\n" +
		"  Matching sources:\n" +
		"    foo@@work\n" +
		"    foo@@work+personal\n"
	if got := FormatResolveError(err); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatResolveError_Joined(t *testing.T) {
	err := errors.Join(
		&resolve.InvalidSelectorError{RepoPath: "a@@work++personal"},
		&resolve.UnknownProfileError{RepoPath: "b@@zzz", Profile: "zzz"},
	)
	want := "ERROR a@@work++personal\n\n  Invalid selector syntax.\n\n" +
		"ERROR b@@zzz\n\n  Unknown profile \"zzz\".\n"
	if got := FormatResolveError(err); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}
