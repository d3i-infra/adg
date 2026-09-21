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
