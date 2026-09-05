package inspect

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/bellwood4486/homux/internal/env"
	"github.com/bellwood4486/homux/internal/resolve"
	"github.com/bellwood4486/homux/internal/scan"
)

// selected は Selected が決まった Resolution を組み立てる。
func selected(target, repoPath string) resolve.Resolution {
	s := scan.Source{RepoPath: repoPath, Target: target}
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

// errored は構造エラーを抱えた Resolution を組み立てる。
func errored(target string) resolve.Resolution {
	return resolve.Resolution{
		Target: target,
		Err:    &resolve.AmbiguousError{Target: target, Matching: []string{"a@@work", "a@@work+personal"}},
		Reason: resolve.ReasonAmbiguous,
	}
}

// fixture は repo / home を用意し、All を呼ぶまでを引き受ける。
type fixture struct {
	env  env.Env
	base string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	base := evalTempDir(t)
	f := &fixture{
		base: base,
		env:  env.Env{Home: filepath.Join(base, "home"), Repo: filepath.Join(base, "dotfiles")},
	}
	mkdirAll(t, f.env.Home, f.env.Repo)
	return f
}

func (f *fixture) home(rel string) string { return filepath.Join(f.env.Home, rel) }
func (f *fixture) repo(rel string) string { return filepath.Join(f.env.Repo, rel) }

// one は All を呼び、target に対応する 1 件を返す。
func (f *fixture) one(t *testing.T, in Input, target string) TargetState {
	t.Helper()
	states, err := All(f.env, in)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	for _, s := range states {
		if s.Resolution.Target == target {
			return s
		}
	}
	t.Fatalf("target %q not found in %d states", target, len(states))
	return TargetState{}
}

// 判定表 #1
func TestAll_ResolutionErrorIsError(t *testing.T) {
	f := newFixture(t)
	got := f.one(t, Input{Resolutions: []resolve.Resolution{errored(".zshrc")}}, ".zshrc")

	if got.Kind != KindError {
		t.Errorf("Kind = %v, want KindError", got.Kind)
	}
}

// 判定表 #1: エラーでも HOME の実状態は読む（explain の Current: 表示のため）。
func TestAll_ResolutionErrorStillReadsCurrent(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".zshrc"), "x")
	symlink(t, f.repo(".zshrc"), f.home(".zshrc"))

	got := f.one(t, Input{Resolutions: []resolve.Resolution{errored(".zshrc")}}, ".zshrc")

	if got.Kind != KindError {
		t.Fatalf("Kind = %v, want KindError", got.Kind)
	}
	if got.Current.Kind != CurrentSymlink {
		t.Errorf("Current.Kind = %v, want CurrentSymlink", got.Current.Kind)
	}
	if got.Current.LinkAbs != f.repo(".zshrc") {
		t.Errorf("Current.LinkAbs = %q, want %q", got.Current.LinkAbs, f.repo(".zshrc"))
	}
}

// 判定表 #2
func TestAll_AncestorNotDirectoryIsErrorNotOccupied(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".config/foo/config"), "x")
	writeFile(t, f.home(".config"), "not a directory")

	got := f.one(t, Input{
		Resolutions: []resolve.Resolution{selected(".config/foo/config", ".config/foo/config")},
	}, ".config/foo/config")

	if got.Kind != KindError {
		t.Fatalf("Kind = %v, want KindError", got.Kind)
	}
	var notDir *AncestorNotDirError
	if !errors.As(got.Err, &notDir) {
		t.Errorf("Err = %v, want *AncestorNotDirError", got.Err)
	}
}

// 判定表 #4
func TestAll_Missing(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".zshrc"), "x")

	got := f.one(t, Input{Resolutions: []resolve.Resolution{selected(".zshrc", ".zshrc")}}, ".zshrc")

	if got.Kind != KindMissing {
		t.Errorf("Kind = %v, want KindMissing", got.Kind)
	}
}

// 判定表 #5
func TestAll_Linked(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".claude/settings.json@@work"), "x")
	symlink(t, f.repo(".claude/settings.json@@work"), f.home(".claude/settings.json"))

	got := f.one(t, Input{
		Resolutions: []resolve.Resolution{selected(".claude/settings.json", ".claude/settings.json@@work")},
	}, ".claude/settings.json")

	if got.Kind != KindLinked {
		t.Errorf("Kind = %v, want KindLinked", got.Kind)
	}
}

// 判定表 #5: 相対リンクでも Linked と判定する。
func TestAll_LinkedViaRelativeSymlink(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".zshrc"), "x")
	symlink(t, "../dotfiles/.zshrc", f.home(".zshrc"))

	got := f.one(t, Input{Resolutions: []resolve.Resolution{selected(".zshrc", ".zshrc")}}, ".zshrc")

	if got.Kind != KindLinked {
		t.Errorf("Kind = %v, want KindLinked", got.Kind)
	}
}

// 判定表 #6: profile 切替や rename で、別の source を指したまま残った symlink。
func TestAll_StaleWhenLinkedToAnotherSourceInRepo(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".claude/settings.json@@work"), "x")
	symlink(t, f.repo(".claude/settings.json@@personal"), f.home(".claude/settings.json"))

	got := f.one(t, Input{
		Resolutions: []resolve.Resolution{selected(".claude/settings.json", ".claude/settings.json@@work")},
	}, ".claude/settings.json")

	if got.Kind != KindStale {
		t.Fatalf("Kind = %v, want KindStale", got.Kind)
	}
	if got.Resolution.Selected == nil {
		t.Error("Selected = nil, want non-nil (plan が Relink と Remove を振り分ける材料)")
	}
}

