package selector

import (
	"errors"
	"testing"
)

func TestParseName_CommonSource(t *testing.T) {
	// "@@" を含まないファイル名はすべて common source である（ADR 0002）。
	// 単一の "@" は特別な意味を持たない通常の文字として扱う。
	tests := []string{
		"foo",
		".zshrc",
		"tunnel@.service", // systemd テンプレートユニット
		".gitconfig@2024", // よくあるバックアップ命名
		"work@example.com.conf",
	}
	for _, name := range tests {
		base, sel, err := ParseName(name)
		if err != nil {
			t.Errorf("ParseName(%q): unexpected error: %v", name, err)
			continue
		}
		if base != name {
			t.Errorf("ParseName(%q): base = %q, want %q", name, base, name)
		}
		if sel != nil {
			t.Errorf("ParseName(%q): selector = %v, want nil", name, sel)
		}
	}
}

func TestParseName_Selector(t *testing.T) {
	tests := []struct {
		name         string
		wantBase     string
		wantProfiles []string
	}{
		{"foo@@work", "foo", []string{"work"}},
		{"settings.json@@work", "settings.json", []string{"work"}},
		{"foo@@work+personal", "foo", []string{"work", "personal"}},
		{"foo@@a+b+c", "foo", []string{"a", "b", "c"}},
		{"foo@@work-mac", "foo", []string{"work-mac"}},
		{"foo@@home_linux", "foo", []string{"home_linux"}},
		// 最後の "@@" 以降が selector である。
		{"foo@@bar@@work", "foo@@bar", []string{"work"}},
		// 単一の "@" は base 名の一部として残る。
		{"tunnel@.service@@work", "tunnel@.service", []string{"work"}},
	}
	for _, tt := range tests {
		base, sel, err := ParseName(tt.name)
		if err != nil {
			t.Errorf("ParseName(%q): unexpected error: %v", tt.name, err)
			continue
		}
		if base != tt.wantBase {
			t.Errorf("ParseName(%q): base = %q, want %q", tt.name, base, tt.wantBase)
		}
		if sel == nil {
			t.Errorf("ParseName(%q): selector = nil, want %v", tt.name, tt.wantProfiles)
			continue
		}
		if !equalStrings(sel.Profiles, tt.wantProfiles) {
			t.Errorf("ParseName(%q): profiles = %v, want %v", tt.name, sel.Profiles, tt.wantProfiles)
		}
	}
}

func TestParseName_InvalidSyntax(t *testing.T) {
	tests := []struct {
		name    string
		wantErr error
	}{
		{"foo@@", ErrEmptySelector},
		{"foo@@work++personal", ErrEmptySelector},
		{"foo@@+work", ErrEmptySelector},
		{"foo@@work+", ErrEmptySelector},
		{"foo@@!server", ErrNegativeSelector}, // ADR 0005: V1 では構文エラー
		{"foo@@work+!server", ErrNegativeSelector},
		{"foo@@Work", ErrInvalidProfileName},
		{"foo@@my work", ErrInvalidProfileName},
		{"foo@@-x", ErrInvalidProfileName},
		{"foo@@foo@bar", ErrInvalidProfileName},
		{"foo@@work+work", ErrDuplicateProfile},
		{"@@work", ErrEmptyBaseName},
	}
	for _, tt := range tests {
		_, _, err := ParseName(tt.name)
		if !errors.Is(err, tt.wantErr) {
			t.Errorf("ParseName(%q): err = %v, want %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestSelector_Matches(t *testing.T) {
	tests := []struct {
		selector string
		profile  string
		want     bool
	}{
		{"work", "work", true},
		{"work", "personal", false},
		{"work+personal", "work", true},
		{"work+personal", "personal", true},
		{"work+personal", "server", false},
		// profile なし（空文字列）では profile-specific source は一切一致しない（INV-09）。
		{"work", "", false},
		{"work+personal", "", false},
	}
	for _, tt := range tests {
		sel, err := Parse(tt.selector)
		if err != nil {
			t.Fatalf("Parse(%q): unexpected error: %v", tt.selector, err)
		}
		if got := sel.Matches(tt.profile); got != tt.want {
			t.Errorf("Parse(%q).Matches(%q) = %v, want %v", tt.selector, tt.profile, got, tt.want)
		}
	}
}

func TestValidProfileName(t *testing.T) {
	valid := []string{"work", "personal", "work-mac", "home_linux", "a", "0", "x1-y_2"}
	invalid := []string{"", "Work", "my work", "foo@bar", "-x", "_x", "work+personal", "!server", "wörk"}

	for _, name := range valid {
		if !ValidProfileName(name) {
			t.Errorf("ValidProfileName(%q) = false, want true", name)
		}
	}
	for _, name := range invalid {
		if ValidProfileName(name) {
			t.Errorf("ValidProfileName(%q) = true, want false", name)
		}
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
