# homux 仕様

> **この文書は「何を作るか」の Source of Truth である。**
> 常に現在の仕様だけを記述する。検討過程・却下案・過去の仕様は書かない。
> 「なぜその選択をしたか」は `docs/adr/` を参照。
> 「どう作るか」は `docs/design.md` を参照。

---

## 1. 目的

`work` / `personal` など複数のマシン・プロファイルで dotfiles を管理する小さな Go CLI。

本質的には次のツールである。

> **HOME mirror resolver + symlink manager + linter**

テンプレート／レンダリングエンジンではない。

管理体験は以下の組み合わせ。

- **yadm / GNU Stow 的な直接編集**: `$HOME` 上のファイルは dotfiles リポジトリ上の実ファイルへ symlink でつながる。ユーザーは普段どおり `$HOME` 上のパスを編集する。バージョン管理は通常の Git コマンドを使う。
- **chezmoi 的な除外**: `README.md` など、リポジトリには置きたいが `$HOME` へは展開したくないファイルを除外できる。

chezmoi より意図的に単純なものとする。

---

## 2. 主なユースケース

会社用 PC と個人用 PC で 1 つの dotfiles リポジトリを使う。

一部は共通。

```text
~/.zshrc
~/.config/ghostty/config
```

一部は配置先が同じで中身が完全に異なる。

```text
work:      ~/.claude/settings.json -> repo/.claude/settings.json@@work
personal:  ~/.claude/settings.json -> repo/.claude/settings.json@@personal
```

work 版と personal 版は完全に独立したファイルとして管理する。JSON の共通部分抽出・merge・template 化・自動合成はいずれも行わない。

Claude Code の `settings.json` のように、頻繁に変更されアプリ自身から書き換えられる設定ファイルでも扱いやすいことを重視する。

---

## 3. コアコンセプト

### 3.1 リポジトリ構造そのものが設定である

ツール固有のマッピング設定を持たない。ファイル名で表現する。

```text
✅ .claude/settings.json@@work
❌ [[mapping]] target = "~/.claude/settings.json"  source = ".claude/settings-work.json"
```

ファイルシステム上の配置を見れば挙動が理解できること。

### 3.2 HOME 上のファイルは直接編集できる

管理対象は symlink である。`vim ~/.claude/settings.json` で Git 管理下の実ファイルを直接編集できる。アプリが書き換えた場合も即座に `git diff` に現れる。

re-add / render / import のような HOME → source の逆同期操作は存在しない。

### 3.3 Git は Git のまま使う

`git status` / `diff` / `add` / `commit` / `pull` / `push` をそのまま使う。`homux commit` のような Git wrapper は作らない。

### 3.4 rendering / merge レイヤーを持たない

template・変数置換・JSON merge・overlay・profile 継承を実装しない。1 つの target は必ず 1 つの source に解決される。

複数の profile-specific source が同時に一致する場合は、優先順位を決めずエラーにする。

### 3.5 普通のファイル操作が正規の操作である

ユーザーは通常の Unix コマンドでリポジトリ構造を変更してよい。

```bash
mv .claude/settings.json .claude/settings.json@@work   # common を work 専用にする
cp .claude/settings.json .claude/settings.json@@work   # work 版を fork する
rm .claude/settings.json@@work                         # work 版を削除する
```

その後 CLI が状態を検出・説明・反映する。すべての変更に専用コマンドを要求しない。

### 3.6 CLI の責務は orchestration / inspection / safety

`mv` / `cp` / `rm` だけでは扱いづらい操作にだけ CLI を使う。初期セットアップ、既存 HOME ファイルの取り込み、profile の作成・削除・改名、desired state の解決、構造問題の診断、安全な symlink 適用。

---

## 4. リポジトリモデル

dotfiles リポジトリは概念的に `$HOME` の mirror である。

```text
dotfiles/
├── .homux.toml
├── README.md
├── .zshrc
├── .gitconfig
├── .gitconfig@@work
├── .claude/
│   ├── settings.json
│   ├── settings.json@@work
│   └── settings.json@@personal
└── .config/
    └── ghostty/
        └── config
```

