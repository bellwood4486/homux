# 0003. 管理判定を「repo 配下を指す symlink」とし、状態 DB を持たない

## 状況

`status` が Stale を報告するには、「この symlink は homux が張ったものか」を判定する必要がある。

## 決定

**状態データベースを一切持たない。** 次の 1 つの規則だけで判定する。

> `$HOME` 上の symlink であり、そのリンク先が repo ルート配下に解決されるものを managed とみなす。

リンク先が存在しない dangling symlink であっても、リンク先パスが repo 配下であれば managed である。

## 却下した案

### manifest ファイル（`~/.local/state/homux/manifest` 等）を持つ

INV-04（profile の挙動はファイル名に表現し、hidden mapping state を持たない）および INV-10（CLI が変更しても結果は CLI なしで理解可能）に正面から反する。

さらに実用上の問題として、ユーザーが `mv` / `cp` / `rm` でリポジトリを直接編集することを正式な運用として認めている以上（spec §3.5）、manifest は容易に実態とずれる。ずれた manifest は、無いよりも危険である。

### 「リンク先が実在する source ファイルであること」も判定条件に加える

source が削除された場合に dangling symlink が managed と判定されなくなり、Stale を検出できなくなる。これは検出したいケースそのものである。

## 帰結

- ユーザーが手動で張った repo 配下への symlink も managed として扱われる。これは実害がなく、むしろ期待される挙動である
- repo path を `filepath.EvalSymlinks` で実体解決してから保存することが**必須**になる。怠るとリンク先の実体パスと repo path が一致せず、すべてが unmanaged と判定される
