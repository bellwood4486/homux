// Package selector はファイル名の "@@" suffix をパースする（spec §6、ADR 0002）。
//
// 純粋な文字列処理のみを行い、os を含む一切の外部依存を持たない
// （docs/design.md §2.1、.golangci.yml の depguard が強制する）。
package selector

import (
	"errors"
	"fmt"
	"strings"
)

// Delimiter は profile 区切り子である。単一の "@" は特別な意味を持たない
// 通常の文字として扱う（ADR 0002）。
const Delimiter = "@@"

// selector の構文エラー（spec §10.2）。errors.Is で判別する。
var (
	ErrEmptySelector      = errors.New("empty selector")
	ErrNegativeSelector   = errors.New("negative selector is reserved for a future version")
	ErrDuplicateProfile   = errors.New("duplicate profile in selector")
	ErrInvalidProfileName = errors.New("invalid profile name")
	ErrEmptyBaseName      = errors.New("empty base name")
)

// Selector は "@@" suffix が表す profile の集合である。
//
// V1 では positive な列挙のみを表現する。negative selector（"@@!server"）は
// 構文エラーとして扱う（ADR 0005）。将来 negative を追加する場合も、
// 一致判定は Matches に閉じているため呼び出し側は影響を受けない。
type Selector struct {
	Profiles []string
}

// ParseName はファイル名を base 名と selector に分割してパースする。
//
// ファイル名の最後の "@@" 以降を selector とし、"@@" が無ければ common source
// （sel == nil）とする。base 名は "@@" suffix を除いた部分であり、
// そのまま HOME 上のファイル名になる（INV-16）。
func ParseName(name string) (base string, sel *Selector, err error) {
	i := strings.LastIndex(name, Delimiter)
	if i < 0 {
		return name, nil, nil
	}

	base = name[:i]
	if base == "" {
		return "", nil, fmt.Errorf("%q: %w", name, ErrEmptyBaseName)
	}

	sel, err = Parse(name[i+len(Delimiter):])
	if err != nil {
		return base, nil, err
	}
	return base, sel, nil
}

// Parse は selector 文字列（"@@" より後ろの部分）をパースする（spec §6.5）。
//
// ここで検証するのは構文だけである。profile が .homux.toml に定義済みかどうかは
// 定義一覧を知る resolve の責務である。
func Parse(s string) (*Selector, error) {
	parts := strings.Split(s, "+")
	profiles := make([]string, 0, len(parts))

	for _, p := range parts {
		switch {
		case p == "":
			return nil, fmt.Errorf("%q: %w", s, ErrEmptySelector)
		case strings.HasPrefix(p, "!"):
			return nil, fmt.Errorf("%q: %w", s, ErrNegativeSelector)
		case !ValidProfileName(p):
			return nil, fmt.Errorf("%q: %w", p, ErrInvalidProfileName)
		}
		for _, seen := range profiles {
			if seen == p {
				return nil, fmt.Errorf("%q: %w", p, ErrDuplicateProfile)
			}
		}
		profiles = append(profiles, p)
	}

	return &Selector{Profiles: profiles}, nil
}

// Matches は active profile がこの selector に一致するかを返す。
//
// profile が空文字列のときは「profile なし」を意味し、profile-specific source は
// 一切一致しない（INV-09）。
func (s *Selector) Matches(profile string) bool {
	if profile == "" {
		return false
	}
	for _, p := range s.Profiles {
		if p == profile {
			return true
		}
	}
	return false
}

// ValidProfileName は profile 名が spec §5.4 の文法
// ^[a-z0-9][a-z0-9_-]*$ に合致するかを返す。
func ValidProfileName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case (c == '_' || c == '-') && i > 0:
		default:
			return false
		}
	}
	return true
}
