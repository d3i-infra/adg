// This file is derived from github.com/adr/ad-guidance-tool (Apache-2.0) and has
// been modified by the adg authors; see the Acknowledgements section of README.md.

package main

import (
	"os"

	"github.com/d3i-infra/adg/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		// Cobra has already printed the error to stderr (unless the command
		// set SilenceErrors=true after handling it itself). All we do here
		// is propagate a non-zero exit code.
		os.Exit(1)
	}
}
