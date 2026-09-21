// This file is derived from github.com/adr/ad-guidance-tool (Apache-2.0) and has
// been modified by the adg authors; see the Acknowledgements section of README.md.

package cmd

import (
	printer "github.com/d3i-infra/adg/v4/internal/adapter/printer"
	"os"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "adg",
	Short: "Architectural Decision Guidance CLI",
	Long:  "CLI tool for managing architectural decision records and models",
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true, // hides completion cmd from help text but it is still available
	},
}

// version is the adg version string. goreleaser overrides it at release time via
// -ldflags "-X github.com/d3i-infra/adg/v4/cmd.version=<tag>" (wired by .goreleaser.yaml).
// A `go install github.com/d3i-infra/adg/v4@<version>` build gets no ldflags, so
// resolveVersion falls back to the module version Go records in the build info.
var version = "dev"

// resolveVersion picks what `adg --version` prints: the linked version when there is
// one, else the main module's version without its leading "v", else "dev". A plain
// `go build` in a checkout reports "(devel)" and stays "dev".
func resolveVersion(linked, mainVersion string) string {
	if linked != "dev" {
		return linked
	}
	if mainVersion == "" || mainVersion == "(devel)" {
		return "dev"
	}
	return strings.TrimPrefix(mainVersion, "v")
}

func buildInfoVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}
	return ""
}

// Quiet is bound to the persistent --quiet flag. Presenters read it
// through Streams.Quiet (pointer) so the parsed value is visible at
// write time even though presenters are constructed during init().
var Quiet bool

func init() {
	rootCmd.Version = resolveVersion(version, buildInfoVersion())
	rootCmd.SetVersionTemplate("adg {{.Version}}\n")
	rootCmd.PersistentFlags().BoolVar(&Quiet, "quiet", false,
		"Suppress success status messages on stderr; machine values on stdout and errors still print")
}

// streams returns the shared Streams the cmd-layer hands to presenters.
// It binds Quiet by pointer so the flag value is observed at write time.
func streams() printer.Streams {
	return printer.Streams{Out: os.Stdout, Err: os.Stderr, Quiet: &Quiet}
}

func Execute() error {
	return rootCmd.Execute()
}
