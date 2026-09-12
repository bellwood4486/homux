// Package exec は plan が作った []Action を実行する（spec §12.4、docs/design.md §1）。
//
// ファイルシステムを変更してよいのはこのパッケージだけである
// （docs/design.md §2.1）。Action を作る判断は一切持たない。それは plan の責務。
package exec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bellwood4486/homux/internal/plan"
)

// ConflictResolution は Occupied（ReplaceTarget）の解決策である
// （spec §12.4.1、ADR 0015）。Relink・RemoveStaleSymlink は
// ResolutionKeepRepo/ResolutionSkip の 2 値だけを使う。
type ConflictResolution int

const (
	// ResolutionSkip は何もしない（既定）。INV-12: 永続化しない。
	ResolutionSkip ConflictResolution = iota
	// ResolutionKeepRepo は repo 優先。ReplaceTarget では退避してから
	// symlink を張る（従来の唯一の動作）。Relink/RemoveStaleSymlink では
	// そのまま実行する。
	ResolutionKeepRepo
	// ResolutionAdopt は HOME 優先（共通 / profile 専用のどちらも）。
	// AdoptPath への move + symlink を行う。ReplaceTarget にのみ現れる。
	ResolutionAdopt
)

// Decision は Confirm の応答である。
type Decision struct {
	// Resolution は選ばれた解決策。
	Resolution ConflictResolution
	// AdoptPath は ResolutionAdopt のときの取り込み先 repo 絶対パス。
	// exec は Home/Repo のパスを知らないため、組み立ては呼び出し側
	// （ui）の責務である（ADR 0015）。他の Resolution では常に空。
	AdoptPath string
}

// Confirm は Action をどう解決するかを問う。nil なら確認なしで
// ResolutionKeepRepo として全件実行する。
type Confirm func(plan.Action) (Decision, error)

// Result は 1 回の Apply の結果である。
type Result struct {
	// Applied は適用が完了した Action。
	Applied []plan.Action
	// Skipped は Confirm が false を返した Action。エラーではないため
	// 後続の適用は続く。この選択は永続化しない（INV-12）。
	Skipped []plan.Action
	// Failed は失敗した Action。Err != nil のときだけ意味を持つ。
	Failed plan.Action
	// Pending は失敗により手つかずで残った Action。ロールバックはしないため、
	// 再実行すれば収束する（apply は冪等である。spec §12.4）。
	Pending []plan.Action
	// Err は停止した理由。nil なら全件を処理しきっている。
	Err error
}

// Apply は actions を先頭から順に実行する。
func Apply(actions []plan.Action, confirm Confirm) Result {
	var res Result
	for i, a := range actions {
		d, err := ask(confirm, a)
		if err != nil {
			res.Failed = a
			res.Pending = actions[i+1:]
			res.Err = err
			return res
		}
		if d.Resolution == ResolutionSkip {
			res.Skipped = append(res.Skipped, a)
			continue
		}
		if err := run(a, d); err != nil {
			res.Failed = a
			res.Pending = actions[i+1:]
			res.Err = err
			return res
		}
		res.Applied = append(res.Applied, a)
	}
	return res
}

// ask は Action の解決策を問う。確認を要さない Action（Missing のみ。
// spec §12.4）と confirm が nil のときは問わず ResolutionKeepRepo を返す。
func ask(confirm Confirm, a plan.Action) (Decision, error) {
	if !a.Confirm || confirm == nil {
		return Decision{Resolution: ResolutionKeepRepo}, nil
	}
	return confirm(a)
}

// run は Action 1 件を実行する。
func run(a plan.Action, d Decision) error {
	switch a.Kind {
	case plan.CreateSymlink:
		return createSymlink(a)
	case plan.Relink:
		if err := removeSymlink(a.Target); err != nil {
			return err
		}
		return createSymlink(a)
	case plan.ReplaceTarget:
		if d.Resolution == ResolutionAdopt {
			return adopt(a, d.AdoptPath)
		}
		if err := backup(a); err != nil {
			return err
		}
		return createSymlink(a)
	case plan.RemoveStaleSymlink:
		// 親ディレクトリが空になっても削除しない（spec §12.4）。
		return removeSymlink(a.Target)
	default:
		return fmt.Errorf("exec: unsupported action kind %v", a.Kind)
	}
}

// backup は unmanaged な target を Backup へ退避する（INV-13）。
//
// 同一ディレクトリ内の移動なので Rename を使う。コピーではないため中身も
// パーミッションもそのまま残り、exec がモードを触ることもない（ADR 0006）。
func backup(a plan.Action) error {
	if a.Backup == "" {
		// plan が Backup を必ず埋める（INV-13）。空なら plan の破れであり、
		// 上書きに倒すのではなく止める。
		return fmt.Errorf("exec: %s has no backup path", a.Target)
	}
	if _, err := os.Lstat(a.Backup); err == nil {
		return fmt.Errorf("exec: backup %s already exists", a.Backup)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.Rename(a.Target, a.Backup)
}

// removeSymlink は target が symlink であることを確かめてから削除する。
// exec は破壊的操作を行う唯一の場所であるため、plan の判定を信用せず
// ここでも symlink 以外を消さないことを保証する（INV-14）。
func removeSymlink(target string) error {
	fi, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("exec: %s is not a symlink", target)
	}
	return os.Remove(target)
}

// createSymlink は target の親を作ってから symlink を張る。
// 親ディレクトリの作成は独立した Action にせず暗黙に行う（docs/design.md §3）。
func createSymlink(a plan.Action) error {
	if err := os.MkdirAll(filepath.Dir(a.Target), 0o755); err != nil {
		return err
	}
	return os.Symlink(a.LinkTo, a.Target)
}
