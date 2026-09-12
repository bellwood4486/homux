package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bellwood4486/homux/internal/exec"
	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/plan"
)

var replaceAction = plan.Action{
	Kind:    plan.ReplaceTarget,
	Current: inspect.CurrentFile,
	Target:  testHome + "/.claude/settings.json",
	LinkTo:  testHome + "/dotfiles/.claude/settings.json@@work",
	Backup:  testHome + "/.claude/settings.json.homux-bak.20260905-153000",
	Confirm: true,
}

func TestPrompter_ReplaceTargetShowsBackupPath(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("r\n"), &out, testHome, testRepo, "")

	d, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if d.Resolution != exec.ResolutionKeepRepo {
		t.Errorf("Resolution = %v, want ResolutionKeepRepo", d.Resolution)
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
		"How do you want to resolve this?\n" +
		"  [r] keep repo version, back up HOME file\n" +
		"  [h] adopt HOME file into repo (common source)\n" +
		"  [n] skip (default)\n" +
		"\n" +
		"Choice [r/h/N]: "
	if got := out.String(); got != want {
		t.Errorf("prompt:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrompter_RelinkPrompt(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("y\n"), &out, testHome, testRepo, "")

	if _, err := p.ConfirmAction(plan.Action{
		Kind:    plan.Relink,
		Target:  testHome + "/.vimrc",
		LinkTo:  testHome + "/dotfiles/.vimrc@@work",
		From:    testHome + "/dotfiles/.vimrc",
		Current: inspect.CurrentSymlink,
		Confirm: true,
	}); err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}

	// spec §12.4: relink は「どこから どこへ」変わるのかを示す。
	want := "Symlink points to a different source:\n" +
		"\n" +
		"  target:\n" +
		"    ~/.vimrc\n" +
		"\n" +
		"  current:\n" +
		"    ~/dotfiles/.vimrc\n" +
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
	p := NewPrompter(strings.NewReader("y\n"), &out, testHome, testRepo, "")

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

// [y/N] の 2 値語彙を使うのは Relink/RemoveStaleSymlink だけである。
func TestPrompter_Answers(t *testing.T) {
	tests := []struct {
		input string
		want  exec.ConflictResolution
	}{
		{"y\n", exec.ResolutionKeepRepo},
		{"Y\n", exec.ResolutionKeepRepo},
		{"yes\n", exec.ResolutionKeepRepo},
		{" yes \n", exec.ResolutionKeepRepo},
		{"n\n", exec.ResolutionSkip},
		{"no\n", exec.ResolutionSkip},
		{"NO\n", exec.ResolutionSkip},
		{"\n", exec.ResolutionSkip},   // 空入力は既定の No（プロンプトの [y/N]）
		{"  \n", exec.ResolutionSkip}, // 空白のみも同じ
	}
	relinkAction := plan.Action{
		Kind:    plan.Relink,
		Target:  testHome + "/.vimrc",
		LinkTo:  testHome + "/dotfiles/.vimrc@@work",
		From:    testHome + "/dotfiles/.vimrc",
		Confirm: true,
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.input)+"|", func(t *testing.T) {
			var out bytes.Buffer
			p := NewPrompter(strings.NewReader(tt.input), &out, testHome, testRepo, "work")

			got, err := p.ConfirmAction(relinkAction)
			if err != nil {
				t.Fatalf("ConfirmAction: %v", err)
			}
			if got.Resolution != tt.want {
				t.Errorf("ConfirmAction(%q).Resolution = %v, want %v", tt.input, got.Resolution, tt.want)
			}
		})
	}
}

// Occupied は r/h/p/n の語彙を使い、y/yes は認識されない。
func TestPrompter_ConfirmOccupied_Answers(t *testing.T) {
	tests := []struct {
		input string
		want  exec.ConflictResolution
	}{
		{"r\n", exec.ResolutionKeepRepo},
		{"R\n", exec.ResolutionKeepRepo},
		{"n\n", exec.ResolutionSkip},
		{"\n", exec.ResolutionSkip},
		{"  \n", exec.ResolutionSkip},
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.input)+"|", func(t *testing.T) {
			var out bytes.Buffer
			p := NewPrompter(strings.NewReader(tt.input), &out, testHome, testRepo, "work")

			got, err := p.ConfirmAction(replaceAction)
			if err != nil {
				t.Fatalf("ConfirmAction: %v", err)
			}
			if got.Resolution != tt.want {
				t.Errorf("ConfirmAction(%q).Resolution = %v, want %v", tt.input, got.Resolution, tt.want)
			}
		})
	}
}

