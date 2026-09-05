# homux

複数のマシン・プロファイルで dotfiles を管理する Go CLI。
本質は **HOME mirror resolver + symlink manager + linter** であり、テンプレートエンジンではない。

現在の状態: **設計完了・実装未着手**（`main.go` は空のスタブ。実装は `docs/design.md` §9 の Phase 1 から始める）

---

## ドキュメントの読み方

| 文書 | 性質 | 内容 |
|---|---|---|
| `docs/spec.md` | **現在形** | 何を作るか。要求・仕様・CLI 定義・不変条件 |
| `docs/design.md` | **現在形** | どう作るか。アーキテクチャ・パッケージ境界・型・実装順序 |
| `docs/adr/` | **履歴** | なぜその選択をしたか。却下した案とその理由 |

**現在形の 2 文書には「今の正解」だけが書かれている。** 検討過程・却下案・過去の仕様は書かない。書きたくなったら ADR に回すこと。

実装に着手する前に、そのタスクに対応する `docs/spec.md` の章と `docs/design.md` の該当節を読むこと。

---

## 実装時の必須ルール

### 1. 不変条件を破らない

`docs/spec.md` §13 に `INV-01` 〜 `INV-16` として定義されている。
実装・レビューでは **ID で参照する**（例: 「これは INV-07 に反する」）。

特に踏みやすいもの:

- **INV-07**: 複数の selector が一致したら、優先順位を設けずエラーにする。「より具体的な方を優先」は実装しない
- **INV-11**: `status` と `apply` は同じ resolver / planner を使う。解決ロジックを二重に書かない
- **INV-13**: unmanaged な HOME ファイルを置換するときは必ず退避を残す
- **INV-14**: 通常の `apply` で repository 内のファイルを削除しない

### 2. パッケージの import 制約を守る

`docs/design.md` §2.1 に定義され、**`.golangci.yml` の `depguard` が機械的に強制している**。
違反すると `just lint` が落ちる。ここに書かれているのは要点の再掲であり、正本は `.golangci.yml` である。

- `resolve` / `plan` / `selector` は **`os` を import しない**（純粋・決定論的であること）
- `scan` は **`$HOME` に触れない**
- `cobra` を知ってよいのは `internal/cli` だけ
- `huh` と色ライブラリを知ってよいのは `internal/ui` だけ
- ファイルシステムを変更してよいのは `internal/exec` だけ

`resolve` が `os` を import した瞬間に設計が壊れたと分かる、という検知装置としてこの境界がある。
`scan` が `$HOME` に触れないことだけは lint で検知できないので、レビューで見ること。

### 3. 区切り子は `@@` である

`settings.json@@work` であり、`settings.json@work` **ではない**。
単一の `@` は特別な意味を持たない通常の文字である（`tunnel@.service` は common source として扱う）。

理由は `docs/adr/0002-double-at-delimiter.md`。

### 4. 状態データベースを作らない

「この symlink は homux が張ったものか」の判定は、**リンク先が repo 配下に解決されるか**という 1 つの規則だけで行う。
manifest / state file / hidden mapping を導入しない（`docs/adr/0003-no-state-database.md`）。

### 5. 新しいコマンドを足す前に

`mv` / `cp` / `rm` / `git` で自然に表現できないかを先に検討する。
表現できる場合は、抽象を増やすより**診断・可視性・安全性**を強化する（`docs/spec.md` §16）。

---

## 開発

**作業の完了を主張する前に、必ず `just check` を通すこと。**

```bash
mise install      # clone 直後に一度だけ。ツールをすべて入れる
just setup        # + git hooks の導入

just              # = just check（fmt-check → vet → lint → test）
just fix          # check が落ちたらまずこれ（自動修正できるものを直す）
just test-one ./internal/selector   # 反復ループ用。パッケージを絞る
```

- ツールのバージョンは `mise.toml` が唯一の正本である。`go install ...@latest` を使わないこと
- **`git commit --no-verify` を使わないこと。** pre-commit hook が落ちたら、逃げずに直す
- Go 1.27 以降
- module: `github.com/bellwood4486/homux`
- `main.go` はルート直下（`cmd/` は使わない。go.dev の "Basic command" パターン）
- ファイルシステムを interface で抽象化しない。テストは `t.TempDir()` 上の実ファイル・実 symlink で行う
- 依存ライブラリの追加は ADR 0007 で決定済みの事項である。`go get` で独断に追加せず、まず相談すること

### 実装順序

`docs/design.md` §9 にフェーズが定義されている。純粋パッケージ（`selector` → `config` → `scan` → `resolve`）を先に固め、その後は縦切りで常に動くバイナリを保つ。

個々のタスク分解と進捗は superpowers の実装計画が持つ。`docs/` にタスク管理文書を作らないこと（二重管理になる）。
