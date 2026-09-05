# homux

[![CI](https://github.com/bellwood4486/homux/actions/workflows/ci.yml/badge.svg)](https://github.com/bellwood4486/homux/actions/workflows/ci.yml)

複数のマシン・プロファイルで dotfiles を管理する Go CLI。

homux は、dotfiles repository を `$HOME` の mirror として扱います。管理対象の HOME 上のファイルは repository の実ファイルへの symlink になり、普段どおり編集した内容をそのまま Git で管理できます。

> **HOME mirror resolver + symlink manager + linter**

## クイックスタート

### 1. インストール

```bash
go install github.com/bellwood4486/homux@latest
```

Go 1.27 以降が必要です。

### 2. リポジトリを準備する

リポジトリの構造そのものが設定です。

```text
dotfiles/
├── .homux.toml
├── .zshrc
├── .gitconfig@@work
├── .gitconfig@@personal
└── .config/
    └── ghostty/
        └── config
```

`.homux.toml` には利用するプロファイルと、HOME に配置しないリポジトリ内のパスを書きます。

```toml
profiles = ["work", "personal"]

ignore = [
  "README.md",
  "LICENSE",
  "docs/**",
]
```

### 3. アクティブなプロファイルを選ぶ

アクティブなプロファイルはローカル設定で指定します。このファイルはリポジトリには commit しません。

```toml
# $XDG_CONFIG_HOME/homux/config.toml
# （XDG_CONFIG_HOME 未設定時は ~/.config/homux/config.toml）
repo = "~/dotfiles"
profile = "work"
```

プロファイルを使わない場合は `profile` を省略します。その場合は共通ソースだけが対象になります。

### 4. 状態を確認して適用する

```bash
homux status
homux apply --dry-run
homux apply
```

`apply` は必要な symlink を作成・差し替え、不要になった管理済み symlink を削除します。既存の管理外ファイルを置き換えるときは、通常は確認してから同じディレクトリへ退避します。`--yes` を指定すると確認を省略できます。

## 仕組み

サフィックスを除いたリポジトリ内のパスが、HOME 側の配置先になります。

```text
repo/.zshrc                         -> ~/.zshrc
repo/.config/ghostty/config         -> ~/.config/ghostty/config
repo/.gitconfig@@work               -> ~/.gitconfig   (active profile = work)
```

プロファイルセレクタの区切り子は `@@` です。単一の `@` は特別扱いされないため、`tunnel@.service` のようなファイル名もそのまま共通ソースとして使えます。

## コマンド

現在利用できるコマンドは次のとおりです。

| コマンド | 説明 |
|---|---|
| `homux status [--all] [--verbose]` | HOME と期待状態の差分を表示する |
| `homux explain <path>` | 1 ファイルがその状態になった理由を説明する |
| `homux apply` | 期待状態を HOME に適用する |
| `homux apply --dry-run` | 適用する操作だけを表示する |
| `homux apply --yes` | 確認を省略して非対話で適用する |
| `homux version` | バージョンを表示する |

`status --all` で Linked / Ignored / Inactive も表示できます。`--repo <path>` と `--color auto|always|never` はグローバルフラグです。
`homux version` のほか、`homux --version` でもバージョンを表示できます。

## 設計上の境界

homux は意図的に小さく保ちます。

- テンプレートエンジン、変数置換、JSON merge、プロファイル継承は行わない
- ソースはファイル単位で symlink にする。ディレクトリ symlink は作らない
- 管理状態の manifest / state database は持たない
- `apply` がリポジトリ内のソースファイルを削除することはない
- 複数のセレクタが一致した場合は、優先順位を付けずエラーにする

リポジトリ内は普通の `mv` / `cp` / `rm` / `git` で編集できます。変更後に `status`、`explain`、`apply --dry-run` で結果を確認してください。

## プロジェクトの状態

開発中です。コマンドや挙動が変わる可能性があります。詳細な要求・設計・履歴は次の文書を参照してください。

- [仕様](docs/spec.md) — 何を作るか
- [設計](docs/design.md) — どう作るか
- [Architecture Decision Records](docs/adr/README.md) — なぜそう決めたか

## 開発

```bash
mise install
just setup
just check
```

`just check` はフォーマットチェック、`go vet`、lint、test をまとめて実行します。

## ライセンス

[MIT](LICENSE)