func TestPrompter_RepromptsOnUnrecognizedAnswer(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("maybe\nr\n"), &out, testHome, testRepo, "work")

	got, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if got.Resolution != exec.ResolutionKeepRepo {
		t.Error("ConfirmAction returned non-KeepRepo, want ResolutionKeepRepo after reprompt")
	}

	s := out.String()
	if !strings.Contains(s, "Please answer one of r/h/p/N.") {
		t.Errorf("output has no reprompt notice:\n%s", s)
	}
	if n := strings.Count(s, "Choice [r/h/p/N]: "); n != 2 {
		t.Errorf("question asked %d times, want 2:\n%s", n, s)
	}
	// 質問の繰り返しに詳細ブロックは付け直さない。
	if n := strings.Count(s, "Existing file detected:"); n != 1 {
		t.Errorf("detail block written %d times, want 1:\n%s", n, s)
	}
}

func TestPrompter_ErrorsOnEOF(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader(""), &out, testHome, testRepo, "")

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

// spec §12.4: 何が退避されるのかは y を打つ判断そのものなので、見出しは
// target の種類ごとに変える。
func TestPrompter_ReplaceTargetHeadlineByCurrentKind(t *testing.T) {
	tests := []struct {
		current inspect.CurrentKind
		want    string
	}{
		{inspect.CurrentFile, "Existing file detected:"},
		{inspect.CurrentDir, "Existing directory detected:"},
		{inspect.CurrentSymlink, "Existing symlink detected:"},
	}
	for _, tt := range tests {
		t.Run(tt.current.String(), func(t *testing.T) {
			var out bytes.Buffer
			p := NewPrompter(strings.NewReader("n\n"), &out, testHome, testRepo, "")

			a := replaceAction
			a.Current = tt.current
			if _, err := p.ConfirmAction(a); err != nil {
				t.Fatalf("ConfirmAction: %v", err)
			}

			if got := out.String(); !strings.HasPrefix(got, tt.want+"\n") {
				t.Errorf("headline:\ngot:\n%s\nwant prefix:\n%s", got, tt.want)
			}
		})
	}
}

// spec §12.4: target が symlink のときは target の次に現在のリンク先を出す。
func TestPrompter_ReplaceSymlinkShowsCurrentLink(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("n\n"), &out, testHome, testRepo, "")

	if _, err := p.ConfirmAction(plan.Action{
		Kind:    plan.ReplaceTarget,
		Target:  testHome + "/.vimrc",
		LinkTo:  testHome + "/dotfiles/.vimrc@@work",
		From:    "/opt/elsewhere/.vimrc",
		Current: inspect.CurrentSymlink,
		Backup:  testHome + "/.vimrc.homux-bak.20260905-153000",
		Confirm: true,
	}); err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}

	want := "Existing symlink detected:\n" +
		"\n" +
		"  target:\n" +
		"    ~/.vimrc\n" +
		"\n" +
		"  current:\n" +
		"    /opt/elsewhere/.vimrc\n" +
		"\n" +
		"  desired:\n" +
		"    ~/dotfiles/.vimrc@@work\n" +
		"\n" +
		"  backup:\n" +
		"    ~/.vimrc.homux-bak.20260905-153000\n" +
		"\n" +
		"How do you want to resolve this?\n" +
		"  [r] keep repo version, back up HOME file\n" +
		"  [n] skip (default)\n" +
		"\n" +
		"Choice [r/N]: "
	if got := out.String(); got != want {
		t.Errorf("prompt:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// spec §12.4.1: Occupied は r/h/p/n の4択（アクティブ profile があるとき）。
func TestPrompter_ConfirmOccupied_KeepRepo(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("r\n"), &out, testHome, testRepo, "work")

	d, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if d.Resolution != exec.ResolutionKeepRepo {
		t.Errorf("Resolution = %v, want ResolutionKeepRepo", d.Resolution)
	}

	want := "How do you want to resolve this?\n" +
		"  [r] keep repo version, back up HOME file\n" +
		"  [h] adopt HOME file into repo (common source)\n" +
		"  [p] adopt HOME file into repo (profile-specific source)\n" +
		"  [n] skip (default)\n" +
		"\n" +
		"Choice [r/h/p/N]: "
	if got := out.String(); !strings.HasSuffix(got, want) {
		t.Errorf("prompt:\ngot:\n%s\nwant suffix:\n%s", got, want)
	}
}

func TestPrompter_ConfirmOccupied_AdoptCommon(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("h\n"), &out, testHome, testRepo, "work")

	d, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if d.Resolution != exec.ResolutionAdopt {
		t.Errorf("Resolution = %v, want ResolutionAdopt", d.Resolution)
	}
	if d.AdoptPath != replaceAction.LinkTo {
		t.Errorf("AdoptPath = %q, want %q (Selected source unchanged)", d.AdoptPath, replaceAction.LinkTo)
	}
}

