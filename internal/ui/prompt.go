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
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/bellwood4486/homux/internal/exec"
	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/plan"
	"github.com/bellwood4486/homux/internal/selector"
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
	in      *bufio.Reader
	out     io.Writer
	home    string
	repo    string
	profile string // active profile。空文字列は「profile なし」（spec §5.3）。
}

// NewPrompter は in から答えを読み、out へ問いを書く Prompter を返す。
// home はパスを "~/" 表記にするために使う。repo は Occupied の HOME 優先
// （profile 専用）で取り込み先の絶対パスを組み立てるために使う
// （spec §12.4.1、ADR 0015）。profile はそのとき有効なアクティブ profile
// で、取り込み先 profile 名の既定値になる。
func NewPrompter(in io.Reader, out io.Writer, home, repo, profile string) *Prompter {
	return &Prompter{in: bufio.NewReader(in), out: out, home: home, repo: repo, profile: profile}
}

// errNoInput は答えを読む前に入力が尽きたことを表す。exec はこれを受けて
// 停止し、残りを Pending として報告する（spec §12.4 の部分適用）。
var errNoInput = errors.New("no answer available on stdin")

// ConfirmAction は Action の解決策を問う。exec.Confirm として渡す。
//
// Occupied（ReplaceTarget）は複数の解決策から選ぶ（confirmOccupied）。
// それ以外は [y/N] の 2 値で、既定は No である。y を選ばなかったことは
// 永続化されず、conflict が残る限り次回の apply でも再び問う（INV-12）。
func (p *Prompter) ConfirmAction(a plan.Action) (exec.Decision, error) {
	p.writeDetails(a)
	if a.Kind == plan.ReplaceTarget {
		return p.confirmOccupied(a)
	}
	ok, err := p.Confirm(questionFor(a.Kind))
	if err != nil {
		return exec.Decision{}, err
	}
	if ok {
		return exec.Decision{Resolution: exec.ResolutionKeepRepo}, nil
	}
	return exec.Decision{Resolution: exec.ResolutionSkip}, nil
}

// confirmOccupied は Occupied の解決策を問う（spec §12.4.1、ADR 0015）。
//
// h/p は対象が repo 外を指す symlink のときは出さない（add と同じ安全
// 基準）。p はアクティブ profile が無いときは出さない（取り込み先の
// profile 名が決まらないため）。
func (p *Prompter) confirmOccupied(a plan.Action) (exec.Decision, error) {
	allowAdopt := a.Current != inspect.CurrentSymlink
	allowProfile := allowAdopt && p.profile != ""

	fmt.Fprintln(p.out, "How do you want to resolve this?")
	fmt.Fprintln(p.out, "  [r] keep repo version, back up HOME file")
	if allowAdopt {
		fmt.Fprintln(p.out, "  [h] adopt HOME file into repo (common source)")
	}
	if allowProfile {
		fmt.Fprintln(p.out, "  [p] adopt HOME file into repo (profile-specific source)")
	}
	fmt.Fprintln(p.out, "  [n] skip (default)")
	fmt.Fprintln(p.out)

	choices := "r/N"
	switch {
	case allowProfile:
		choices = "r/h/p/N"
	case allowAdopt:
		choices = "r/h/N"
	}

	for {
		fmt.Fprintf(p.out, "Choice [%s]: ", choices)
		line, err := p.readLine()
		if err != nil {
			return exec.Decision{}, err
		}
		switch strings.ToLower(line) {
		case "r":
			return exec.Decision{Resolution: exec.ResolutionKeepRepo}, nil
		case "h":
			if allowAdopt {
				return exec.Decision{Resolution: exec.ResolutionAdopt, AdoptPath: a.LinkTo}, nil
			}
		case "p":
			if allowProfile {
				return p.confirmAdoptProfile(a)
			}
		case "", "n":
			return exec.Decision{Resolution: exec.ResolutionSkip}, nil
		}
		fmt.Fprintf(p.out, "Please answer one of %s.\n", choices)
	}
}

// confirmAdoptProfile は profile 名を尋ね、target@@<profile> への取り込み
// を組み立てる。既定値はアクティブ profile。
func (p *Prompter) confirmAdoptProfile(a plan.Action) (exec.Decision, error) {
	name, err := p.AskLine("profile name", p.profile)
	if err != nil {
		return exec.Decision{}, err
	}
	rel, err := filepath.Rel(p.home, a.Target)
	if err != nil {
		return exec.Decision{}, err
	}
	repoRel := selector.BuildName(filepath.ToSlash(rel), name)
	return exec.Decision{
		Resolution: exec.ResolutionAdopt,
		AdoptPath:  filepath.Join(p.repo, filepath.FromSlash(repoRel)),
	}, nil
}

// writeDetails は問いの前に「何が起きているのか」を示す（spec §12.4）。
// 退避先を明示するのは INV-13 の要請である。
func (p *Prompter) writeDetails(a plan.Action) {
	fmt.Fprintf(p.out, "%s\n\n", headlineFor(a))
	p.writeField("target", a.Target)
	// spec §12.4: target が symlink のときは target の次に現在のリンク先を出す。
	// relink も「どこから どこへ」変わるのかをこの行で示す。
	if a.From != "" {
		p.writeField("current", a.From)
	}
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

// headlineFor は見出しを返す。Occupied の退避では「何が退避されるのか」が
// y を打つ判断そのものなので、target の種類ごとに変える（spec §12.4）。
func headlineFor(a plan.Action) string {
	switch a.Kind {
	case plan.ReplaceTarget:
		switch a.Current {
		case inspect.CurrentDir:
			return "Existing directory detected:"
		case inspect.CurrentSymlink:
			return "Existing symlink detected:"
		default:
			return "Existing file detected:"
		}
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
