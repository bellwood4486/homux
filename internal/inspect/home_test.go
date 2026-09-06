package inspect

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bellwood4486/homux/internal/env"
	"github.com/bellwood4486/homux/internal/resolve"
)

func (f *fixture) all(t *testing.T, in Input) []TargetState {
	t.Helper()
	states, err := All(f.env, in)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	return states
}

// paths は状態の Path を Kind 付きで返す。
func paths(states []TargetState) []string {
	out := make([]string, len(states))
	for i, s := range states {
		out[i] = s.Kind.String() + " " + s.Path()
	}
	return out
}

// 判定表 #11: source を削除したあとに残った managed symlink（spec §9.2 の種類2）。
// desired に無いので HOME 走査でしか見つからない。
func TestAll_OrphanManagedSymlinkIsStale(t *testing.T) {
	f := newFixture(t)
	mkdirAll(t, f.repo(".config"))
	writeFile(t, f.repo(".config/new.conf"), "x")
	// .config/old.conf は repo から削除済み。symlink だけが残っている。
	symlink(t, f.repo(".config/old.conf"), f.home(".config/old.conf"))

	states := f.all(t, Input{
		Resolutions: []resolve.Resolution{selected(".config/new.conf", ".config/new.conf")},
	})

	var orphan *TargetState
	for i := range states {
		if states[i].Path() == ".config/old.conf" {
			orphan = &states[i]
		}
	}
	if orphan == nil {
		t.Fatalf("orphan not found: %v", paths(states))
	}
	if orphan.Kind != KindStale {
		t.Errorf("Kind = %v, want KindStale", orphan.Kind)
	}
	if orphan.Resolution.Selected != nil {
		t.Error("Selected != nil, want nil")
	}
	if orphan.Resolution.Reason != resolve.ReasonAbsent {
		t.Errorf("Reason = %v, want ReasonAbsent", orphan.Resolution.Reason)
	}
	if orphan.Current.Kind != CurrentSymlink || !orphan.Current.Managed {
		t.Errorf("Current = %+v, want managed symlink", orphan.Current)
	}
}

// ADR 0004 / 0014: 起点にならないディレクトリの配下は見に行かない。
// repo に .cache が無く、~/.cache が実ディレクトリなら、その配下は走査対象外。
func TestAll_HomeScanStaysWithinRepoTopLevelEntries(t *testing.T) {
	f := newFixture(t)
	mkdirAll(t, f.repo(".config"))
	symlink(t, f.repo(".cache/blob"), f.home(".cache/blob"))

	states := f.all(t, Input{})

	if len(states) != 0 {
		t.Errorf("states = %v, want empty (~/.cache は走査対象外)", paths(states))
	}
}

// ADR 0004: repo トップレベルの「ファイル」に対応する HOME パスも起点になる。
// source が repo に残ったまま ignore されたときの残骸はこの経路で見つかる。
func TestAll_HomeScanEvaluatesTopLevelFileRoots(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".zshrc"), "x")
	symlink(t, f.repo(".zshrc"), f.home(".zshrc"))

	// .zshrc は ignore されており Resolutions には現れない。
	states := f.all(t, Input{Ignored: []string{".zshrc"}})

	want := []string{"ignored .zshrc", "stale .zshrc"}
	got := paths(states)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("states = %v, want %v", got, want)
	}
}

// ADR 0004 / 0014: 起点自体が symlink なら評価はするが、その先には降りない。
func TestAll_HomeScanDoesNotDescendIntoSymlinkedRoot(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".claude/settings.json"), "x")
	// ~/.claude ごと repo/.claude を指している（homux が張る形ではないが managed）。
	symlink(t, f.repo(".claude"), f.home(".claude"))

	states := f.all(t, Input{})

	want := []string{"stale .claude"}
	if got := paths(states); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("states = %v, want %v", got, want)
	}
}

