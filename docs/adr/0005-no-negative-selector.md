# 0005. negative selector を V1 に含めない

## 状況

「`server` 以外のすべての profile で使う」を表現する negative selector（`foo@@!server`）が構想されていた。positive multi-profile selector（`foo@@work+personal`）と対になる機能である。

## 決定

**V1 では `@@!server` を構文エラーとして報告する。** selector のパーサは将来 negative を追加できる形（`Selector` を式として抽象化する）にしておく。

positive multi-profile（`@@work+personal`）は V1 に含める。

## 却下した案

### negative selector を V1 から実装する

negative selector の意味は「`.homux.toml` の `profiles` に定義された profile のうち `server` 以外」である。つまり**そのファイルの意味が `.homux.toml` の内容に依存する**。

新しい profile を 1 つ定義しただけで、既存ファイルの適用先が黙って増える。これは spec §3.1（ファイルシステム上の配置を見れば挙動が理解できる）と INV-10 に反する。実装コストではなく、思想上の理由で外している。

### `+` も含めて単一 profile のみとする

`profile rename` / `profile delete` の仕様が既に multi-profile selector の存在を前提としている（`foo@@work+personal` から `work` を削除したら `foo@@personal` に rewrite する、など）。また `+` は resolver 上ほぼコストゼロ（マッチ集合に含まれるかの判定）である。

## 帰結

- 将来 negative を追加する場合、`.homux.toml` への依存が持ち込まれることを改めて評価する必要がある
- `foo@@!server` は「未実装」ではなく「構文エラー」として報告する。将来の予約であることを併せて伝える
