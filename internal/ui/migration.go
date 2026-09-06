// migration.go は homux profile create（spec §12.9）の出力整形と、
// fork 対象を選ぶウィザードを担う。
//
// 選択画面だけが huh であり、plan の表示・確認・結果の報告はすべて素の
// io.Writer である。破壊的変更を伴う判断の側をテスト可能な位置に置くための
// 分割である（ADR 0010 の帰結）。
package ui

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/bellwood4486/homux/internal/exec"
)

// ForkLine は plan に並ぶ 1 行である。どちらも repo ルートからの相対パス。
type ForkLine struct {
	Common string
	Fork   string
}

// MigrationPlan は profile create が repository に加える変更である。
type MigrationPlan struct {
	// Profile は新しく作る profile 名。
	Profile string
	// KeepTargets は common のまま残る target（HOME からの相対パス）。
	KeepTargets []string
	// Forks は複製する source。
	Forks []ForkLine
	// SkippedNoCommon は common source を持たないため候補にできなかった
	// target の数である。
	SkippedNoCommon int
}

// RenderMigrationPlan は spec §12.9 の plan を w に書き出す。
//
// 「何が変わらないか」を先に見せるのは、profile を足しても既定では
// すべてが common のまま、という spec §12.9 の前提を確認の場で示すためである。
func RenderMigrationPlan(w io.Writer, pal Palette, p MigrationPlan) {
	fmt.Fprintf(w, "Profile migration plan: %s\n\n", p.Profile)

	fmt.Fprintln(w, "Keep common:")
	if len(p.KeepTargets) == 0 {
		fmt.Fprintf(w, "  %s\n", noneChoice)
	}
	for _, t := range p.KeepTargets {
		fmt.Fprintf(w, "  ~/%s\n", t)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, pal.Warn("Fork:"))
	if len(p.Forks) == 0 {
		fmt.Fprintf(w, "  %s\n", noneChoice)
	}
	width := 0
	for _, f := range p.Forks {
		if len(f.Common) > width {
			width = len(f.Common)
		}
	}
	for _, f := range p.Forks {
		fmt.Fprintf(w, "  %-*s  -> %s\n", width, f.Common, f.Fork)
	}
	fmt.Fprintln(w)

	if p.SkippedNoCommon > 0 {
		fmt.Fprintf(w, "%s no common source and cannot be forked here.\n\n",
			countPhrase(p.SkippedNoCommon, "target has", "targets have"))
	}
}

// RenderMigrationResult は exec.ForkAll の結果を w に書き出す。
//
// profiles への追加は fork より先に済んでいるため、成否にかかわらず
// 「profile は作られた」と伝える。部分適用はロールバックしない
// （RenderAddResult と同じ方針）。
func RenderMigrationResult(w io.Writer, pal Palette, repo, profile string, res exec.ForkResult) {
	fmt.Fprintf(w, "Created profile %q.\n", profile)

	if res.Err == nil {
		fmt.Fprintf(w, "Forked %s.\n\n", countPhrase(len(res.Applied), "file", "files"))
		fmt.Fprintf(w,
			"Run \"homux profile use %s\" on machines that should use it, then \"homux apply\" to update HOME.\n",
			profile)
		return
	}

	fmt.Fprintf(w, "Forked %s before failing.\n\n", countPhrase(len(res.Applied), "file", "files"))
	fmt.Fprintln(w, pal.Error("Failed:"))
	fmt.Fprintf(w, "  %s\n", displayRepoPath(repo, res.Failed.Fork))
	fmt.Fprintf(w, "  %s\n", res.Err)

	if len(res.Pending) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Not forked:")
		for _, it := range res.Pending {
			fmt.Fprintf(w, "  %s\n", displayRepoPath(repo, it.Fork))
		}
	}
}

// displayRepoPath は repository 内の絶対パスを repo 相対で見せる。
// repo の外だった場合は絶対パスのまま返す（表示のためだけの関数であり、
// ここで失敗させる理由がない）。
func displayRepoPath(repo, abs string) string {
	rel, err := filepath.Rel(repo, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}
