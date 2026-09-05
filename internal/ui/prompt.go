// prompt.go は apply の確認プロンプト（spec §12.4）を担う。
//
// huh を使わず素の [y/N] で問うのは、spec §12.4 の対話が 1 問 1 答であり
// Select の表現力を必要としないためである（huh は init / profile create の
// ウィザードで使う。ADR 0007）。
package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/term"

	"github.com/bellwood4486/homux/internal/plan"
)

// IsInteractive は対話 UI を起動してよいかを返す（spec §11.4）。
// 標準入力と標準出力の両方が TTY のときだけ真になる。TTY 判定を ui に置くのは
// --color auto と同じ理由で、term を知るのを 1 パッケージに閉じるためである
// （ADR 0009）。
func IsInteractive(inFd, outFd int) bool {
	return term.IsTerminal(inFd) && term.IsTerminal(outFd)
}

// Prompter は 1 回の apply の対話を担う。TTY 判定は呼び出し側の責務であり、
// ここは与えられた Reader / Writer だけを見る（テストは文字列を流し込む）。
type Prompter struct {
	in   *bufio.Reader
	out  io.Writer
	home string
}

// NewPrompter は in から答えを読み、out へ問いを書く Prompter を返す。
// home はパスを "~/" 表記にするために使う。
func NewPrompter(in io.Reader, out io.Writer, home string) *Prompter {
	return &Prompter{in: bufio.NewReader(in), out: out, home: home}
}

// errNoInput は答えを読む前に入力が尽きたことを表す。exec はこれを受けて
// 停止し、残りを Pending として報告する（spec §12.4 の部分適用）。
var errNoInput = errors.New("no answer available on stdin")

// ConfirmAction は Action を実行してよいかを問う。exec.Confirm として渡す。
//
// 既定は No である。y を選ばなかったことは永続化されず、conflict が残る
// 限り次回の apply でも再び問う（INV-12）。
func (p *Prompter) ConfirmAction(a plan.Action) (bool, error) {
	p.writeDetails(a)
	question := questionFor(a.Kind)

	for {
		fmt.Fprintf(p.out, "%s [y/N]: ", question)
		line, err := p.in.ReadString('\n')
		if line == "" && err != nil {
			return false, errNoInput
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		case "", "n", "no":
			return false, nil
		}
		fmt.Fprintln(p.out, `Please answer "y" or "n".`)
	}
}

// writeDetails は問いの前に「何が起きているのか」を示す（spec §12.4）。
// 退避先を明示するのは INV-13 の要請である。
func (p *Prompter) writeDetails(a plan.Action) {
	fmt.Fprintf(p.out, "%s\n\n", headlineFor(a.Kind))
	p.writeField("target", a.Target)
	if a.LinkTo != "" {
		p.writeField("desired", a.LinkTo)
	}
	if a.Backup != "" {
		p.writeField("backup", a.Backup)
	}
}

func (p *Prompter) writeField(label, abs string) {
	fmt.Fprintf(p.out, "  %s:\n    %s\n\n", label, displayAbsPath(p.home, abs))
}

func headlineFor(k plan.ActionKind) string {
	switch k {
	case plan.ReplaceTarget:
		return "Existing file detected:"
	case plan.Relink:
		return "Symlink points to a different source:"
	case plan.RemoveStaleSymlink:
		return "Stale symlink detected:"
	default:
		return "Change detected:"
	}
}

func questionFor(k plan.ActionKind) string {
	switch k {
	case plan.ReplaceTarget:
		return "Replace it?"
	case plan.Relink:
		return "Relink it?"
	case plan.RemoveStaleSymlink:
		return "Remove it?"
	default:
		return "Apply it?"
	}
}
