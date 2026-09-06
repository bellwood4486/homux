# Architecture Decision Records

このディレクトリは**履歴に意味がある決定**を記録する。

- 「今の仕様」は `docs/spec.md`、「今の設計」は `docs/design.md` を見ること。ADR は現在の状態を記述しない。
- ADR の価値は **「なぜ他の選択肢ではないのか」** にある。採用した案が何かは spec / design に書いてある。
- 一度書いた ADR は書き換えない。決定が覆った場合は新しい ADR を追加し、古い方に `Superseded by 00NN` と追記する。

| # | 決定 |
|---|---|
| [0001](0001-file-level-symlinks.md) | symlink はファイル単位とする |
| [0002](0002-double-at-delimiter.md) | profile の区切り子を `@@` とする |
| [0003](0003-no-state-database.md) | 管理判定を「repo 配下を指す symlink」とし、状態 DB を持たない |
| [0004](0004-limited-home-scan.md) | HOME 走査を repo 対応部分に限定し、検出漏れを受容する |
| [0005](0005-no-negative-selector.md) | negative selector を V1 に含めない |
| [0006](0006-no-permission-management.md) | ファイルパーミッションを管理しない |
| [0007](0007-library-selection.md) | ライブラリ選定（cobra / huh / doublestar / go-toml v2） |
| [0008](0008-toml-range-replace.md) | `.homux.toml` は `profiles` 配列だけを範囲置換する |
| [0009](0009-tty-detection-library.md) | TTY 判定に golang.org/x/term を採用する |
| [0010](0010-plain-prompts-for-init.md) | `init` の対話を素のプロンプトで実装し、huh の導入を遅らせる |
| [0011](0011-huh-for-fork-selection-only.md) | `profile create` の選択画面にだけ huh を使い、素のプロンプトと同居させる |