// BEL-20 / INV-14: repo が走査起点の配下にある配置では、再帰が repo 自身に
// 到達しうる。repo の中には決して降りず、repo 内パスを Target とする
// TargetState を生成しない。
func TestAll_HomeScanDoesNotDescendIntoRepoItself(t *testing.T) {
	home := evalTempDir(t)
	repo := filepath.Join(home, ".config", "dotfiles")
	mkdirAll(t, repo)
	e := env.Env{Home: home, Repo: repo}

	// repo が ~/.config/dotfiles で、repo 内に .config/ghostty/config がある配置。
	// repo のトップレベルに .config があるため、走査起点に ~/.config が含まれ、
	// dir() がそこから再帰すると ~/.config/dotfiles（= repo 自身）に到達する。
	writeFile(t, filepath.Join(repo, ".config", "ghostty", "config"), "x")
	// repo 内に紛れ込んだ symlink。repo 配下を指しているので managed に見えてしまう。
	// ガードが無ければ、これが repo 内パスを Target とする KindStale を生む。
	symlink(t,
		filepath.Join(repo, ".config", "ghostty", "config"),
		filepath.Join(repo, ".config", "ghostty", "stray"),
	)

	states, err := All(e, Input{})
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	for _, s := range states {
		if strings.HasPrefix(s.Path(), ".config/dotfiles") {
			t.Errorf("states に repo 内パスが含まれている: %v", paths(states))
		}
	}
}

// Q7-4: unmanaged なものは HOME 走査の結果に一切現れない。
// これを守らないと ~/.config 配下の無関係なファイルが全部湧く。
func TestAll_HomeScanIgnoresUnmanagedEntries(t *testing.T) {
	f := newFixture(t)
	mkdirAll(t, f.repo(".config"))
	writeFile(t, f.home(".config/other-tool.conf"), "x")
	mkdirAll(t, f.home(".config/nested"))
	writeFile(t, f.home(".config/nested/deep.conf"), "x")
	symlink(t, "/etc/hosts", f.home(".config/hosts"))

	states := f.all(t, Input{})

	if len(states) != 0 {
		t.Errorf("states = %v, want empty", paths(states))
	}
}

// Q4: desired 側で判定済みの target は HOME 走査で二重に出ない。
func TestAll_HomeScanDoesNotDuplicateDesiredTargets(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".config/foo.conf"), "x")
	symlink(t, f.repo(".config/foo.conf"), f.home(".config/foo.conf"))

	states := f.all(t, Input{
		Resolutions: []resolve.Resolution{selected(".config/foo.conf", ".config/foo.conf")},
	})

	want := []string{"linked .config/foo.conf"}
	if got := paths(states); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("states = %v, want %v", got, want)
	}
}

// spec §8.2 の暗黙除外は走査起点にもならない。
func TestAll_HomeScanSkipsGitAndConfigFile(t *testing.T) {
	f := newFixture(t)
	mkdirAll(t, f.repo(".git"))
	writeFile(t, f.repo(".homux.toml"), "profiles = []")
	symlink(t, f.repo(".git/config"), f.home(".git/config"))
	symlink(t, f.repo(".homux.toml"), f.home(".homux.toml"))

	states := f.all(t, Input{})

	if len(states) != 0 {
		t.Errorf("states = %v, want empty", paths(states))
	}
}

// issue の受け入れ条件: HOME を読むだけで、一切変更しない。
func TestAll_DoesNotModifyHome(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".config/foo.conf"), "x")
	writeFile(t, f.repo(".zshrc"), "x")
	writeFile(t, f.home(".zshrc"), "user's own file")
	symlink(t, f.repo(".config/gone.conf"), f.home(".config/gone.conf"))
	mkdirAll(t, f.home(".config/nested"))

	before := snapshot(t, f.env.Home)
	f.all(t, Input{
		Resolutions: []resolve.Resolution{
			selected(".config/foo.conf", ".config/foo.conf"),
			selected(".zshrc", ".zshrc"),
			selected(".missing/x", ".missing/x"),
		},
	})
	after := snapshot(t, f.env.Home)

	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Errorf("HOME changed:\nbefore:\n%s\nafter:\n%s",
			strings.Join(before, "\n"), strings.Join(after, "\n"))
	}
}

// snapshot は root 配下のツリーを、種類とリンク先まで含めて文字列化する。
func snapshot(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		entry := p + " " + fi.Mode().String()
		if fi.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			entry += " -> " + link
		} else if !fi.IsDir() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			entry += " " + string(b)
		}
		out = append(out, entry)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	sort.Strings(out)
	return out
}

