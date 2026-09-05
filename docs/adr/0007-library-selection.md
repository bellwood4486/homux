# 0007. ライブラリ選定

## 状況

CLI フレームワーク、対話 UI、glob、TOML パーサを選ぶ必要がある。2026 年 9 月時点の各ライブラリの状況を調査した。

| ライブラリ | 最新 | 日付 |
|---|---|---|
| `spf13/cobra` | v1.10.2 | 2025-12（2026-04 にもコミットあり） |
| `alecthomas/kong` | v1.16.1 | 2026-08 |
| `urfave/cli/v3` | v3.11.0 | 2026-08 |
| `charmbracelet/huh` | v1.0.0 | 2026-02 |
| `bmatcuk/doublestar/v4` | v4.10.0 | 2026-01 |
| `pelletier/go-toml/v2` | v2.4.3 | 2026-07 |

## 決定

| 用途 | 採用 |
|---|---|
| CLI フレームワーク | `spf13/cobra` |
| 対話 UI | `charmbracelet/huh` |
| glob | `bmatcuk/doublestar/v4` |
| TOML | `pelletier/go-toml/v2` |

## 却下した案

### CLI: `alecthomas/kong` / `urfave/cli/v3` / 標準 `flag`

`homux profile {list,create,use,rename,delete}` という 2 階層のサブコマンドがあり、shell 補完生成の価値が高い。cobra はタグの間隔こそ空くが 44.1k star でエコシステムが厚く、依然デファクトである。kong は 8.0k star で活発だが、エコシステムの差が大きい。標準 `flag` は 2 階層のディスパッチを自前で書く手間に見合わない。

`internal/cli` の外に cobra の型を漏らさないことで、必要になれば移行可能な状態を保つ。

### glob: gitignore 互換ライブラリ

gitignore 互換は `!` によるネガーションを持ち込むが、**`!` は negative selector として予約済み**（ADR 0005）であり、意味が二重になって混乱する。doublestar なら「repo ルートからの相対パスに glob をかける」の一文で説明が終わる。

### glob: 標準 `filepath.Match`

`**` も `/` 跨ぎも扱えず、`docs/**` を表現できない。

### TOML: `BurntSushi/toml`

安定しているが更新頻度が低め（v1.6.0 / 2025-12）。go-toml/v2 の方が活発である。

## 帰結

- **huh v1.0.0 は bubbletea v1.3.6 に依存する。** bubbletea 本体は既に v2 系（v2.0.9 / 2026-08）に移行しており、将来 huh v2 への追従が発生する
- huh は間接依存を含め約 30 パッケージを持ち込む。`internal/ui` に閉じ込め、huh の型を外部に漏らさない
