package resolve

import "fmt"

// AmbiguousError は複数の profile-specific source が一致したことを表す（spec §10.3、INV-07）。
type AmbiguousError struct {
	Target   string
	Profile  string
	Matching []string // RepoPath 昇順
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("ambiguous profile match for %q: %v", e.Target, e.Matching)
}

// UnknownProfileError は .homux.toml に未定義の profile が参照されたことを表す（spec §10.1）。
//
// RepoPath が空の場合は、source ではなく active profile 自体が未定義であることを表す。
type UnknownProfileError struct {
	RepoPath   string
	Profile    string
	Suggestion string // 近い profile 名。無ければ空
}

func (e *UnknownProfileError) Error() string {
	msg := fmt.Sprintf("unknown profile %q", e.Profile)
	if e.RepoPath != "" {
		msg = fmt.Sprintf("%s: %s", e.RepoPath, msg)
	}
	if e.Suggestion != "" {
		msg += fmt.Sprintf(" (did you mean %q?)", e.Suggestion)
	}
	return msg
}

// InvalidSelectorError は selector の構文エラーである（spec §10.2）。
type InvalidSelectorError struct {
	RepoPath string
	Err      error
}

func (e *InvalidSelectorError) Error() string {
	return fmt.Sprintf("%s: invalid selector syntax: %v", e.RepoPath, e.Err)
}

func (e *InvalidSelectorError) Unwrap() error { return e.Err }
