package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPrompter_AskLineUsesDefaultOnEmptyInput(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("\n"), &out, testHome)

	got, err := p.AskLine("Repository path", testHome+"/dotfiles")
	if err != nil {
		t.Fatalf("AskLine: %v", err)
	}
	if want := testHome + "/dotfiles"; got != want {
		t.Errorf("AskLine = %q, want %q", got, want)
	}
	if want := "Repository path [~/dotfiles]: "; out.String() != want {
		t.Errorf("prompt = %q, want %q", out.String(), want)
	}
}

func TestPrompter_AskLineTrimsAnswer(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("  /srv/dotfiles  \n"), &out, testHome)

	got, err := p.AskLine("Repository path", testHome+"/dotfiles")
	if err != nil {
		t.Fatalf("AskLine: %v", err)
	}
	if got != "/srv/dotfiles" {
		t.Errorf("AskLine = %q, want %q", got, "/srv/dotfiles")
	}
}

func TestPrompter_AskLineNoInput(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader(""), &out, testHome)

	if _, err := p.AskLine("Repository path", ""); err == nil {
		t.Fatal("AskLine err = nil, want error")
	}
}

func TestPrompter_ConfirmDefaultsToNo(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("\n"), &out, testHome)

	ok, err := p.Confirm("Initialize it as a new homux repository?")
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if ok {
		t.Error("Confirm returned true for empty answer")
	}
	if want := "Initialize it as a new homux repository? [y/N]: "; out.String() != want {
		t.Errorf("prompt = %q, want %q", out.String(), want)
	}
}

// spec §12.1 の選択肢の並び。(none) は最後に置き、profile なしを空文字列で返す。
func TestPrompter_SelectProfile(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("2\n"), &out, testHome)

	got, err := p.SelectProfile([]string{"work", "personal"})
	if err != nil {
		t.Fatalf("SelectProfile: %v", err)
	}
	if got != "personal" {
		t.Errorf("SelectProfile = %q, want %q", got, "personal")
	}

	want := "Available profiles:\n" +
		"\n" +
		"  1. work\n" +
		"  2. personal\n" +
		"  3. (none)\n" +
		"\n" +
		"Select profile: "
	if out.String() != want {
		t.Errorf("prompt:\ngot:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestPrompter_SelectProfileNone(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("3\n"), &out, testHome)

	got, err := p.SelectProfile([]string{"work", "personal"})
	if err != nil {
		t.Fatalf("SelectProfile: %v", err)
	}
	if got != "" {
		t.Errorf("SelectProfile = %q, want empty (no profile)", got)
	}
}

func TestPrompter_SelectProfileRejectsOutOfRange(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("9\nx\n1\n"), &out, testHome)

	got, err := p.SelectProfile([]string{"work", "personal"})
	if err != nil {
		t.Fatalf("SelectProfile: %v", err)
	}
	if got != "work" {
		t.Errorf("SelectProfile = %q, want %q", got, "work")
	}
	if n := strings.Count(out.String(), "Please enter a number between 1 and 3."); n != 2 {
		t.Errorf("re-prompt count = %d, want 2\n%s", n, out.String())
	}
}

func TestPrompter_SelectProfileNoInput(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader(""), &out, testHome)

	_, err := p.SelectProfile([]string{"work"})
	if !errors.Is(err, errNoInput) {
		t.Fatalf("SelectProfile err = %v, want errNoInput", err)
	}
}