func TestPrompter_ConfirmOccupied_DefaultIsSkip(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("\n"), &out, testHome, testRepo, "work")

	d, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if d.Resolution != exec.ResolutionSkip {
		t.Errorf("Resolution = %v, want ResolutionSkip", d.Resolution)
	}
}

// spec §12.4.1: 対象が repo 外を指す symlink のときは h/p を出さない。
func TestPrompter_ConfirmOccupied_HidesAdoptForSymlinkTarget(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("n\n"), &out, testHome, testRepo, "work")

	a := replaceAction
	a.Current = inspect.CurrentSymlink
	a.From = "/opt/elsewhere/.vimrc"
	if _, err := p.ConfirmAction(a); err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "[h]") || strings.Contains(got, "[p]") {
		t.Errorf("prompt should not offer h/p for a symlink target:\n%s", got)
	}
	if !strings.Contains(got, "Choice [r/N]: ") {
		t.Errorf("prompt should show a 2-way choice:\n%s", got)
	}
}

// spec §12.4.1: アクティブ profile が無いときは p を出さない。
func TestPrompter_ConfirmOccupied_HidesProfileWhenNoActiveProfile(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("n\n"), &out, testHome, testRepo, "")

	if _, err := p.ConfirmAction(replaceAction); err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "[p]") {
		t.Errorf("prompt should not offer p without an active profile:\n%s", got)
	}
	if !strings.Contains(got, "Choice [r/h/N]: ") {
		t.Errorf("prompt should show a 3-way choice:\n%s", got)
	}
}

// spec §12.4.1: p を選ぶと profile 名の入力を求め、既定値はアクティブ profile。
func TestPrompter_ConfirmOccupied_AdoptProfileDefaultsToActiveProfile(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("p\n\n"), &out, testHome, testRepo, "work")

	d, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if d.Resolution != exec.ResolutionAdopt {
		t.Fatalf("Resolution = %v, want ResolutionAdopt", d.Resolution)
	}
	want := testRepo + "/.claude/settings.json@@work"
	if d.AdoptPath != want {
		t.Errorf("AdoptPath = %q, want %q", d.AdoptPath, want)
	}
	if !strings.Contains(out.String(), "profile name [work]: ") {
		t.Errorf("prompt should ask for a profile name with the active profile as default:\n%s", out.String())
	}
}

// profile 名は対話中に変更できる。
func TestPrompter_ConfirmOccupied_AdoptProfileCanOverrideName(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("p\npersonal\n"), &out, testHome, testRepo, "work")

	d, err := p.ConfirmAction(replaceAction)
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	want := testRepo + "/.claude/settings.json@@personal"
	if d.AdoptPath != want {
		t.Errorf("AdoptPath = %q, want %q", d.AdoptPath, want)
	}
}
