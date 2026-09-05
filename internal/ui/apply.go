// apply.go は homux apply（spec §12.4）と homux apply --dry-run（spec §12.5）の
// 出力整形を担う。
package ui

import (
	"fmt"
	"io"

	"github.com/bellwood4486/homux/internal/exec"
	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/plan"
)

// planGroup は dry-run で 1 つの見出しにまとめる Action の種類である。
// 見出しの並びは spec §12.5 の例に従い、plan.Plan.Actions の順序ではなく
// この固定順で出す（同種の操作が散らばらないようにするため）。
type planGroup struct {
	kind   plan.ActionKind
	header string
	// linkTo は "-> <desired>" を添えるか。削除だけがリンク先を持たない。
	linkTo bool
}

var planGroups = []planGroup{
	{plan.CreateSymlink, "Would create symlink:", true},
	{plan.ReplaceTarget, "Would ask before replacing:", true},
	{plan.Relink, "Would relink:", true},
	{plan.RemoveStaleSymlink, "Would remove stale symlink:", false},
}

// RenderPlan は actions を種類ごとにまとめて w に書き出す。各ブロックの後ろには
// 空行が入る。Action が無ければ何も書かない。
//
// dry-run（spec §12.5）と apply 冒頭のサマリの両方がこれを使う。「実行すると
// 何をするか」の表現を 1 箇所に置くためである。
func RenderPlan(w io.Writer, home string, actions []plan.Action) {
	for _, g := range planGroups {
		var matched []plan.Action
		for _, a := range actions {
			if a.Kind == g.kind {
				matched = append(matched, a)
			}
		}
		if len(matched) == 0 {
			continue
		}
		fmt.Fprintln(w, g.header)
		for _, a := range matched {
			fmt.Fprintf(w, "  %s\n", displayAbsPath(home, a.Target))
			if g.linkTo {
				fmt.Fprintf(w, "  -> %s\n", displayAbsPath(home, a.LinkTo))
			}
		}
		fmt.Fprintln(w)
	}
}

// RenderDryRun は homux apply --dry-run の出力を書き出す（spec §12.5）。
// 何も実行しないことを明示し、構造エラーは status と同じ診断ブロックで示す。
func RenderDryRun(w io.Writer, home string, p plan.Plan) {
	RenderPlan(w, home, p.Actions)
	fmt.Fprintln(w, "No changes made.")
	writeDiagnostics(w, p.States)
}

// writeDiagnostics は KindError の target について spec §10 の診断ブロックを
// 並べる。status / apply / apply --dry-run が同じ見え方をするための共通部分。
func writeDiagnostics(w io.Writer, states []inspect.TargetState) {
	for _, s := range states {
		if s.Kind != inspect.KindError {
			continue
		}
		fmt.Fprintln(w)
		fmt.Fprint(w, diagnosticFor(s))
	}
}

// RenderApplyResult は 1 回の apply の結果を書き出す（spec §12.4）。
//
// p は実行に使った Plan である。Action を作れなかった Error の target の件数は
// p.Errors() をそのまま使い、ここで数え直さない（INV-11）。
func RenderApplyResult(w io.Writer, home string, res exec.Result, p plan.Plan) {
	writeApplyCounts(w, res)

	if n := p.Errors(); n > 0 {
		fmt.Fprintf(w, "%s skipped due to errors.\n", countPhrase(n, "target", "targets"))
	}

	if res.Err != nil {
		writePartialApply(w, home, res)
	}

	writeDiagnostics(w, p.States)
}

// writeApplyCounts は「何件適用し、何件を今回スキップしたか」を書く。
// 失敗した場合でも Applied の行を必ず出す（0 件でも出す）。spec §12.4 の
// 「ここまで適用済み / ここから未適用」を無言で省略しないためである。
func writeApplyCounts(w io.Writer, res exec.Result) {
	if len(res.Applied) == 0 && len(res.Skipped) == 0 && res.Err == nil {
		fmt.Fprintln(w, "No changes.")
		return
	}
	if len(res.Applied) > 0 || res.Err != nil {
		fmt.Fprintf(w, "Applied %s.\n", countPhrase(len(res.Applied), "change", "changes"))
	}
	if n := len(res.Skipped); n > 0 {
		// INV-12: この「スキップ」は今回限りの選択であり、永続化しない。
		fmt.Fprintf(w, "Skipped %s (answered no).\n", countPhrase(n, "change", "changes"))
	}
}

// writePartialApply は途中で停止したときに、失敗した 1 件と手つかずで残った
// 分を示す。ロールバックはしないため、再実行で収束することも書く（spec §12.4）。
func writePartialApply(w io.Writer, home string, res exec.Result) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Failed:")
	fmt.Fprintf(w, "  %s\n", displayAbsPath(home, res.Failed.Target))
	fmt.Fprintf(w, "  %s\n", res.Err)

	if len(res.Pending) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Not applied:")
		for _, a := range res.Pending {
			fmt.Fprintf(w, "  %s\n", displayAbsPath(home, a.Target))
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, `Nothing was rolled back. Run "homux apply" again to continue.`)
}
