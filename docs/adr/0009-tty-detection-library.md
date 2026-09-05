# 0009. TTY 判定ライブラリに golang.org/x/term を採用する

## 状況

`--color auto`（既定）は非 TTY で色出力を無効にする（spec §11.1）。この判定には標準出力が
端末に接続されているかどうかの検出が必要で、標準ライブラリだけでは提供されない。

## 決定

**`golang.org/x/term`** を採用する。`term.IsTerminal(fd)` で判定する。

Go 公式チームが管理する準標準ライブラリであり、`charmbracelet/bubbletea`（`huh` の間接依存）を
含め事実上のデファクトである。`internal/ui` に閉じ込め、外部に型を漏らさない
（docs/design.md §2.1 の「色ライブラリを知ってよいのは internal/ui だけ」に従う）。

## 却下した案

### 標準ライブラリのみで簡易判定する（`os.Stdout.Stat()` の `ModeCharDevice` を見る）

追加依存なしで実現できるが、named pipe やソケット越しの挙動で `term.IsTerminal` と判定が
食い違うことがあり、`ls --color` や `git` など主要ツールが採用する検出方法と異なる。将来
`internal/ui` に実際の色出力（huh 経由）を実装する際、`bubbletea` が内部で `x/term` 相当の
判定を使うため、判定方法を二重に持つことになる。

## 帰結

- `internal/ui` のみが `golang.org/x/term` を import する。`.golangci.yml` の `ui-only-in-ui`
  ルールは huh/lipgloss/bubbletea/fatih/color の直接依存を禁じるものであり、`golang.org/x/term`
  は対象外だが、運用として `internal/ui` の外からは使わない
