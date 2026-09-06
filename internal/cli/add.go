// add.go は homux add（spec §12.6）のコマンド定義である。
//
// add は resolve/inspect/plan のパイプラインを経由しない。取り込み対象は
// まだ repository に存在せず Source になりようがないため、status / apply /
// explain が共有する buildPlan（INV-11）とは別の、独立した経路である。
// 代わりに inspect.ReadCurrent で「単一のルール」（ADR 0003）を再利用し、
// 破壊的操作は internal/exec に委ねる（issue の「触ってよい範囲」）。
package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/bellwood4486/homux/internal/config"
	"github.com/bellwood4486/homux/internal/env"
	"github.com/bellwood4486/homux/internal/exec"
	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/selector"
	"github.com/bellwood4486/homux/internal/ui"
)

// addOptions は homux add のフラグである（spec §11）。
type addOptions struct {
	profile string
}

// newAddCmd は homux add を構築する。HOME と repository を変更し、local
// config には触れない（spec §11.2）。
func newAddCmd(flags *globalFlags) *cobra.Command {
	var opts addOptions

	cmd := &cobra.Command{
		Use:   "add <path>...",
		Short: "既存の unmanaged な HOME ファイルをリポジトリに取り込む",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			interactive := ui.IsInteractive(int(os.Stdin.Fd()), int(os.Stdout.Fd()))
			return runAdd(cmd, flags.repo, args, opts, interactive)
		},
	}

	cmd.Flags().StringVar(&opts.profile, "profile", "", "この source の profile selector（省略時は common source）")

	return cmd
}

// runAdd は spec §12.6 の手順をそのままなぞる: 対象を集めて検証し、plan を
// 見せて確認を取り、確認が得られたら exec.AddAll に委ねる。
func runAdd(cmd *cobra.Command, repoFlag string, args []string, opts addOptions, interactive bool) error {
	ws, err := loadWorkspace(repoFlag)
	if err != nil {
		return err
	}
	if opts.profile != "" && !ws.repo.HasProfile(opts.profile) {
		return fmt.Errorf("unknown profile %q: %s defines %s",
			opts.profile, config.RepoFileName, profileList(ws.repo.Profiles))
	}

	var items []exec.AddItem
	for _, arg := range args {
		found, err := collectAddItems(ws.env, arg, opts.profile)
		if err != nil {
			return err
		}
		items = append(items, found...)
	}

	out := cmd.OutOrStdout()
	ui.RenderAddPlan(out, ws.env.Home, items)

	if !interactive {
		return errors.New(
			"confirmation is required, but this is not an interactive terminal: homux add always asks for confirmation")
	}
	ok, err := ui.NewPrompter(cmd.InOrStdin(), out, ws.env.Home).Confirm("Add these files?")
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(out, "Nothing added.")
		return nil
	}

	res := exec.AddAll(items)
	ui.RenderAddResult(out, ws.env.Home, res)
	if res.Err != nil {
		return silentExitError{}
	}
	return nil
}

// collectAddItems は 1 つの CLI 引数から取り込み対象を集める。ディレクトリなら
// 配下の全ファイルを再帰的に対象とする（spec §12.6）。
func collectAddItems(e env.Env, argPath, profile string) ([]exec.AddItem, error) {
	abs, rel, err := resolveHomeArg(e.Home, argPath)
	if err != nil {
		return nil, err
	}

	current, err := inspect.ReadCurrent(e.Repo, abs)
	if err != nil {
		return nil, err
	}
	if current.Kind == inspect.CurrentDir {
		return collectDirItems(e, abs, profile)
	}

	item, err := buildAddItem(e, abs, rel, profile, current)
	if err != nil {
		return nil, err
	}
	return []exec.AddItem{item}, nil
}

// collectDirItems はディレクトリ配下の全ファイルを再帰的に集める。symlink は
// ファイル単位でしか張らない（ADR 0001）ため、ディレクトリ自体を 1 つの対象には
// しない。
func collectDirItems(e env.Env, dirAbs, profile string) ([]exec.AddItem, error) {
	var items []exec.AddItem
	err := filepath.WalkDir(dirAbs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dirAbs || d.IsDir() {
			return nil
		}

		rel := filepath.ToSlash(mustRel(e.Home, p))
		current, err := inspect.ReadCurrent(e.Repo, p)
		if err != nil {
			return err
		}
		item, err := buildAddItem(e, p, rel, profile, current)
		if err != nil {
			return err
		}
		items = append(items, item)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

// resolveHomeArg は CLI 引数を絶対パス化し、$HOME 配下であることを確かめた
// うえで HOME からの相対パス（slash 区切り）を返す（spec §12.6:
// 「$HOME 外のパスはエラー」）。
//
// explainTarget と同じ理由で、比較にだけ canonicalize を使う
// （macOS の /tmp -> /private/tmp のような経路差を吸収するため）。
func resolveHomeArg(home, argPath string) (abs, rel string, err error) {
	expanded, err := expandTilde(argPath)
	if err != nil {
		return "", "", err
	}
	abs, err = filepath.Abs(expanded)
	if err != nil {
		return "", "", err
	}

	canonAbs := canonicalize(abs)
	canonHome := canonicalize(home)
	if !within(canonHome, canonAbs) {
		return "", "", fmt.Errorf("%s: not inside HOME (%s)", argPath, home)
	}
	rel = filepath.ToSlash(mustRel(canonHome, canonAbs))
	return abs, rel, nil
}

// buildAddItem は 1 ファイルを検証し、exec.AddItem を組み立てる（spec §12.6 の
// ケース表）。current は abs の呼び出し側での読み取り結果である。
func buildAddItem(e env.Env, abs, rel, profile string, current inspect.Current) (exec.AddItem, error) {
	switch current.Kind {
	case inspect.CurrentAbsent:
		return exec.AddItem{}, fmt.Errorf("~/%s: does not exist", rel)
	case inspect.CurrentSymlink:
		if current.Managed {
			return exec.AddItem{}, fmt.Errorf("~/%s: is already a managed symlink", rel)
		}
		return exec.AddItem{}, fmt.Errorf("~/%s: is a symlink to somewhere else", rel)
	case inspect.CurrentDir:
		// collectAddItems / collectDirItems がディレクトリを別枝で処理するため
		// ここには来ない。
		return exec.AddItem{}, fmt.Errorf("~/%s: is a directory", rel)
	}

	repoRel := repoDestPath(rel, profile)
	repoAbs := filepath.Join(e.Repo, filepath.FromSlash(repoRel))
	if _, err := os.Lstat(repoAbs); err == nil {
		return exec.AddItem{}, fmt.Errorf("%s: already exists in the repository", repoRel)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return exec.AddItem{}, err
	}

	fork := false
	if profile != "" {
		commonAbs := filepath.Join(e.Repo, filepath.FromSlash(rel))
		if _, err := os.Lstat(commonAbs); err == nil {
			fork = true
		} else if !errors.Is(err, fs.ErrNotExist) {
			return exec.AddItem{}, err
		}
	}

	return exec.AddItem{Target: abs, RepoPath: repoAbs, Fork: fork}, nil
}

// repoDestPath は HOME 相対パス rel と profile から repository 相対パスを
// 組み立てる。suffix はファイル名にだけ付く（spec §4.2、INV-16）。
func repoDestPath(rel, profile string) string {
	dir, name := path.Split(rel)
	if profile != "" {
		name += selector.Delimiter + profile
	}
	return dir + name
}
