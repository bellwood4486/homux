// add.go は homux add（spec §12.6）の出力整形を担う。
package ui

import (
	"fmt"
	"io"

	"github.com/bellwood4486/homux/internal/exec"
)

// RenderAddPlan は取り込み対象を「move してから symlink を作る」の 2 段に
// 分けて表示する（spec §12.6）。apply の RenderPlan と同じ、target 列の幅を
// 揃えた 1 行 1 対応の表形式を踏襲する。
func RenderAddPlan(w io.Writer, pal Palette, home, repo string, items []exec.AddItem) {
	left := make([]string, len(items))
	width := 0
	for i, it := range items {
		left[i] = displayAbsPath(home, it.Target)
		if len(left[i]) > width {
			width = len(left[i])
		}
	}

	fmt.Fprintln(w, "Would move into the repository:")
	for i, it := range items {
		fmt.Fprintf(w, "  %-*s -> %s\n", width, left[i], displayRepoRelPath(repo, it.RepoPath))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Would create symlink:")
	for i, it := range items {
		fmt.Fprintf(w, "  %-*s -> %s\n", width, left[i], displayRepoRelPath(repo, it.RepoPath))
		if it.Fork {
			fmt.Fprintln(w, "  "+pal.Warn("(forks the existing common source)"))
		}
	}
	fmt.Fprintln(w)
}

// RenderAddResult は exec.AddAll の結果を書き出す。exec.AddResult は
// plan.Action ではなく exec.AddItem を持ち回るため、apply の
// RenderApplyResult とは型が異なり共有できない。
func RenderAddResult(w io.Writer, pal Palette, home string, res exec.AddResult) {
	if res.Err == nil {
		fmt.Fprintf(w, "Added %s.\n", countPhrase(len(res.Applied), "file", "files"))
		return
	}

	fmt.Fprintf(w, "Added %s before failing.\n", countPhrase(len(res.Applied), "file", "files"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, pal.Error("Failed:"))
	fmt.Fprintf(w, "  %s\n", displayAbsPath(home, res.Failed.Target))
	fmt.Fprintf(w, "  %s\n", res.Err)

	if len(res.Pending) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Not added:")
		for _, it := range res.Pending {
			fmt.Fprintf(w, "  %s\n", displayAbsPath(home, it.Target))
		}
	}
}
