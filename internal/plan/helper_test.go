package plan

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/bellwood4486/homux/internal/env"
	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/resolve"
	"github.com/bellwood4486/homux/internal/scan"
)

// testEnv は plan がファイルシステムに触れないことを利用し、実在しない
// 絶対パスを使う。
func testEnv() env.Env {
	return env.Env{
		Home: filepath.Join(string(filepath.Separator), "home", "u"),
		Repo: filepath.Join(string(filepath.Separator), "home", "u", "dotfiles"),
	}
}

func homePath(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join(testEnv().Home, filepath.FromSlash(rel))
}

func repoPath(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join(testEnv().Repo, filepath.FromSlash(rel))
}

// selected は source が選択された Resolution を組み立てる。
func selected(target, repoRel string) resolve.Resolution {
	s := scan.Source{RepoPath: repoRel, Target: target}
	return resolve.Resolution{
		Target:     target,
		Candidates: []scan.Source{s},
		Selected:   &s,
		Reason:     resolve.ReasonCommonFallback,
	}
}

// absent は選択できる source が無かった Resolution を組み立てる。
func absent(target string) resolve.Resolution {
	return resolve.Resolution{Target: target, Reason: resolve.ReasonAbsent}
}

// now はテストで使う固定時刻。spec §12.4 の例と同じ値である。
func now() time.Time {
	return time.Date(2026, 9, 5, 15, 30, 0, 0, time.UTC)
}

func planFor(states ...inspect.TargetState) Plan {
	return All(testEnv(), Input{States: states, Now: now()})
}
