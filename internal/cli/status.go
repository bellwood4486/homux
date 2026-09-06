// status.go は homux status（spec §12.2）のコマンド定義である。
package cli

import (
	"github.com/spf13/cobra"

	"github.com/bellwood4486/homux/internal/ui"
)

// newStatusCmd は homux status を構築する。HOME・repo・local config の
// いずれも変更しない（spec §11.2）。
func newStatusCmd(flags *globalFlags) *cobra.Command {
	var all, verbose bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "現在の HOME が desired state と同期しているかを表示する",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cmd, flags, all, verbose)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Linked / Ignored / Inactive も含めて表示する")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "選択された source やリンク先など、1 件あたりの情報量を増やす")

	return cmd
}

// runStatus は scan → resolve → inspect → plan のパイプラインを一度だけ
// 実行し、その結果を ui.RenderStatus に渡す。status と apply が同じ
// resolver / planner を使うのは INV-11 の要請である。
func runStatus(cmd *cobra.Command, flags *globalFlags, all, verbose bool) error {
	ws, _, p, err := buildPlan(cmd, flags)
	if err != nil {
		return err
	}

	ui.RenderStatus(cmd.OutOrStdout(), flags.colorOut, ws.env.Home, ws.profile, p.States, ui.StatusOptions{All: all, Verbose: verbose})

	if p.Errors() > 0 {
		return silentExitError{}
	}
	return nil
}