- `repo/.zshrc` → target `~/.zshrc`
- `repo/.claude/settings.json@@work` → target `~/.claude/settings.json`

suffix は **どの source を選ぶか** にのみ影響し、target path には含まれない。

### 4.1 symlink はファイル単位である

管理対象は常にファイル単位の symlink であり、ディレクトリを symlink にすることはない。

```text
✅ ~/.config/ghostty/config -> repo/.config/ghostty/config   (ファイルへの symlink)
❌ ~/.config/ghostty        -> repo/.config/ghostty          (ディレクトリへの symlink)
```

`apply` は target の親ディレクトリを必要に応じて実ディレクトリとして作成する（`MkdirAll`）。

→ ADR 0001

### 4.2 ファイル名は HOME 側と一致する

`@@` suffix を除いた repo 上のファイル名は、常に HOME 上のファイル名と一致する。homux がファイル名を変換・エスケープすることはない。

→ INV-16

---

## 5. Profile

### 5.1 profile の定義

利用可能な profile はリポジトリ直下の `.homux.toml` で明示的に定義する。この一覧が authoritative な定義である。

```toml
profiles = [
  "work",
  "personal",
]
```

これにより、新 PC セットアップ時の選択肢表示、suffix の typo 検出、profile 名の妥当性検証、profile 削除時の関連ファイル列挙が可能になる。

### 5.2 active profile

現在の PC で利用する profile はローカル状態であり、dotfiles リポジトリには commit しない。

```text
repo/.homux.toml     -> どの profile が存在するか
repository filename  -> 各 source がどの profile に属するか
local config         -> この PC でどの profile を使うか
```

### 5.3 profile なし

active profile を持たない状態を許可する。これを `default` や `none` という名前の profile として扱わない。

profile なしの場合、suffix なしの common source のみを利用し、profile-specific source はすべて inactive となる。

```text
repo に  foo  と  foo@@work  がある場合
  profile なし  -> selected = foo
  profile work  -> selected = foo@@work
```

### 5.4 profile 名の文法

```regexp
^[a-z0-9][a-z0-9_-]*$
```

| 有効 | 無効 |
|---|---|
| `work` `personal` `work-mac` `home_linux` | `Work` `my work` `foo@bar` `-x` |

---

## 6. Selector 構文

区切り子は **`@@`** である。単一の `@` は特別な意味を持たない通常の文字として扱う。

```text
ファイル名の最後の "@@" 以降を selector とする（"@@" が無ければ common source）
```

これにより `tunnel@.service`（systemd テンプレートユニット）や `.gitconfig@2024` のような `@` を含むファイル名を、エスケープなしでそのまま管理できる。

→ ADR 0002

### 6.1 common source

```text
foo
```

profile-specific source が一致しない限り、すべての profile で利用される。

### 6.2 単一 profile

```text
foo@@work
```

active profile が `work` のときに一致する。

### 6.3 複数 profile（positive selector）

```text
foo@@work+personal
```

意味は `profile == work OR profile == personal`。

### 6.4 negative selector は V1 で実装しない

`foo@@!server` のような否定形は V1 では**構文エラー**として報告する。resolver は将来追加できる形で設計する。

→ ADR 0005

### 6.5 selector の妥当性

selector 全体は次を満たす必要がある。

- `+` 区切りの 1 つ以上の profile 名からなる
- 各 profile 名は §5.4 の文法に合致する
- 各 profile 名は `.homux.toml` の `profiles` に定義済みである
- 同じ profile 名を重複して含まない

いずれかに反する場合は診断エラー（§10）となる。

---

## 7. Source 解決ルール

target `foo`、active profile `P` に対して以下を適用する。

```text
1. repository path が ignore 対象なら           -> ignored
2. target に対応する source 候補を列挙する
3. 各 source の selector を検証する
4. P に一致する profile-specific source の数を数える
     == 1                          -> それを選択
     >= 2                          -> ERROR: ambiguous
     == 0 かつ common source あり  -> common を選択
     == 0 かつ common source なし  -> absent
```

specificity による優先順位は設けない。

```text
foo@@work
foo@@work+personal
```

active profile が `work` のとき、両方が一致するため **ambiguous エラー**となる。「`@@work` の方がより具体的だから優先」という暗黙ルールは存在しない。

