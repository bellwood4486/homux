package plan

import (
	"errors"
	"testing"

	"github.com/bellwood4486/homux/internal/inspect"
)

func TestAll_MissingCreatesSymlinkWithoutConfirmation(t *testing.T) {
	got := planFor(inspect.TargetState{
		Resolution: selected(".zshrc", ".zshrc"),
		Kind:       inspect.KindMissing,
		Current:    inspect.Current{Kind: inspect.CurrentAbsent},
	})

	if len(got.Actions) != 1 {
		t.Fatalf("len(Actions) = %d, want 1", len(got.Actions))
	}
	a := got.Actions[0]
	if a.Kind != CreateSymlink {
		t.Errorf("Kind = %v, want CreateSymlink", a.Kind)
	}
	if want := homePath(t, ".zshrc"); a.Target != want {
		t.Errorf("Target = %q, want %q", a.Target, want)
	}
	if want := repoPath(t, ".zshrc"); a.LinkTo != want {
		t.Errorf("LinkTo = %q, want %q", a.LinkTo, want)
	}
	if a.Confirm {
		t.Error("Confirm = true, want false (spec §12.4: Missing は確認なし)")
	}
	if a.Backup != "" {
		t.Errorf("Backup = %q, want empty", a.Backup)
	}
}

func TestAll_OccupiedReplacesTargetWithBackup(t *testing.T) {
	got := planFor(inspect.TargetState{
		Resolution: selected(".claude/settings.json", ".claude/settings.json@@work"),
		Kind:       inspect.KindOccupied,
		Current:    inspect.Current{Kind: inspect.CurrentFile},
	})

	if len(got.Actions) != 1 {
		t.Fatalf("len(Actions) = %d, want 1", len(got.Actions))
	}
	a := got.Actions[0]
	if a.Kind != ReplaceTarget {
		t.Errorf("Kind = %v, want ReplaceTarget", a.Kind)
	}
	if want := homePath(t, ".claude/settings.json"); a.Target != want {
		t.Errorf("Target = %q, want %q", a.Target, want)
	}
	if want := repoPath(t, ".claude/settings.json@@work"); a.LinkTo != want {
		t.Errorf("LinkTo = %q, want %q", a.LinkTo, want)
	}
	// spec §12.4: 同一ディレクトリに <name>.homux-bak.<timestamp> として退避する。
	if want := homePath(t, ".claude/settings.json.homux-bak.20260905-153000"); a.Backup != want {
		t.Errorf("Backup = %q, want %q", a.Backup, want)
	}
	if !a.Confirm {
		t.Error("Confirm = false, want true (spec §12.4: Occupied は確認のうえ退避)")
	}
}

// spec §9.2 種類1: desired は存在するが、リンク先が変わった。
func TestAll_StaleWithSelectedRelinks(t *testing.T) {
	got := planFor(inspect.TargetState{
		Resolution: selected(".gitconfig", ".gitconfig@@work"),
		Kind:       inspect.KindStale,
		Current: inspect.Current{
			Kind:    inspect.CurrentSymlink,
			Link:    repoPath(t, ".gitconfig"),
			LinkAbs: repoPath(t, ".gitconfig"),
			Managed: true,
		},
	})

	if len(got.Actions) != 1 {
		t.Fatalf("len(Actions) = %d, want 1", len(got.Actions))
	}
	a := got.Actions[0]
	if a.Kind != Relink {
		t.Errorf("Kind = %v, want Relink", a.Kind)
	}
	if want := repoPath(t, ".gitconfig@@work"); a.LinkTo != want {
		t.Errorf("LinkTo = %q, want %q", a.LinkTo, want)
	}
	if !a.Confirm {
		t.Error("Confirm = false, want true (spec §12.4: Stale 種類1 は確認のうえ relink)")
	}
	// INV-13 は unmanaged な HOME ファイルの保護である。managed symlink の
	// 張り替えに退避は不要。
	if a.Backup != "" {
		t.Errorf("Backup = %q, want empty", a.Backup)
	}
}

