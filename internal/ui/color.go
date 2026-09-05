// Package ui は色出力の制御と、将来の対話 UI 出力整形を集約する。
// huh・色ライブラリを import してよいのはこのパッケージだけである（docs/design.md §2.1）。
package ui

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// ColorMode は --color フラグの値を表す（spec §11.1）。
type ColorMode int

const (
	ColorAuto ColorMode = iota
	ColorAlways
	ColorNever
)

// ParseColorMode は --color フラグの文字列値をパースする。
func ParseColorMode(s string) (ColorMode, error) {
	switch s {
	case "auto":
		return ColorAuto, nil
	case "always":
		return ColorAlways, nil
	case "never":
		return ColorNever, nil
	default:
		return 0, fmt.Errorf("invalid --color value %q: must be one of auto, always, never", s)
	}
}

func (m ColorMode) String() string {
	switch m {
	case ColorAlways:
		return "always"
	case ColorNever:
		return "never"
	default:
		return "auto"
	}
}

// ResolveColorEnabled は mode・NO_COLOR 環境変数・fd の TTY 判定から
// 色を出力すべきかどうかを決定する（spec §11.1、design.md §8）。
//
// NO_COLOR は値によらず「設定されていること」自体が色出力を無効にする
// （spec §11.1: 「NO_COLOR が設定されている場合は色を出力しない」）。
func ResolveColorEnabled(mode ColorMode, fd int) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	switch mode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	default:
		return term.IsTerminal(fd)
	}
}
