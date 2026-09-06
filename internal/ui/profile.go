// profile.go は homux profile list / use（spec §12.7、§12.8）の出力整形を担う。
package ui

import (
	"fmt"
	"io"
	"sort"
)

// RenderProfileList は .homux.toml の profiles と active profile を
// spec §12.7 のレイアウトで w に書き出す。定義順ではなくアルファベット順に
// 並べる（spec の例が profiles = ["work", "personal"] に対し
// "personal" を先に表示している）。
func RenderProfileList(w io.Writer, profiles []string, active string) {
	fmt.Fprintln(w, "Profiles:")
	fmt.Fprintln(w)

	if len(profiles) == 0 {
		fmt.Fprintf(w, "  %s\n", noneChoice)
	} else {
		sorted := append([]string(nil), profiles...)
		sort.Strings(sorted)
		for _, p := range sorted {
			marker := " "
			if p == active {
				marker = "*"
			}
			fmt.Fprintf(w, "%s %s\n", marker, p)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Active profile: %s\n", profileLabel(active))
}

// RenderProfileSwitch は homux profile use の結果を w に書き出す（spec §12.8）。
//
// HOME は switch では変更されない。applyNeeded が true なら desired state との
// 差異が生じたことを示し、"homux apply" が必要である旨を表示する
// （spec §12.8、§11.2）。
func RenderProfileSwitch(w io.Writer, from, to string, applyNeeded bool) {
	fmt.Fprintf(w, "Active profile: %s -> %s\n\n", profileLabel(from), profileLabel(to))
	if applyNeeded {
		fmt.Fprintln(w, `Run "homux apply" to update HOME.`)
		return
	}
	fmt.Fprintln(w, "HOME already matches this profile.")
}
