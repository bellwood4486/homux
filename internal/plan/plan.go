// Package plan は inspect が作った []TargetState を、exec が実行する
// []Action へ変換する（spec §12.4、docs/design.md §1/§3）。
//
// 純粋・決定論的であり、os / os/exec / io/fs を import しない
// （docs/design.md §2.1、.golangci.yml の depguard）。時刻のような
// 環境依存の値は Input で受け取る。
//
// status / apply --dry-run / apply はこの同じ Plan を消費する。解決も
// 集計もここより下流で書き直さないことで INV-11 を守る。
package plan

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bellwood4486/homux/internal/env"
	"github.com/bellwood4486/homux/internal/inspect"
)

// ActionKind は exec が行う操作の種類である。
type ActionKind int

const (
	// CreateSymlink は target が無い場所に symlink を張る（spec §9 Missing）。
	CreateSymlink ActionKind = iota
	// Relink は managed symlink のリンク先を張り替える（spec §9.2 種類1）。
	Relink
	// ReplaceTarget は unmanaged な target を退避してから symlink を張る
	// （spec §9 Occupied）。必ず Backup を伴う（INV-13）。
	ReplaceTarget
	// RemoveStaleSymlink は desired から消えた managed symlink を削除する
	// （spec §9.2 種類2）。削除するのは symlink のみである（INV-14）。
	RemoveStaleSymlink
)

func (k ActionKind) String() string {
	switch k {
	case CreateSymlink:
		return "create symlink"
	case Relink:
		return "relink"
	case ReplaceTarget:
		return "replace target"
	case RemoveStaleSymlink:
		return "remove stale symlink"
	default:
		return "unknown"
	}
}

// Action は exec が行う 1 操作である。
//
// target の親ディレクトリの作成（spec §4.1 の MkdirAll）は独立した Action に
// せず、CreateSymlink / Relink / ReplaceTarget の暗黙の一部として exec が行う。
type Action struct {
	// Kind は操作の種類。
	Kind ActionKind
	// Target は操作対象の絶対パス。
	Target string
	// LinkTo はこれから張るリンク先の絶対パス。RemoveStaleSymlink では空である
	// （「今指している先」を入れると Kind によって意味が変わってしまうため）。
	LinkTo string
	// From は今のリンク先の絶対パス。Relink と、symlink の ReplaceTarget でのみ
	// 非空である。
	From string
	// Current は Target が今何であるか（File / Dir / Symlink）。
	//
	// From と Current は exec が使わない。ui が確認プロンプトの文言（spec §12.4）と
	// dry-run の注記（spec §12.5）を決めるためだけに持つ。ui に TargetState を
	// 引き直させないための経路である（docs/design.md §3）。
	Current inspect.CurrentKind
	// Backup は ReplaceTarget の退避先の絶対パス。ReplaceTarget では必ず
	// 非空であり（INV-13）、それ以外の Kind では常に空である。
	Backup string
	// Confirm は実行前に確認が必要か。Missing だけが確認不要である（spec §12.4）。
	Confirm bool
}

// Input は plan の入力である。
type Input struct {
	// States は inspect.All の結果。Path 昇順に並んでいること。
	States []inspect.TargetState
	// Now は退避先の timestamp に使う時刻。plan は純粋であるため呼び出し側が
	// 与える。spec §12.4 の例はローカル時刻なので time.Now() をそのまま渡す。
	Now time.Time
}

// Plan は 1 回の実行で扱う全体像である。
//
// Actions はファイルシステムを実際に触るものだけを含み、何もしない target の
// ための Skip は持たない。表示と件数集計は States が唯一の入力である。
type Plan struct {
	// Actions は exec が上から順に実行する操作。States の順序を保つ。
	Actions []Action
	// States は入力の状態に plan の判定（INV-14 違反の Error 化）を反映したもの。
	// 入力のスライスは変更しない。
	States []inspect.TargetState
}

// Errors は適用をスキップした target の件数を返す。spec §12.4 の
// 「最後にスキップ件数を表示し、終了コード 1 を返す」の根拠であり、
// cli/status と cli/apply が別々に集計しないためにここに置く（INV-11）。
func (p Plan) Errors() int {
	n := 0
	for _, s := range p.States {
		if s.Kind == inspect.KindError {
			n++
		}
	}
	return n
}

