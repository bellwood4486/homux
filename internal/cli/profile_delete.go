// profile_delete.go は homux profile delete（spec §12.11）のコマンド定義である。
//
// repository と local config を変更し、HOME には触れない（spec §11.2）。
// 消した source を指す HOME 側の symlink はそのまま残るため、完了後に
// "homux apply" が必要である旨を表示する。
//
// これは repository 内の source file を消す唯一の経路である。INV-14 が禁じて
// いるのは通常の apply による削除であり、ここでは削除対象を plan に全件
// 並べ、確認を取ってからでなければ 1 件も消さない。
//
// rename と同じく事前に全件を検証してから実行する（INV-15 は rename に対する
// 要求だが、delete も「途中まで消えた repository」を残さない方が良い）。
// 実行順序は「rewrite → 削除 → .homux.toml の profiles → local config」である。
// rewrite を先に置くのは、情報を失わない操作を先に済ませるためである。設定を
// 最後に書くのは rename と同じで、想定外の I/O エラーで途中終了しても
// .homux.toml に profile が残っていることが利用者への手掛かりになる。
package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bellwood4486/homux/internal/config"
	"github.com/bellwood4486/homux/internal/exec"
	"github.com/bellwood4486/homux/internal/resolve"
	"github.com/bellwood4486/homux/internal/scan"
	"github.com/bellwood4486/homux/internal/selector"
	"github.com/bellwood4486/homux/internal/ui"
)

// rewriteCollisionError は事前検証で見つかった rewrite 先の衝突である。
// 診断は ui が整形済みで出すため、runProfileDelete はこれを受けて
// silentExitError を返す。
type rewriteCollisionError struct {
	collision ui.RewriteCollision
}

func (e *rewriteCollisionError) Error() string {
	return fmt.Sprintf("rewrite collision: %s -> %s", e.collision.Line.From, e.collision.Line.To)
}

// newProfileDeleteCmd は homux profile delete <name> を構築する。
func newProfileDeleteCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "profile を削除し、repository 全体の参照を整理する",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			interactive := ui.IsInteractive(int(os.Stdin.Fd()), int(os.Stdout.Fd()))
			return runProfileDelete(cmd, flags.repo, args[0], interactive)
		},
	}
}

// runProfileDelete は spec §12.11 の手順をそのままなぞる。
func runProfileDelete(cmd *cobra.Command, repoFlag, name string, interactive bool) error {
	ws, err := loadWorkspace(repoFlag)
	if err != nil {
		return err
	}
	if !ws.repo.HasProfile(name) {
		return fmt.Errorf("unknown profile %q: %s defines %s",
			name, config.RepoFileName, profileList(ws.repo.Profiles))
	}

	scanned, err := scan.Repository(ws.env.Repo, ws.repo.Ignore)
	if err != nil {
		return err
	}
	// 壊れた repository の参照を機械的に消しても状況が良くならない。
	// status / apply と同じ診断で先に止める（spec §10、rename と同じ）。
	resolutions, err := resolve.All(resolve.Input{
		Sources:  scanned.Sources,
		Profiles: ws.repo.Profiles,
		Active:   ws.profile,
	})
	if err != nil {
		fmt.Fprint(cmd.ErrOrStderr(), ui.FormatResolveError(err))
		return silentExitError{}
	}
	if err := structuralError(resolutions); err != nil {
		fmt.Fprint(cmd.ErrOrStderr(), ui.FormatResolveError(err))
		return silentExitError{}
	}

	plan, items, remaining, err := buildDeletePlan(ws.env.Repo, scanned.Sources, name, ws.profile == name)
	if err != nil {
		var collision *rewriteCollisionError
		if errors.As(err, &collision) {
			fmt.Fprint(cmd.ErrOrStderr(), ui.FormatRewriteCollision(collision.collision))
			return silentExitError{}
		}
		return err
	}

	plan.HomeChanges, err = homeChanges(resolutions, remaining, ws.repo.Profiles, name, ws.profile)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	ui.RenderDeletePlan(out, plan)

	if !interactive {
		return errors.New(
			"confirmation is required, but this is not an interactive terminal: homux profile delete always asks for confirmation")
	}
	ok, err := ui.NewPrompter(cmd.InOrStdin(), out, ws.env.Home).Confirm("Apply?")
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(out, "Nothing changed.")
		return nil
	}

	res := exec.DeleteAll(items)
	if res.Err != nil {
		ui.RenderDeleteResult(out, ws.env.Repo, plan, res)
		return silentExitError{}
	}

	if err := deleteProfileDefinition(ws, name); err != nil {
		return err
	}

	ui.RenderDeleteResult(out, ws.env.Repo, plan, res)
	return nil
}

// deleteProfileDefinition は .homux.toml の profiles から name を落とし、
// 必要なら local active profile を「なし」にする。参照の整理が全件終わった
// 後にだけ呼ばれる。
//
// active profile が未定義の profile を指したまま残ると、以降の status も
// apply も unknown profile で止まる。profiles から消すなら local config も
// 同時に片付ける（spec §12.11）。
func deleteProfileDefinition(ws *workspace, name string) error {
	profiles := make([]string, 0, len(ws.repo.Profiles))
	for _, p := range ws.repo.Profiles {
		if p != name {
			profiles = append(profiles, p)
		}
	}

	repoFilePath := filepath.Join(ws.env.Repo, config.RepoFileName)
	if err := config.ReplaceProfiles(repoFilePath, profiles); err != nil {
		return fmt.Errorf(
			"the repository files were updated, but updating %s failed: %w", config.RepoFileName, err)
	}

	if ws.profile != name {
		return nil
	}
	localPath := config.LocalPath(os.Getenv("XDG_CONFIG_HOME"), ws.env.Home)
	if err := config.SaveLocal(localPath, &config.Local{Repo: ws.env.Repo}); err != nil {
		return fmt.Errorf(
			"the profile was deleted, but clearing the local active profile failed: %w", err)
	}
	return nil
}

