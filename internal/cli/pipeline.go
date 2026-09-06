// pipeline.go は scan → resolve → inspect → plan の 1 本道を組み立てる。
//
// status / explain / apply / apply --dry-run がすべてこの関数を通ることで、
// コマンドごとに解決ロジックを書き分ける余地を消す（INV-11）。
package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/plan"
	"github.com/bellwood4486/homux/internal/resolve"
	"github.com/bellwood4486/homux/internal/scan"
	"github.com/bellwood4486/homux/internal/ui"
)

// buildPlan は repository を walk して Plan まで組み立てる。
//
// scan.Result も返すのは explain が repo 側の引数を target へ引き直すために
// 必要だからであり、呼び出し側が repository を二度 walk しないためである。
//
// resolve が repository 全体の構造エラー（unknown profile / invalid selector）
// を返した場合は、その場で診断を stderr へ書いて silentExitError を返す
// （spec §10 の整形済み出力に汎用の "Error: ..." を重ねないため）。
func buildPlan(cmd *cobra.Command, flags *globalFlags) (*workspace, *scan.Result, plan.Plan, error) {
	ws, err := loadWorkspace(flags.repo)
	if err != nil {
		return nil, nil, plan.Plan{}, err
	}

	scanned, err := scan.Repository(ws.env.Repo, ws.repo.Ignore)
	if err != nil {
		return nil, nil, plan.Plan{}, err
	}

	resolutions, err := resolve.All(resolve.Input{
		Sources:  scanned.Sources,
		Profiles: ws.repo.Profiles,
		Active:   ws.profile,
	})
	if err != nil {
		fmt.Fprint(cmd.ErrOrStderr(), ui.FormatResolveError(flags.colorErr, err))
		return nil, nil, plan.Plan{}, silentExitError{}
	}

	states, err := inspect.All(ws.env, inspect.Input{Resolutions: resolutions, Ignored: scanned.Ignored})
	if err != nil {
		return nil, nil, plan.Plan{}, err
	}

	return ws, scanned, plan.All(ws.env, plan.Input{States: states, Now: time.Now()}), nil
}