→ INV-07

---

## 8. `.homux.toml`

homux 固有の repository-level metadata を置く唯一の設定ファイル。V1 では `profiles` と `ignore` のみを持つ。

```toml
profiles = [
  "work",
  "personal",
]

ignore = [
  "README.md",
  "LICENSE",
  "docs/**",
]
```

per-file mapping・template・profile 継承・hostname 条件などをこのファイルに追加しない。

### 8.1 ignore の意味

ignore の意味は 1 つだけである。

> リポジトリには存在するが、dotfiles 配置対象にはしない

パターンは **repo ルートからの相対パスに対する glob** であり、`**` を含められる（doublestar 文法）。gitignore のネガーション（`!`）はサポートしない。

ignore を profile-specific にしない。「work only」「personal でだけ使わない」は ignore ではなく selector の責務である。

```text
foo@@personal          personal のみ
foo@@work+personal     work と personal の両方
```

### 8.2 常に暗黙除外されるもの

設定に関わらず、以下は常に deployment 対象外である。

```text
.homux.toml
.git/**
```

---

## 9. 状態モデル

`git status` が答えるのは「リポジトリ内で何が変更されたか」。homux の `status` が答えるのは次である。

> 現在の `$HOME` は、リポジトリ + profile から導かれる期待状態と同期しているか？

| 状態 | 意味 |
|---|---|
| **Linked** | 期待どおりの symlink が存在する |
| **Missing** | desired source は存在するが HOME に target がない |
| **Occupied** | desired source はあるが、HOME target が管理外のファイル・ディレクトリ・symlink で占有されている |
| **Stale** | HOME 上に管理済み symlink が残っているが、現在の desired state と一致しない |
| **Ignored** | repository path が ignore ルールで対象外 |
| **Inactive** | profile-specific source だが、現在の active profile では選択されていない |
| **Error** | unknown profile / ambiguous / selector 構文エラー / `.homux.toml` 不正 |

### 9.1 managed symlink の定義

homux は状態データベースを持たない。管理下かどうかは次の 1 つの規則だけで判定する。

> **`$HOME` 上の symlink であり、そのリンク先が repo ルート配下に解決されるものを managed とみなす。**

リンク先が存在しない dangling symlink であっても、リンク先パスが repo 配下であれば managed である。

→ ADR 0003 / INV-04

### 9.2 Stale の 2 種類と検出範囲

| 種類 | 例 | 検出方法 |
|---|---|---|
| **種類1** | profile 切替や source rename により、desired target のリンク先が変わった | desired targets の走査で検出できる |
| **種類2** | source が削除・ignore 化され、desired から消えたのに symlink が残った | HOME 側の走査が必要 |

種類2 のための HOME 走査は、**repo のトップレベルエントリに対応する HOME パスを起点とし、そこから再帰する**（symlink 自体は評価するが、その先には降りない）。

```text
repo に .zshrc, .claude/, .config/ がある場合
  -> 走査対象は ~/.claude/ と ~/.config/ の配下のみ
```

**既知の制限**: 「repo からディレクトリごと source を削除した」場合、その残骸を検出できない。残骸は dangling symlink となるため実害は小さいものとして受容する。

`.homux.toml` の `ignore` は repo path に対する規則であり、HOME 走査には適用しない。

→ ADR 0004

---

## 10. 診断

ユーザーがファイル名を直接編集する運用を正式に認めるため、診断は本ツールのコア機能である。

### 10.1 unknown profile

```text
ERROR .claude/settings.json@@wrok

  Unknown profile "wrok".
  Did you mean "work"?
```

suggestion は編集距離（Levenshtein）に基づく単純なもので十分とする。

### 10.2 invalid selector syntax

```text
ERROR foo@@work++personal

  Invalid selector syntax.
```

### 10.3 ambiguous resolution

```text
ERROR ambiguous profile match

  Target:
    ~/foo

  Matching sources:
    foo@@work
    foo@@work+personal
```

---

## 11. CLI 仕様

