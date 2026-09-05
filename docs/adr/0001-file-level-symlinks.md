# 0001. symlink はファイル単位とする

## 状況

`$HOME` 上に symlink を張る粒度には 2 通りある。ファイル単位（`~/.config/ghostty/config` を symlink にする）と、ディレクトリ単位（`~/.config/ghostty` 自体を symlink にする。GNU Stow でいう folding）。

ディレクトリ単位はリンク数が減り、見た目も簡潔になる。

## 決定

**常にファイル単位の symlink とする。** ディレクトリは symlink の対象ではなく「経路」として扱い、`apply` は必要な親ディレクトリを実ディレクトリとして `MkdirAll` する。

## 却下した案

### ディレクトリ単位の symlink（Stow 的 folding）

そのディレクトリに未管理のファイルが 1 つ置かれた瞬間、それがリポジトリの中に出現してしまう。

本ツールの中心的ユースケースは `~/.claude/settings.json` の管理だが、`~/.claude/` は Claude Code 自身が `todos/`、`projects/`、`shell-snapshots/` などを大量に生成するディレクトリである。ここをディレクトリ単位で symlink にすると、`git status` が生成物で埋まり、INV-01（repository が Source of Truth）が事実上崩壊する。

### 設定で切り替え可能にする

「repo 構造そのものが設定である」という思想（spec §3.1）に反する設定項目が増える。切り替えの判断をユーザーに押し付けるだけの価値がない。

## 帰結

- リンク数は多くなる。`.config` 配下が深い場合、target ごとに symlink が 1 つ増える
- `add` でディレクトリを指定した場合、配下の全ファイルを再帰的に処理する必要がある
- stale symlink を削除した後に空ディレクトリが残る。homux はそれを削除しない（作成者を判別できないため）
