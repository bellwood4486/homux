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

// Palette は色を出力するかどうかを表す。ゼロ値（ColorOff）が無色である
// （spec §11.1: 既定は色なし相当の auto + 非 TTY）。
//
// 表す色数は状態の深刻度 3 段階（OK / Warn / Error）に限定する。8 色 ANSI の
// 範囲でこれ以上の色分けは行わない（spec 外・256色やテーマはスコープ外）。
type Palette bool

const (
	ColorOff Palette = false
	ColorOn  Palette = true
)

// ANSI エスケープの直書き。ライブラリは使わない（ADR 0009）。
const (
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiReset  = "\x1b[0m"
)

func (p Palette) paint(code, s string) string {
	if !p {
		return s
	}
	return code + s + ansiReset
}

// OK は正常・成功を表す文言に使う（例: Linked ラベル、Applied の件数）。
func (p Palette) OK(s string) string { return p.paint(ansiGreen, s) }

// Warn は要対応だが異常ではない状態に使う（例: Missing / Occupied / Stale、
// 確認や退避の発生）。
func (p Palette) Warn(s string) string { return p.paint(ansiYellow, s) }

// Error は構造エラーや失敗に使う（例: ERROR ブロック、Failed）。
func (p Palette) Error(s string) string { return p.paint(ansiRed, s) }

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
