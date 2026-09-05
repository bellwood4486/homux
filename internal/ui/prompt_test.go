package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bellwood4486/homux/internal/plan"
)

var replaceAction = plan.Action{
	Kind:    plan.ReplaceTarget,
	Target:  testHome + "/.claude/settings.json",
	LinkTo:  testHome + "/dotfiles/.claude/settings.json@@work",
	Backup:  testHome + "/.claude/settings.json.homux-bak.20260905-153000",
	Confirm: true,
}

func TestPrompter_ReplaceTargetShowsBackupPath(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("y\n"), &out, testHome)

	ok, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if !ok {
		t.Error("ConfirmAction returned false for \"y\"")
	}

	want := "Existing file detected:\n" +
		"\n" +
		"  target:\n" +
		"    ~/.claude/settings.json\n" +
		"\n" +
		"  desired:\n" +
		"    ~/dotfiles/.claude/settings.json@@work\n" +
		"\n" +
		"  backup:\n" +
		"    ~/.claude/settings.json.homux-bak.20260905-153000\n" +
		"\n" +
		"Replace it? [y/N]: "
	if got := out.String(); got != want {
		t.Errorf("prompt:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrompter_RelinkPrompt(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("y\n"), &out, testHome)

	if _, err := p.ConfirmAction(plan.Action{
		Kind:    plan.Relink,
		Target:  testHome + "/.vimrc",
		LinkTo:  testHome + "/dotfiles/.vimrc@@work",
		Confirm: true,
	}); err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}

	want := "Symlink points to a different source:\n" +
		"\n" +
		"  target:\n" +
		"    ~/.vimrc\n" +
		"\n" +
		"  desired:\n" +
		"    ~/dotfiles/.vimrc@@work\n" +
		"\n" +
		"Relink it? [y/N]: "
	if got := out.String(); got != want {
		t.Errorf("prompt:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrompter_RemoveStaleSymlinkPrompt(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("y\n"), &out, testHome)

	if _, err := p.ConfirmAction(plan.Action{
		Kind:    plan.RemoveStaleSymlink,
		Target:  testHome + "/.config/orphan",
		Confirm: true,
	}); err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}

	want := "Stale symlink detected:\n" +
		"\n" +
		"  target:\n" +
		"    ~/.config/orphan\n" +
		"\n" +
		"Remove it? [y/N]: "
	if got := out.String(); got != want {
		t.Errorf("prompt:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrompter_Answers(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{" yes \n", true},
		{"n\n", false},
		{"no\n", false},
		{"NO\n", false},
		{"\n", false},   // 空入力は既定の No（プロンプトの [y/N]）
		{"  \n", false}, // 空白のみも同じ
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.input)+"|", func(t *testing.T) {
			var out bytes.Buffer
			p := NewPrompter(strings.NewReader(tt.input), &out, testHome)

			got, err := p.ConfirmAction(replaceAction)
			if err != nil {
				t.Fatalf("ConfirmAction: %v", err)
			}
			if got != tt.want {
				t.Errorf("ConfirmAction(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestPrompter_RepromptsOnUnrecognizedAnswer(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("maybe\ny\n"), &out, testHome)

	got, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if !got {
		t.Error("ConfirmAction returned false, want true after reprompt")
	}

	s := out.String()
	if !strings.Contains(s, `Please answer "y" or "n".`) {
		t.Errorf("output has no reprompt notice:\n%s", s)
	}
	if n := strings.Count(s, "Replace it? [y/N]: "); n != 2 {
		t.Errorf("question asked %d times, want 2:\n%s", n, s)
	}
	// 質問の繰り返しに詳細ブロックは付け直さない。
	if n := strings.Count(s, "Existing file detected:"); n != 1 {
		t.Errorf("detail block written %d times, want 1:\n%s", n, s)
	}
}

func TestPrompter_ErrorsOnEOF(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader(""), &out, testHome)

	if _, err := p.ConfirmAction(replaceAction); err == nil {
		t.Fatal("ConfirmAction returned nil error at EOF, want error")
	}
}

func TestIsInteractive_FalseWhenNotATerminal(t *testing.T) {
	fd := notATerminalFd(t)
	if IsInteractive(fd, fd) {
		t.Error("IsInteractive should be false when neither fd is a terminal")
	}
}
