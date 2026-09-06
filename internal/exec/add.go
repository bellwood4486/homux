// add.go は homux add（spec §12.6）が行う「repo へ move → symlink 作成」を実行する。
//
// apply.go の Action / Apply とは別の型を持つ。add は resolve/plan の
// パイプラインを経由せず、cli/add が直接組み立てた対象に対して動作するため
// （target がまだ repository に存在しない以上、Resolution を作りようがない）。
package exec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// AddItem は 1 ファイルの取り込み対象である（spec §12.6）。
type AddItem struct {
	// Target は移動元の HOME 上の絶対パス。move 後、この場所に symlink を張り直す。
	Target string
	// RepoPath は移動先の repository 上の絶対パス。
	RepoPath string
	// Fork は --profile 指定時、同名の common source が既に存在する
	// （fork として作成する）ことを表す。表示にのみ使い、addOne の実行には
	// 関与しない（spec §12.6: 「plan に明示する」）。
	Fork bool
}

// AddResult は 1 回の AddAll の結果である。Apply の Result と同じ形で部分適用を
// 報告する。
type AddResult struct {
	// Applied は取り込みが完了した AddItem。
	Applied []AddItem
	// Failed は失敗した AddItem。Err != nil のときだけ意味を持つ。
	Failed AddItem
	// Pending は失敗により手つかずで残った AddItem。ロールバックはしない。
	Pending []AddItem
	// Err は停止した理由。nil なら全件を処理しきっている。
	Err error
}

// AddAll は items を先頭から順に取り込む。
func AddAll(items []AddItem) AddResult {
	var res AddResult
	for i, it := range items {
		if err := addOne(it); err != nil {
			res.Failed = it
			res.Pending = items[i+1:]
			res.Err = err
			return res
		}
		res.Applied = append(res.Applied, it)
	}
	return res
}

// addOne は 1 件を取り込む。RepoPath が既に存在する場合は黙って上書きせず
// エラーで止める（spec §12.6: 「repo 側に同名 source が既に存在する」はエラー。
// cli/add が事前に検査済みだが、os.Rename は既存の宛先を黙って上書きしうるため
// exec でも確かめる。removeSymlink が INV-14 を守るのと同じ考え方）。
func addOne(it AddItem) error {
	if _, err := os.Lstat(it.RepoPath); err == nil {
		return fmt.Errorf("exec: %s already exists in the repository", it.RepoPath)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(it.RepoPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(it.Target, it.RepoPath); err != nil {
		return err
	}
	return os.Symlink(it.RepoPath, it.Target)
}
