package ui

import (
	"os"
	"testing"
)

func TestParseColorMode(t *testing.T) {
	tests := []struct {
		in      string
		want    ColorMode
		wantErr bool
	}{
		{"auto", ColorAuto, false},
		{"always", ColorAlways, false},
		{"never", ColorNever, false},
		{"rainbow", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseColorMode(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseColorMode(%q): expected error, got nil", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseColorMode(%q): unexpected error: %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("ParseColorMode(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestColorMode_String(t *testing.T) {
	tests := []struct {
		mode ColorMode
		want string
	}{
		{ColorAuto, "auto"},
		{ColorAlways, "always"},
		{ColorNever, "never"},
	}
	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("ColorMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func notATerminalFd(t *testing.T) int {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	return int(w.Fd())
}

func TestResolveColorEnabled_Always(t *testing.T) {
	fd := notATerminalFd(t)
	if !ResolveColorEnabled(ColorAlways, fd) {
		t.Error("ColorAlways should enable color even when fd is not a terminal")
	}
}

func TestResolveColorEnabled_Never(t *testing.T) {
	fd := notATerminalFd(t)
	if ResolveColorEnabled(ColorNever, fd) {
		t.Error("ColorNever should never enable color")
	}
}

func TestResolveColorEnabled_AutoNonTTY(t *testing.T) {
	fd := notATerminalFd(t)
	if ResolveColorEnabled(ColorAuto, fd) {
		t.Error("ColorAuto should disable color when fd is not a terminal")
	}
}

func TestResolveColorEnabled_NoColorOverridesAlways(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	fd := notATerminalFd(t)
	if ResolveColorEnabled(ColorAlways, fd) {
		t.Error("NO_COLOR must disable color even with --color always")
	}
}

func TestResolveColorEnabled_NoColorEmptyValueStillDisables(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	fd := notATerminalFd(t)
	if ResolveColorEnabled(ColorAlways, fd) {
		t.Error("NO_COLOR set to empty string must still disable color (presence, not value, matters)")
	}
}
