// wizard.go は init のセットアップ対話（spec §12.1）を担う。
//
// huh の Select ではなく素の番号入力で実装している。理由は ADR 0010 を見ること。
package ui

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// noneChoice は「profile なし」を選ぶための表示名である（spec §12.1）。
// これは表示のためだけの文字列であり、local config には保存されない
// （profile なしは profile キーの不在で表す。docs/design.md §4.2）。
const noneChoice = "(none)"

// AskLine は 1 行の自由入力を求める。空 Enter は defaultValue を選んだことに
// なる。defaultValue が空なら既定候補を表示しない。
//
// 表示は "~/" 表記に畳むが、返すのは常にユーザーが打った文字列そのものである。
func (p *Prompter) AskLine(question, defaultValue string) (string, error) {
	if defaultValue == "" {
		fmt.Fprintf(p.out, "%s: ", question)
	} else {
		fmt.Fprintf(p.out, "%s [%s]: ", question, displayAbsPath(p.home, defaultValue))
	}

	line, err := p.readLine()
	if err != nil {
		return "", err
	}
	if line == "" {
		return defaultValue, nil
	}
	return line, nil
}

// Confirm は [y/N] で問う。既定は No である。
func (p *Prompter) Confirm(question string) (bool, error) {
	for {
		fmt.Fprintf(p.out, "%s [y/N]: ", question)
		line, err := p.readLine()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(line) {
		case "y", "yes":
			return true, nil
		case "", "n", "no":
			return false, nil
		}
		fmt.Fprintln(p.out, `Please answer "y" or "n".`)
	}
}

// SelectProfile は active profile を選ばせる（spec §12.1）。最後の選択肢は
// 常に (none) であり、それが選ばれたときは空文字列を返す。
//
// profiles が空のときにこれを呼んではならない。選択肢が (none) しか無く、
// 問う意味がないためである。呼び出し側が選択自体を省く。
func (p *Prompter) SelectProfile(profiles []string) (string, error) {
	fmt.Fprint(p.out, "Available profiles:\n\n")
	for i, name := range profiles {
		fmt.Fprintf(p.out, "  %d. %s\n", i+1, name)
	}
	fmt.Fprintf(p.out, "  %d. %s\n\n", len(profiles)+1, noneChoice)

	for {
		fmt.Fprint(p.out, "Select profile: ")
		line, err := p.readLine()
		if err != nil {
			return "", err
		}
		n, convErr := strconv.Atoi(line)
		switch {
		case convErr != nil || n < 1 || n > len(profiles)+1:
			fmt.Fprintf(p.out, "Please enter a number between 1 and %d.\n", len(profiles)+1)
		case n == len(profiles)+1:
			return "", nil
		default:
			return profiles[n-1], nil
		}
	}
}

// AskRepoPath は repository のパスを尋ねる（spec §12.1 の 1 番目）。
// defaultValue はカレントディレクトリを想定している。
func (p *Prompter) AskRepoPath(defaultValue string) (string, error) {
	return p.AskLine("Repository path", defaultValue)
}

// ConfirmNewRepository は .homux.toml を持たないディレクトリを新規 repository
// として初期化してよいかを尋ねる（spec §12.1 の 2 番目）。
func (p *Prompter) ConfirmNewRepository(repo string) (bool, error) {
	fmt.Fprintf(p.out, "%s has no %s.\n\n", displayAbsPath(p.home, repo), repoFileName)
	return p.Confirm("Initialize it as a new homux repository?")
}

// repoFileName は表示のためだけに持つ。config を import すると ui が設定の
// 読み書きを知ることになるため、文字列で持つに留める。
const repoFileName = ".homux.toml"

// RenderRejectedInput は対話中に入力を採用できなかった理由を示し、聞き直す前の
// 区切りを入れる。
func RenderRejectedInput(w io.Writer, pal Palette, err error) {
	fmt.Fprintf(w, "  %s\n\n", pal.Warn(fmt.Sprint(err)))
}

// RenderRepoFileCreated は雛形を書き出したことを伝える。
func RenderRepoFileCreated(w io.Writer, pal Palette, home, path string) {
	fmt.Fprintf(w, "\n%s\n", pal.OK("Created "+displayAbsPath(home, path)))
}

// RenderInitSummary は init が local config に保存した内容を要約する。
// この後に apply の出力が続くため、末尾で 1 行空ける。
func RenderInitSummary(w io.Writer, pal Palette, home, localPath, repo, profile string) {
	fmt.Fprintf(w, "\n%s\n", pal.OK("Saved "+displayAbsPath(home, localPath)))
	fmt.Fprintf(w, "  repository: %s\n", displayAbsPath(home, repo))
	fmt.Fprintf(w, "  profile:    %s\n\n", profileLabel(profile))
}

// readLine は 1 行読んで前後の空白を落とす。答えを読む前に入力が尽きた場合は
// errNoInput を返す。
func (p *Prompter) readLine() (string, error) {
	line, err := p.in.ReadString('\n')
	if line == "" && err != nil {
		return "", errNoInput
	}
	return strings.TrimSpace(line), nil
}
