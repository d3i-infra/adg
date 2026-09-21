// This file is derived from github.com/adr/ad-guidance-tool (Apache-2.0) and has
// been modified by the adg authors; see the Acknowledgements section of README.md.

package commands

import (
	"fmt"
	"strconv"
)

// DefaultModelPath is the conventional lean model location. adg keeps no global
// user state: resolution is flag-or-convention, decided per invocation.
const DefaultModelPath = "docs/decisions"

// ResolveModelPath resolves the model directory for a command: the --model flag
// if set, else the docs/decisions convention. It never touches the filesystem —
// `lean new` may legitimately create into a not-yet-populated model, and read
// commands surface a load failure through ModelLoadHint instead.
func ResolveModelPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return DefaultModelPath
}

// ModelLoadHint decorates a model-load failure with the resolved path and the
// --model escape hatch, so a bare invocation outside a governed repo says what
// was assumed and how to override it.
func ModelLoadHint(resolved string, err error) error {
	if resolved == DefaultModelPath {
		return fmt.Errorf("cannot load lean model at %q: %w (pass --model <dir> if the ADRs live elsewhere)", resolved, err)
	}
	return fmt.Errorf("cannot load lean model at %q: %w", resolved, err)
}

// NormalizeID accepts "22" or "0022" and returns "0022". Rejects values outside
// 1..9999 and non-numeric input. 0000 is reserved.
func NormalizeID(input string) (string, error) {
	n, err := strconv.Atoi(input)
	if err != nil {
		return "", fmt.Errorf("invalid --id %q: must be a number 1-9999", input)
	}
	if n < 1 || n > 9999 {
		return "", fmt.Errorf("invalid --id %q: must be in range 1-9999", input)
	}
	return fmt.Sprintf("%04d", n), nil
}
