package cli

import (
	"bytes"
	"strings"
	"testing"
)

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
