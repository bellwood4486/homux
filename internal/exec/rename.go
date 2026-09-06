// rename.go は homux profile rename（spec §12.10）が行う「repository 上の
// "@@" suffix を改名する」を実行する。
//
// 改名するのはファイル名の suffix だけであり、親ディレクトリは変わらない
// （INV-16: "@@" を除いた名前は HOME 上の名前と一致するため、rename しても
// target は動かない）。したがってディレクトリを作る必要はない。
//
// HOME には一切触れない（spec §11.2: profile rename は repository と local
// config だけを変更する）。
package exec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// RenameItem は 1 ファイルの改名対象である。どちらも絶対パスである。
type RenameItem struct {
	// From は改名元（"@@<old>" を含む）。
	From string
	// To は改名先（"@@<new>" を含む）。
	To string
}

// RenameResult は 1 回の RenameAll の結果である。ForkResult と同じ形で
// 部分適用を報告する。
type RenameResult struct {
	// Applied は改名が完了した RenameItem。
	Applied []RenameItem
	// Failed は失敗した RenameItem。Err != nil のときだけ意味を持つ。
	Failed RenameItem
	// Pending は失敗により手つかずで残った RenameItem。ロールバックはしない。
	Pending []RenameItem
	// Err は停止した理由。nil なら全件を処理しきっている。
	Err error
}

// RenameAll は items を先頭から順に改名する。
//
// cli 側が全件の衝突を事前検証してから呼ぶ（INV-15）。ここまで来て失敗するのは
// 想定外の I/O エラーだけであり、その場合もロールバックはせず、どこまで進んだ
// かをそのまま返す。
func RenameAll(items []RenameItem) RenameResult {
	var res RenameResult
	for i, it := range items {
		if err := renameOne(it); err != nil {
			res.Failed = it
			res.Pending = items[i+1:]
			res.Err = err
			return res
		}
		res.Applied = append(res.Applied, it)
	}
	return res
}

// renameOne は 1 件を改名する。To が既に存在する場合は上書きせずエラーで
// 止める。
//
// os.Rename は宛先を黙って上書きするため、fork の O_EXCL のような原子的な
// 保証は得られない。それでも直前に確かめるのは、cli の事前検証を通り抜けた
// 状態変化（別プロセスの書き込みなど）で利用者のファイルを失わないための
// 最後の防波堤である。
func renameOne(it RenameItem) error {
	if _, err := os.Lstat(it.To); err == nil {
		return fmt.Errorf("exec: %s already exists in the repository", it.To)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.Rename(it.From, it.To)
}
