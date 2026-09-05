package inspect

import (
	"fmt"
	"os"
	"path/filepath"
)

// CurrentKind は HOME 上の target が実際に何であるかを表す。
type CurrentKind int

const (
	// CurrentAbsent は target が存在しないことを表す。
	CurrentAbsent CurrentKind = iota
	// CurrentFile は通常ファイルである。
	CurrentFile
	// CurrentDir はディレクトリである。
	CurrentDir
	// CurrentSymlink は symlink である。managed かどうかは Managed を見る。
	CurrentSymlink
)

func (k CurrentKind) String() string {
	switch k {
	case CurrentAbsent:
		return "absent"
	case CurrentFile:
		return "file"
	case CurrentDir:
		return "directory"
	case CurrentSymlink:
		return "symlink"
	default:
		return "unknown"
	}
}

// Current は HOME 上の target の実状態である。
//
// inspect が HOME を読むのは 1 度だけであり、plan / ui はこの値だけを見る。
// plan は os / io/fs を import できない（docs/design.md §2.1）ため、
// fs.FileMode ではなく CurrentKind で公開する。
type Current struct {
	// Kind は target が何であるか。
	Kind CurrentKind
	// Link は Readlink の生の値。相対リンクなら相対のまま。Kind が
	// CurrentSymlink のときのみ意味を持つ。
	Link string
	// LinkAbs は Link を絶対化・正規化したもの。
	LinkAbs string
	// Managed はリンク先が repo 配下に解決されるか（spec §9.1、ADR 0003）。
	Managed bool
}

// AncestorNotDirError は target の祖先が通常ファイル等で、ディレクトリを
// 作れないことを表す。Occupied の退避フロー（spec §12.4）では解決できないため、
// この target は Error として扱う。
type AncestorNotDirError struct {
	// Path は障害となっている祖先の絶対パス。
	Path string
}

func (e *AncestorNotDirError) Error() string {
	return fmt.Sprintf("%s is not a directory", e.Path)
}

// readCurrent は target（絶対パス）の HOME 上の実状態を読む。
// HOME を読むだけで、一切変更しない。
func readCurrent(repo, target string) (Current, error) {
	fi, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return Current{Kind: CurrentAbsent}, nil
		}
		if bad := firstNonDirAncestor(target); bad != "" {
			return Current{}, &AncestorNotDirError{Path: bad}
		}
		return Current{}, err
	}

	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		link, err := os.Readlink(target)
		if err != nil {
			return Current{}, err
		}
		abs, managed := resolveLink(repo, target, link)
		return Current{Kind: CurrentSymlink, Link: link, LinkAbs: abs, Managed: managed}, nil
	case fi.IsDir():
		return Current{Kind: CurrentDir}, nil
	default:
		return Current{Kind: CurrentFile}, nil
	}
}

// firstNonDirAncestor は target の祖先のうち、存在するがディレクトリでない
// 最も浅いものを返す。無ければ空文字列を返す。
func firstNonDirAncestor(target string) string {
	dir := filepath.Dir(target)
	var ancestors []string
	for {
		ancestors = append(ancestors, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		fi, err := os.Lstat(ancestors[i])
		if err != nil {
			continue
		}
		if !fi.IsDir() {
			return ancestors[i]
		}
	}
	return ""
}
