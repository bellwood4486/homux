package resolve

// maxSuggestDistance は「もしかして」を出す編集距離の上限である。
// これを超える距離の候補は typo ではなく別の名前とみなす。
const maxSuggestDistance = 2

// suggest は candidates の中から name に最も近いものを返す（spec §10.1）。
// 十分に近いものが無ければ空文字列を返す。
func suggest(name string, candidates []string) string {
	best := ""
	bestDist := maxSuggestDistance + 1
	for _, c := range candidates {
		d := levenshtein(name, c)
		if d < bestDist {
			best, bestDist = c, d
		}
	}
	if bestDist > maxSuggestDistance {
		return ""
	}
	return best
}

// levenshtein は 2 つの文字列の編集距離を返す。
// profile 名は ASCII に限られる（spec §5.4）ためバイト単位で比較する。
func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}
