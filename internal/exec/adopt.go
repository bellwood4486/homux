// adopt.go は Occupied の HOME 優先の解決策（spec §12.4.1、ADR 0015）を
// 実行する。add.go の「repo へ move → 元の位置に symlink」と同じ操作だが、
// repoPath に既存の source があっても退避せず上書きする点が異なる
// （元の内容の保全は git 履歴に委ねる。INV-17）。
package exec

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/plan"
)

// adopt は a.Target（HOME 側の実体）を repoPath へ move し、symlink を
// 張り直す。
//
// a.Current が symlink（repo 外を指す）のときはエラーを返す。リンク先の
// 実体をどこまで安全に move してよいか判断できないためで、add（spec
// §12.6）が同じ状況をエラーにしているのと同じ理由である。ui が h/p を
// symlink 対象には出さない設計だが、ここでも不整合として弾く。
func adopt(a plan.Action, repoPath string) error {
	if a.Current == inspect.CurrentSymlink {
		return fmt.Errorf("exec: %s is a symlink; adopt is not supported", a.Target)
	}
	if err := os.RemoveAll(repoPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(repoPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(a.Target, repoPath); err != nil {
		return err
	}
	return os.Symlink(repoPath, a.Target)
}
