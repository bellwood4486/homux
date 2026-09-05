// diagnostic.go は resolve パッケージのエラーを spec §10 のレイアウトで
// 整形する。ui が唯一の出力整形の集約点であるため、ここに置く
// （docs/design.md §8）。
package ui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bellwood4486/homux/internal/resolve"
)

// FormatResolveError は resolve パッケージのエラーを spec §10.1〜10.3 の
// レイアウトで文字列化する。末尾は改行で終わる。
//
// errors.Join でまとめられた複数エラー（同一 target に複数の selector 問題が
// あるケース）は、それぞれを個別のブロックとして空行区切りで並べる。
func FormatResolveError(err error) string {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		errs := joined.Unwrap()
		parts := make([]string, 0, len(errs))
		for _, e := range errs {
			parts = append(parts, strings.TrimRight(FormatResolveError(e), "\n"))
		}
		return strings.Join(parts, "\n\n") + "\n"
	}

	var unknownProfile *resolve.UnknownProfileError
	if errors.As(err, &unknownProfile) {
		return formatUnknownProfile(unknownProfile)
	}
	var invalidSelector *resolve.InvalidSelectorError
	if errors.As(err, &invalidSelector) {
		return formatInvalidSelector(invalidSelector)
	}
	var ambiguous *resolve.AmbiguousError
	if errors.As(err, &ambiguous) {
		return formatAmbiguous(ambiguous)
	}
	return fmt.Sprintf("ERROR\n\n  %s\n", err)
}

// formatUnknownProfile は spec §10.1 を実装する。RepoPath が空なのは
// active profile 自体が未定義の場合だけである（source ではなく設定の問題）。
func formatUnknownProfile(e *resolve.UnknownProfileError) string {
	header := e.RepoPath
	if header == "" {
		header = "active profile"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "ERROR %s\n\n", header)
	fmt.Fprintf(&b, "  Unknown profile %q.\n", e.Profile)
	if e.Suggestion != "" {
		fmt.Fprintf(&b, "  Did you mean %q?\n", e.Suggestion)
	}
	return b.String()
}

// formatInvalidSelector は spec §10.2 を実装する。
func formatInvalidSelector(e *resolve.InvalidSelectorError) string {
	return fmt.Sprintf("ERROR %s\n\n  Invalid selector syntax.\n", e.RepoPath)
}

// formatAmbiguous は spec §10.3 を実装する。Target は HOME からの相対パスなので
// "~/" を前置して表示する。
func formatAmbiguous(e *resolve.AmbiguousError) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ERROR ambiguous profile match\n\n")
	fmt.Fprintf(&b, "  Target:\n    ~/%s\n\n", e.Target)
	fmt.Fprintf(&b, "  Matching sources:\n")
	for _, m := range e.Matching {
		fmt.Fprintf(&b, "    %s\n", m)
	}
	return b.String()
}
