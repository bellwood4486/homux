package resolve

import (
	"errors"
	"path"
	"testing"

	"github.com/bellwood4486/homux/internal/scan"
	"github.com/bellwood4486/homux/internal/selector"
)

// src は repo 相対パスから scan.Source を組み立てるテストヘルパである。
func src(t *testing.T, repoPath string) scan.Source {
	t.Helper()
	dir, name := path.Split(repoPath)
	base, sel, err := selector.ParseName(name)
	if err != nil {
		if base == "" {
			base = name
		}
		return scan.Source{RepoPath: repoPath, Target: dir + base, SelectorErr: err}
	}
	return scan.Source{RepoPath: repoPath, Target: dir + base, Selector: sel}
}

func sources(t *testing.T, repoPaths ...string) []scan.Source {
	t.Helper()
	out := make([]scan.Source, 0, len(repoPaths))
	for _, p := range repoPaths {
		out = append(out, src(t, p))
	}
	return out
}

func resolveOne(t *testing.T, in Input, target string) Resolution {
	t.Helper()
	rs, err := All(in)
	if err != nil {
		t.Fatalf("All: unexpected error: %v", err)
	}
	for _, r := range rs {
		if r.Target == target {
			return r
		}
	}
	t.Fatalf("target %q not found in %d resolutions", target, len(rs))
	return Resolution{}
}

func TestAll_ProfileSpecificWins(t *testing.T) {
	r := resolveOne(t, Input{
		Sources:  sources(t, ".claude/settings.json", ".claude/settings.json@@work", ".claude/settings.json@@personal"),
		Profiles: []string{"work", "personal"},
		Active:   "work",
	}, ".claude/settings.json")

	if r.Err != nil {
		t.Fatalf("Err = %v, want nil", r.Err)
	}
	if r.Selected == nil || r.Selected.RepoPath != ".claude/settings.json@@work" {
		t.Fatalf("Selected = %v, want .claude/settings.json@@work", r.Selected)
	}
	if r.Reason != ReasonProfileMatch {
		t.Errorf("Reason = %v, want ReasonProfileMatch", r.Reason)
	}
	if len(r.Candidates) != 3 {
		t.Errorf("len(Candidates) = %d, want 3", len(r.Candidates))
	}
}

func TestAll_CommonFallback(t *testing.T) {
	// INV-08: 一致する profile-specific source がなければ common を使う。
	r := resolveOne(t, Input{
		Sources:  sources(t, "foo", "foo@@personal"),
		Profiles: []string{"work", "personal"},
		Active:   "work",
	}, "foo")

	if r.Err != nil {
		t.Fatalf("Err = %v, want nil", r.Err)
	}
	if r.Selected == nil || r.Selected.RepoPath != "foo" {
		t.Fatalf("Selected = %v, want foo", r.Selected)
	}
	if r.Reason != ReasonCommonFallback {
		t.Errorf("Reason = %v, want ReasonCommonFallback", r.Reason)
	}
}

func TestAll_NoActiveProfileUsesCommonOnly(t *testing.T) {
	// INV-09: profile なしでは profile-specific source は一切選択されない。
	r := resolveOne(t, Input{
		Sources:  sources(t, "foo", "foo@@work"),
		Profiles: []string{"work"},
		Active:   "",
	}, "foo")

	if r.Err != nil {
		t.Fatalf("Err = %v, want nil", r.Err)
	}
	if r.Selected == nil || r.Selected.RepoPath != "foo" {
		t.Fatalf("Selected = %v, want foo", r.Selected)
	}
	if r.Reason != ReasonNoActiveProfile {
		t.Errorf("Reason = %v, want ReasonNoActiveProfile", r.Reason)
	}
}

func TestAll_NoActiveProfileAndNoCommonIsAbsent(t *testing.T) {
	r := resolveOne(t, Input{
		Sources:  sources(t, "foo@@work"),
		Profiles: []string{"work"},
		Active:   "",
	}, "foo")

	if r.Err != nil {
		t.Fatalf("Err = %v, want nil", r.Err)
	}
	if r.Selected != nil {
		t.Fatalf("Selected = %v, want nil (absent)", r.Selected)
	}
	if r.Reason != ReasonAbsent {
		t.Errorf("Reason = %v, want ReasonAbsent", r.Reason)
	}
}

func TestAll_AmbiguousIsAnErrorWithoutSpecificityRanking(t *testing.T) {
	// INV-07: foo@@work と foo@@work+personal はどちらも work に一致する。
	// 「より具体的な方を優先」は実装しない。
	r := resolveOne(t, Input{
		Sources:  sources(t, "foo@@work", "foo@@work+personal"),
		Profiles: []string{"work", "personal"},
		Active:   "work",
	}, "foo")

	var ambiguous *AmbiguousError
	if !errors.As(r.Err, &ambiguous) {
		t.Fatalf("Err = %v, want *AmbiguousError", r.Err)
	}
	if r.Selected != nil {
		t.Errorf("Selected = %v, want nil", r.Selected)
	}
	if r.Reason != ReasonAmbiguous {
		t.Errorf("Reason = %v, want ReasonAmbiguous", r.Reason)
	}
	want := []string{"foo@@work", "foo@@work+personal"}
	if !equalStrings(ambiguous.Matching, want) {
		t.Errorf("Matching = %v, want %v", ambiguous.Matching, want)
	}
}

