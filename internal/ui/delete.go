// delete.go は homux profile delete（spec §12.11）の出力整形を担う。
//
// 参照の列挙（spec §12.11 の "is referenced by"）と削除 plan を 1 つの出力に
// まとめている。参照は必ず「削除」か「rewrite」のどちらかになるため、
// 動作で分けて並べたものがそのまま全参照の列挙になる。二度並べない。
//
// 選択画面を持たないため huh は登場しない（ADR 0011、rename.go と同じ）。
package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/bellwood4486/homux/internal/exec"
)

// RewriteLine は selector を書き換える 1 行である。どちらも repo ルートからの
// 相対パス。複数 profile selector からその profile を落とした形になる。
type RewriteLine struct {
	From string
	To   string
}

// HomeChange は削除の結果、target に配置される source が変わることを表す。
//
// これは desired state の差分であり、HOME の実状態ではない（この型を作るのに
// HOME を読まない）。実際に HOME を書き換えるのは "homux apply" である。
type HomeChange struct {
	// Target は HOME からの相対パス。
	Target string
	// From は現在選ばれている source の repo 相対パス。空なら今は未管理。
	From string
	// To は削除後に選ばれる source の repo 相対パス。空なら未管理になる。
	To string
}

// DeletePlan は profile delete が加える変更の全体である。
type DeletePlan struct {
	// Profile は削除する profile 名。
	Profile string
	// Removals は削除する source（"foo@@work"）の repo 相対パス。
	Removals []string
	// Rewrites は selector を書き換える source（"foo@@work+personal"）。
	// ファイルは消えない（spec §12.11）。
	Rewrites []RewriteLine
	// LocalActive は この PC の active profile が Profile であることを表す。
	// true なら local config が「profile なし」になる。
	LocalActive bool
	// HomeChanges は削除によって配置先が変わる target。HOME 自体は変更しない。
	HomeChanges []HomeChange
}

// RewriteCollision は rewrite 先の衝突を検出した 1 件である。
// 種類は rename と共通である（CollisionExists / CollisionDuplicate）。
type RewriteCollision struct {
	Line RewriteLine
	Kind CollisionKind
}

