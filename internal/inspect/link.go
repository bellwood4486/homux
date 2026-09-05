package inspect

import (
	"path/filepath"
	"strings"
)

// resolveLink は symlink の生のリンク先 link を絶対パスへ正規化し、
// それが repo 配下に解決されるか（= managed か、spec §9.1）を返す。
//
// target は symlink 自身の絶対パスであり、相対リンクの基準になる。
//
// 単純な正規化で repo 配下と判定できなければ、経路上の symlink を解決してから
// 再判定する。env.Repo は EvalSymlinks 済み（ADR 0003）である一方、リンク先の
// 文字列はそうとは限らないためで、macOS の /var -> /private/var がこれに当たる。
// リンク先が dangling でも親ディレクトリから解決できる。
func resolveLink(repo, target, link string) (abs string, managed bool) {
	abs = link
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(filepath.Dir(target), abs)
	}
	abs = filepath.Clean(abs)

	if under(repo, abs) {
		return abs, true
	}
	if canonical, ok := canonicalize(abs); ok && under(repo, canonical) {
		return canonical, true
	}
	return abs, false
}

// canonicalize は abs の経路上の symlink を解決する。
// abs 自体が存在しなくても、親ディレクトリが解決できれば成功する。
func canonicalize(abs string) (string, bool) {
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, true
	}
	dir, base := filepath.Split(abs)
	resolvedDir, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		return "", false
	}
	return filepath.Join(resolvedDir, base), true
}

// under は path が root 配下（root 自身は含まない）かを返す。
func under(root, path string) bool {
	return strings.HasPrefix(path, root+string(filepath.Separator))
}