// ADR 0014 / 表 2 行目: repo のトップレベルにある「ファイル」を削除すると、
// repo 由来の起点が消える。HOME 直下の symlink を起点に加えることで検出する。
func TestAll_HomeScanDetectsOrphanFromDeletedTopLevelFile(t *testing.T) {
	f := newFixture(t)
	// repo には別のファイルだけが残っている。.zshrc は削除済み。
	writeFile(t, f.repo(".gitconfig"), "x")
	symlink(t, f.repo(".gitconfig"), f.home(".gitconfig"))
	symlink(t, f.repo(".zshrc"), f.home(".zshrc"))

	states := f.all(t, Input{
		Resolutions: []resolve.Resolution{selected(".gitconfig", ".gitconfig")},
	})

	want := []string{"linked .gitconfig", "stale .zshrc"}
	got := paths(states)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("states = %v, want %v", got, want)
	}
}

// ADR 0014 / 表 1 行目: 2 階層目以降のディレクトリ削除は、トップレベルの起点が
// 残るので従来どおり検出できる。回帰させない。
func TestAll_HomeScanDetectsOrphanFromDeletedNestedDir(t *testing.T) {
	f := newFixture(t)
	// repo/.config は残っているが、repo/.config/nvim/ は削除済み。
	writeFile(t, f.repo(".config/ghostty/config"), "x")
	symlink(t, f.repo(".config/ghostty/config"), f.home(".config/ghostty/config"))
	symlink(t, f.repo(".config/nvim/init.lua"), f.home(".config/nvim/init.lua"))

	states := f.all(t, Input{
		Resolutions: []resolve.Resolution{selected(".config/ghostty/config", ".config/ghostty/config")},
	})

	want := []string{"linked .config/ghostty/config", "stale .config/nvim/init.lua"}
	got := paths(states)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("states = %v, want %v", got, want)
	}
}

// ADR 0014 / 表 3 行目: repo のトップレベルにある「ディレクトリ」を削除した場合の
// 残骸は検出できない。これは受容した仕様であり、テストで固定しておく。
// 検出するには HOME 直下の実ディレクトリを全部起点にするしかなく、それは
// ADR 0004 が却下した HOME 全走査そのものになる。
func TestAll_HomeScanDoesNotDetectOrphanFromDeletedTopLevelDir(t *testing.T) {
	f := newFixture(t)
	// repo/.config/ ごと削除済み。~/.config は実ディレクトリなので起点にならない。
	writeFile(t, f.repo(".zshrc"), "x")
	symlink(t, f.repo(".config/ghostty/config"), f.home(".config/ghostty/config"))

	states := f.all(t, Input{
		Resolutions: []resolve.Resolution{selected(".zshrc", ".zshrc")},
	})

	want := []string{"missing .zshrc"}
	if got := paths(states); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("states = %v, want %v (受容した検出漏れ)", got, want)
	}
}

// ADR 0014: 起点が repo 由来と HOME 由来で重複しても、二重に報告しない。
func TestAll_HomeScanDeduplicatesRoots(t *testing.T) {
	f := newFixture(t)
	// repo に .zshrc があるので repo 由来の起点になり、~/.zshrc は symlink なので
	// HOME 由来の起点にもなる。desired には無い（ignore 済み）。
	writeFile(t, f.repo(".zshrc"), "x")
	symlink(t, f.repo(".zshrc"), f.home(".zshrc"))

	states := f.all(t, Input{Ignored: []string{".zshrc"}})

	want := []string{"ignored .zshrc", "stale .zshrc"}
	got := paths(states)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("states = %v, want %v", got, want)
	}
}

// ADR 0014: HOME 直下に repo を指さない symlink があっても拾わない。
func TestAll_HomeScanIgnoresUnmanagedTopLevelSymlinks(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(".zshrc"), "x")
	symlink(t, "/etc/hosts", f.home(".hosts"))
	symlink(t, f.home("Documents/notes"), f.home("notes"))

	states := f.all(t, Input{
		Resolutions: []resolve.Resolution{selected(".zshrc", ".zshrc")},
	})

	want := []string{"missing .zshrc"}
	if got := paths(states); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("states = %v, want %v", got, want)
	}
}
