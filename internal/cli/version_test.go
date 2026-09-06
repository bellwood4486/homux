package cli

import (
	"bytes"
	"runtime/debug"
	"strings"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	buildInfo := func(mainVersion, revision string) *debug.BuildInfo {
		info := &debug.BuildInfo{}
		info.Main.Version = mainVersion
		if revision != "" {
			info.Settings = append(info.Settings, debug.BuildSetting{Key: "vcs.revision", Value: revision})
		}
		return info
	}

	tests := []struct {
		name      string
		ldVersion string
		ldCommit  string
		info      *debug.BuildInfo
		ok        bool
		want      string
	}{
		{
			name:      "ldflags が両方入っていればそれを使う",
			ldVersion: "v0.1.0",
			ldCommit:  "0123456789abcdef0123",
			info:      buildInfo("(devel)", "ffffffffffffffffffff"),
			ok:        true,
			want:      "v0.1.0 (0123456789ab)",
		},
		{
			name:      "ldflags の version だけでも ldflags を優先する",
			ldVersion: "v0.1.0",
			info:      buildInfo("(devel)", "ffffffffffffffffffff"),
			ok:        true,
			want:      "v0.1.0 (ffffffffffff)",
		},
		{
			name: "ldflags が無ければ BuildInfo にフォールバックする",
			info: buildInfo("v0.2.0", "0123456789abcdef0123"),
			ok:   true,
			want: "v0.2.0 (0123456789ab)",
		},
		{
			name: "module proxy 経由でなければ (devel) になる",
			info: buildInfo("", "0123456789abcdef0123"),
			ok:   true,
			want: "(devel) (0123456789ab)",
		},
		{
			name: "revision が無ければバージョンだけを返す",
			info: buildInfo("v0.2.0", ""),
			ok:   true,
			want: "v0.2.0",
		},
		{
			name: "BuildInfo が読めなければ (unknown)",
			ok:   false,
			want: "(unknown)",
		},
		{
			name:      "BuildInfo が読めなくても ldflags があればそれを使う",
			ldVersion: "v0.1.0",
			ldCommit:  "0123456789abcdef0123",
			ok:        false,
			want:      "v0.1.0 (0123456789ab)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.ldVersion, tt.ldCommit, tt.info, tt.ok); got != tt.want {
				t.Errorf("resolveVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildVersion_NotEmpty(t *testing.T) {
	if got := buildVersion(); strings.TrimSpace(got) == "" {
		t.Error("buildVersion() must not return an empty string")
	}
}

func TestVersionCmd_PrintsBuildVersion(t *testing.T) {
	cmd := newVersionCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Error("expected non-empty version output")
	}
}
