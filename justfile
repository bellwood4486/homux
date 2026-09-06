# homux の開発タスク。
#
# エージェント向けの契約:
#   作業完了を主張する前に、必ず `just check` を通すこと。
#   `just` を引数なしで叩いても同じことが起きる。

set shell := ["bash", "-euo", "pipefail", "-c"]

# 引数なしで実行したときの既定レシピ
default: check

# 完了前に必ず通すべき検証。CI と lefthook もこれと同じものを叩く
check: fmt-check vet lint test

# 自動修正できるものをすべて直す。check が落ちたらまずこれ
fix:
    golangci-lint fmt
    go mod tidy

# フォーマット差分がないことの検査（修正はしない）
fmt-check:
    golangci-lint fmt --diff

vet:
    go vet ./...

lint:
    golangci-lint run

test:
    go test ./...

# 反復ループ用。パッケージを絞って速く回す
#   例: just test-one ./internal/selector
test-one pkg:
    go test -count=1 {{pkg}}

# 競合検出つき。symlink 操作の並行性を疑うときに使う
test-race:
    go test -race ./...

cover:
    go test -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out | tail -1

build:
    go build -o homux .

# clone 直後のセットアップ
setup:
    mise install
    lefthook install
    go mod download

# リリースの下見。dist/ に成果物を作るだけで、tag も push も作らない
release-dry:
    goreleaser release --snapshot --clean
