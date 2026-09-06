// Package cli は cobra を使ったコマンド定義を持つ。
// cobra を import してよいのはこのパッケージだけである（docs/design.md §2.1）。
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/bellwood4486/homux/internal/ui"
)

// 終了コードの規約（spec §11.3）。
const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

// usageError は「使い方の誤り」（spec §11.3 の終了コード 2）を表す。
type usageError struct {
	err error
}

func newUsageError(err error) error { return &usageError{err: err} }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

// silentExitError は RunE が自前で診断メッセージを出力済みであることを表す
// （spec §10 の整形済み出力に、runRoot の汎用 "Error: ..." を重ねて出さないため）。
// exit code は通常のエラーと同じ扱いになる（ExitError）。
type silentExitError struct{}

func (silentExitError) Error() string { return "" }

type globalFlags struct {
	repo     string
	colorRaw string

	// colorOut / colorErr は PersistentPreRunE で解決される。stdout と
	// stderr は別の fd であり、リダイレクトで一方だけ非 TTY になりうるため
	// （例: "homux status 2> file"）、1 つの bool にまとめない。
	colorOut ui.Palette
	colorErr ui.Palette
}

// NewRootCmd は homux のルートコマンドを構築する。サブコマンドは後から AddCommand で足せる。
func NewRootCmd() *cobra.Command {
	flags := &globalFlags{}

	root := &cobra.Command{
		Use:           "homux",
		Short:         "複数のマシン・プロファイルで dotfiles を管理する",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       buildVersion(),
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			mode, err := ui.ParseColorMode(flags.colorRaw)
			if err != nil {
				return newUsageError(err)
			}
			flags.colorOut = ui.Palette(ui.ResolveColorEnabled(mode, int(os.Stdout.Fd())))
			flags.colorErr = ui.Palette(ui.ResolveColorEnabled(mode, int(os.Stderr.Fd())))
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	root.PersistentFlags().StringVar(&flags.repo, "repo", "", "repository のパス（local config の repo を上書きする）")
	root.PersistentFlags().StringVar(&flags.colorRaw, "color", "auto", "色出力の制御 (auto|always|never)")

	root.SetVersionTemplate("{{.Version}}\n")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return newUsageError(err)
	})

	root.AddCommand(newVersionCmd())
	root.AddCommand(newInitCmd(flags))
	root.AddCommand(newStatusCmd(flags))
	root.AddCommand(newExplainCmd(flags))
	root.AddCommand(newApplyCmd(flags))
	root.AddCommand(newAddCmd(flags))
	root.AddCommand(newProfileCmd(flags))

	return root
}

// Execute はルートコマンドを構築して実行し、spec §11.3 の終了コードを返す。
func Execute() int {
	return runRoot(NewRootCmd())
}

// runRoot は構築済みの cmd を実行する。テストは NewRootCmd() で SetArgs/SetOut した cmd を渡せる。
func runRoot(cmd *cobra.Command) int {
	err := cmd.Execute()
	if err == nil {
		return ExitOK
	}

	var se silentExitError
	if !errors.As(err, &se) {
		fmt.Fprintln(cmd.ErrOrStderr(), "Error:", err)
	}

	var ue *usageError
	if errors.As(err, &ue) {
		return ExitUsage
	}
	return ExitError
}
