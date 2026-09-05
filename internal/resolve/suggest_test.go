package resolve

import "testing"

func TestSuggest(t *testing.T) {
	profiles := []string{"work", "personal", "server"}
	tests := []struct {
		name string
		want string
	}{
		{"worq", "work"}, // 1 文字違い
		{"wor", "work"},  // 1 文字欠け
		{"persona", "personal"},
		{"zzzzzzzz", ""}, // 遠すぎる候補は提案しない
		{"", ""},
	}
	for _, tt := range tests {
		if got := suggest(tt.name, profiles); got != tt.want {
			t.Errorf("suggest(%q, %v) = %q, want %q", tt.name, profiles, got, tt.want)
		}
	}
	if got := suggest("work", nil); got != "" {
		t.Errorf("suggest with no candidates = %q, want %q", got, "")
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"work", "work", 0},
		{"work", "", 4},
		{"", "work", 4},
		{"worq", "work", 1},
		{"work", "fork", 1},
	}
	for _, tt := range tests {
		if got := levenshtein(tt.a, tt.b); got != tt.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestReason_String(t *testing.T) {
	reasons := []Reason{
		ReasonProfileMatch, ReasonCommonFallback, ReasonNoActiveProfile,
		ReasonAbsent, ReasonAmbiguous, ReasonUnknownProfile, ReasonInvalidSelector,
	}
	seen := map[string]bool{}
	for _, r := range reasons {
		s := r.String()
		if s == "" || s == "unknown" {
			t.Errorf("Reason(%d).String() = %q, want a description", r, s)
		}
		if seen[s] {
			t.Errorf("Reason(%d).String() = %q is duplicated", r, s)
		}
		seen[s] = true
	}
	if got := Reason(99).String(); got != "unknown" {
		t.Errorf("Reason(99).String() = %q, want %q", got, "unknown")
	}
}
