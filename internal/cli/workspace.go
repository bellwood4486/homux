// workspace.go は status / explain など読み取り専用コマンドが共通して必要な
// env.Env と設定の組み立てを担う（docs/design.md §4）。
package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bellwood4486/homux/internal/config"
	"github.com/bellwood4486/homux/internal/env"
)

// workspace は 1 回のコマンド実行に必要な環境と設定である。
type workspace struct {
	env     env.Env
	repo    *config.Repo
	profile string // active profile。空文字列は「profile なし」（spec §5.3）。
}

// notConfiguredError は repository path が --repo にも local config にも
// 無いことを表す。init は Phase 4 でまだ実装されていないが、その前段の状態
// として自然に発生するため、ここで扱う。
type notConfiguredError struct{}

func (notConfiguredError) Error() string {
	return `repository is not configured: pass --repo <path>, or run "homux init" first`
}

// loadWorkspace は --repo フラグ（repoFlag、空文字列なら未指定）と
// local config から env.Env・active profile・.homux.toml を組み立てる
// （docs/design.md §4.1: 解決順序は --repo フラグ → local config の repo）。
//
// status / explain のような読み取り専用コマンドはこれだけで完結する。
func loadWorkspace(repoFlag string) (*workspace, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}

	localPath := config.LocalPath(os.Getenv("XDG_CONFIG_HOME"), home)
	local, err := config.LoadLocal(localPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		local = &config.Local{}
	}

	repoPath := repoFlag
	if repoPath == "" {
		repoPath = local.Repo
	}
	if repoPath == "" {
		return nil, notConfiguredError{}
	}

	repoAbs, err := resolveRepoPath(repoPath)
	if err != nil {
		return nil, err
	}

	repoCfg, err := config.LoadRepo(filepath.Join(repoAbs, config.RepoFileName))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s: not a homux repository (missing %s)", repoAbs, config.RepoFileName)
		}
		return nil, err
	}

	return &workspace{
		env:     env.Env{Home: home, Repo: repoAbs},
		repo:    repoCfg,
		profile: local.Profile,
	}, nil
}

// resolveRepoPath は repo path を絶対パス化し、filepath.EvalSymlinks で実体
// まで解決する（docs/design.md §4.2）。これを怠るとリンク先の実体パスと
// 一致せず、すべての symlink が unmanaged と判定される（ADR 0003）。
func resolveRepoPath(path string) (string, error) {
	expanded, err := expandTilde(path)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("repository path %q: %w", path, err)
	}
	return resolved, nil
}

// expandTilde は先頭の "~" を os.UserHomeDir() に展開する
// （docs/design.md §4.2 の local config 例: repo = "~/dotfiles"）。
func expandTilde(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}
