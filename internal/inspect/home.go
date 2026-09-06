package inspect

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bellwood4486/homux/internal/env"
	"github.com/bellwood4486/homux/internal/resolve"
)

// spec §8.2 の暗黙除外。走査起点にもならない。
const (
	gitDir         = ".git"
	repoConfigFile = ".homux.toml"
)

// scanHome は spec §9.2 の種類2、すなわち desired から消えたのに $HOME に
// 残った managed symlink を探す。
//
// 走査起点は repo のトップレベルエントリに対応する HOME パスに限り、そこから
// 再帰する。symlink 自体は評価するが、その先には降りない（ADR 0004）。
// desired に含まれる target は desired 側で判定済みなので除外する。
// .homux.toml の ignore は repo path に対する規則なので、ここでは適用しない。
func scanHome(e env.Env, desired map[string]bool) ([]TargetState, error) {
	entries, err := os.ReadDir(e.Repo)
	if err != nil {
		return nil, err
	}

	s := homeScan{env: e, desired: desired}
	for _, entry := range entries {
		if name := entry.Name(); name == gitDir || name == repoConfigFile {
			continue
		}
		s.root(filepath.Join(e.Home, entry.Name()))
	}
	return s.found, nil
}

type homeScan struct {
	env     env.Env
	desired map[string]bool
	found   []TargetState
}

// root は走査起点 1 つを処理する。起点が symlink なら評価だけして降りない。
func (s *homeScan) root(path string) {
	fi, err := os.Lstat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			s.fail(path, err)
		}
		return
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		s.symlink(path)
	case fi.IsDir():
		s.dir(path)
	}
}

func (s *homeScan) dir(path string) {
	// repo が走査起点の配下にある配置（BEL-20）では、再帰がここで repo 自身に
	// 到達しうる。repo の中には決して降りない（INV-14）。
	if path == s.env.Repo {
		return
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		s.fail(path, err)
		return
	}
	for _, entry := range entries {
		child := filepath.Join(path, entry.Name())
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			s.symlink(child)
		case entry.IsDir():
			s.dir(child)
		}
		// 通常ファイルは unmanaged であり homux と無関係なので何もしない。
	}
}

// symlink は 1 つの symlink を評価し、managed かつ desired に無ければ
// 孤児（Stale 種類2）として記録する。
func (s *homeScan) symlink(path string) {
	rel := s.rel(path)
	if s.desired[rel] {
		return
	}
	link, err := os.Readlink(path)
	if err != nil {
		s.fail(path, err)
		return
	}
	abs, managed := resolveLink(s.env.Repo, path, link)
	if !managed {
		return
	}
	s.found = append(s.found, TargetState{
		Resolution: resolve.Resolution{Target: rel, Reason: resolve.ReasonAbsent},
		Kind:       KindStale,
		Current:    Current{Kind: CurrentSymlink, Link: link, LinkAbs: abs, Managed: true},
	})
}

// fail は 1 件の I/O エラーを記録する。走査全体は続行する。
func (s *homeScan) fail(path string, err error) {
	s.found = append(s.found, TargetState{
		Resolution: resolve.Resolution{Target: s.rel(path), Reason: resolve.ReasonAbsent},
		Kind:       KindError,
		Err:        err,
	})
}

func (s *homeScan) rel(path string) string {
	rel, err := filepath.Rel(s.env.Home, path)
	if err != nil {
		// 走査は必ず Home 配下のパスだけを辿るため到達しない。
		panic(err)
	}
	return filepath.ToSlash(rel)
}
