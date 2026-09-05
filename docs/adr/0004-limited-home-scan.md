# 0004. HOME 走査を repo 対応部分に限定し、検出漏れを受容する

## 状況

Stale には 2 種類ある。

- **種類1**: profile 切替や source rename により、desired target のリンク先が変わった → desired targets を舐めるだけで検出できる
- **種類2**: source が削除・ignore 化され、desired から消えたのに symlink が残った → **desired に無いパスなので、HOME 側を走査しないと見つからない**

種類2 のために `$HOME` をどこまで歩くかを決める必要がある。

## 決定

**走査起点を「repo のトップレベルエントリに対応する HOME パス」に限定し、そこから再帰する。** symlink 自体は評価するが、その先には降りない。

```text
repo に .zshrc, .claude/, .config/ がある場合
  -> 走査対象は ~/.claude/ と ~/.config/ の配下のみ
```

これにより「repo からディレクトリごと source を削除した」場合の残骸を検出できないが、**この検出漏れを仕様として受容する。**

## 却下した案

### `$HOME` 全体を walk する

`~/Library`、`~/.cache`、`~/.rustup`、`~/node_modules` などにより、`status` が実用的な速度で終わらない。除外リストの運用も、ユーザー環境ごとに破綻する。

### `$HOME` 直下のドット始まりエントリすべてを起点にする

`~/.cache`、`~/.npm`、`~/.cargo` が巨大であり、全走査とほぼ同じ問題を抱える。

### git 履歴から過去の source path を引く

Git への依存が発生し、INV-03（Git はそのまま使う。ラップしない）の精神に反する。また shallow clone や履歴改変で壊れる。

## 帰結

- 検出漏れは dangling symlink として残るため、実害は小さい
- `homux explain <その残骸のパス>` を打てば個別には診断できる
- フルスキャン用のフラグは、実際に困る事例が出るまで追加しない
- `.homux.toml` の `ignore` は repo path に対する規則であり、HOME 走査には適用しない
