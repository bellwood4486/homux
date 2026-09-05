// init.go は homux init（spec §12.1）のコマンド定義である。
//
// init は例外的に repository・local config・HOME のすべてを変更する
// （spec §11.2）が、HOME への適用は apply と同じ Planner / Executor を
// 通す。runInit の最後は runApply をそのまま呼ぶだけである（INV-11）。
package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bellwood4486/homux/internal/config"
	"github.com/bellwood4486/homux/internal/ui"
)

// initOptions は homux init のフラグである（spec §11）。
//
// profileSet は --profile が明示されたかを持つ。--profile= を「profile なし」の
// 明示指定として扱うため、空文字列だけでは未指定と区別できない。
type initOptions struct {
	profile    string
	profileSet bool
}

func newInitCmd(flags *globalFlags) *cobra.Command {
	var opts initOptions

	cmd := &cobra.Command{
		Use:   "init",
		Short: "repository を設定し、HOME へ適用する",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.profileSet = cmd.Flags().Changed("profile")
			interactive := ui.IsInteractive(int(os.Stdin.Fd()), int(os.Stdout.Fd()))
			return runInit(cmd, flags.repo, opts, interactive)
		},
	}

	cmd.Flags().StringVar(&opts.profile, "profile", "", `active profile（--profile= で「profile なし」）`)

	return cmd
}

// runInit は spec §12.1 の手順をそのままなぞる。
//
// interactive は「対話 UI を起動してよいか」であり、呼び出し側が決める
// （runApply と同じ規約）。非対話で対話が必要になった時点で止まる（spec §11.4）。
func runInit(cmd *cobra.Command, repoFlag string, opts initOptions, interactive bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	out := cmd.OutOrStdout()
	prompter := ui.NewPrompter(cmd.InOrStdin(), out, home)

	repoPath, err := initRepoPath(prompter, out, repoFlag, interactive)
	if err != nil {
		return err
	}
	if err := ensureRepoFile(prompter, out, home, repoPath, interactive); err != nil {
		return err
	}

	repoCfg, err := config.LoadRepo(filepath.Join(repoPath, config.RepoFileName))
	if err != nil {
		return err
	}
	profile, err := initProfile(prompter, repoCfg, opts, interactive)
	if err != nil {
		return err
	}

	localPath := config.LocalPath(os.Getenv("XDG_CONFIG_HOME"), home)
	if err := config.SaveLocal(localPath, &config.Local{Repo: repoPath, Profile: profile}); err != nil {
		return err
	}
	ui.RenderInitSummary(out, home, localPath, repoPath, profile)

	// ここから先は apply と 1 バイトも違わない経路を通る（INV-11）。
	// local config は保存済みだが、解決済みのパスを明示的に渡す。
	return runApply(cmd, repoPath, applyOptions{}, interactive)
}

// initRepoPath は repository のパスを決める。
//
// --repo で渡された値は聞き直さずその場で失敗する。対話入力は、有効なパスを
// 得るまで聞き直す。既定候補はカレントディレクトリである（spec §12.1）。
func initRepoPath(prompter *ui.Prompter, out io.Writer, repoFlag string, interactive bool) (string, error) {
	if repoFlag != "" {
		return validateRepoDir(repoFlag)
	}
	if !interactive {
		return "", errors.New(
			"repository path is required, but this is not an interactive terminal: re-run with --repo <path>")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		answer, err := prompter.AskRepoPath(cwd)
		if err != nil {
			return "", err
		}
		resolved, err := validateRepoDir(answer)
		if err == nil {
			return resolved, nil
		}
		ui.RenderRejectedInput(out, err)
	}
}

// validateRepoDir はパスを絶対パス化・実体解決したうえで、ディレクトリで
// あることを確かめる（docs/design.md §4.2）。
func validateRepoDir(path string) (string, error) {
	resolved, err := resolveRepoPath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repository path %q: not a directory", path)
	}
	return resolved, nil
}

// ensureRepoFile は .homux.toml が無ければ確認のうえ雛形を書き出す
// （spec §12.1）。既存ファイルには一切触れない。
func ensureRepoFile(prompter *ui.Prompter, out io.Writer, home, repoPath string, interactive bool) error {
	path := filepath.Join(repoPath, config.RepoFileName)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	if !interactive {
		return fmt.Errorf(
			"%s: not a homux repository (missing %s), and this is not an interactive terminal: create %s first",
			repoPath, config.RepoFileName, config.RepoFileName)
	}

	ok, err := prompter.ConfirmNewRepository(repoPath)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s: not a homux repository (missing %s)", repoPath, config.RepoFileName)
	}
	if err := config.CreateRepoFile(path); err != nil {
		return err
	}
	ui.RenderRepoFileCreated(out, home, path)
	return nil
}

// initProfile は active profile を決める。空文字列は「profile なし」である
// （spec §5.3）。
//
// profiles が空のときは選択肢が (none) しか無いため問わない。
func initProfile(prompter *ui.Prompter, repoCfg *config.Repo, opts initOptions, interactive bool) (string, error) {
	if opts.profileSet {
		if opts.profile == "" {
			return "", nil
		}
		if !repoCfg.HasProfile(opts.profile) {
			return "", fmt.Errorf("unknown profile %q: %s defines %s",
				opts.profile, config.RepoFileName, profileList(repoCfg.Profiles))
		}
		return opts.profile, nil
	}
	if len(repoCfg.Profiles) == 0 {
		return "", nil
	}
	if !interactive {
		return "", errors.New(
			"an active profile must be chosen, but this is not an interactive terminal: " +
				"re-run with --profile <name>, or --profile= for no profile")
	}
	return prompter.SelectProfile(repoCfg.Profiles)
}

func profileList(profiles []string) string {
	if len(profiles) == 0 {
		return "no profiles"
	}
	return strings.Join(profiles, ", ")
}
