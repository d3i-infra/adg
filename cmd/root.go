package cmd

import (
	printer "github.com/d3i-infra/adg/internal/adapter/printer"
	"os"

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

// version is the adg version string. It defaults to "dev" for source builds and
// is overridden at release time via -ldflags "-X github.com/d3i-infra/adg/cmd.version=<tag>" (wired by
// .goreleaser.yaml). init() reads it into rootCmd.Version, so the value injected
// at link time is what `adg --version` prints.
var version = "dev"

// Quiet is bound to the persistent --quiet flag. Presenters read it
// through Streams.Quiet (pointer) so the parsed value is visible at
// write time even though presenters are constructed during init().
var Quiet bool

func init() {
	rootCmd.Version = version
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
