// select.go は fork 対象を選ぶ MultiSelect を担う。homux で huh を使う唯一の
// 場所である（ADR 0007、ADR 0010）。
//
// ここだけが端末を直接掴むため io.Writer 越しに検証できない。したがって
// この関数は「候補を受け取り、選ばれたものを返す」以外の判断を持たない。
// 衝突検証・plan の組み立て・確認・repository の変更はすべて呼び出し側にあり、
// テストは選択結果を差し替えて残り全体を検証する。
package ui

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
)

// ErrSelectionAborted は選択画面がユーザーの中断（Ctrl-C / Esc）で
// 終わったことを表す。
var ErrSelectionAborted = errors.New("selection aborted")

// maxSelectHeight は選択画面の高さの上限である。候補が数十件あっても端末を
// 埋め尽くさないための頭打ちであり、これを超える分はスクロールになる。
const maxSelectHeight = 17

// selectHeight は候補数に合わせた画面の高さを返す。候補が数件しかないときに
// 空行で画面を埋めないための調整である。+2 は title と description の分。
func selectHeight(candidates int) int {
	h := candidates + 2
	if h > maxSelectHeight {
		return maxSelectHeight
	}
	return h
}

// SelectForkTargets は candidates（repo 相対の common source）から、新しい
// profile 用に複製するものを選ばせる（spec §12.9）。
//
// 既定はすべて未選択である。profile を足しても、すべてのファイルを
// profile-specific に fork する必要はない（spec §12.9）。
func SelectForkTargets(profile string, candidates []string) ([]string, error) {
	var selected []string

	err := huh.NewMultiSelect[string]().
		Title(fmt.Sprintf("Select targets that should receive a %s-specific copy", profile)).
		Description("Space to toggle, / to filter, Enter to confirm. Unselected targets stay common.").
		Options(huh.NewOptions(candidates...)...).
		Value(&selected).
		Filterable(true).
		Height(selectHeight(len(candidates))).
		Run()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, ErrSelectionAborted
		}
		return nil, err
	}
	return selected, nil
}