```text
homux init [--repo <path>] [--profile <name>]

homux add <path>... [--profile <name>]

homux status [--all] [--verbose]
homux explain <path>

homux apply [--dry-run|-n] [--yes]

homux profile list
homux profile create <name>
homux profile use <name>
homux profile rename <old> <new>
homux profile delete <name>

homux version
```

### 11.1 グローバルフラグ

| フラグ | 意味 |
|---|---|
| `--repo <path>` | local config の `repo` を上書きする |
| `--color auto\|always\|never` | 色出力の制御。既定は `auto`（非 TTY では無効） |
| `--version` | バージョンを表示して終了 |

環境変数 `NO_COLOR` が設定されている場合は色を出力しない。

### 11.2 コマンド別仕様

| コマンド | HOME を変更 | repo を変更 | local config を変更 | 対話 |
|---|---|---|---|---|
| `init` | ✅ | ✅ (`.homux.toml` 新規作成時) | ✅ | ✅ |
| `add` | ✅ | ✅ | ❌ | ✅ (plan 確認) |
| `status` | ❌ | ❌ | ❌ | ❌ |
| `explain` | ❌ | ❌ | ❌ | ❌ |
| `apply` | ✅ | ❌ | ❌ | ✅ (`--yes` で抑止) |
| `profile list` | ❌ | ❌ | ❌ | ❌ |
| `profile create` | ❌ | ✅ | ❌ | ✅ |
| `profile use` | ❌ | ❌ | ✅ | ❌ |
| `profile rename` | ❌ | ✅ | ✅ | ✅ |
| `profile delete` | ❌ | ✅ | ✅ | ✅ |

**`init` を除き、HOME を変更するのは `add` と `apply` だけである。** profile 系コマンドは repository と local config のみを変更し、その後 `homux apply` が必要であることを表示する。

→ INV-11

### 11.3 終了コード

| コード | 意味 |
|---|---|
| `0` | 正常終了 |
| `1` | 実行エラー、または repository に構造エラー（unknown profile / ambiguous / selector 構文エラー）が存在する |
| `2` | 使い方の誤り（不正な引数・フラグ） |

**`status` は drift（Missing / Occupied / Stale）が存在しても `0` を返す。** `git status` と同様、drift は異常ではないため。構造エラーがある場合のみ `1` を返す。

### 11.4 非 TTY での動作

標準入力・標準出力が TTY でない場合、対話 UI は起動せず**エラー終了する**。

- `apply` は `--yes` を指定すればすべての確認を肯定として非対話実行できる
- `init` は `--repo` と `--profile` をすべて指定すれば非対話実行できる
- `profile create` / `rename` / `delete` は非 TTY では実行できない

エラーになるのは**対話 UI を起動する必要が生じたとき**である。`apply` の plan に確認を要する操作（Occupied / Stale）が 1 件も無ければ、`--yes` なしの非 TTY でもそのまま実行する。そうしないと、収束済みの HOME に対する再実行がパイプ越しに永久に失敗する。

---

## 12. コマンド詳細

### 12.1 `init`

新しい PC でリポジトリを clone した後、最初に実行する。

1. **repo path を対話的に入力させる**（カレントディレクトリを既定候補として提示）
2. バリデーション: ディレクトリが存在するか、`.homux.toml` があるか
   - `.homux.toml` が**ない**場合、「新規リポジトリとして初期化しますか？」と確認し、雛形を書き出す。雛形の `ignore` には `README.md` / `LICENSE` / `docs/**` をコメントアウトした状態で含める
3. 入力パスを絶対パスに展開し、`filepath.EvalSymlinks` で実体を解決して local config に保存する
4. `.homux.toml` の `profiles` から選択肢を表示し、active profile を選ばせる（`none` を含む）
5. plan を表示し、確認のうえ `apply` を実行する

```text
Available profiles:

  1. work
  2. personal
  3. (none)

Select profile:
```

`init` は例外的に HOME への適用まで行うが、その処理は `apply` と同一の Planner / Executor を通る。

### 12.2 `status`

actionable / problematic な状態のみを表示する。

```text
Profile: work

Missing    ~/.config/foo/config
Occupied   ~/.claude/settings.json
Stale      ~/.config/old/config

3 changes pending
```