// spec §9.2 種類2: desired から消えたのに managed symlink が残っている。
func TestAll_StaleWithoutSelectedRemovesSymlink(t *testing.T) {
	got := planFor(inspect.TargetState{
		Resolution: absent(".config/old/config"),
		Kind:       inspect.KindStale,
		Current: inspect.Current{
			Kind:    inspect.CurrentSymlink,
			Link:    repoPath(t, ".config/old/config"),
			LinkAbs: repoPath(t, ".config/old/config"),
			Managed: true,
		},
	})

	if len(got.Actions) != 1 {
		t.Fatalf("len(Actions) = %d, want 1", len(got.Actions))
	}
	a := got.Actions[0]
	if a.Kind != RemoveStaleSymlink {
		t.Errorf("Kind = %v, want RemoveStaleSymlink", a.Kind)
	}
	if want := homePath(t, ".config/old/config"); a.Target != want {
		t.Errorf("Target = %q, want %q", a.Target, want)
	}
	// LinkTo は「これから張る先」であり、削除には存在しない。今指している先を
	// 入れると Kind によって意味が変わってしまうため空にする。
	if a.LinkTo != "" {
		t.Errorf("LinkTo = %q, want empty", a.LinkTo)
	}
	if !a.Confirm {
		t.Error("Confirm = false, want true (spec §12.4: Stale 種類2 は確認のうえ削除)")
	}
}

// 受け入れ条件: 適用済みの状態から生成される Action は空である。
func TestAll_SettledStatesProduceNoActions(t *testing.T) {
	got := planFor(
		inspect.TargetState{
			Resolution: selected(".zshrc", ".zshrc"),
			Kind:       inspect.KindLinked,
			Current: inspect.Current{
				Kind: inspect.CurrentSymlink, LinkAbs: repoPath(t, ".zshrc"), Managed: true,
			},
		},
		inspect.TargetState{
			Resolution: absent(".gitconfig"),
			Kind:       inspect.KindInactive,
			Current:    inspect.Current{Kind: inspect.CurrentAbsent},
		},
		inspect.TargetState{RepoPath: "README.md", Kind: inspect.KindIgnored},
	)

	if len(got.Actions) != 0 {
		t.Errorf("Actions = %+v, want none", got.Actions)
	}
	if len(got.States) != 3 {
		t.Errorf("len(States) = %d, want 3 (status は全 state を表示する)", len(got.States))
	}
}

// spec §12.4: Error はスキップして続行し、最後に件数を表示して exit 1 を返す。
// 件数の集計を cli/status と cli/apply が別々に書かないよう Plan が持つ（INV-11）。
func TestPlan_ErrorsCountsSkippedTargets(t *testing.T) {
	got := planFor(
		inspect.TargetState{
			Resolution: selected(".zshrc", ".zshrc"),
			Kind:       inspect.KindMissing,
		},
		inspect.TargetState{
			Resolution: absent(".gitconfig"),
			Kind:       inspect.KindError,
			Err:        &inspect.AncestorNotDirError{Path: homePath(t, ".config")},
		},
		inspect.TargetState{
			Resolution: absent(".vimrc"),
			Kind:       inspect.KindError,
		},
	)

	if got.Errors() != 2 {
		t.Errorf("Errors() = %d, want 2", got.Errors())
	}
	if len(got.Actions) != 1 {
		t.Errorf("len(Actions) = %d, want 1 (Error はスキップし残りを続行する)", len(got.Actions))
	}
}

