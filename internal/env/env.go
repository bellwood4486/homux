// Package env は homux が動作する環境を型として持つ（docs/design.md §4）。
//
// グローバル変数と os.UserHomeDir() の直接呼び出しを禁止し、環境は明示的に
// 持ち回る。テストは Home / Repo を t.TempDir() に差し替えるだけで実 $HOME から
// 完全に切り離せる。
package env

// Env は Home と Repo の絶対パスである。
type Env struct {
	// Home は $HOME の絶対パス。
	Home string
	// Repo は repository の絶対パス。EvalSymlinks 済みであること。
	// これを怠るとリンク先の実体パスと一致せず、すべてが unmanaged と
	// 判定される（ADR 0003）。
	Repo string
}