| フラグ | 効果 |
|---|---|
| `--all` | **表示対象の範囲を広げる**。Linked / Ignored / Inactive も含める |
| `--verbose` | **1 件あたりの情報量を増やす**。選択された source パス、リンク先の実体などを表示 |

両者は併用可能で、直交する。

### 12.3 `explain`

1 ファイルについて、なぜその状態になったのかを説明する。

引数は **HOME 側の target パスと repo 側の source パスの両方を受け付ける**。引数を絶対パス化し、repo ルート配下なら source として、`$HOME` 配下なら target として解釈する（repo が `$HOME` 配下にある場合は repo 判定を優先）。どちらでもない場合はエラー。

```text
Target:
  ~/.claude/settings.json

Active profile:
  work

Candidates:
  .claude/settings.json
  .claude/settings.json@@work
  .claude/settings.json@@personal

Selected:
  .claude/settings.json@@work

Reason:
  exact profile-specific source matches "work"

Current:
  ~/.claude/settings.json
  -> ~/dotfiles/.claude/settings.json

State:
  stale link

Would apply:
  relink to ~/dotfiles/.claude/settings.json@@work
```

selector と解釈されなかった `@` を含むファイル名についても、その判定結果を明示する。

### 12.4 `apply`

HOME を desired state に合わせる。**ファイルの生成・レンダリングは行わない。**

行うのは symlink の作成・差し替え・削除と、target conflict の安全な処理のみ。

| 状態 | 動作 |
|---|---|
| Missing | 確認なしで symlink を作成する |
| Occupied | 確認のうえ、既存の target を退避してから symlink を作成する |
| Stale (種類1) | 確認のうえ relink する |
| Stale (種類2) | 確認のうえ symlink を削除する |
| Error | **その target をスキップし、残りの適用を続行する**。最後にスキップ件数を表示し、終了コード `1` を返す |

**Occupied の退避**: 既存の target は同一ディレクトリに `<name>.homux-bak.<timestamp>` へ rename して退避してから symlink を張る。プロンプトに退避先を明示する。

退避の対象は 3 種類あり、いずれも同じ規則で退避する。

| Current | 退避されるもの | プロンプトの見出し |
|---|---|---|
| 通常ファイル | ファイルそのもの | `Existing file detected:` |
| ディレクトリ | ディレクトリごと（中身を含む） | `Existing directory detected:` |
| repo 外を指す symlink | symlink 本体のみ。リンク先の実体は動かさない | `Existing symlink detected:` |

rename であるため、ディレクトリの中身がどれだけ多くても退避の費用は変わらず、内容もパーミッションもそのまま残る。

```text
Existing file detected:

  target:
    ~/.claude/settings.json

  desired:
    ~/dotfiles/.claude/settings.json@@work

  backup:
    ~/.claude/settings.json.homux-bak.20260905-153000

Replace it? [y/N]:
```

target が symlink の場合は、`target` の次に現在のリンク先を `current` として出す。何が退避されるのかは `y` を打つ判断そのものであるため、見出しと項目は上の表に従って target の種類ごとに変える。

**退避先の衝突**: timestamp は秒単位である。退避先が既に存在する場合は、**その target を変更せずエラーで停止する**（後述の「実行時の失敗」）。空いている名前を探して退避先をずらすことはしない。退避先は plan が決めるものであり、dry-run が示した退避先と実際の退避先は常に一致する（ADR 0012）。

**Stale (種類1) の relink**: プロンプトはリンク先が「どこから どこへ」変わるのかを示す。relink は「リンク先が変わった」ことそのものが操作の理由であるため、新しいリンク先だけでは確認の材料にならない。

`n` を選んだ場合、target は変更されず、その選択は保存されない。conflict が残っている限り次回の `apply` でも再度確認する（INV-12）。

**削除するのは symlink のみである。** 通常の `apply` が repository 内の source file を削除することはない（INV-14）。

**「スキップして続行」と「停止」の関係**: apply が正常に完了しない理由には 2 つの層があり、扱いが異なる。

