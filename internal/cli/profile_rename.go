// profile_rename.go は homux profile rename（spec §12.10）のコマンド定義である。
//
// repository と local config を変更し、HOME には触れない（spec §11.2）。
// "@@" suffix を除いた名前は変わらないため（INV-16）、rename の前後で HOME に
// 配置される内容は同一である。したがって "homux apply" は促さない。
//
// 他の破壊的コマンドと違い、これは部分適用を許さない（INV-15）。全件の衝突を
// 先に検証し、1 件でも通らなければ repository を 1 バイトも変更せずに止まる。
// 検証と実行を分けるために、plan の組み立て（buildRenamePlan、純粋な計算 +
// 存在検査のみ）と実行を別の関数に置く。
//
// 実行順序は「ファイルの rename → .homux.toml の profiles → local config」で
// ある。設定は正本の宣言であり、参照の付け替えが全件終わって初めて
// 「profile が改名された」と言える。想定外の I/O エラーで途中終了した場合も、
// .homux.toml が旧名のままであることが利用者への手掛かりになる。
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

// renameCollisionError は事前検証で見つかった衝突である。診断は ui が整形
// 済みで出すため、runProfileRename はこれを受けて silentExitError を返す。
type renameCollisionError struct {
	collision ui.RenameCollision
}

func (e *renameCollisionError) Error() string {
	return fmt.Sprintf("rename collision: %s -> %s", e.collision.Line.From, e.collision.Line.To)
}

// newProfileRenameCmd は homux profile rename <old> <new> を構築する。
func newProfileRenameCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "profile を改名し、repository 全体の参照を更新する",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			interactive := ui.IsInteractive(int(os.Stdin.Fd()), int(os.Stdout.Fd()))
			return runProfileRename(cmd, flags, args[0], args[1], interactive)
		},
	}
}

// runProfileRename は spec §12.10 の手順をそのままなぞる。
func runProfileRename(cmd *cobra.Command, flags *globalFlags, from, to string, interactive bool) error {
	if !selector.ValidProfileName(to) {
		return newUsageError(fmt.Errorf(
			"invalid profile name %q: must match ^[a-z0-9][a-z0-9_-]*$", to))
	}

	ws, err := loadWorkspace(flags.repo)
	if err != nil {
		return err
	}
	if !ws.repo.HasProfile(from) {
		return fmt.Errorf("unknown profile %q: %s defines %s",
			from, config.RepoFileName, profileList(ws.repo.Profiles))
	}
	// 既存 profile への rename は統合になる。1 ファイル単位の付け替えが
	// mv で足りるのと同じ理由で、統合は CLI の責務ではない（spec §16）。
	if ws.repo.HasProfile(to) {
		return fmt.Errorf(
			"profile %q already exists in %s: profile rename does not merge profiles",
			to, config.RepoFileName)
	}

	scanned, err := scan.Repository(ws.env.Repo, ws.repo.Ignore)
	if err != nil {
		return err
	}
	// 壊れた repository の参照を機械的に付け替えても状況が良くならない。
	// status / apply と同じ診断で先に止める（spec §10、profile create と同じ）。
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

	plan, items, err := buildRenamePlan(ws.env.Repo, scanned.Sources, from, to, ws.profile == from)
	if err != nil {
		var collision *renameCollisionError
		if errors.As(err, &collision) {
			fmt.Fprint(cmd.ErrOrStderr(), ui.FormatRenameCollision(flags.colorErr, collision.collision))
			return silentExitError{}
		}
		return err
	}

	out := cmd.OutOrStdout()
	ui.RenderRenamePlan(out, flags.colorOut, plan)

	if !interactive {
		return errors.New(
			"confirmation is required, but this is not an interactive terminal: homux profile rename always asks for confirmation")
	}
	ok, err := ui.NewPrompter(cmd.InOrStdin(), out, ws.env.Home).Confirm("Apply?")
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(out, "Nothing changed.")
		return nil
	}

	res := exec.RenameAll(items)
	if res.Err != nil {
		ui.RenderRenameResult(out, flags.colorOut, ws.env.Repo, plan, res)
		return silentExitError{}
	}

	if err := renameProfileDefinition(ws, from, to); err != nil {
		return err
	}

	ui.RenderRenameResult(out, flags.colorOut, ws.env.Repo, plan, res)
	return nil
}

// renameProfileDefinition は .homux.toml の profiles と、必要なら local active
// profile を更新する。参照の付け替えが全件終わった後にだけ呼ばれる。
func renameProfileDefinition(ws *workspace, from, to string) error {
	profiles := make([]string, 0, len(ws.repo.Profiles))
	for _, p := range ws.repo.Profiles {
		if p == from {
			p = to
		}
		profiles = append(profiles, p)
	}

	repoFilePath := filepath.Join(ws.env.Repo, config.RepoFileName)
	if err := config.ReplaceProfiles(repoFilePath, profiles); err != nil {
		return fmt.Errorf(
			"the repository files were renamed, but updating %s failed: %w", config.RepoFileName, err)
	}

	if ws.profile != from {
		return nil
	}
	localPath := config.LocalPath(os.Getenv("XDG_CONFIG_HOME"), ws.env.Home)
	if err := config.SaveLocal(localPath, &config.Local{Repo: ws.env.Repo, Profile: to}); err != nil {
		return fmt.Errorf(
			"the repository was renamed, but updating the local active profile failed: %w", err)
	}
	return nil
}

// buildRenamePlan は改名対象を全件集め、そのすべてが適用可能であることを
// 確かめる。1 件でも衝突すれば renameCollisionError を返し、呼び出し側は
// 何も実行しない（INV-15）。
//
// 対象は selector に from を含む source だけである。selector の順序は保つ
// （"work+personal" は "company+personal" になる）。common source は profile を
// 参照しないため対象外である。
func buildRenamePlan(repo string, sources []scan.Source, from, to string, localActive bool) (ui.RenamePlan, []exec.RenameItem, error) {
	plan := ui.RenamePlan{From: from, To: to, LocalActive: localActive}
	var items []exec.RenameItem
	seen := make(map[string]bool, len(sources))

	for _, s := range sources {
		if s.Selector == nil || !s.Selector.Matches(from) {
			continue
		}

		profiles := make([]string, 0, len(s.Selector.Profiles))
		for _, p := range s.Selector.Profiles {
			if p == from {
				p = to
			}
			profiles = append(profiles, p)
		}

		// "@@" を除いた名前は変わらないため、宛先は Target に新しい selector を
		// 付け直したものになる（INV-16）。"+" は複数指定の区切りである（spec §6.5）。
		line := ui.RenameLine{
			From: s.RepoPath,
			To:   s.Target + selector.Delimiter + strings.Join(profiles, "+"),
		}

		if seen[line.To] {
			return ui.RenamePlan{}, nil, &renameCollisionError{
				collision: ui.RenameCollision{Line: line, Kind: ui.CollisionDuplicate},
			}
		}
		seen[line.To] = true

		toAbs := filepath.Join(repo, filepath.FromSlash(line.To))
		if _, err := os.Lstat(toAbs); err == nil {
			return ui.RenamePlan{}, nil, &renameCollisionError{
				collision: ui.RenameCollision{Line: line, Kind: ui.CollisionExists},
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return ui.RenamePlan{}, nil, err
		}

		if len(profiles) == 1 {
			plan.Files = append(plan.Files, line)
		} else {
			plan.Selectors = append(plan.Selectors, line)
		}
		items = append(items, exec.RenameItem{
			From: filepath.Join(repo, filepath.FromSlash(line.From)),
			To:   toAbs,
		})
	}

	return plan, items, nil
}
