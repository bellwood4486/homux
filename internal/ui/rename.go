// rename.go は homux profile rename（spec §12.10）の出力整形を担う。
//
// plan・衝突エラー・結果のすべてが素の io.Writer への書き出しである。
// 選択画面を持たないため huh は登場しない（ADR 0011）。
package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/bellwood4486/homux/internal/exec"
)

// RenameLine は plan に並ぶ 1 行である。どちらも repo ルートからの相対パス。
type RenameLine struct {
	From string
	To   string
}

// RenamePlan は profile rename が加える変更の全体である。
//
// exec に渡す絶対パスではなく repo 相対パスを持つのは、これが表示のための
// 型だからである。cli が両方を組み立てる。
type RenamePlan struct {
	// From / To は改名前後の profile 名。
	From string
	To   string
	// Files は単一 profile suffix を持つ source（"foo@@work"）。
	Files []RenameLine
	// Selectors は複数 profile selector を持つ source（"foo@@work+personal"）。
	// 名前の置換であり、削除ではない。
	Selectors []RenameLine
	// LocalActive は この PC の active profile が From であることを表す。
	// true なら local config も書き換わる（spec §11.2）。
	LocalActive bool
}

// CollisionKind は衝突の理由である。文言は ui が持ち、cli は種類だけを渡す。
type CollisionKind int

const (
	// CollisionExists は改名先が repository に既に存在する場合である。
	CollisionExists CollisionKind = iota
	// CollisionDuplicate は複数の source が同じ改名先を持つ場合である。
	CollisionDuplicate
)

// RenameCollision は衝突を検出した 1 件である。
type RenameCollision struct {
	Line RenameLine
	Kind CollisionKind
}

// RenderRenamePlan は spec §12.10 の plan を w に書き出す。
//
// Files と Selectors を分けるのは、後者が「ファイルを消さずに selector の
// 一部だけを置き換える」ことを確認の場で見せるためである。矢印の桁は
// 両方の節を通して揃える（spec §12.10 の例がそうなっている）。
func RenderRenamePlan(w io.Writer, pal Palette, p RenamePlan) {
	fmt.Fprintf(w, "Rename profile %q -> %q\n\n", p.From, p.To)

	fmt.Fprintln(w, "Profile definition:")
	fmt.Fprintf(w, "  %s -> %s\n\n", p.From, p.To)

	width := arrowWidth(p.Files, p.Selectors)
	writeRenameSection(w, "Files:", p.Files, width)
	writeRenameSection(w, "Selectors:", p.Selectors, width)

	if p.LocalActive {
		fmt.Fprintln(w, "Local active profile:")
		fmt.Fprintf(w, "  %s -> %s\n\n", p.From, p.To)
	}
}

func writeRenameSection(w io.Writer, header string, lines []RenameLine, width int) {
	fmt.Fprintln(w, header)
	if len(lines) == 0 {
		fmt.Fprintf(w, "  %s\n", noneChoice)
	}
	for _, l := range lines {
		fmt.Fprintf(w, "  %-*s  -> %s\n", width, l.From, l.To)
	}
	fmt.Fprintln(w)
}

func arrowWidth(groups ...[]RenameLine) int {
	width := 0
	for _, lines := range groups {
		for _, l := range lines {
			if len(l.From) > width {
				width = len(l.From)
			}
		}
	}
	return width
}

// FormatRenameCollision は spec §12.10 の衝突エラーを文字列化する。
// 末尾は改行で終わる（FormatResolveError と同じ約束）。
//
// この出力が出たとき repository は 1 バイトも変更されていない（INV-15）。
// 「どこまで進んだか」を書かないのはそのためである。
//
// pal が ColorOff なら出力は色を持たない従来通りの文字列である。
func FormatRenameCollision(pal Palette, c RenameCollision) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", pal.Error("ERROR rename collision"))
	fmt.Fprintf(&b, "  %s\n", c.Line.From)
	fmt.Fprintf(&b, "  -> %s\n\n", c.Line.To)
	fmt.Fprintf(&b, "%s\n", collisionReason(c.Kind))
	return b.String()
}

func collisionReason(k CollisionKind) string {
	if k == CollisionDuplicate {
		return "Another source is renamed to the same name."
	}
	return "Target already exists."
}

// RenderRenameResult は exec.RenameAll の結果を w に書き出す。
//
// 成功時に "homux apply" を促さないのは、rename が repo 上の名前だけを変え、
// HOME に配置される内容（target と中身）を変えないためである。profile use と
// 違い、desired state は rename の前後で同一である。
func RenderRenameResult(w io.Writer, pal Palette, repo string, p RenamePlan, res exec.RenameResult) {
	if res.Err != nil {
		renderRenameFailure(w, pal, repo, p, res)
		return
	}

	fmt.Fprintf(w, "Renamed profile %q -> %q.\n", p.From, p.To)
	fmt.Fprintf(w, "Renamed %s.\n", countPhrase(len(res.Applied), "file", "files"))
	if p.LocalActive {
		fmt.Fprintf(w, "Active profile: %s -> %s\n", p.From, p.To)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "HOME is unchanged: renaming a profile does not change what is deployed.")
}

// renderRenameFailure は部分適用を報告する。
//
// ここに来るのは事前検証（INV-15）を通り抜けた想定外の I/O エラーだけである。
// ロールバックはしないため、利用者が手で終わらせるための手順を示す。
// .homux.toml はファイルの後に書くため、この時点では必ず旧名のままである。
func renderRenameFailure(w io.Writer, pal Palette, repo string, p RenamePlan, res exec.RenameResult) {
	fmt.Fprintf(w, "Renamed %s before failing.\n\n", countPhrase(len(res.Applied), "file", "files"))

	fmt.Fprintln(w, pal.Error("Failed:"))
	fmt.Fprintf(w, "  %s\n", displayRepoPath(repo, res.Failed.From))
	fmt.Fprintf(w, "  %s\n", res.Err)

	if len(res.Pending) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Not renamed:")
		for _, it := range res.Pending {
			fmt.Fprintf(w, "  %s\n", displayRepoPath(repo, it.From))
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, ".homux.toml still defines %q. Rename the remaining files with \"mv\", then update \"profiles\".\n", p.From)
}
