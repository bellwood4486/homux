# 0015. Occupied の対話に HOME 優先の取り込みを選べるようにする

## 状況

`apply` は Occupied な target（unmanaged な HOME 上の実体があり、repo 側には既に対応する source がある状態）を、常に「HOME 側を退避してから repo 側の内容で symlink を張る」の一択で処理してきた（INV-13）。

これは repo が Source of Truth であることを前提にした動作だが、実際には「他マシンで先に HOME 側の設定が育ってしまっていて、その内容を repo に取り込みたい」というケースが起きる。現状では `apply` を一旦諦めて `add` をやり直す、あるいは手で退避ファイルを repo に move し直す、といった回避策しかない。

## 決定

Occupied の対話に、既存の `r`（repo 優先＝従来の退避）に加えて次の選択肢を追加する。

- `h`：HOME 優先（共通）。HOME 側の実体を、今まさに解決されている source（`Resolution.Selected`。profile 専用でもそのまま）へ move し、そこへ symlink を張る
- `p`：HOME 優先（profile 専用）。HOME 側の実体を `target@@<profile>` へ move し、そこへ symlink を張る。profile 名は対話中に入力させ、既定値はそのとき有効なアクティブ profile とする
- `n`（既定）：何もしない。従来通り永続化しない（INV-12）

対象が repo 外を指す symlink の場合は `h`/`p` を選択肢に出さない。`add`（spec §12.6）が「対象が他所を指す symlink」をエラーにしているのと同じ理由で、リンク先の実体をどこまで安全に move してよいか判断できないためである。

repo 側に同名の source が既に存在する場合（`h`/`p` いずれも起こりうる）、退避を作らず黙って上書きする。元の内容の保全は git 履歴（commit 済みであれば `git diff` / `git checkout` で復元可能）に委ねる。homux は git wrapper を作らない方針（INV-03）であり、ここでも例外を作らない。

型としては `exec.Confirm` の戻り値を `bool` から `Decision{ Resolution ConflictResolution; Profile string }` に拡張する。`ConflictResolution` は `ResolutionSkip` / `ResolutionKeepRepo` / `ResolutionAdoptCommon` / `ResolutionAdoptProfile` の enum とし、`exec` 内の `switch` で分岐する。`plan.Action` / `plan.ActionKind` は変更しない。Occupied は引き続き `plan` が純粋に `ReplaceTarget` 1 種類だけを生成し、「どう解決するか」という対話結果に依存する分岐は元から `exec.Confirm` の応答を見ていた場所（`ask` → `run`）にそのまま収める。

`apply --yes`（非対話実行）は引き続き repo 優先のみを行う。`h`/`p` を選ぶための新しいフラグ（例: `--on-conflict`）は追加しない。

## 却下した案

### `ConflictResolution` をポリモーフィズム（interface）で表現する

Occupied の解決策は仕様上 4 つに閉じており、後から外部プラグインのように増減する設計ではない。既存の `plan.ActionKind` は enum + `exec.run()` の `switch` で表現されており、ここだけ interface にすると同じ「種類によって実行を変える」という構造が 2 つのスタイルで混在する。加えて `golangci-lint` の `exhaustive` による網羅性チェックは enum の `switch` には効くが、interface の type switch は `default` を挟まざるを得ず、新しい型を足しても静的検査で漏れを検知できなくなる。

### `plan` が代替 Action を事前に複数生成する（Alternatives 方式）

Occupied に対して `plan` が「repo優先」「HOME優先(common)」「HOME優先(profile)」の Action をあらかじめ全部作っておき、`ui` が選ぶだけにする案。`exec` の責務（言われた通り実行するだけ）は保てるが、`plan`（純粋・`os` を import しない層）が profile 一覧や selector 構文の組み立てを知る必要が生じ、「`plan` は状態を Action に変換するだけ」という現設計の単純さを崩す。profile 名を対話中に変更したい場合、事前生成した Action をどう作り直すかという二度手間も生む。

### `apply --yes` でも HOME 優先を選べるようにする

`--on-conflict=repo|home|home-profile` のようなフラグを新設する案。非対話実行の分岐が増えて複雑になる一方、動機になるユースケース（CI などでの自動 HOME 優先取り込み）は現時点で無い。必要になれば改めて ADR を切る。

### repo 外を指す symlink も HOME 優先の対象にする

symlink のリンク先の実体を move する案。`add` が同じ状況をエラーにしている安全基準と矛盾する。

## 帰結

- `exec.Confirm` のシグネチャ変更に伴い、`ui.Prompter.ConfirmAction` と `internal/cli/apply.go` の `confirmFunc` を追従させる
- `internal/selector` に「HOME からの相対パス + profile 名 → repo 相対パス（`foo@@work` 形式）」を組み立てる逆変換ヘルパーを追加する
- 新しい不変条件 INV-17 を設ける。「`apply` が repository 内の source file を書き換えるのは、対話で HOME 優先（h/p）を明示的に選んだときに限る」
- Relink・RemoveStaleSymlink の確認は従来通り `[y/N]` のままで、`Decision` は `ResolutionKeepRepo`/`ResolutionSkip` の 2 値のみを使う
