// explain.go は homux explain（spec §12.3）の出力整形を担う。
package ui

import (
	"fmt"
	"io"

	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/plan"
	"github.com/bellwood4486/homux/internal/resolve"
	"github.com/bellwood4486/homux/internal/scan"
)

// RenderExplain は 1 target について state（plan.Plan.States の 1 件）と、
// それに対応する action（無ければ nil）を spec §12.3 のレイアウトで w に
// 書き出す。出力は state.Resolution.Reason に由来し、resolve / plan の
// 判定を explain のために書き直さない（INV-11）。
func RenderExplain(w io.Writer, home, profile string, state inspect.TargetState, action *plan.Action) {
	r := state.Resolution
	fmt.Fprintf(w, "Target:\n  ~/%s\n\n", r.Target)

	if diag := diagnosticFor(state); diag != "" {
		fmt.Fprint(w, diag)
		return
	}

	fmt.Fprintf(w, "Active profile:\n  %s\n\n", profileLabel(profile))

	fmt.Fprintln(w, "Candidates:")
	if len(r.Candidates) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, c := range r.Candidates {
		fmt.Fprintf(w, "  %s%s\n", c.RepoPath, candidateNote(c, r, profile))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Selected:")
	if r.Selected != nil {
		fmt.Fprintf(w, "  %s\n", r.Selected.RepoPath)
	} else {
		fmt.Fprintln(w, "  (none)")
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Reason:\n  %s\n\n", r.Reason)

	fmt.Fprintln(w, "Current:")
	fmt.Fprintf(w, "  ~/%s\n", r.Target)
	if line := currentLine(home, state.Current); line != "" {
		fmt.Fprintf(w, "  %s\n", line)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "State:\n  %s\n", state.Kind)

	if action != nil {
		fmt.Fprintf(w, "\nWould apply:\n  %s\n", actionDescription(home, *action))
	}
}

// candidateNote は選ばれなかった候補について、なぜ選ばれなかったかを一言
// 添える（explain の受け入れ条件: 選ばれた理由だけでなく、選ばれなかった
// 候補とその理由も示す）。ここに至る時点で state.Resolution.Err は nil
// （diagnosticFor が先に処理する）であり、selector 構文エラーや未定義
// profile は無い。INV-06 によりこの target で active profile に一致する
// profile-specific source は高々 1 つなので、profile-specific source が
// 「一致するが選ばれていない」ケースは起こらない。
func candidateNote(c scan.Source, r resolve.Resolution, profile string) string {
	if r.Selected != nil && c.RepoPath == r.Selected.RepoPath {
		return "  (selected)"
	}
	if c.Selector == nil {
		return "  (not selected: a profile-specific source matches the active profile)"
	}
	if profile == "" {
		return "  (not selected: no active profile)"
	}
	return fmt.Sprintf("  (not selected: does not match active profile %q)", profile)
}

// actionDescription は 1 つの Action を人間可読な文にする。plan が既に
// 判断した操作の種類をそのまま流用し、explain のためだけの判定は追加しない
// （INV-11）。
func actionDescription(home string, a plan.Action) string {
	switch a.Kind {
	case plan.CreateSymlink:
		return "create symlink to " + displayAbsPath(home, a.LinkTo)
	case plan.Relink:
		return "relink to " + displayAbsPath(home, a.LinkTo)
	case plan.ReplaceTarget:
		return fmt.Sprintf("replace target (backup to %s), then link to %s", displayAbsPath(home, a.Backup), displayAbsPath(home, a.LinkTo))
	case plan.RemoveStaleSymlink:
		return "remove stale symlink"
	default:
		return a.Kind.String()
	}
}
