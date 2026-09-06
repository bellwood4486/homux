package ui

import "testing"

// 候補が数件しかないときに空行で画面を埋めない。多いときは頭打ちにして
// 端末を占有しない。
func TestSelectHeight(t *testing.T) {
	tests := []struct {
		candidates int
		want       int
	}{
		{1, 3},
		{2, 4},
		{maxSelectHeight - 2, maxSelectHeight},
		{47, maxSelectHeight},
	}
	for _, tt := range tests {
		if got := selectHeight(tt.candidates); got != tt.want {
			t.Errorf("selectHeight(%d) = %d, want %d", tt.candidates, got, tt.want)
		}
	}
}