| 層 | いつ判明するか | 動作 |
|---|---|---|
| **Error 状態**（unknown profile / ambiguous / selector 構文エラー / target が repository 配下） | 適用を始める前。そもそも操作を組み立てられない | その target には何もせず、残りの適用を続行する。最後にスキップ件数を表示する |
| **実行時の失敗**（退避先の衝突、rename の失敗、削除対象が symlink でなかった等） | 実際に HOME を触った瞬間 | その時点で停止する |

どちらも終了コードは `1` である（§11.3）。前者は最初から適用の対象になりえない target を除いているだけなのに対し、後者は既に HOME を変更し始めた後であり、続行すると「何が適用され、何が適用されなかったか」が読めなくなる。退避先の衝突は後者である。

**部分適用**: 実行時の失敗が起きた場合はその時点で停止し、「ここまで適用済み / ここから未適用」を報告する。ロールバックは行わない。`apply` は冪等であり、再実行すれば収束する。

**空ディレクトリ**: stale symlink を削除した結果、親ディレクトリが空になっても削除しない。

### 12.5 `apply --dry-run`

`status` が「現在どんな状態か」を答えるのに対し、dry-run は「`apply` を実行すると何をするか」を答える。

```text
Would create symlink:
  ~/.config/foo/config
  -> ~/dotfiles/.config/foo/config@@work

Would ask before replacing:
  ~/.claude/settings.json (directory)
  -> ~/dotfiles/.claude/settings.json@@work

Would relink:
  ~/.vimrc
  ~/dotfiles/.vimrc -> ~/dotfiles/.vimrc@@work

Would remove stale symlink:
  ~/.config/old/config

No changes made.
```

同じ種類の操作は 1 つの見出しにまとめる。見出しの順序は上記で固定する。

`Would ask before replacing:` では、target がディレクトリまたは repo 外を指す symlink のときだけ `(directory)` / `(symlink)` を添える。通常ファイルには何も添えない。注記は「驚きのある退避」を先に見せるためにあり、既定の期待である通常ファイルに付けても行が伸びるだけだからである。

`Would relink:` はリンク先が「どこから どこへ」変わるのかを 1 行で示す。

`apply` も実行前にこの同じブロックを表示する。最初の確認を出す前に全体像を示すためであり、`--yes` の非対話実行でも「何をしたか」がログに残る。

repository に構造エラーがある場合は、`status` と同じ診断ブロックを出して終了コード `1` を返す（§11.3 はコマンドに依らない）。何も実行しないことは終了コードを `0` にする理由にならない。

**dry-run が示すのは plan が見える範囲である。** 実際に HOME を触ってはじめて分かる失敗（退避先の衝突など）は予告しない。dry-run が何も問題を示さなくても、`apply` が実行時の失敗で途中停止することはありうる（§12.4）。

独立した `homux dry-run` コマンドは作らない。

### 12.6 `add`

既存の unmanaged な HOME ファイルをリポジトリに取り込む。

```bash
homux add ~/.config/foo/config
homux add ~/.claude/settings.json --profile work
```

動作は「repo へ move → 元の位置に symlink を作成」である。

```text
move:    ~/.config/foo/config  ->  repo/.config/foo/config
create:  ~/.config/foo/config  ->  repo/.config/foo/config
```

`--profile work` を指定した場合、repository 上の source は `repo/.claude/settings.json@@work` となる。

| ケース | 動作 |
|---|---|
| ディレクトリを指定 | 配下の全ファイルを**再帰的に** add する。実行前に plan を表示して確認する（symlink はあくまでファイル単位に張る） |
| 既に managed symlink になっているパス | エラー |
| `--profile` 指定時に common source が既に存在する | 許可する（fork として `foo@@work` を作成する）。plan に明示する |
| `$HOME` 外のパス | エラー |
| repo 側に同名 source が既に存在する | エラー（黙って上書きしない） |
| 対象が他所を指す symlink | エラー |

### 12.7 `profile list`

```text
Profiles:

  personal
* work

Active profile: work
```

profile なしの状態も明示する。

### 12.8 `profile use`

この PC の active profile を切り替える。

- 対象は `.homux.toml` に定義済みでなければならない。unknown profile はエラー
- `use` から暗黙に profile を作成しない（typo を profile 定義として正当化しない）
- HOME は変更しない。切替後に desired state と差異がある場合、`homux apply` が必要である旨を表示する

