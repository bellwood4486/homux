package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bellwood4486/homux/internal/inspect"
	"github.com/bellwood4486/homux/internal/plan"
)

// ADR 0015: HOME 優先（共通）。HOME の実体が repo へ move され、symlink が
// 張られる。
func TestApplyReplaceTargetAdoptsHomeFile(t *testing.T) {
	dir := evalTempDir(t)
	source := filepath.Join(dir, "repo", ".claude", "settings.json@@work")
	target := filepath.Join(dir, "home", ".claude", "settings.json")
	writeFile(t, source, "from repo")
	writeFile(t, target, "handwritten")

	res := Apply([]plan.Action{
		{Kind: plan.ReplaceTarget, Target: target, LinkTo: source, Current: inspect.CurrentFile, Confirm: true},
	}, func(a plan.Action) (Decision, error) {
		return Decision{Resolution: ResolutionAdopt, AdoptPath: source}, nil
	})

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if got := readFile(t, source); got != "handwritten" {
		t.Errorf("repo content = %q, want %q", got, "handwritten")
	}
	if got := readLink(t, target); got != source {
		t.Errorf("link = %s, want %s", got, source)
	}
}

// ADR 0015: 取り込み先に同名の source が既に存在しても、退避せず上書きする。
func TestApplyReplaceTargetAdoptOverwritesExistingSource(t *testing.T) {
	dir := evalTempDir(t)
	repoPath := filepath.Join(dir, "repo", ".vimrc@@work")
	target := filepath.Join(dir, "home", ".vimrc")
	writeFile(t, repoPath, "old work vimrc")
	writeFile(t, target, "new handwritten vimrc")

	res := Apply([]plan.Action{
		{Kind: plan.ReplaceTarget, Target: target, LinkTo: repoPath, Current: inspect.CurrentFile, Confirm: true},
	}, func(a plan.Action) (Decision, error) {
		return Decision{Resolution: ResolutionAdopt, AdoptPath: repoPath}, nil
	})

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if got := readFile(t, repoPath); got != "new handwritten vimrc" {
		t.Errorf("repo content = %q, want overwritten", got)
	}
	if _, err := os.Lstat(target + ".homux-bak.20260905-153000"); !os.IsNotExist(err) {
		t.Errorf("a backup was created for an adopt, want none")
	}
}

// ADR 0015: ディレクトリは中身ごと move する。
func TestApplyReplaceTargetAdoptsHomeDirectory(t *testing.T) {
	dir := evalTempDir(t)
	repoPath := filepath.Join(dir, "repo", ".config", "foo@@work")
	target := filepath.Join(dir, "home", ".config", "foo")
	writeFile(t, filepath.Join(target, "config"), "handwritten")
	writeFile(t, filepath.Join(target, "nested", "extra"), "nested file")

	res := Apply([]plan.Action{
		{Kind: plan.ReplaceTarget, Target: target, LinkTo: repoPath, Current: inspect.CurrentDir, Confirm: true},
	}, func(a plan.Action) (Decision, error) {
		return Decision{Resolution: ResolutionAdopt, AdoptPath: repoPath}, nil
	})

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if got := readFile(t, filepath.Join(repoPath, "config")); got != "handwritten" {
		t.Errorf("repo config content = %q, want %q", got, "handwritten")
	}
	if got := readFile(t, filepath.Join(repoPath, "nested", "extra")); got != "nested file" {
		t.Errorf("repo nested content = %q, want %q", got, "nested file")
	}
	if got := readLink(t, target); got != repoPath {
		t.Errorf("link = %s, want %s", got, repoPath)
	}
}

// ADR 0015: 対象が repo 外を指す symlink のときは adopt できない
// （add の spec §12.6 と同じ安全基準）。
func TestApplyReplaceTargetAdoptRefusesSymlinkTarget(t *testing.T) {
	dir := evalTempDir(t)
	repoPath := filepath.Join(dir, "repo", ".vimrc@@work")
	target := filepath.Join(dir, "home", ".vimrc")
	elsewhere := filepath.Join(dir, "elsewhere", ".vimrc")
	writeFile(t, elsewhere, "elsewhere content")
	symlink(t, elsewhere, target)

	res := Apply([]plan.Action{
		{Kind: plan.ReplaceTarget, Target: target, LinkTo: repoPath, Current: inspect.CurrentSymlink, From: elsewhere, Confirm: true},
	}, func(a plan.Action) (Decision, error) {
		return Decision{Resolution: ResolutionAdopt, AdoptPath: repoPath}, nil
	})

	if res.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if got := readLink(t, target); got != elsewhere {
		t.Errorf("target link = %s, want unchanged %s", got, elsewhere)
	}
	if _, err := os.Lstat(repoPath); !os.IsNotExist(err) {
		t.Errorf("repoPath was created despite the refusal: %v", err)
	}
}