func TestAll_AmbiguousEvenWhenCommonExists(t *testing.T) {
	r := resolveOne(t, Input{
		Sources:  sources(t, "foo", "foo@@work", "foo@@work+personal"),
		Profiles: []string{"work", "personal"},
		Active:   "work",
	}, "foo")

	if r.Err == nil {
		t.Fatal("Err = nil, want ambiguous error")
	}
	if r.Selected != nil {
		t.Errorf("Selected = %v, want nil (common に fallback してはならない)", r.Selected)
	}
}

func TestAll_UnknownProfile(t *testing.T) {
	r := resolveOne(t, Input{
		Sources:  sources(t, ".claude/settings.json", ".claude/settings.json@@worq"),
		Profiles: []string{"work", "personal"},
		Active:   "work",
	}, ".claude/settings.json")

	var unknown *UnknownProfileError
	if !errors.As(r.Err, &unknown) {
		t.Fatalf("Err = %v, want *UnknownProfileError", r.Err)
	}
	if unknown.Profile != "worq" {
		t.Errorf("Profile = %q, want %q", unknown.Profile, "worq")
	}
	if unknown.Suggestion != "work" {
		t.Errorf("Suggestion = %q, want %q", unknown.Suggestion, "work")
	}
	if r.Selected != nil {
		t.Errorf("Selected = %v, want nil", r.Selected)
	}
	if r.Reason != ReasonUnknownProfile {
		t.Errorf("Reason = %v, want ReasonUnknownProfile", r.Reason)
	}
}

func TestAll_UnknownProfileWithoutCloseMatchHasNoSuggestion(t *testing.T) {
	r := resolveOne(t, Input{
		Sources:  sources(t, "foo@@zzzzzzz"),
		Profiles: []string{"work"},
		Active:   "work",
	}, "foo")

	var unknown *UnknownProfileError
	if !errors.As(r.Err, &unknown) {
		t.Fatalf("Err = %v, want *UnknownProfileError", r.Err)
	}
	if unknown.Suggestion != "" {
		t.Errorf("Suggestion = %q, want %q", unknown.Suggestion, "")
	}
}

func TestAll_InvalidSelector(t *testing.T) {
	r := resolveOne(t, Input{
		Sources:  sources(t, "foo@@work++personal"),
		Profiles: []string{"work", "personal"},
		Active:   "work",
	}, "foo")

	var invalid *InvalidSelectorError
	if !errors.As(r.Err, &invalid) {
		t.Fatalf("Err = %v, want *InvalidSelectorError", r.Err)
	}
	if invalid.RepoPath != "foo@@work++personal" {
		t.Errorf("RepoPath = %q, want %q", invalid.RepoPath, "foo@@work++personal")
	}
	if !errors.Is(r.Err, selector.ErrEmptySelector) {
		t.Errorf("errors.Is(err, ErrEmptySelector) = false, want true")
	}
	if r.Reason != ReasonInvalidSelector {
		t.Errorf("Reason = %v, want ReasonInvalidSelector", r.Reason)
	}
}

func TestAll_SingleAtIsCommonSource(t *testing.T) {
	// design.md §7.1: tunnel@.service は common source として扱われる。
	in := Input{
		Sources:  sources(t, ".config/systemd/user/tunnel@.service"),
		Profiles: []string{"work"},
		Active:   "work",
	}
	r := resolveOne(t, in, ".config/systemd/user/tunnel@.service")

	if r.Err != nil {
		t.Fatalf("Err = %v, want nil", r.Err)
	}
	if r.Selected == nil || r.Selected.Selector != nil {
		t.Fatalf("Selected = %v, want common source", r.Selected)
	}
	if r.Reason != ReasonCommonFallback {
		t.Errorf("Reason = %v, want ReasonCommonFallback", r.Reason)
	}
}

func TestAll_ResultsAreSortedByTarget(t *testing.T) {
	rs, err := All(Input{
		Sources:  sources(t, ".zshrc", ".claude/settings.json@@work", ".gitconfig"),
		Profiles: []string{"work"},
		Active:   "work",
	})
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	want := []string{".claude/settings.json", ".gitconfig", ".zshrc"}
	got := make([]string, len(rs))
	for i, r := range rs {
		got[i] = r.Target
	}
	if !equalStrings(got, want) {
		t.Errorf("targets = %v, want %v", got, want)
	}
}

func TestAll_UnknownActiveProfile(t *testing.T) {
	// local config の active profile が .homux.toml に定義されていない場合は
	// repository 全体の解決が成立しない。
	_, err := All(Input{
		Sources:  sources(t, "foo"),
		Profiles: []string{"work"},
		Active:   "worq",
	})
	var unknown *UnknownProfileError
	if !errors.As(err, &unknown) {
		t.Fatalf("err = %v, want *UnknownProfileError", err)
	}
	if unknown.Suggestion != "work" {
		t.Errorf("Suggestion = %q, want %q", unknown.Suggestion, "work")
	}
}

func TestAll_MultiProfileSelectorMatchesEachProfile(t *testing.T) {
	for _, active := range []string{"work", "personal"} {
		r := resolveOne(t, Input{
			Sources:  sources(t, "foo", "foo@@work+personal"),
			Profiles: []string{"work", "personal", "server"},
			Active:   active,
		}, "foo")
		if r.Selected == nil || r.Selected.RepoPath != "foo@@work+personal" {
			t.Errorf("active=%q: Selected = %v, want foo@@work+personal", active, r.Selected)
		}
	}

	r := resolveOne(t, Input{
		Sources:  sources(t, "foo", "foo@@work+personal"),
		Profiles: []string{"work", "personal", "server"},
		Active:   "server",
	}, "foo")
	if r.Selected == nil || r.Selected.RepoPath != "foo" {
		t.Errorf("active=server: Selected = %v, want foo", r.Selected)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
