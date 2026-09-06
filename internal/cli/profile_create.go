// profile_create.go は homux profile create（spec §12.9）のコマンド定義である。
//
// repository だけを変更し、HOME と local config には触れない（spec §11.2）。
// 完了後は "homux apply" が必要である旨を表示する。
//
// 実行順序は「衝突の事前検証 → .homux.toml の profiles を範囲置換 → fork の
// copy」である。逆順にすると、途中で失敗したときに "@@<profile>" のファイルは
// あるが profiles に定義が無い状態が残り、resolve が unknown profile で
// 止まって status すら読めなくなる。profiles を先に足しておけば、fork が
// 一部しか進まなくても common source への fallback（INV-08）が効き、
// status で差分を見て続きを判断できる。
package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/bellwood4486/homux/internal/config"
	"github.com/bellwood4486/homux/internal/exec"
	"github.com/bellwood4486/homux/internal/resolve"
	"github.com/bellwood4486/homux/internal/scan"
	"github.com/bellwood4486/homux/internal/selector"
	"github.com/bellwood4486/homux/internal/ui"
)

// forkSelector は候補から fork するものを選ばせる。本番では
// ui.SelectForkTargets（huh の MultiSelect）であり、テストは選択結果を
// 直接返す関数に差し替える。
type forkSelector func(profile string, candidates []string) ([]string, error)

// newProfileCreateCmd は homux profile create <name> を構築する。
func newProfileCreateCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "新しい profile を追加し、common source の fork を選ぶ",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			interactive := ui.IsInteractive(int(os.Stdin.Fd()), int(os.Stdout.Fd()))
			return runProfileCreate(cmd, flags, args[0], ui.SelectForkTargets, interactive)
		},
	}
}

// runProfileCreate は spec §12.9 の手順をそのままなぞる。
func runProfileCreate(cmd *cobra.Command, flags *globalFlags, name string, sel forkSelector, interactive bool) error {
	if !selector.ValidProfileName(name) {
		return newUsageError(fmt.Errorf(
			"invalid profile name %q: must match ^[a-z0-9][a-z0-9_-]*$", name))
	}

	ws, err := loadWorkspace(flags.repo)
	if err != nil {
		return err
	}
	if ws.repo.HasProfile(name) {
		return fmt.Errorf(
			"profile %q already exists in %s: to fork one more file into it, run \"cp <file> <file>%s%s\"",
			name, config.RepoFileName, selector.Delimiter, name)
	}
	if !interactive {
		return errors.New(
			"homux profile create is interactive, but this is not an interactive terminal")
	}

	scanned, err := scan.Repository(ws.env.Repo, ws.repo.Ignore)
	if err != nil {
		return err
	}
	// 壊れた repository に profile を足しても状況が良くならない。status /
	// apply と同じ診断で先に止める（spec §10）。
	resolutions, err := resolve.All(resolve.Input{
		Sources:  scanned.Sources,
		Profiles: ws.repo.Profiles,
		Active:   ws.profile,
	})
	if err != nil {
		fmt.Fprint(cmd.ErrOrStderr(), ui.FormatResolveError(flags.colorErr, err))
		return silentExitError{}
	}
	if err := structuralError(resolutions); err != nil {
		fmt.Fprint(cmd.ErrOrStderr(), ui.FormatResolveError(flags.colorErr, err))
		return silentExitError{}
	}

	candidates, skipped := forkCandidates(scanned.Sources)

	var selected []string
	if len(candidates) > 0 {
		selected, err = sel(name, candidates)
		if err != nil {
			return err
		}
	}

	plan, forks, err := buildMigration(ws.env.Repo, name, candidates, selected, skipped)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	ui.RenderMigrationPlan(out, flags.colorOut, plan)

	ok, err := ui.NewPrompter(cmd.InOrStdin(), out, ws.env.Home).Confirm("Apply this migration?")
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(out, "Nothing changed.")
		return nil
	}

	repoFilePath := filepath.Join(ws.env.Repo, config.RepoFileName)
	if err := config.ReplaceProfiles(repoFilePath, append(ws.repo.Profiles, name)); err != nil {
		return err
	}

	res := exec.ForkAll(forks)
	ui.RenderMigrationResult(out, flags.colorOut, ws.env.Repo, name, res)
	if res.Err != nil {
		return silentExitError{}
	}
	return nil
}

// structuralError は target ごとの解決エラー（unknown profile / ambiguous /
// invalid selector）をまとめて返す。1 件も無ければ nil。
//
// resolve.All の返り値の error は「active profile 自体が未定義」だけを表す。
// 個々の target の問題は Resolution.Err に載るため、status が
// Plan.Errors() で数えているのと同じものをここでも見る必要がある。
func structuralError(resolutions []resolve.Resolution) error {
	var errs []error
	for _, r := range resolutions {
		if r.Err != nil {
			errs = append(errs, r.Err)
		}
	}
	return errors.Join(errs...)
}

// forkCandidates は fork できる common source（repo 相対パス）と、common
// source を持たないため候補にできなかった target の数を返す。
//
// common source は selector を持たないため、その repo 相対パスは target と
// 一致する（INV-16）。fork は common の複製なので、common が無い target
// （foo@@personal しか存在しない、など）は複製元が定まらず候補にならない。
func forkCandidates(sources []scan.Source) (candidates []string, skipped int) {
	withCommon := make(map[string]bool, len(sources))
	targets := make(map[string]bool, len(sources))
	for _, s := range sources {
		targets[s.Target] = true
		if s.Selector == nil && s.SelectorErr == nil {
			withCommon[s.Target] = true
			candidates = append(candidates, s.RepoPath)
		}
	}
	sort.Strings(candidates)
	return candidates, len(targets) - len(withCommon)
}

// buildMigration は選択結果から plan と fork 対象を組み立てる。
//
// fork 先の衝突はここで全件を検証する。1 件でも作れないものがあれば、
// repository を 1 バイトも変更する前に止まる。
func buildMigration(repo, name string, candidates, selected []string, skipped int) (ui.MigrationPlan, []exec.ForkItem, error) {
	chosen := make(map[string]bool, len(selected))
	for _, s := range selected {
		chosen[s] = true
	}

	plan := ui.MigrationPlan{Profile: name, SkippedNoCommon: skipped}
	var forks []exec.ForkItem
	for _, c := range candidates {
		if !chosen[c] {
			// common source の repo 相対パスは target と一致する（INV-16）。
			plan.KeepTargets = append(plan.KeepTargets, c)
			continue
		}

		forkRel := c + selector.Delimiter + name
		forkAbs := filepath.Join(repo, filepath.FromSlash(forkRel))
		if _, err := os.Lstat(forkAbs); err == nil {
			return ui.MigrationPlan{}, nil, fmt.Errorf("%s: already exists in the repository", forkRel)
		} else if !errors.Is(err, os.ErrNotExist) {
			return ui.MigrationPlan{}, nil, err
		}

		plan.Forks = append(plan.Forks, ui.ForkLine{Common: c, Fork: forkRel})
		forks = append(forks, exec.ForkItem{
			Common: filepath.Join(repo, filepath.FromSlash(c)),
			Fork:   forkAbs,
		})
	}
	return plan, forks, nil
}
