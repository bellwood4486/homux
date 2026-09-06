// Package inspect は desired state（resolve の結果）と $HOME の実状態を
// 突き合わせて []TargetState を作る（spec §9、docs/design.md §3）。
//
// HOME を読むだけで、一切変更しない。ファイルシステムを読むのはこの
// パッケージだけであり、plan / ui はここが返した値だけを見る
// （docs/design.md §2.1）。解決ロジックは resolve が持つものを再実装しない（INV-11）。
package inspect

import (
	"path/filepath"
	"sort"

	"github.com/bellwood4486/homux/internal/env"
	"github.com/bellwood4486/homux/internal/resolve"
)

// StateKind は spec §9 の状態モデルである。
type StateKind int

const (
	// KindLinked は期待どおりの symlink が存在することを表す。
	KindLinked StateKind = iota
	// KindMissing は desired source はあるが HOME に target が無いことを表す。
	KindMissing
	// KindOccupied は HOME target が管理外のファイル／ディレクトリで
	// 占有されていることを表す。
	KindOccupied
	// KindStale は managed symlink が現在の desired state と一致しないことを表す。
	// Resolution.Selected が非 nil なら relink、nil なら削除の対象である（spec §9.2）。
	KindStale
	// KindIgnored は repository path が ignore ルールで対象外であることを表す。
	KindIgnored
	// KindInactive は profile-specific source が現在の active profile では
	// 選択されていないことを表す。
	KindInactive
	// KindError は unknown profile / ambiguous / selector 構文エラー、
	// または HOME の読み取り自体が失敗したことを表す。
	KindError
)

func (k StateKind) String() string {
	switch k {
	case KindLinked:
		return "linked"
	case KindMissing:
		return "missing"
	case KindOccupied:
		return "occupied"
	case KindStale:
		return "stale"
	case KindIgnored:
		return "ignored"
	case KindInactive:
		return "inactive"
	case KindError:
		return "error"
	default:
		return "unknown"
	}
}

// TargetState は 1 件の突き合わせ結果である。
type TargetState struct {
	// Resolution は desired 側の解決結果。Kind が KindIgnored のときはゼロ値。
	// HOME 走査で見つかった孤児 symlink（spec §9.2 の種類2）では
	// Target だけが埋まり、Selected は nil である。
	Resolution resolve.Resolution
	// RepoPath は Kind が KindIgnored のときの repo 相対パス。それ以外では空。
	RepoPath string
	// Kind は状態。
	Kind StateKind
	// Current は HOME 上の実状態。
	Current Current
	// Err は HOME の読み取りで生じたエラー（Resolution.Err とは別物）。
	Err error
}

// Path は並び替えと表示に使う識別子を返す。
// Ignored は HOME target を持たないため repo 相対パスを返す。
func (s TargetState) Path() string {
	if s.Kind == KindIgnored {
		return s.RepoPath
	}
	return s.Resolution.Target
}

// Input は突き合わせの入力である。resolve と scan の結果をそのまま渡す。
type Input struct {
	// Resolutions は resolve.All の結果。
	Resolutions []resolve.Resolution
	// Ignored は scan.Result.Ignored（ignore された repo 相対パス）。
	Ignored []string
}

// All は desired 側の突き合わせ、HOME 走査による孤児 symlink の検出、
// ignore された source を 1 本のスライスにまとめ、Path の辞書昇順で返す。
//
// 個々の target の I/O エラーは TargetState.Err に載せて続行する。
// 返り値の error は HOME 走査自体が成立しなかった場合のみ非 nil になる。
func All(e env.Env, in Input) ([]TargetState, error) {
	states := make([]TargetState, 0, len(in.Resolutions)+len(in.Ignored))

	desired := make(map[string]bool, len(in.Resolutions))
	for _, r := range in.Resolutions {
		desired[r.Target] = true
		states = append(states, inspectResolution(e, r))
	}

	orphans, err := scanHome(e, desired)
	if err != nil {
		return nil, err
	}
	states = append(states, orphans...)

	for _, p := range in.Ignored {
		states = append(states, TargetState{RepoPath: p, Kind: KindIgnored})
	}

	sort.Slice(states, func(i, j int) bool { return states[i].Path() < states[j].Path() })
	return states, nil
}

// inspectResolution は判定表 #1〜#10 を実装する。
func inspectResolution(e env.Env, r resolve.Resolution) TargetState {
	s := TargetState{Resolution: r}

	current, err := ReadCurrent(e.Repo, filepath.Join(e.Home, filepath.FromSlash(r.Target)))
	s.Current = current
	if err != nil {
		// #2 / #3: HOME を読めなければ状態を判定できない。
		s.Err = err
		s.Kind = KindError
		return s
	}

	// #1: 構造エラー。Current は explain のために読み終えてある。
	if r.Err != nil {
		s.Kind = KindError
		return s
	}

	if r.Selected == nil {
		// #9 / #10
		if current.Kind == CurrentSymlink && current.Managed {
			s.Kind = KindStale
		} else {
			s.Kind = KindInactive
		}
		return s
	}

	switch {
	case current.Kind == CurrentAbsent:
		s.Kind = KindMissing // #4
	case current.Kind != CurrentSymlink, !current.Managed:
		s.Kind = KindOccupied // #7 / #8
	case current.LinkAbs == filepath.Join(e.Repo, filepath.FromSlash(r.Selected.RepoPath)):
		s.Kind = KindLinked // #5
	default:
		s.Kind = KindStale // #6
	}
	return s
}
