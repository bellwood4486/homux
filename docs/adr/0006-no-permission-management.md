# 0006. ファイルパーミッションを管理しない

## 状況

symlink 方式では、実効パーミッションは repo 側のファイルモードになる。ところが **Git は実行ビット以外のモードを保持しない。**

その結果、`~/.ssh/config` や `~/.ssh/known_hosts` を homux で管理すると、clone 直後は 644 となり、ssh が `Bad owner or permissions` で接続を拒否する。

## 決定

**V1 のスコープ外とする。** README に「パーミッションが重要なファイルは homux の管理対象にしない」と明記するに留め、`status` での警告も行わない。

## 却下した案

### `.homux.toml` に `[permissions]` セクションを設け、`apply` 時に chmod する

これは per-file mapping そのものであり、spec §8（`.homux.toml` に per-file mapping を追加しない）が明示的に禁じているものである。「repo 構造そのものが設定」という思想の最初の綻びになる。

### `status` で `.ssh/` 配下の source を検出して警告する

パス名によるヒューリスティックであり、`.gnupg/` や `.aws/credentials` など際限なく増える。また警告が出ても homux 側に解決手段がないため、ユーザーは警告を無視することを学習するだけになる。

## 帰結

- `~/.ssh/config` を管理したい場合、ユーザーは homux を使わないか、clone 後に手動で `chmod` する必要がある
- この制限は spec §15（既知の制限）に明記する。知らずに踏むとバグに見えるため
