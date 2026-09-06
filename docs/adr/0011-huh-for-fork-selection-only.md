# 0011. `profile create` の選択画面にだけ huh を使い、素のプロンプトと同居させる

## 状況

ADR 0010 は `init` の対話を素のプロンプトで実装し、huh の導入を
「`profile create` の migration wizard で MultiSelect が必要になった時点」まで
遅らせた。そのうえで、導入時に「素のプロンプトと huh の同居を許すか、
素のプロンプトへ寄せきるか」を決めることを宿題として残した。

`profile create`（spec §12.9）は、管理対象が数十件ある repository から
fork したいものを選ばせる。素のプロンプトで実装するなら番号付きリストを出して
`2,5,9` のようなカンマ区切りを受けることになる。

## 決定

**huh を導入し、fork 対象の MultiSelect にだけ使う。** 素のプロンプトと同居させる。

`internal/ui/select.go` が huh を使う唯一の場所である。同じコマンドの中でも、
plan の表示・`Apply this migration? [y/N]` の確認・結果の報告は
`ui.Prompter` と素の `io.Writer` のままとする。

## 却下した案

### 番号のカンマ区切り入力で実装する（ADR 0010 の延長）

候補が数十件ある状態で「今どれを選んでいるか」が画面に残らない。番号を数え間違えても
plan を見るまで気づけず、`Apply this migration? [y/N]` の直前まで誤りが表面化しない。
選択が 1 問 1 答に収まらないという点で、`init` の 3 つの対話とは質が違う。

矢印キーでの移動、Space によるトグル、`/` による絞り込みは、この画面では実際に効く。
ADR 0010 が huh を却下したのは「Select 1 つのために 30 パッケージを入れる理由がない」
という比較であり、MultiSelect が要る場面ではその比較が成り立たない。

### 対話全体を huh に寄せきる

huh は自前で端末を掴むため、`cmd.SetIn` / `cmd.SetOut` 越しに駆動できない。
寄せきると `init` の対話フロー全体を検証している `internal/cli/init_test.go` と、
`apply` の確認プロンプトのテストが同時に失われる。得るものは見た目の統一だけである。

## 帰結

- huh が端末を掴む範囲を「候補を受け取り、選ばれたものを返す」1 関数に閉じた。
  衝突の事前検証・plan の組み立て・確認・repository の変更はすべてその外側にある
- `internal/cli` は `forkSelector`（`func(profile string, candidates []string) ([]string, error)`）
  として選択を受け取る。テストは選択結果を返す関数を渡し、選択画面以外の全経路を検証する
- huh の描画そのものにテストはない。ここは意図的な空白である
- go.mod の依存が間接を含めて約 30 増えた（ADR 0007 の想定どおり）