// buildDeletePlan は削除対象を全件集め、そのすべてが適用可能であることを
// 確かめる。1 件でも rewrite が衝突すれば rewriteCollisionError を返し、
// 呼び出し側は何も実行しない。
//
// 返す remaining は削除後に repository へ残る source である。HOME の解決先が
// どう変わるかを同じ resolver で求めるために使う（INV-11）。
//
// items は「rewrite が先、削除が後」の順に並べる。exec は並べ替えない。
func buildDeletePlan(repo string, sources []scan.Source, name string, localActive bool) (ui.DeletePlan, []exec.DeleteItem, []scan.Source, error) {
	plan := ui.DeletePlan{Profile: name, LocalActive: localActive}
	var rewrites, removals []exec.DeleteItem
	remaining := make([]scan.Source, 0, len(sources))
	seen := make(map[string]bool, len(sources))

	for _, s := range sources {
		if s.Selector == nil || !s.Selector.Matches(name) {
			remaining = append(remaining, s)
			continue
		}

		// 単一 profile suffix はファイルごと消す。この profile が無くなれば
		// 二度と選ばれることのない source だからである（spec §12.11 の表）。
		if len(s.Selector.Profiles) == 1 {
			plan.Removals = append(plan.Removals, s.RepoPath)
			removals = append(removals, exec.DeleteItem{
				Path: filepath.Join(repo, filepath.FromSlash(s.RepoPath)),
			})
			continue
		}

		// 複数指定は他の profile からも参照されている。消してはならず、
		// この profile を落とした形へ rewrite する（spec §12.11 の表）。
		profiles := make([]string, 0, len(s.Selector.Profiles)-1)
		for _, p := range s.Selector.Profiles {
			if p != name {
				profiles = append(profiles, p)
			}
		}

		// "@@" を除いた名前は変わらないため、宛先は Target に新しい selector を
		// 付け直したものになる（INV-16）。"+" は複数指定の区切りである（spec §6.5）。
		line := ui.RewriteLine{
			From: s.RepoPath,
			To:   s.Target + selector.Delimiter + strings.Join(profiles, "+"),
		}

		if seen[line.To] {
			return ui.DeletePlan{}, nil, nil, &rewriteCollisionError{
				collision: ui.RewriteCollision{Line: line, Kind: ui.CollisionDuplicate},
			}
		}
		seen[line.To] = true

		toAbs := filepath.Join(repo, filepath.FromSlash(line.To))
		if _, err := os.Lstat(toAbs); err == nil {
			return ui.DeletePlan{}, nil, nil, &rewriteCollisionError{
				collision: ui.RewriteCollision{Line: line, Kind: ui.CollisionExists},
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return ui.DeletePlan{}, nil, nil, err
		}

		plan.Rewrites = append(plan.Rewrites, line)
		rewrites = append(rewrites, exec.DeleteItem{
			Path:    filepath.Join(repo, filepath.FromSlash(line.From)),
			Rewrite: toAbs,
		})
		remaining = append(remaining, scan.Source{
			RepoPath: line.To,
			Target:   s.Target,
			Selector: &selector.Selector{Profiles: profiles},
		})
	}

	return plan, append(rewrites, removals...), remaining, nil
}

// homeChanges は削除の前後で target の解決先がどう変わるかを返す
// （spec §12.11: その結果 HOME がどう変わるかも plan に含める）。
//
// 削除後の source と profile 定義を同じ resolver に通して比べる（INV-11）。
// HOME の実状態は見ない。ここに載せるのは delete が引き起こす desired state の
// 差分だけであり、既存の drift まで並べると delete のせいでそうなったかの
// ように読めてしまう。実際に HOME を書き換えるのは "homux apply" である。
func homeChanges(before []resolve.Resolution, remaining []scan.Source, profiles []string, name, active string) ([]ui.HomeChange, error) {
	rest := make([]string, 0, len(profiles))
	for _, p := range profiles {
		if p != name {
			rest = append(rest, p)
		}
	}
	// 削除後の active profile。削除対象を使っていたなら「なし」になる。
	if active == name {
		active = ""
	}

	after, err := resolve.All(resolve.Input{Sources: remaining, Profiles: rest, Active: active})
	if err != nil {
		return nil, err
	}

	selected := func(rs []resolve.Resolution) map[string]string {
		m := make(map[string]string, len(rs))
		for _, r := range rs {
			if r.Selected != nil {
				m[r.Target] = r.Selected.RepoPath
			} else {
				m[r.Target] = ""
			}
		}
		return m
	}
	from, to := selected(before), selected(after)

	var changes []ui.HomeChange
	// before は Target 昇順である。削除で target ごと消えることはあっても、
	// 増えることはない（source は減るだけ）。before を回れば全件を見られる。
	for _, r := range before {
		if from[r.Target] == to[r.Target] {
			continue
		}
		changes = append(changes, ui.HomeChange{
			Target: r.Target,
			From:   from[r.Target],
			To:     to[r.Target],
		})
	}
	return changes, nil
}
