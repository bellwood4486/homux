// delete.go は homux profile delete（spec §12.11）が repository 上の source に
// 加える変更を実行する。
//
// 1 件は「削除」か「selector の rewrite」のどちらかである。複数 profile を
// 指す source（"foo@@work+personal"）を削除してはならず、残る profile だけを
// 指す名前へ改名する（spec §12.11 の表）。
//
// repository 内の source file を消すのはここだけである。INV-14 が禁じているのは
// 通常の apply による削除であり、profile delete は plan を見せて確認を取った
// うえでのみここへ来る。
//
// HOME には一切触れない（spec §11.2）。消した source を指していた HOME 側の
// symlink は dangling のまま残り、その解消は "homux apply" の仕事である。
package exec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// DeleteItem は 1 ファイルへの変更である。パスはすべて絶対パスである。
type DeleteItem struct {
	// Path は対象の source。
	Path string
	// Rewrite は selector を書き換えた後の名前。空なら Path を削除する。
	Rewrite string
}

// DeleteResult は 1 回の DeleteAll の結果である。RenameResult と同じ形で
// 部分適用を報告する。
type DeleteResult struct {
	// Applied は処理が完了した DeleteItem。
	Applied []DeleteItem
	// Failed は失敗した DeleteItem。Err != nil のときだけ意味を持つ。
	Failed DeleteItem
	// Pending は失敗により手つかずで残った DeleteItem。ロールバックはしない。
	Pending []DeleteItem
	// Err は停止した理由。nil なら全件を処理しきっている。
	Err error
}

// DeleteAll は items を先頭から順に処理する。
//
// cli 側が全件を事前検証してから呼ぶ。ここまで来て失敗するのは想定外の
// I/O エラーだけであり、その場合もロールバックはせず、どこまで進んだかを
// そのまま返す（RenameAll と同じ）。
//
// 並べ替えはしない。rewrite を先、削除を後に置くのは cli の責務である。
func DeleteAll(items []DeleteItem) DeleteResult {
	var res DeleteResult
	for i, it := range items {
		if err := deleteOne(it); err != nil {
			res.Failed = it
			res.Pending = items[i+1:]
			res.Err = err
			return res
		}
		res.Applied = append(res.Applied, it)
	}
	return res
}

// deleteOne は 1 件を処理する。
//
// rewrite 先が既に存在する場合は上書きせずエラーで止める。os.Rename は宛先を
// 黙って上書きするため、cli の事前検証を通り抜けた状態変化（別プロセスの
// 書き込みなど）で利用者のファイルを失わないための最後の防波堤である
// （renameOne と同じ考え方）。
func deleteOne(it DeleteItem) error {
	if it.Rewrite == "" {
		return os.Remove(it.Path)
	}
	if _, err := os.Lstat(it.Rewrite); err == nil {
		return fmt.Errorf("exec: %s already exists in the repository", it.Rewrite)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.Rename(it.Path, it.Rewrite)
}
