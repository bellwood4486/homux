package inspect

import (
	"path/filepath"
	"testing"
)

// TestResolveLink_AbsoluteLinkIntoRepoIsManaged は、repo 配下を指す絶対 symlink が
// managed と判定されることを確認する（spec §9.1、ADR 0003）。
func TestResolveLink_AbsoluteLinkIntoRepoIsManaged(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	target := filepath.Join(home, ".zshrc")
	link := filepath.Join(repo, ".zshrc")

	abs, managed := resolveLink(repo, target, link)
	if abs != link {
		t.Errorf("abs = %q, want %q", abs, link)
	}
	if !managed {
		t.Error("managed = false, want true")
	}
}

// TestResolveLink_RelativeLinkIsResolvedAgainstTargetDir は、相対リンクが
// symlink 自身の置かれたディレクトリ基準で絶対化されることを確認する。
func TestResolveLink_RelativeLinkIsResolvedAgainstTargetDir(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "dotfiles")
	home := filepath.Join(base, "home")

	target := filepath.Join(home, ".zshrc")

	abs, managed := resolveLink(repo, target, "../dotfiles/.zshrc")

	want := filepath.Join(repo, ".zshrc")
	if abs != want {
		t.Errorf("abs = %q, want %q", abs, want)
	}
	if !managed {
		t.Error("managed = false, want true")
	}
}

// TestResolveLink_OutsideRepoIsUnmanaged は、repo 外を指す symlink が
// managed と判定されないことを確認する。
func TestResolveLink_OutsideRepoIsUnmanaged(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	abs, managed := resolveLink(repo, filepath.Join(home, ".zshrc"), "/etc/zshrc")

	if abs != "/etc/zshrc" {
		t.Errorf("abs = %q, want %q", abs, "/etc/zshrc")
	}
	if managed {
		t.Error("managed = true, want false")
	}
}

// TestResolveLink_RepoRootItselfIsNotUnderRepo は、repo ルートそのものを指す
// symlink を managed と誤判定しないことを確認する（"配下" に自身は含まない）。
func TestResolveLink_RepoRootItselfIsNotUnderRepo(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	if _, managed := resolveLink(repo, filepath.Join(home, "d"), repo); managed {
		t.Error("managed = true, want false")
	}
}

// TestResolveLink_SiblingWithSharedPrefixIsUnmanaged は、repo と文字列前方一致する
// 別ディレクトリ（"/x/repo-old" と "/x/repo"）を取り違えないことを確認する。
func TestResolveLink_SiblingWithSharedPrefixIsUnmanaged(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	sibling := filepath.Join(base, "repo-old", ".zshrc")

	if _, managed := resolveLink(repo, filepath.Join(base, "home", ".zshrc"), sibling); managed {
		t.Error("managed = true, want false")
	}
}

// TestResolveLink_LinkThroughSymlinkedPathIsManaged は、リンク先が symlink 経由の
// パスで書かれていても managed と判定されることを確認する。
//
// macOS の /var -> /private/var のような環境で実際に踏む。env.Repo は
// EvalSymlinks 済み（ADR 0003）である一方、リンク先の文字列はそうとは限らない。
func TestResolveLink_LinkThroughSymlinkedPathIsManaged(t *testing.T) {
	base := evalTempDir(t)
	repo := filepath.Join(base, "real")
	home := filepath.Join(base, "home")
	mkdirAll(t, filepath.Join(repo, ".config"), home)

	// base/alias -> base/real
	symlink(t, repo, filepath.Join(base, "alias"))

	target := filepath.Join(home, ".config", "foo")
	link := filepath.Join(base, "alias", ".config", "foo")

	_, managed := resolveLink(repo, target, link)
	if !managed {
		t.Error("managed = false, want true")
	}
}

// TestResolveLink_DanglingLinkThroughSymlinkedPathIsManaged は、リンク先が
// 存在しなくても親ディレクトリ経由で managed と判定できることを確認する。
// ADR 0003 は dangling symlink も managed とすることを要求している。
func TestResolveLink_DanglingLinkThroughSymlinkedPathIsManaged(t *testing.T) {
	base := evalTempDir(t)
	repo := filepath.Join(base, "real")
	home := filepath.Join(base, "home")
	mkdirAll(t, filepath.Join(repo, ".config"), home)
	symlink(t, repo, filepath.Join(base, "alias"))

	// .config/gone は存在しない。親の .config だけが存在する。
	link := filepath.Join(base, "alias", ".config", "gone")

	if _, managed := resolveLink(repo, filepath.Join(home, "gone"), link); !managed {
		t.Error("managed = false, want true")
	}
}

// TestResolveLink_UnresolvableLinkFallsBackToPlainNormalization は、リンク先も
// その親も存在しない場合に、単純な正規化の結果で判定することを確認する。
func TestResolveLink_UnresolvableLinkFallsBackToPlainNormalization(t *testing.T) {
	base := evalTempDir(t)
	repo := filepath.Join(base, "real")
	mkdirAll(t, repo)

	link := filepath.Join(base, "nowhere", "deep", "foo")

	abs, managed := resolveLink(repo, filepath.Join(base, "home", "foo"), link)
	if abs != link {
		t.Errorf("abs = %q, want %q", abs, link)
	}
	if managed {
		t.Error("managed = true, want false")
	}
}
