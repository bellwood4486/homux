# 0010. `init` の対話を素のプロンプトで実装し、huh の導入を遅らせる

## 状況

ADR 0007 で対話 UI に `charmbracelet/huh` を採用した。`init`（spec §12.1）は homux で
最初に対話 UI を必要とするコマンドであり、ここが huh の初導入点になる。

一方 `apply` の確認は、Select の表現力を必要としないという理由で huh ではなく
`internal/ui` の素の `[y/N]` プロンプト（`ui.Prompter`）で実装済みである。

`init` が必要とする対話は 3 つで、いずれも 1 問 1 答である。

- repository パスの 1 行入力
- 「新規リポジトリとして初期化しますか？」の `[y/N]`
- profile の単一選択

## 決定

`init` の 3 つの対話をすべて `ui.Prompter` の素のプロンプトで実装する。profile 選択は
spec §12.1 が示す番号付きリストと `Select profile:` をそのまま出力し、番号で受け取る。

huh は go.mod に入れない。導入するのは `profile create` の migration wizard で MultiSelect が
必要になった時点とする。ADR 0007 の「対話 UI に huh を採用する」という決定自体は覆さない。

## 却下した案

### `init` から huh を使う（ADR 0007 の素直な実装）

- spec §12.1 のプロンプト例は番号付きリストであり、矢印キー Select 特有の表現を要求していない。
  huh の Select はこの出力を再現しない
- huh は間接依存を含め約 30 パッケージを持ち込む（ADR 0007）。V1 で唯一 MultiSelect を要する
  `profile create` より先に、Select 1 つのために入れる理由がない
- huh は自前で端末を掴むため、`internal/cli` の統合テストから `cmd.SetIn` / `cmd.SetOut` 越しに
  駆動できない。`init` の対話フロー全体を `t.TempDir()` 上で検証する手段が失われる。これは
  `apply` の確認を素のプロンプトにしたときと同じ判断である（docs/design.md §5）

### 対話 UI 全体を素のプロンプトで実装し、huh を採用しないと決める

`profile create` の migration wizard は既存ファイルの複数選択を伴い、MultiSelect の表現力が
実際に効く。そこを見ないまま huh を却下するのは早い。判断は当該 issue に残す。

## 帰結

- `internal/ui/wizard.go` が `AskLine` / `Confirm` / `SelectProfile` を提供する
- `init` の対話フローは `internal/cli/init_test.go` から文字列の流し込みで丸ごと検証できる
- huh を導入する際は、`ui.Prompter` の素のプロンプトと huh のウィザードが同居することになる。
  同居を許すか、素のプロンプトへ寄せきるかは、そのときに決める
