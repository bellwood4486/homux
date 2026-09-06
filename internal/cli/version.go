package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// リリースビルドで -ldflags により埋め込まれる値（docs/design.md §5.1）。
//
// debug.ReadBuildInfo() の Main.Version にタグが入るのは module proxy 経由で
// 取得したときだけで、GoReleaser がローカル checkout からビルドしたバイナリでは
// "(devel)" になる。そのため配布物では ldflags を正本とする。
var (
	ldVersion string
	ldCommit  string
)

// buildVersion は表示用のバージョン文字列を組み立てる。
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	return resolveVersion(ldVersion, ldCommit, info, ok)
}

// resolveVersion は ldflags の値を優先し、欠けている項目だけ BuildInfo で補う。
func resolveVersion(ldVersion, ldCommit string, info *debug.BuildInfo, ok bool) string {
	version := ldVersion
	revision := ldCommit

	if ok && info != nil {
		if version == "" {
			version = info.Main.Version
			if version == "" {
				version = "(devel)"
			}
		}
		if revision == "" {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" {
					revision = s.Value
					break
				}
			}
		}
	}

	if version == "" {
		return "(unknown)"
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
