# 0002. profile の区切り子を `@@` とする

## 状況

profile-specific な source をファイル名で表現するために区切り子が必要である。当初の案は単一の `@`（`settings.json@work`）だった。

しかし `@` を含む正当な dotfile が現実に存在する。

```text
~/.config/systemd/user/tunnel@.service   systemd テンプレートユニット
~/.gitconfig@2024                        よくあるバックアップ命名
~/.ssh/config.d/work@example.com.conf
```

単一 `@` を区切り子にすると、これらは「profile 名が空」または「未定義 profile」としてエラーになる。

## 決定

**区切り子を `@@` とする。** 単一の `@` は特別な意味を持たない通常の文字として扱う。

```text
ファイル名の最後の "@@" 以降を selector とする（"@@" が無ければ common source）
```

## 却下した案

### 単一 `@` + profile 名文法による判定

「`@` の後ろが profile 名文法 `[a-z0-9][a-z0-9_-]*` に合致すれば selector、合致しなければファイル名の一部」とする案。

`tunnel@.service` は救えるが、**`.gitconfig@2024` を救えない**。`2024` は profile 名文法に合致してしまうため「未定義 profile」エラーになり、しかも逃げ道がない。さらに「合致すれば selector、しなければ literal」という二段構えの判定は説明が難しく、ユーザーが挙動を予測できない。

### 単一 `@` + `@@` によるエスケープ

`tunnel@@.service` と書けば literal な `@` として解釈する案。

repo 上のファイル名が `$HOME` 上のファイル名と一致しなくなる。`mv ~/.config/systemd/user/tunnel@.service repo/.config/systemd/user/` という**素直な直接操作が失敗する**ようになり、spec §3.5 / §16（普通のファイル操作を正規の操作として認める）と正面から衝突する。

## 帰結

- ファイル名が 1 文字長くなる（`settings.json@@work`）
- エスケープ機構が不要になり、パース規則が 1 行で説明できるようになった
- 「repo 上のファイル名は常に HOME 上のファイル名と一致する」を不変条件として立てられるようになった（INV-16）
- `@@` を含む正当なファイル名は管理できないが、実在しないと判断する
