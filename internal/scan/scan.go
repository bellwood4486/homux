// Package scan は repository を walk して []Source を得る（docs/design.md §1）。
//
// このパッケージは $HOME に一切触れない。HOME 上の実状態との突き合わせは
// inspect の責務である（docs/design.md §2.1）。この境界は lint では検知できないため、
// 実装・レビューで守ること。
package scan

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/bellwood4486/homux/internal/selector"
)

// spec §8.2 の暗黙除外。設定に関わらず常に deployment 対象外である。
// configFileName は config.RepoFileName と同じ値だが、
// scan が config に依存しないよう定数で持つ。
const (
	gitDir         = ".git"
	configFileName = ".homux.toml"
)

// Source は repository 上の 1 ファイルである。
type Source struct {
	// RepoPath は repo ルートからの相対パス（slash 区切り）。".claude/settings.json@@work"
	RepoPath string
	// Target は HOME からの相対パス（slash 区切り）。".claude/settings.json"
	// "@@" suffix は含まない（INV-16）。
	Target string
	// Selector は nil なら common source である。
	Selector *selector.Selector
	// SelectorErr は selector の構文エラー。scan では落とさず、
	// 診断を担う resolve まで運ぶ（spec §10.2）。非 nil なら Selector は nil。
	SelectorErr error
}

// Result は repository の走査結果である。
type Result struct {
	// Sources は deployment 候補となる source（ignore されなかったもの）。
	// RepoPath の辞書順に並ぶ。
	Sources []Source
	// Ignored は ignore ルールで除外された repo 相対パス。
	// spec §8.2 の暗黙除外（.homux.toml / .git）はここに含まない。
	Ignored []string
}

// Repository は root 配下を walk して Source を集める。
// ignore は repo ルートからの相対パスに対する doublestar パターンである（spec §8.1）。
func Repository(root string, ignore []string) (*Result, error) {
	for _, pattern := range ignore {
		if !doublestar.ValidatePattern(pattern) {
			return nil, fmt.Errorf("invalid ignore pattern %q", pattern)
		}
	}

	res := &Result{}
	err := filepath.WalkDir(root, func(abs string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if abs == root {
			return nil
		}

		rel := filepath.ToSlash(mustRel(root, abs))
		if d.IsDir() {
			if rel == gitDir {
				return fs.SkipDir
			}
			return nil
		}
		if rel == configFileName {
			return nil
		}

		ignored, err := matchAny(ignore, rel)
		if err != nil {
			return err
		}
		if ignored {
			res.Ignored = append(res.Ignored, rel)
			return nil
		}

		res.Sources = append(res.Sources, newSource(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func newSource(rel string) Source {
	dir, name := path.Split(rel)
	base, sel, err := selector.ParseName(name)
	if err != nil {
		if base == "" {
			// "@@work" のように base 名が無いファイル。target を一意にするため
			// ファイル名をそのまま使い、エラーは resolve に運ぶ。
			base = name
		}
		return Source{RepoPath: rel, Target: dir + base, SelectorErr: err}
	}
	return Source{RepoPath: rel, Target: dir + base, Selector: sel}
}

func matchAny(patterns []string, rel string) (bool, error) {
	for _, pattern := range patterns {
		ok, err := doublestar.Match(pattern, rel)
		if err != nil {
			return false, fmt.Errorf("invalid ignore pattern %q: %w", pattern, err)
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func mustRel(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		// WalkDir は必ず root 配下のパスを渡すため到達しない。
		panic(err)
	}
	return rel
}
