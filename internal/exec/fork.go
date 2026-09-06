// fork.go は homux profile create（spec §12.9）が行う「common source を
// profile-specific source へ複製する」を実行する。
//
// move ではなく copy である。common source を残さないと、profile なしの
// マシンと他 profile のマシンが一斉に配置先を失う（spec §17.2 の結果ツリーは
// .gitconfig と .gitconfig@@work の両方を持つ）。
//
// HOME には一切触れない（spec §11.2: profile create は repository だけを
// 変更する）。
package exec

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ForkItem は 1 ファイルの fork 対象である。どちらも絶対パスである。
type ForkItem struct {
	// Common は複製元の common source。
	Common string
	// Fork は作成する profile-specific source（"@@<profile>" 付き）。
	Fork string
}

// ForkResult は 1 回の ForkAll の結果である。AddResult と同じ形で部分適用を
// 報告する。
type ForkResult struct {
	// Applied は複製が完了した ForkItem。
	Applied []ForkItem
	// Failed は失敗した ForkItem。Err != nil のときだけ意味を持つ。
	Failed ForkItem
	// Pending は失敗により手つかずで残った ForkItem。ロールバックはしない。
	Pending []ForkItem
	// Err は停止した理由。nil なら全件を処理しきっている。
	Err error
}

// ForkAll は items を先頭から順に複製する。
func ForkAll(items []ForkItem) ForkResult {
	var res ForkResult
	for i, it := range items {
		if err := forkOne(it); err != nil {
			res.Failed = it
			res.Pending = items[i+1:]
			res.Err = err
			return res
		}
		res.Applied = append(res.Applied, it)
	}
	return res
}

// forkOne は 1 件を複製する。Fork が既に存在する場合は黙って上書きせず
// エラーで止める（cli 側が事前に検査済みだが、書き込みは宛先を切り詰めうる
// ため exec でも確かめる。addOne と同じ考え方）。
func forkOne(it ForkItem) error {
	src, err := os.Open(it.Common)
	if err != nil {
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("exec: %s is not a regular file", it.Common)
	}

	if err := os.MkdirAll(filepath.Dir(it.Fork), 0o755); err != nil {
		return err
	}

	// O_EXCL で開くことが「既存を上書きしない」の保証である。事前の Lstat と
	// 違い、検査と作成の間に割り込まれる余地がない。
	dst, err := os.OpenFile(it.Fork, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("exec: %s already exists in the repository", it.Fork)
		}
		return err
	}

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}