### 12.9 `profile create`

repository-level に新しい profile を追加する。

**既定では、既存の common source はそのまま common のまま**である。profile を追加しても、すべてのファイルを profile-specific に fork する必要はない。

新 profile 用に fork したい common file だけを対話的に選択できる。

```text
Creating profile: work

All 47 managed targets will continue using common sources by default.

Select targets that should receive a work-specific copy:
  [ ] .zshrc
  [x] .gitconfig
  [x] .claude/settings.json
  [ ] .config/ghostty/config
```

実ファイルを変更する前に plan を表示する。

```text
Profile migration plan: work

Keep common:
  ~/.zshrc
  ~/.config/ghostty/config

Fork:
  .gitconfig             -> .gitconfig@@work
  .claude/settings.json  -> .claude/settings.json@@work

Apply this migration? [y/N]
```

確認後のみリポジトリを変更する。HOME は変更しない。この wizard は大量変更のための convenience であり、ユーザーが `cp` / `mv` で手動変更してもよい。

### 12.10 `profile rename`

repository 全体に存在する profile 参照を一括で更新する。

```text
Rename profile "work" -> "company"

Profile definition:
  work -> company

Files:
  .gitconfig@@work             -> .gitconfig@@company
  .claude/settings.json@@work  -> .claude/settings.json@@company

Selectors:
  foo@@work+personal           -> foo@@company+personal

Local active profile:
  work -> company

Apply? [y/N]
```

更新対象は `.homux.toml` の定義、単一 profile suffix、multi-profile selector、および（対象が active な場合）local active profile である。

**衝突時**: rename 後のファイル名が既に存在する場合は上書きせずエラーとする。

```text
ERROR rename collision

  .gitconfig@@work
  -> .gitconfig@@company

Target already exists.
```

**`profile rename` は部分適用を残してはならない。** 全変更を事前に検証し、適用可能であることを確認してから一括実行する（INV-15）。これは `apply` の部分適用ポリシー（§12.4）の唯一の例外である。

1 ファイル単位の suffix 変更は通常の `mv` で行えばよい。profile 全体の rename のみが CLI の責務である。

### 12.11 `profile delete`

destructive operation として対話を必須とする。

repository 全体を走査し、その profile を参照するファイルを列挙する。

```text
Profile "work" is referenced by:

  .gitconfig@@work
  .claude/settings.json@@work
  .config/foo/config@@work+personal
```

| 参照の形 | 動作 |
|---|---|
| `foo@@work`（単一 profile） | 確認のうえファイルを削除する |
| `foo@@work+personal`（複数 profile） | **ファイルを削除してはならない。** `foo@@personal` へ rewrite する |

rewrite も削除 plan に表示する。現在の PC が削除対象 profile を利用中の場合、確認のうえ local active profile を「なし」にする。その結果 HOME がどう変わるかも plan に含めるが、**HOME 自体は変更しない**（`homux apply` が必要である旨を表示する）。

---

## 13. 不変条件

実装は以下を守らなければならない。レビュー時はこの ID で参照する。

| ID | 内容 |
|---|---|
| **INV-01** | 管理対象ファイルの内容について repository が Source of Truth である |
| **INV-02** | managed な HOME ファイルは symlink 経由で直接編集できる |
| **INV-03** | バージョン管理は通常の Git を使う。Git wrapper を作らない |
| **INV-04** | profile の挙動はファイル名に表現し、hidden mapping state を持たない |
| **INV-05** | ignore は repository path を deployment 対象にするかだけを決める |
| **INV-06** | 1 つの target は高々 1 つの profile-specific source に解決される |
| **INV-07** | 複数の selector が一致した場合、優先順位を設けずエラーとする |
| **INV-08** | 一致する profile-specific source がなければ common source を fallback として使う |
| **INV-09** | profile なしでは common source のみを使う |
| **INV-10** | CLI が repository を変更しても、その結果は CLI なしで理解可能でなければならない |
| **INV-11** | `status` と `apply` は同一の resolver / planner を使う。別々の解決ロジックを実装してはならない |
| **INV-12** | 対話で `n` を選んでも永続化せず、「今回はスキップ」の意味に留める |
| **INV-13** | unmanaged な HOME ファイルを黙って上書きしない。置換時は必ず退避を残す |
| **INV-14** | 通常の `apply` で repository 内の source file を削除しない |
| **INV-15** | `profile rename` は repository 全体の参照を整合的に更新し、衝突時に部分適用を残さない |
| **INV-16** | `@@` suffix を除いた repo 上のファイル名は、常に HOME 上のファイル名と一致する |