// INV-14: repo 配下のパスに対する Action を生成してはならない。
// repo が HOME 配下にあると inspect の HOME 走査が repo 内に降りうるため、
// plan が最後の関門として構造的に防ぐ。
func TestAll_TargetInsideRepoNeverProducesAction(t *testing.T) {
	// testEnv の Repo は Home 配下（~/dotfiles）である。
	inside := "dotfiles/.config/foo/config"

	got := planFor(inspect.TargetState{
		Resolution: absent(inside),
		Kind:       inspect.KindStale,
		Current: inspect.Current{
			Kind: inspect.CurrentSymlink, Managed: true,
			LinkAbs: repoPath(t, ".config/foo/config"),
		},
	})

	if len(got.Actions) != 0 {
		t.Fatalf("Actions = %+v, want none (INV-14)", got.Actions)
	}
	if len(got.States) != 1 {
		t.Fatalf("len(States) = %d, want 1", len(got.States))
	}
	if got.States[0].Kind != inspect.KindError {
		t.Errorf("States[0].Kind = %v, want KindError", got.States[0].Kind)
	}
	var repoErr *RepoTargetError
	if !errors.As(got.States[0].Err, &repoErr) {
		t.Fatalf("States[0].Err = %v, want *RepoTargetError", got.States[0].Err)
	}
	if want := homePath(t, inside); repoErr.Target != want {
		t.Errorf("RepoTargetError.Target = %q, want %q", repoErr.Target, want)
	}
	if got.Errors() != 1 {
		t.Errorf("Errors() = %d, want 1", got.Errors())
	}
}

// inspect は managed symlink のときしか Selected 無しの Stale を立てない。
// この契約が破れたまま Action を作ると symlink 以外を削除しかねない（INV-14）。
func TestAll_StaleThatIsNotAManagedSymlinkPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("panic しなかった。inspect との契約違反を握り潰している")
		}
	}()

	planFor(inspect.TargetState{
		Resolution: absent(".config/old/config"),
		Kind:       inspect.KindStale,
		Current:    inspect.Current{Kind: inspect.CurrentFile},
	})
}

// INV-13: 退避なしの置換 Action を生成しない。Occupied は File / Dir /
// repo 外を指す symlink の 3 通りがあり、いずれも退避を伴う。
func TestAll_EveryReplaceTargetCarriesABackup(t *testing.T) {
	occupiers := map[string]inspect.Current{
		"regular file": {Kind: inspect.CurrentFile},
		"directory":    {Kind: inspect.CurrentDir},
		"symlink outside the repository": {
			Kind: inspect.CurrentSymlink, Link: "/elsewhere/x", LinkAbs: "/elsewhere/x",
		},
	}

	for name, current := range occupiers {
		t.Run(name, func(t *testing.T) {
			got := planFor(inspect.TargetState{
				Resolution: selected(".zshrc", ".zshrc"),
				Kind:       inspect.KindOccupied,
				Current:    current,
			})

			if len(got.Actions) != 1 {
				t.Fatalf("len(Actions) = %d, want 1", len(got.Actions))
			}
			a := got.Actions[0]
			if a.Kind != ReplaceTarget {
				t.Fatalf("Kind = %v, want ReplaceTarget", a.Kind)
			}
			if a.Backup == "" {
				t.Error("Backup が空である（INV-13）")
			}
		})
	}
}

// exec の部分適用の報告（spec §12.4）は Action の並びに依存する。
func TestAll_PreservesStateOrder(t *testing.T) {
	got := planFor(
		inspect.TargetState{Resolution: selected(".a", ".a"), Kind: inspect.KindMissing},
		inspect.TargetState{Resolution: selected(".b", ".b"), Kind: inspect.KindLinked},
		inspect.TargetState{Resolution: selected(".c", ".c@@work"), Kind: inspect.KindOccupied},
	)

	want := []string{homePath(t, ".a"), homePath(t, ".c")}
	if len(got.Actions) != len(want) {
		t.Fatalf("len(Actions) = %d, want %d", len(got.Actions), len(want))
	}
	for i, w := range want {
		if got.Actions[i].Target != w {
			t.Errorf("Actions[%d].Target = %q, want %q", i, got.Actions[i].Target, w)
		}
	}
}

func TestAll_DoesNotMutateInputStates(t *testing.T) {
	in := Input{
		States: []inspect.TargetState{{
			Resolution: absent("dotfiles/.config/foo/config"), // repo 配下（INV-14）
			Kind:       inspect.KindStale,
			Current:    inspect.Current{Kind: inspect.CurrentSymlink, Managed: true},
		}},
		Now: now(),
	}

	All(testEnv(), in)

	if in.States[0].Kind != inspect.KindStale {
		t.Errorf("入力の Kind が %v に書き換えられた", in.States[0].Kind)
	}
	if in.States[0].Err != nil {
		t.Errorf("入力の Err が %v に書き換えられた", in.States[0].Err)
	}
}

