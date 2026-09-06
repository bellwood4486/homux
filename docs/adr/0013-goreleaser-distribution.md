# 0013. 配布は GoReleaser で行い、GitHub Releases にバイナリと provenance を出す

## 状況

homux は `go install` でしか手に入らなかった。基本の利用者は作者自身だが、Go CLI の配布まわりの標準的な作法を一通り通すことも目的に含めて `v0.1.0` を出す。

この経路には制約が 2 つある。

- **タグは PR CI を通っていないコミットにも打てる。** `main` への push で `just check` が走っているだけでは、リリース対象のコミットが検証済みであることを保証できない
- **`debug.ReadBuildInfo()` の `Main.Version` にタグが入るのは module proxy 経由で取得したときだけである。** ローカル checkout からビルドしたバイナリでは `(devel)` になる。`go install` しか経路がなかった間はこれで足りていた（`docs/design.md` §5.1 は「`-ldflags` によるバージョン埋め込みは、リリース用ビルドを導入する時点で追加する」と予告していた）

## 決定

**GoReleaser を採用する。** 設定は `.goreleaser.yaml`、バージョンは `mise.toml` で固定する。`git push origin vX.Y.Z` を起点に `.github/workflows/release.yml` が走り、GitHub Releases にビルド済みバイナリと `checksums.txt` を出す。`go install` 経路も維持する。

- **対象は `darwin/{amd64,arm64}` と `linux/{amd64,arm64}`。** Windows は対象外とする。`os.Symlink` が管理者権限または開発者モードを要求し、homux の中心的な操作がそのまま動かないため
- **release job は GoReleaser の前に `just check` を再走させる。** 上記の「タグは未検証のコミットにも打てる」への対処である
- **SLSA build provenance を `actions/attest-build-provenance` で付ける。** 利用者は `gh attestation verify` で検証できる
- **バージョンは `-ldflags` で埋め込み、`debug.ReadBuildInfo()` はフォールバックとする**（`internal/cli/version.go`）。`go install` で入れたバイナリの挙動は変わらない
- **`CHANGELOG.md` は作らない。** Conventional Commits が既に効いているので、リリースノートは GoReleaser がコミットから組み立てる

タグは `v0.1.0`（SemVer 3 要素）。v0 系のままとし、CLI の挙動と `.homux.toml` 書式がまだ動きうることを示す。

## 却下した案

### 素の GitHub Actions workflow で自前クロスビルドする

`go build` を matrix で 4 回回し、`tar` で固めて `gh release upload` する案。依存を 1 つ増やさずに済む。

しかし実際に必要なものを数えると、matrix、アーカイブの命名規則、`checksums.txt` の生成、リリースノートの組み立て、`dist/` の後始末、そして「手元で下見する手段」が要る。これは GoReleaser が既に持っているものの再実装であり、**その再実装自身が保守対象になる**。とくに `just release-dry` に相当するもの（CI を汚さずにタグ push 前に成果物を確認する手段）を自前で用意するのは、workflow をローカルで走らせることになって割に合わない。

homux の方針は「抽象を増やす前に `mv` / `cp` / `rm` / `git` で表現できないかを先に検討する」（spec §16）だが、これは **homux 自身の CLI 表面**に対する規律であって、開発ツールチェーンには適用されない。ツールチェーンはすでに mise / just / golangci-lint / lefthook で構成されており、GoReleaser はその列に並ぶ。

### Homebrew tap を v0.1 に含める

作者自身の利用体験としては `brew install` が最も自然で、GoReleaser は tap の生成を設定数行で持っている。

しかし tap は**別 repository の運用**を伴う。formula を push するためのトークン管理、tap 側の壊れ方（formula は出たがバイナリの URL が 404 など）を追う責任、そして tap を消せなくなること（一度入れた人の `brew update` が壊れる）が付いてくる。

決定的なのは、**これは後付けできる**という点である。同じ `.goreleaser.yaml` に `brews:` を足すだけで、今の決定を覆さずに乗る。利用者が作者しかいない段階で先に運用コストだけを負う理由がない。

### cosign による keyless 署名と syft による SBOM を同時に入れる

供給網まわりを一度に揃える案。GoReleaser はどちらも設定項目として持っている。

しかし `actions/attest-build-provenance` は「どのソースからどの workflow がビルドしたか」をトークンの交換だけで証明し、利用者側の検証も `gh attestation verify` の 1 コマンドで済む。**鍵運用の知識を要求せずに、検証したい人が実際に検証できる状態**という目的は、これだけで満たされる。

cosign は署名の検証手順を README に書く分だけ利用者の負担が増え、SBOM は依存が 30 個ほどの CLI に対して読む人がいない。鍵運用の学習は配布とは別のテーマとして切り離し、必要になった時点で足す。

### shell 補完・man ページ・deb/rpm パッケージ

いずれも GoReleaser が生成できるが、生成物にはそれぞれ「インストール手順を README に書く」「壊れていないか確認する」責任が付く。利用者が作者しかいない段階では、tar.gz を展開して PATH に置く 1 経路だけを正しく保つ方が価値が高い。

## 帰結

- リリース手順は `git tag v0.1.0 && git push origin v0.1.0` の 2 コマンドになる。専用の just レシピは作らない（生の git コマンドで足りる）
- タグを push した後に `just check` が落ちれば、Releases は作られないままタグだけが残る。タグを消して直してから打ち直すことになる
- `go install` で入れたバイナリは ldflags を持たないため、従来どおり `(devel)` とリビジョンを表示する。Releases から取ったバイナリだけが `v0.1.0` を返す
- `go.mod` の `go` directive は `1.27` に緩めた。パッチ固定は `mise.toml` の責務であり、go directive は**利用者に要求する最低バージョン**として役割を分ける
