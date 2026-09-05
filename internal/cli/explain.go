// explain.go は homux explain（spec §12.3）のコマンド定義である。
package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bellwood4486/homux/internal/env"
	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/plan"
	"github.com/bellwood4486/homux/internal/scan"
	"github.com/bellwood4486/homux/internal/ui"
)

// newExplainCmd は homux explain を構築する。HOME・repo・local config の
// いずれも変更しない読み取り専用コマンドである（spec §11.2）。
func newExplainCmd(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain <path>",
		Short: "1 ファイルについて、なぜその状態になったのかを説明する",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExplain(cmd, flags.repo, args[0])
		},
	}
	return cmd
}

// runExplain は scan → resolve → inspect → plan のパイプラインを status
// （runStatus）と同一の順序で一度だけ実行し、引数が指す 1 target を抜き出して
// ui.RenderExplain に渡す。status と同じ resolver / planner を使うことで
// INV-11 を守る（解決ロジックを explain のために書き直さない）。
func runExplain(cmd *cobra.Command, repoFlag, argPath string) error {
	ws, scanned, p, err := buildPlan(cmd, repoFlag)
	if err != nil {
		return err
	}

	target, err := explainTarget(ws.env, scanned, argPath)
	if err != nil {
		return err
	}

	state, action, err := findExplainState(ws.env, p, target)
	if err != nil {
		return err
	}

	ui.RenderExplain(cmd.OutOrStdout(), ws.env.Home, ws.profile, state, action)

	if state.Kind == inspect.KindError {
		return silentExitError{}
	}
	return nil
}

// explainTarget は spec §12.3 の引数解釈を行う。引数を絶対パス化し、repo
// ルート配下なら source として repo 相対パスから対応する target を引き、
// $HOME 配下なら target としてそのまま HOME 相対パスにする（repo が $HOME
// 配下にある場合は repo 判定を優先する）。どちらでもない場合はエラーにする。
func explainTarget(e env.Env, scanned *scan.Result, argPath string) (string, error) {
	expanded, err := expandTilde(argPath)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	// loadWorkspace が e.Repo を filepath.EvalSymlinks 済みにする一方、ユーザーが
	// 打つ引数はそうとは限らない（macOS の /tmp -> /private/tmp など）。比較の
	// 基準を揃えるため、比較用にだけ両辺を正規化する（canonicalize は
	// resolveLink と同じ理由で存在しないパスも許容する）。
	canonAbs := canonicalize(abs)
	canonHome := canonicalize(e.Home)

	switch {
	case within(e.Repo, canonAbs):
		rel := filepath.ToSlash(mustRel(e.Repo, canonAbs))
		for _, s := range scanned.Sources {
			if s.RepoPath == rel {
				return s.Target, nil
			}
		}
		for _, ignored := range scanned.Ignored {
			if ignored == rel {
				return "", fmt.Errorf("%s is ignored by .homux.toml and has no target", rel)
			}
		}
		return "", fmt.Errorf("%s: not a managed source in the repository", rel)

	case within(canonHome, canonAbs):
		return filepath.ToSlash(mustRel(canonHome, canonAbs)), nil

	default:
		return "", newUsageError(fmt.Errorf("%s is neither inside HOME (%s) nor the repository (%s)", argPath, e.Home, e.Repo))
	}
}

// canonicalize は abs の経路上の symlink をできる限り解決する。abs 自体が
// 存在しなくても、存在する最も近い祖先まで遡って解決し、残りを結合し直す
// （internal/inspect/link.go の canonicalize と同じ考え方）。何も解決できなければ
// abs をそのまま返す。
func canonicalize(abs string) string {
	path := abs
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved
		}
		parent := filepath.Dir(path)
		if parent == path {
			return abs
		}
		suffix = append(suffix, filepath.Base(path))
		path = parent
	}
}

// findExplainState は target に対応する 1 件の TargetState と、（あれば）
// それに対応する Action を p から探す。plan.All が既に組み立てた結果を
// そのまま引くだけであり、状態や操作の再判定は行わない（INV-11）。
func findExplainState(e env.Env, p plan.Plan, target string) (inspect.TargetState, *plan.Action, error) {
	absTarget := filepath.Join(e.Home, filepath.FromSlash(target))
	for i := range p.States {
		s := p.States[i]
		if s.Kind == inspect.KindIgnored || s.Resolution.Target != target {
			continue
		}
		for j := range p.Actions {
			if p.Actions[j].Target == absTarget {
				return s, &p.Actions[j], nil
			}
		}
		return s, nil, nil
	}
	return inspect.TargetState{}, nil, fmt.Errorf("~/%s: no source in the repository for this target", target)
}

// within は path が base 自身または base 配下かを返す（plan.withinRepo と同じ判定）。
func within(base, path string) bool {
	return path == base || strings.HasPrefix(path, base+string(filepath.Separator))
}

// mustRel は within で確認済みの base/path について相対パスを返す。
func mustRel(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		// within で base 配下であることを確認済みのため到達しない。
		panic(err)
	}
	return rel
}
