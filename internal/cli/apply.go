// apply.go は homux apply（spec §12.4）と homux apply --dry-run（spec §12.5）の
// コマンド定義である。
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/bellwood4486/homux/internal/exec"
	"github.com/bellwood4486/homux/internal/plan"
	"github.com/bellwood4486/homux/internal/ui"
)

// applyOptions は homux apply のフラグである（spec §11）。
type applyOptions struct {
	dryRun bool
	yes    bool
}

// newApplyCmd は homux apply を構築する。HOME だけを変更し、repository と
// local config には触れない（spec §11.2）。
func newApplyCmd(flags *globalFlags) *cobra.Command {
	var opts applyOptions

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "HOME を desired state に合わせる",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			interactive := ui.IsInteractive(int(os.Stdin.Fd()), int(os.Stdout.Fd()))
			return runApply(cmd, flags.repo, opts, interactive)
		},
	}

	cmd.Flags().BoolVarP(&opts.dryRun, "dry-run", "n", false, "実行せず、何をするかだけを表示する")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "すべての確認を肯定とみなし、非対話で実行する")

	return cmd
}

// runApply は buildPlan が返した Plan を消費する。status と同じ 1 本の
// パイプラインを通り、apply 専用の解決ロジックを持たない（INV-11）。
//
// interactive は「対話 UI を起動してよいか」であり、呼び出し側が決める。
// 実際の CLI では stdin / stdout の TTY 判定がそれを決める（spec §11.4）。
func runApply(cmd *cobra.Command, repoFlag string, opts applyOptions, interactive bool) error {
	ws, _, p, err := buildPlan(cmd, repoFlag)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()

	if opts.dryRun {
		ui.RenderDryRun(out, ws.env.Home, p)
		if p.Errors() > 0 {
			return silentExitError{}
		}
		return nil
	}

	// 最初の確認を出す前に全体像を見せる。--yes の非対話実行でも
	// 「何をしたか」がログに残る。
	ui.RenderPlan(out, ws.env.Home, p.Actions)

	confirm, err := confirmFunc(cmd, ws.env.Home, p.Actions, opts, interactive)
	if err != nil {
		return err
	}

	res := exec.Apply(p.Actions, confirm)
	ui.RenderApplyResult(out, ws.env.Home, res, p)

	// 部分適用で終わった場合と、Action を作れなかった Error が残っている場合の
	// どちらも終了コード 1 である（spec §11.3 / §12.4）。
	if res.Err != nil || p.Errors() > 0 {
		return silentExitError{}
	}
	return nil
}

// confirmFunc は exec に渡す確認関数を決める。nil は「確認なしで全件実行」を
// 意味する（exec.Apply の規約）。
//
// 非 TTY で確認が必要な場合は spec §11.4 に従いエラーで止める。確認が
// 1 件も要らない plan は対話 UI を起動しないので、非 TTY でもそのまま実行
// できる。これがないとパイプ越しの再実行が永久に収束しない。
func confirmFunc(cmd *cobra.Command, home string, actions []plan.Action, opts applyOptions, interactive bool) (exec.Confirm, error) {
	if opts.yes {
		return nil, nil
	}
	if interactive {
		return ui.NewPrompter(cmd.InOrStdin(), cmd.OutOrStdout(), home).ConfirmAction, nil
	}
	if n := countConfirm(actions); n > 0 {
		return nil, fmt.Errorf(
			"confirmation is required for %d of these changes, but this is not an interactive terminal: re-run with --yes", n)
	}
	return nil, nil
}

func countConfirm(actions []plan.Action) int {
	n := 0
	for _, a := range actions {
		if a.Confirm {
			n++
		}
	}
	return n
}