// RenderDeletePlan は spec §12.11 の plan を w に書き出す。
func RenderDeletePlan(w io.Writer, p DeletePlan) {
	n := len(p.Removals) + len(p.Rewrites)
	if n == 0 {
		fmt.Fprintf(w, "Profile %q is not referenced by any source.\n\n", p.Profile)
	} else {
		fmt.Fprintf(w, "Profile %q is referenced by %s.\n\n", p.Profile, countPhrase(n, "source", "sources"))
	}

	fmt.Fprintln(w, "Remove:")
	if len(p.Removals) == 0 {
		fmt.Fprintf(w, "  %s\n", noneChoice)
	}
	for _, r := range p.Removals {
		fmt.Fprintf(w, "  %s\n", r)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Rewrite selectors:")
	if len(p.Rewrites) == 0 {
		fmt.Fprintf(w, "  %s\n", noneChoice)
	}
	width := 0
	for _, r := range p.Rewrites {
		if len(r.From) > width {
			width = len(r.From)
		}
	}
	for _, r := range p.Rewrites {
		fmt.Fprintf(w, "  %-*s  -> %s\n", width, r.From, r.To)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Profile definition:")
	fmt.Fprintf(w, "  %s -> (removed)\n\n", p.Profile)

	if p.LocalActive {
		fmt.Fprintln(w, "Local active profile:")
		fmt.Fprintf(w, "  %s -> %s\n\n", p.Profile, noneChoice)
	}

	writeHomeChanges(w, p.HomeChanges)
}

// writeHomeChanges は「その結果 HOME がどう変わるか」を示す（spec §12.11）。
//
// 変化が無ければ節ごと出さない。この節は delete が引き起こす差分だけを
// 載せる。既に HOME にあった無関係な drift まで並べると、delete のせいで
// そうなったかのように読めてしまうためである。
func writeHomeChanges(w io.Writer, changes []HomeChange) {
	if len(changes) == 0 {
		return
	}

	fmt.Fprintln(w, "HOME after this change:")
	width := 0
	for _, c := range changes {
		if n := len(c.Target) + 2; n > width {
			width = n
		}
	}
	for _, c := range changes {
		fmt.Fprintf(w, "  %-*s  %s -> %s\n", width, "~/"+c.Target, sourceLabel(c.From), sourceLabel(c.To))
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "HOME is not touched here. Run %q afterwards.\n\n", "homux apply")
}

// sourceLabel は解決先の repo 相対パスを表示する。空は「その target を
// 管理する source が無い」ことを意味する。
func sourceLabel(repoPath string) string {
	if repoPath == "" {
		return "(unmanaged)"
	}
	return repoPath
}

// FormatRewriteCollision は rewrite 先の衝突を文字列化する。
// 末尾は改行で終わる（FormatResolveError と同じ約束）。
//
// この出力が出たとき repository は 1 バイトも変更されていない。delete は
// rename と同じく事前に全件を検証してから実行する。
func FormatRewriteCollision(c RewriteCollision) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ERROR selector rewrite collision\n\n")
	fmt.Fprintf(&b, "  %s\n", c.Line.From)
	fmt.Fprintf(&b, "  -> %s\n\n", c.Line.To)
	fmt.Fprintf(&b, "%s\n", rewriteCollisionReason(c.Kind))
	return b.String()
}

func rewriteCollisionReason(k CollisionKind) string {
	if k == CollisionDuplicate {
		return "Another source is rewritten to the same name."
	}
	return "Target already exists."
}

// RenderDeleteResult は exec.DeleteAll の結果を w に書き出す。
func RenderDeleteResult(w io.Writer, repo string, p DeletePlan, res exec.DeleteResult) {
	removed, rewritten := countApplied(res.Applied)

	if res.Err != nil {
		renderDeleteFailure(w, repo, p, res, removed, rewritten)
		return
	}

	fmt.Fprintf(w, "Deleted profile %q.\n", p.Profile)
	fmt.Fprintf(w, "Removed %s, rewrote %s.\n",
		countPhrase(removed, "file", "files"), countPhrase(rewritten, "selector", "selectors"))
	if p.LocalActive {
		fmt.Fprintf(w, "Active profile: %s -> %s\n", p.Profile, noneChoice)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Run %q to update HOME.\n", "homux apply")
}

func countApplied(items []exec.DeleteItem) (removed, rewritten int) {
	for _, it := range items {
		if it.Rewrite == "" {
			removed++
			continue
		}
		rewritten++
	}
	return removed, rewritten
}

// renderDeleteFailure は部分適用を報告する。
//
// ここに来るのは事前検証を通り抜けた想定外の I/O エラーだけである。
// ロールバックはしないため、利用者が手で終わらせるための手順を示す。
// .homux.toml はファイルの後に書くため、この時点では profile が残っている。
func renderDeleteFailure(w io.Writer, repo string, p DeletePlan, res exec.DeleteResult, removed, rewritten int) {
	fmt.Fprintf(w, "Removed %s and rewrote %s before failing.\n\n",
		countPhrase(removed, "file", "files"), countPhrase(rewritten, "selector", "selectors"))

	fmt.Fprintln(w, "Failed:")
	fmt.Fprintf(w, "  %s\n", displayRepoPath(repo, res.Failed.Path))
	fmt.Fprintf(w, "  %s\n", res.Err)

	if len(res.Pending) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Not processed:")
		for _, it := range res.Pending {
			fmt.Fprintf(w, "  %s\n", displayRepoPath(repo, it.Path))
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w,
		".homux.toml still defines %q. Finish the remaining changes with \"rm\" and \"mv\", then remove it from \"profiles\".\n",
		p.Profile)
}
