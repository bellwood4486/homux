// profile.go は homux profile list / use（spec §12.7、§12.8）のコマンド定義
// である。
//
// どちらも HOME を変更しない（spec §11.2）。use は repository を一切変更せず
// local config だけを書き換える。unknown profile の判定は resolve.All が
// active profile に対して行うのと同じ規則を再利用し、判定ロジックを
// 二重に書かない（INV-11 の精神）。
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/bellwood4486/homux/internal/config"
	"github.com/bellwood4486/homux/internal/resolve"
	"github.com/bellwood4486/homux/internal/ui"
)

// newProfileCmd は homux profile の親コマンドを構築する。
func newProfileCmd(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "profile を管理する",
	}

	cmd.AddCommand(newProfileListCmd(flags))
	cmd.AddCommand(newProfileCreateCmd(flags))
	cmd.AddCommand(newProfileUseCmd(flags))
	cmd.AddCommand(newProfileRenameCmd(flags))

	return cmd
}

// newProfileListCmd は homux profile list を構築する。何も変更しない
// （spec §11.2）。
func newProfileListCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: ".homux.toml の profiles と active profile を表示する",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProfileList(cmd, flags.repo)
		},
	}
}

func runProfileList(cmd *cobra.Command, repoFlag string) error {
	ws, err := loadWorkspace(repoFlag)
	if err != nil {
		return err
	}
	ui.RenderProfileList(cmd.OutOrStdout(), ws.repo.Profiles, ws.profile)
	return nil
}

// newProfileUseCmd は homux profile use <name> を構築する。local config だけ
// を変更し、repository と HOME は変更しない（spec §11.2）。
func newProfileUseCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "この PC の active profile を切り替える",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfileUse(cmd, flags.repo, args[0])
		},
	}
}

// runProfileUse は spec §12.8 の手順をそのままなぞる。
//
// 対象 profile は .homux.toml に定義済みでなければならない（unknown profile
// はエラー）。use から暗黙に profile を作成しない。切替後、buildPlan
// （status / apply と同じパイプライン、INV-11）を新しい active profile で
// 走らせ、desired state と差異があるときだけ "homux apply" のヒントを出す
// （spec §12.8）。
func runProfileUse(cmd *cobra.Command, repoFlag, name string) error {
	ws, err := loadWorkspace(repoFlag)
	if err != nil {
		return err
	}

	if _, err := resolve.All(resolve.Input{Profiles: ws.repo.Profiles, Active: name}); err != nil {
		fmt.Fprint(cmd.ErrOrStderr(), ui.FormatResolveError(err))
		return silentExitError{}
	}

	localPath := config.LocalPath(os.Getenv("XDG_CONFIG_HOME"), ws.env.Home)
	if err := config.SaveLocal(localPath, &config.Local{Repo: ws.env.Repo, Profile: name}); err != nil {
		return err
	}

	_, _, p, err := buildPlan(cmd, repoFlag)
	if err != nil {
		return err
	}

	ui.RenderProfileSwitch(cmd.OutOrStdout(), ws.profile, name, len(p.Actions) > 0)
	return nil
}