// 判定表 #6: リンク先が dangling でも repo 配下なら managed であり Stale になる
// （spec §9.1、design.md §7.1 の必須項目）。
func TestAll_StaleWhenDanglingLinkPointsIntoRepo(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".zshrc"), "x")
	symlink(t, f.repo(".zshrc@@personal"), f.home(".zshrc")) // リンク先は存在しない

	got := f.one(t, Input{Resolutions: []resolve.Resolution{selected(".zshrc", ".zshrc")}}, ".zshrc")

	if got.Kind != KindStale {
		t.Errorf("Kind = %v, want KindStale", got.Kind)
	}
}

// 判定表 #7
func TestAll_OccupiedWhenSymlinkPointsOutsideRepo(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".zshrc"), "x")
	writeFile(t, filepath.Join(f.base, "elsewhere", ".zshrc"), "x")
	symlink(t, filepath.Join(f.base, "elsewhere", ".zshrc"), f.home(".zshrc"))

	got := f.one(t, Input{Resolutions: []resolve.Resolution{selected(".zshrc", ".zshrc")}}, ".zshrc")

	if got.Kind != KindOccupied {
		t.Errorf("Kind = %v, want KindOccupied", got.Kind)
	}
}

// 判定表 #8
func TestAll_OccupiedByRegularFile(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".zshrc"), "x")
	writeFile(t, f.home(".zshrc"), "user's own file")

	got := f.one(t, Input{Resolutions: []resolve.Resolution{selected(".zshrc", ".zshrc")}}, ".zshrc")

	if got.Kind != KindOccupied {
		t.Fatalf("Kind = %v, want KindOccupied", got.Kind)
	}
	if got.Current.Kind != CurrentFile {
		t.Errorf("Current.Kind = %v, want CurrentFile", got.Current.Kind)
	}
}

// 判定表 #8: ディレクトリでも Occupied。StateKind は増やさず Current.Kind で区別する。
func TestAll_OccupiedByDirectory(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".claude/settings.json"), "x")
	mkdirAll(t, f.home(".claude/settings.json"))

	got := f.one(t, Input{
		Resolutions: []resolve.Resolution{selected(".claude/settings.json", ".claude/settings.json")},
	}, ".claude/settings.json")

	if got.Kind != KindOccupied {
		t.Fatalf("Kind = %v, want KindOccupied", got.Kind)
	}
	if got.Current.Kind != CurrentDir {
		t.Errorf("Current.Kind = %v, want CurrentDir", got.Current.Kind)
	}
}

// 判定表 #9: profile 切替で desired から消えたが managed symlink が残っている。
// Inactive ではなく Stale でなければ apply が古いリンクを消せない。
func TestAll_StaleWhenAbsentButManagedSymlinkRemains(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo("foo@@work"), "x")
	symlink(t, f.repo("foo@@work"), f.home("foo"))

	got := f.one(t, Input{Resolutions: []resolve.Resolution{absent("foo")}}, "foo")

	if got.Kind != KindStale {
		t.Fatalf("Kind = %v, want KindStale", got.Kind)
	}
	if got.Resolution.Selected != nil {
		t.Error("Selected != nil, want nil (plan は削除を選ぶ)")
	}
}

// 判定表 #10
func TestAll_InactiveWhenAbsentAndNothingInHome(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo("foo@@work"), "x")

	got := f.one(t, Input{Resolutions: []resolve.Resolution{absent("foo")}}, "foo")

	if got.Kind != KindInactive {
		t.Errorf("Kind = %v, want KindInactive", got.Kind)
	}
}

// 判定表 #10: unmanaged なファイルが居座っていても、desired が無いなら Inactive。
// homux が触るべきものではない。
func TestAll_InactiveWhenAbsentAndUnmanagedFileExists(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo("foo@@work"), "x")
	writeFile(t, f.home("foo"), "user's own file")

	got := f.one(t, Input{Resolutions: []resolve.Resolution{absent("foo")}}, "foo")

	if got.Kind != KindInactive {
		t.Errorf("Kind = %v, want KindInactive", got.Kind)
	}
}

// 判定表 #12
func TestAll_Ignored(t *testing.T) {
	f := newFixture(t)

	states, err := All(f.env, Input{Ignored: []string{".config/secret.token"}})
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("len(states) = %d, want 1", len(states))
	}
	if states[0].Kind != KindIgnored {
		t.Errorf("Kind = %v, want KindIgnored", states[0].Kind)
	}
	if states[0].RepoPath != ".config/secret.token" {
		t.Errorf("RepoPath = %q, want %q", states[0].RepoPath, ".config/secret.token")
	}
	if states[0].Resolution.Target != "" {
		t.Errorf("Resolution = %+v, want zero value", states[0].Resolution)
	}
}

// 並び順は Target（Ignored は RepoPath）の辞書昇順で安定する。
// 表示順は ui の関心事であり、inspect は決定論だけを保証する。
func TestAll_IsSortedByPath(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".zshrc"), "x")
	writeFile(t, f.repo(".gitconfig"), "x")

	states, err := All(f.env, Input{
		Resolutions: []resolve.Resolution{selected(".zshrc", ".zshrc"), selected(".gitconfig", ".gitconfig")},
		Ignored:     []string{".aws/credentials"},
	})
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	want := []string{".aws/credentials", ".gitconfig", ".zshrc"}
	got := make([]string, len(states))
	for i, s := range states {
		got[i] = s.Path()
	}
	if len(got) != len(want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("paths = %v, want %v", got, want)
		}
	}
}
