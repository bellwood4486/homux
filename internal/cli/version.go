package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// buildVersion は debug.ReadBuildInfo() からバージョン文字列を組み立てる。
// -ldflags による埋め込みは行わない（docs/design.md §5.1）。
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(unknown)"
	}

	version := info.Main.Version
	if version == "" {
		version = "(devel)"
	}

	var revision string
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			revision = s.Value
			break
		}
	}
	if revision == "" {
		return version
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	return fmt.Sprintf("%s (%s)", version, revision)
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "バージョンを表示する",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), buildVersion())
			return nil
		},
	}
}