---

## 14. V1 の Non-goals

要件が変わらない限り実装しない。

- template / 変数置換
- secrets・password manager 連携
- アプリ固有の config merge、JSON/YAML/TOML の構造 merge
- profile 継承
- hostname ベースの自動 profile 判定、OS 条件式
- Git wrapper、自動 commit / push
- overlapping selector の優先順位
- `apply` の対話回答の永続化
- hidden per-file mapping database
- negative selector（`@@!server`）
- ファイルパーミッションの管理
- 機械可読出力（`--json`）
- `fork` / `scope` のような convenience コマンド

ツール全体は次の 3 つを見れば理解できる状態を維持する。

```text
repository structure + .homux.toml + local active profile
```

---

## 15. 既知の制限

意図的に受け入れている制限。バグではない。

| 制限 | 理由 |
|---|---|
| repo からディレクトリごと source を削除した場合、HOME に残る symlink を検出できない | HOME 全走査のコストを避けるため（ADR 0004） |
| ファイルパーミッションを管理しない。`~/.ssh/config` などは clone 後に 644 となり利用できない | per-file mapping を持たない思想との衝突（ADR 0006） |
| `@@` を含む正当なファイル名は管理できない | 実在しないと判断（ADR 0002） |
| `profiles` 配列**内部**に書かれたコメントは `profile` コマンドで失われる | TOML の AST 編集を避けるため（ADR 0008） |

---

## 16. 直接ファイル操作と CLI の線引き

この区別は意図的なものである。

**直接操作を優先する（ピンポイントで明白な repository 変更）**

```bash
mv foo foo@@work    # common を work 専用にする
cp foo foo@@work    # work 用に fork する
rm foo@@work        # work 版を削除する
mv foo bar/foo      # target path を変更する
```

変更後に `homux status` / `homux apply` を使う。

**CLI を優先する（coordination / safety が必要な操作）**

```text
homux init / add / status / explain / apply
homux profile list / create / use / rename / delete
```

新しいコマンドを追加する前に、まず `mv` / `cp` / `rm` / `git` で自然に表現できないかを検討する。表現できる場合は、新しい抽象を増やすより**診断・可視性・安全性**を強化する。

---

## 17. End-to-End 例

### 17.1 最初は個人利用のみ（profile なし）

```text
dotfiles/
├── .homux.toml
├── .zshrc
├── .gitconfig
└── .claude/settings.json
```

```text
~/.zshrc                 -> repo/.zshrc
~/.gitconfig             -> repo/.gitconfig
~/.claude/settings.json  -> repo/.claude/settings.json
```

### 17.2 後から work profile を追加する

```bash
homux profile create work
```

選択:

```text
.zshrc                  common のまま
.gitconfig              fork
.claude/settings.json   fork
```

リポジトリ:

```text
dotfiles/
├── .zshrc
├── .gitconfig
├── .gitconfig@@work
└── .claude/
    ├── settings.json
    └── settings.json@@work
```

`profile create` は HOME を変更しないため、続けて `homux apply` が必要になる。

work machine:

```text
~/.zshrc                 -> .zshrc
~/.gitconfig             -> .gitconfig@@work
~/.claude/settings.json  -> .claude/settings.json@@work
```

profile なしの machine:

```text
~/.zshrc                 -> .zshrc
~/.gitconfig             -> .gitconfig
~/.claude/settings.json  -> .claude/settings.json
```

### 17.3 後から 1 ファイルだけ手動で fork する

```bash
cp .config/ghostty/config .config/ghostty/config@@work
homux status   # work profile における selected source の変更と relink の必要を検出
homux apply    # symlink を更新
```

metadata の更新は不要である。これが spec §3.1（リポジトリ構造そのものが設定である）の帰結である。