// docs/design.md §3: Relink は「どこから どこへ」を ui が出せるよう From を持つ。
func TestAll_RelinkCarriesCurrentLinkTarget(t *testing.T) {
	got := planFor(inspect.TargetState{
		Resolution: selected(".vimrc", ".vimrc@@work"),
		Kind:       inspect.KindStale,
		Current: inspect.Current{
			Kind:    inspect.CurrentSymlink,
			Link:    repoPath(t, ".vimrc"),
			LinkAbs: repoPath(t, ".vimrc"),
			Managed: true,
		},
	})

	a := got.Actions[0]
	if want := repoPath(t, ".vimrc"); a.From != want {
		t.Errorf("From = %q, want %q", a.From, want)
	}
	if a.Current != inspect.CurrentSymlink {
		t.Errorf("Current = %v, want CurrentSymlink", a.Current)
	}
}

// spec §12.4: repo 外を指す symlink の退避は、現在のリンク先を示して確認する。
func TestAll_OccupiedSymlinkCarriesCurrentLinkTarget(t *testing.T) {
	got := planFor(inspect.TargetState{
		Resolution: selected(".vimrc", ".vimrc@@work"),
		Kind:       inspect.KindOccupied,
		Current: inspect.Current{
			Kind:    inspect.CurrentSymlink,
			Link:    "/opt/elsewhere/.vimrc",
			LinkAbs: "/opt/elsewhere/.vimrc",
		},
	})

	a := got.Actions[0]
	if want := "/opt/elsewhere/.vimrc"; a.From != want {
		t.Errorf("From = %q, want %q", a.From, want)
	}
	if a.Current != inspect.CurrentSymlink {
		t.Errorf("Current = %v, want CurrentSymlink", a.Current)
	}
}

// symlink 以外の退避には「現在のリンク先」が無い（docs/design.md §3）。
func TestAll_OccupiedDirHasNoFrom(t *testing.T) {
	got := planFor(inspect.TargetState{
		Resolution: selected(".claude", ".claude@@work"),
		Kind:       inspect.KindOccupied,
		Current:    inspect.Current{Kind: inspect.CurrentDir},
	})

	a := got.Actions[0]
	if a.From != "" {
		t.Errorf("From = %q, want empty", a.From)
	}
	if a.Current != inspect.CurrentDir {
		t.Errorf("Current = %v, want CurrentDir", a.Current)
	}
}

// CreateSymlink / RemoveStaleSymlink も Current を持つ（ui の注記の判断材料）。
func TestAll_CurrentKindIsCarriedForEveryAction(t *testing.T) {
	got := planFor(
		inspect.TargetState{
			Resolution: selected(".zshrc", ".zshrc"),
			Kind:       inspect.KindMissing,
			Current:    inspect.Current{Kind: inspect.CurrentAbsent},
		},
		inspect.TargetState{
			Resolution: absent(".config/old/config"),
			Kind:       inspect.KindStale,
			Current: inspect.Current{
				Kind:    inspect.CurrentSymlink,
				Link:    repoPath(t, ".config/old/config"),
				LinkAbs: repoPath(t, ".config/old/config"),
				Managed: true,
			},
		},
	)

	if got.Actions[0].Current != inspect.CurrentAbsent {
		t.Errorf("CreateSymlink Current = %v, want CurrentAbsent", got.Actions[0].Current)
	}
	if got.Actions[0].From != "" {
		t.Errorf("CreateSymlink From = %q, want empty", got.Actions[0].From)
	}
	if got.Actions[1].Current != inspect.CurrentSymlink {
		t.Errorf("RemoveStaleSymlink Current = %v, want CurrentSymlink", got.Actions[1].Current)
	}
	// 削除は「今どこを指しているか」を操作の理由にしない（docs/design.md §3）。
	if got.Actions[1].From != "" {
		t.Errorf("RemoveStaleSymlink From = %q, want empty", got.Actions[1].From)
	}
}
