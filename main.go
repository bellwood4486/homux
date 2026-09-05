// Command homux は複数のマシン・プロファイルで dotfiles を管理する。
package main

import (
	"os"

	"github.com/bellwood4486/homux/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