// RepoTargetError は HOME target が repository 配下に解決されたことを表す。
//
// repo が $HOME 配下にあると inspect の HOME 走査が repo 内へ降りうるため、
// plan が最後の関門としてこれを検出し、Action を生成せず Error に落とす（INV-14）。
type RepoTargetError struct {
	// Target は repo 配下に解決された絶対パス。
	Target string
}

func (e *RepoTargetError) Error() string {
	return fmt.Sprintf("%s is inside the repository", e.Target)
}

// All は状態を Action へ変換する。States の順序をそのまま保つため、exec の
// 部分適用の報告（spec §12.4）は Action の並びで「ここまで／ここから」を示せる。
func All(e env.Env, in Input) Plan {
	p := Plan{States: make([]inspect.TargetState, len(in.States))}
	copy(p.States, in.States)

	for i := range p.States {
		s := &p.States[i]
		if s.Kind == inspect.KindIgnored {
			// Ignored は repo path の話であり、HOME target を持たない。
			continue
		}

		target := filepath.Join(e.Home, filepath.FromSlash(s.Resolution.Target))
		if withinRepo(e.Repo, target) {
			s.Kind = inspect.KindError
			s.Err = &RepoTargetError{Target: target}
			continue
		}

		if a, ok := actionFor(e, *s, target, in.Now); ok {
			p.Actions = append(p.Actions, a)
		}
	}
	return p
}

// actionFor は 1 つの状態に対応する Action を返す。何もしない状態では ok が false になる。
func actionFor(e env.Env, s inspect.TargetState, target string, now time.Time) (Action, bool) {
	switch s.Kind {
	case inspect.KindMissing:
		return Action{Kind: CreateSymlink, Target: target, LinkTo: sourcePath(e, s), Current: s.Current.Kind}, true

	case inspect.KindOccupied:
		// INV-13: unmanaged な target を黙って上書きしない。退避先は Target から
		// 一意に決まるため、Backup が空になることはない。
		return Action{
			Kind:    ReplaceTarget,
			Target:  target,
			LinkTo:  sourcePath(e, s),
			From:    currentLink(s),
			Current: s.Current.Kind,
			Backup:  backupPath(target, now),
			Confirm: true,
		}, true

	case inspect.KindStale:
		if s.Resolution.Selected != nil {
			// 種類1。managed symlink の張り替えなので退避は不要である
			// （INV-13 は unmanaged な HOME ファイルの保護であり、
			// managed かどうかの判定は Current.Managed 一本である。ADR 0003）。
			return Action{
				Kind:    Relink,
				Target:  target,
				LinkTo:  sourcePath(e, s),
				From:    currentLink(s),
				Current: s.Current.Kind,
				Confirm: true,
			}, true
		}
		// 種類2。inspect は managed symlink のときしか Selected 無しの Stale を
		// 立てない（internal/inspect/inspect.go）。破れていれば symlink 以外を
		// 削除しかねないため、握り潰さず落とす（INV-14）。
		if s.Current.Kind != inspect.CurrentSymlink || !s.Current.Managed {
			panic(fmt.Sprintf("plan: stale target %s is not a managed symlink (%v)", target, s.Current.Kind))
		}
		return Action{Kind: RemoveStaleSymlink, Target: target, Current: s.Current.Kind, Confirm: true}, true

	default:
		// Linked / Inactive / Error は何もしない。
		return Action{}, false
	}
}

// currentLink は今のリンク先の絶対パスを返す。symlink 以外では空である
// （docs/design.md §3 の From の定義）。
func currentLink(s inspect.TargetState) string {
	if s.Current.Kind != inspect.CurrentSymlink {
		return ""
	}
	return s.Current.LinkAbs
}

// sourcePath は選択された source の絶対パスを返す。
func sourcePath(e env.Env, s inspect.TargetState) string {
	return filepath.Join(e.Repo, filepath.FromSlash(s.Resolution.Selected.RepoPath))
}

// backupSuffixFormat は退避先の timestamp 部分の書式である（spec §12.4）。
const backupSuffixFormat = "20060102-150405"

// backupPath は退避先を返す。同一ディレクトリに <name>.homux-bak.<timestamp> を
// 置く。退避先が既に存在するかの検査は plan では行えないため exec の責務である
// （spec §12.4）。
func backupPath(target string, now time.Time) string {
	return target + ".homux-bak." + now.Format(backupSuffixFormat)
}

// withinRepo は path が repo 自身または repo 配下かを返す。
func withinRepo(repo, path string) bool {
	return path == repo || strings.HasPrefix(path, repo+string(filepath.Separator))
}
