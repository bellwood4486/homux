// write.go は init が必要とする書き込みを担う。
//
// local config は homux が全体を所有するため丸ごと Marshal してよい。
// これに対し .homux.toml はユーザーのコメントを保持する必要があり、
// 既存ファイルの更新は profiles 配列の範囲置換で行う（ADR 0008）。
// ここが提供するのは「まだ存在しないときの新規作成」だけである。
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// repoTemplate は init が書き出す .homux.toml の雛形である。
//
// ignore の候補をコメントアウトした状態で含めるのは spec §12.1 の要請であり、
// 生成物がコメント込みで人間に理解できることは INV-10 の要請である。
const repoTemplate = `# homux repository configuration (spec §8)

# 利用可能な profile。homux profile create で追加する。
profiles = []

# 配置対象から外すパス。repo ルートからの相対 glob で、** を含められる。
ignore = [
  # "README.md",
  # "LICENSE",
  # "docs/**",
]
`

// localFile は local config を書き出すための表現である。
//
// Local をそのまま Marshal しないのは、profile なしを profile キーの不在として
// 表すためである（docs/design.md §4.2）。omitempty のためだけに Local へ
// タグを足すと、読み取り側の「profile = "" も profile なしとして寛容に読む」
// という規約と紛らわしくなる。
type localFile struct {
	Repo    string `toml:"repo"`
	Profile string `toml:"profile,omitempty"`
}

// SaveLocal は local config を path へ書き出す。親ディレクトリは必要なら作る。
//
// 書き込みは一時ファイル + rename で行う。途中で失敗しても、読める古い設定が
// 残る方が、切り詰められた設定が残るより安全である。
func SaveLocal(path string, l *Local) error {
	b, err := toml.Marshal(localFile{Repo: l.Repo, Profile: l.Profile})
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // rename 成功後は存在しないので害はない

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// CreateRepoFile は .homux.toml の雛形を path へ新規作成する。
//
// 既に存在する場合は fs.ErrExist を包んだエラーを返し、1 バイトも書かない。
// ユーザーが書いたリポジトリ設定を init が踏み潰さないための保証である。
func CreateRepoFile(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(repoTemplate); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
