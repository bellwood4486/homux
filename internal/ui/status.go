// status.go は homux status（spec §12.2）の出力整形を担う。
package ui

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/bellwood4486/homux/internal/inspect"
)

// StatusOptions は homux status のフラグを表す（spec §12.2）。
// All と Verbose は直交する: All は表示対象の範囲、Verbose は 1 件あたりの情報量。
type StatusOptions struct {
	All     bool
	Verbose bool
}

// labelWidth は状態ラベル列の幅。spec §12.2 の例に合わせた固定幅である
// （"Missing" + 4 spaces = "Occupied" + 3 spaces = "Stale" + 6 spaces = 11）。
const labelWidth = 11

// RenderStatus は states（plan.Plan.States、path 昇順）を spec §12.2 の
// レイアウトで w に書き出す。states は inspect.All または plan.All の出力を
// そのまま渡すことを想定し、ここでは並び替えない。
func RenderStatus(w io.Writer, home, profile string, states []inspect.TargetState, opts StatusOptions) {
	fmt.Fprintf(w, "Profile: %s\n\n", profileLabel(profile))

	changes, errs := 0, 0
	var shown []inspect.TargetState
	for _, s := range states {
		switch s.Kind {
		case inspect.KindMissing, inspect.KindOccupied, inspect.KindStale:
			changes++
		case inspect.KindError:
			errs++
		}
		if visible(s.Kind, opts.All) {
			shown = append(shown, s)
		}
	}

	for i, s := range shown {
		if opts.Verbose && i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "%-*s%s\n", labelWidth, stateLabel(s.Kind), displayPath(s))
		if opts.Verbose {
			writeVerboseDetails(w, home, s)
		}
	}

	if len(shown) > 0 {
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, summaryLine(changes, errs))

	for _, s := range states {
		if s.Kind != inspect.KindError {
			continue
		}
		fmt.Fprintln(w)
		fmt.Fprint(w, diagnosticFor(s))
	}
}

// visible は --all 無指定時に表示する状態を判定する（spec §12.2:
// 「actionable / problematic な状態のみを表示する」）。
func visible(k inspect.StateKind, all bool) bool {
	if all {
		return true
	}
	switch k {
	case inspect.KindMissing, inspect.KindOccupied, inspect.KindStale, inspect.KindError:
		return true
	default:
		return false
	}
}

func stateLabel(k inspect.StateKind) string {
	switch k {
	case inspect.KindLinked:
		return "Linked"
	case inspect.KindMissing:
		return "Missing"
	case inspect.KindOccupied:
		return "Occupied"
	case inspect.KindStale:
		return "Stale"
	case inspect.KindIgnored:
		return "Ignored"
	case inspect.KindInactive:
		return "Inactive"
	case inspect.KindError:
		return "Error"
	default:
		return "Unknown"
	}
}

// displayPath は Ignored なら repo 相対パスのまま、それ以外は "~/" を前置した
// HOME 相対パスを返す（spec §9: Ignored は repo path の話であり HOME target
// を持たない）。
func displayPath(s inspect.TargetState) string {
	if s.Kind == inspect.KindIgnored {
		return s.RepoPath
	}
	return "~/" + s.Resolution.Target
}

func profileLabel(profile string) string {
	if profile == "" {
		return "(none)"
	}
	return profile
}

func summaryLine(changes, errs int) string {
	switch {
	case changes == 0 && errs == 0:
		return "No changes."
	case changes > 0 && errs > 0:
		return countPhrase(changes, "change", "changes") + " pending, " + countPhrase(errs, "error", "errors")
	case changes > 0:
		return countPhrase(changes, "change", "changes") + " pending"
	default:
		return countPhrase(errs, "error", "errors")
	}
}

func countPhrase(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// writeVerboseDetails は --verbose のときに 1 件あたりの情報量を増やす
// （spec §12.2: 選択された source パス、リンク先の実体など）。
// Ignored / Error は Resolution を実質持たないため何も書かない
// （Error は末尾の診断ブロックで詳細を示す）。
func writeVerboseDetails(w io.Writer, home string, s inspect.TargetState) {
	if s.Kind == inspect.KindIgnored || s.Kind == inspect.KindError {
		return
	}
	indent := strings.Repeat(" ", labelWidth)

	source := "(none)"
	if s.Resolution.Selected != nil {
		source = s.Resolution.Selected.RepoPath
	}
	fmt.Fprintf(w, "%ssource: %s\n", indent, source)

	if line := currentLine(home, s.Current); line != "" {
		fmt.Fprintf(w, "%scurrent: %s\n", indent, line)
	}
}

func currentLine(home string, c inspect.Current) string {
	switch c.Kind {
	case inspect.CurrentSymlink:
		return "-> " + displayAbsPath(home, c.LinkAbs)
	case inspect.CurrentFile:
		return "file"
	case inspect.CurrentDir:
		return "directory"
	default:
		return ""
	}
}

// displayAbsPath は abs が home 配下なら "~/" 形式に、そうでなければ絶対パスの
// ままにする。
func displayAbsPath(home, abs string) string {
	rel, err := filepath.Rel(home, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return abs
	}
	return "~/" + filepath.ToSlash(rel)
}

// diagnosticFor は KindError の 1 件について spec §10 の診断ブロックを返す。
// Resolution.Err（resolve 由来の構造エラー）を優先し、無ければ
// TargetState.Err（inspect / plan 由来の I/O エラーなど、spec §10 に定義の
// 無い種類）を汎用フォーマットで表示する。
func diagnosticFor(s inspect.TargetState) string {
	if s.Resolution.Err != nil {
		return FormatResolveError(s.Resolution.Err)
	}
	if s.Err != nil {
		return fmt.Sprintf("ERROR %s\n\n  %s\n", displayPath(s), s.Err)
	}
	return ""
}
