// Package config は .homux.toml（repository）と config.toml（local）の
// 読み取りを担う（spec §8、docs/design.md §4.2）。
//
// V1 では読み取りのみを提供する。.homux.toml の書き込みは profiles 配列の
// 範囲置換で行う（ADR 0008）。
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"github.com/bellwood4486/homux/internal/selector"
)

// ファイル名。RepoFileName は repository ルート直下に置かれる。
const (
	RepoFileName  = ".homux.toml"
	LocalFileName = "config.toml"
)

// Repo は repository 直下の .homux.toml の内容である（spec §8）。
type Repo struct {
	// Profiles は利用可能な profile の authoritative な定義である（spec §5.1）。
	Profiles []string `toml:"profiles"`
	// Ignore は repo ルートからの相対パスに対する glob である（spec §8.1）。
	Ignore []string `toml:"ignore"`
}

// LoadRepo は .homux.toml を読み取る。path はファイル自身のパスである。
func LoadRepo(path string) (*Repo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var repo Repo
	if err := toml.Unmarshal(b, &repo); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := repo.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &repo, nil
}

func (r *Repo) validate() error {
	seen := make(map[string]bool, len(r.Profiles))
	for _, p := range r.Profiles {
		if !selector.ValidProfileName(p) {
			return fmt.Errorf("invalid profile name %q in profiles: must match ^[a-z0-9][a-z0-9_-]*$", p)
		}
		if seen[p] {
			return fmt.Errorf("duplicate profile %q in profiles", p)
		}
		seen[p] = true
	}
	return nil
}

// HasProfile は name が profiles に定義済みかを返す。
func (r *Repo) HasProfile(name string) bool {
	for _, p := range r.Profiles {
		if p == name {
			return true
		}
	}
	return false
}

// Local はこの PC のローカル設定である（docs/design.md §4.2）。
// リポジトリには commit しない。
type Local struct {
	// Repo は repository のパス。--repo フラグで上書きされうる。
	Repo string `toml:"repo"`
	// Profile は active profile。空文字列は「profile なし」を意味する（spec §5.3）。
	Profile string `toml:"profile"`
}

// LoadLocal は local config を読み取る。path はファイル自身のパスである。
//
// ファイルが存在しない場合のエラーは fs.ErrNotExist を包むため、
// 呼び出し側は errors.Is で「未 init」を判別できる。
func LoadLocal(path string) (*Local, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var local Local
	if err := toml.Unmarshal(b, &local); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	// profile キーの不在と profile = "" はどちらも「profile なし」である。
	if local.Profile != "" && !selector.ValidProfileName(local.Profile) {
		return nil, fmt.Errorf("%s: invalid profile name %q: must match ^[a-z0-9][a-z0-9_-]*$", path, local.Profile)
	}
	return &local, nil
}

// LocalPath は local config のパスを返す（docs/design.md §4.2）。
// xdgConfigHome が空なら home/.config を使う。
func LocalPath(xdgConfigHome, home string) string {
	base := xdgConfigHome
	if base == "" {
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "homux", LocalFileName)
}
