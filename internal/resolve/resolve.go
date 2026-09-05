// Package resolve は Source と active profile から target ごとの Resolution を求める
// （spec §7、docs/design.md §1）。
//
// 純粋・決定論的であり、os / io/fs を import しない（docs/design.md §2.1）。
// status と apply がこの同じ resolver を通ることで INV-11 を守る。
package resolve

import (
	"errors"
	"sort"

	"github.com/bellwood4486/homux/internal/scan"
)

// Reason は Resolution がなぜその結論になったかを表す（explain の土台）。
type Reason int

const (
	// ReasonProfileMatch は active profile に一致する profile-specific source が
	// ちょうど 1 つあったことを表す。
	ReasonProfileMatch Reason = iota
	// ReasonCommonFallback は一致する profile-specific source が無く、
	// common source に fallback したことを表す（INV-08）。
	ReasonCommonFallback
	// ReasonNoActiveProfile は profile なしのため common source のみを見たことを表す（INV-09）。
	ReasonNoActiveProfile
	// ReasonAbsent は選択できる source が無かったことを表す。
	ReasonAbsent
	// ReasonAmbiguous は複数の profile-specific source が一致したことを表す（INV-07）。
	ReasonAmbiguous
	// ReasonUnknownProfile は .homux.toml に未定義の profile が参照されていたことを表す。
	ReasonUnknownProfile
	// ReasonInvalidSelector は selector が構文エラーだったことを表す。
	ReasonInvalidSelector
)

func (r Reason) String() string {
	switch r {
	case ReasonProfileMatch:
		return "profile-specific source matches the active profile"
	case ReasonCommonFallback:
		return "no profile-specific source matches; using the common source"
	case ReasonNoActiveProfile:
		return "no active profile; only common sources are used"
	case ReasonAbsent:
		return "no source is available for this target"
	case ReasonAmbiguous:
		return "multiple profile-specific sources match the active profile"
	case ReasonUnknownProfile:
		return "a selector references a profile that is not defined in .homux.toml"
	case ReasonInvalidSelector:
		return "a selector is syntactically invalid"
	default:
		return "unknown"
	}
}

// Resolution は 1 つの target についての解決結果である。
type Resolution struct {
	// Target は HOME からの相対パス。
	Target string
	// Candidates はこの target に対応する全 source（RepoPath 昇順）。
	Candidates []scan.Source
	// Selected は選ばれた source。nil なら absent、または Err あり。
	Selected *scan.Source
	// Reason はなぜそう選ばれた／選ばれなかったか。
	Reason Reason
	// Err は ambiguous / unknown profile / invalid selector（spec §10）。
	Err error
}

// Input は解決に必要な入力である。
type Input struct {
	// Sources は scan の結果（ignore 済みのもの）。
	Sources []scan.Source
	// Profiles は .homux.toml の profiles。authoritative な定義（spec §5.1）。
	Profiles []string
	// Active は active profile。空文字列は「profile なし」を意味する（spec §5.3）。
	Active string
}

// All は target ごとの Resolution を Target 昇順で返す。
//
// 返り値の error は repository 全体が解決できない場合（active profile が
// .homux.toml に未定義）のみ非 nil になる。個々の target の問題は
// Resolution.Err に載る。
func All(in Input) ([]Resolution, error) {
	if in.Active != "" && !contains(in.Profiles, in.Active) {
		return nil, &UnknownProfileError{
			Profile:    in.Active,
			Suggestion: suggest(in.Active, in.Profiles),
		}
	}

	byTarget := make(map[string][]scan.Source)
	targets := make([]string, 0)
	for _, s := range in.Sources {
		if _, ok := byTarget[s.Target]; !ok {
			targets = append(targets, s.Target)
		}
		byTarget[s.Target] = append(byTarget[s.Target], s)
	}
	sort.Strings(targets)

	out := make([]Resolution, 0, len(targets))
	for _, t := range targets {
		candidates := byTarget[t]
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].RepoPath < candidates[j].RepoPath
		})
		out = append(out, resolveTarget(t, candidates, in))
	}
	return out, nil
}

// resolveTarget は spec §7 の手順をそのまま実装する。
// specificity による優先順位は設けない（INV-07）。
func resolveTarget(target string, candidates []scan.Source, in Input) Resolution {
	r := Resolution{Target: target, Candidates: candidates}

	// 手順 3: 各 source の selector を検証する。
	// 一致するかどうかに関わらず、この target の候補すべてを検証する。
	if reason, err := validate(candidates, in.Profiles); err != nil {
		r.Err = err
		r.Reason = reason
		return r
	}

	// 手順 4: active profile に一致する profile-specific source を数える。
	var matched []int
	var common *scan.Source
	for i := range candidates {
		c := &candidates[i]
		if c.Selector == nil {
			common = c
			continue
		}
		if c.Selector.Matches(in.Active) {
			matched = append(matched, i)
		}
	}

	switch {
	case len(matched) >= 2:
		names := make([]string, 0, len(matched))
		for _, i := range matched {
			names = append(names, candidates[i].RepoPath)
		}
		r.Err = &AmbiguousError{Target: target, Profile: in.Active, Matching: names}
		r.Reason = ReasonAmbiguous
	case len(matched) == 1:
		// INV-06: 1 つの target は高々 1 つの profile-specific source に解決される。
		r.Selected = &candidates[matched[0]]
		r.Reason = ReasonProfileMatch
	case common != nil:
		// INV-08 / INV-09
		r.Selected = common
		if in.Active == "" {
			r.Reason = ReasonNoActiveProfile
		} else {
			r.Reason = ReasonCommonFallback
		}
	default:
		r.Reason = ReasonAbsent
	}
	return r
}

// validate は候補の selector をすべて検証し、問題があればまとめて返す。
func validate(candidates []scan.Source, profiles []string) (Reason, error) {
	var invalid, unknown []error
	for _, c := range candidates {
		if c.SelectorErr != nil {
			invalid = append(invalid, &InvalidSelectorError{RepoPath: c.RepoPath, Err: c.SelectorErr})
			continue
		}
		if c.Selector == nil {
			continue
		}
		for _, p := range c.Selector.Profiles {
			if !contains(profiles, p) {
				unknown = append(unknown, &UnknownProfileError{
					RepoPath:   c.RepoPath,
					Profile:    p,
					Suggestion: suggest(p, profiles),
				})
			}
		}
	}

	switch {
	case len(invalid) > 0:
		return ReasonInvalidSelector, errors.Join(append(invalid, unknown...)...)
	case len(unknown) > 0:
		return ReasonUnknownProfile, errors.Join(unknown...)
	default:
		return 0, nil
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
