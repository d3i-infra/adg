package lean

import (
	"fmt"
	leandomain "github.com/d3i-infra/adg/internal/domain/decision/lean"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// projectConfigFile is the per-project adg settings file, read from beside the lean
// model root (e.g. docs/decisions/.adg.yaml). It travels with the repo.
const projectConfigFile = ".adg.yaml"

// projectConfig is the on-disk shape of <model-root>/.adg.yaml. Only body_budget is
// defined today; unknown keys are ignored by yaml.Unmarshal (forward-compatible).
type projectConfig struct {
	BodyBudget string `yaml:"body_budget"`
}

// loadBudget reads <root>/.adg.yaml and maps body_budget to a lean.Budget. It never
// fails a command: a bad or absent config degrades to DefaultBudget. The second return
// is a stderr warning ("" when clean):
//   - no file / "" / "lean"   -> DefaultBudget,   ""
//   - "narrative"             -> NarrativeBudget, ""
//   - unknown value           -> DefaultBudget,   warning
//   - unreadable / malformed  -> DefaultBudget,   warning
func loadBudget(root string) (leandomain.Budget, string) {
	path := filepath.Join(root, projectConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return leandomain.DefaultBudget(), ""
		}
		return leandomain.DefaultBudget(), fmt.Sprintf("warning: could not read %s: %v; using default body_budget", path, err)
	}
	var pc projectConfig
	if err := yaml.Unmarshal(data, &pc); err != nil {
		return leandomain.DefaultBudget(), fmt.Sprintf("warning: could not parse %s: %v; using default body_budget", path, err)
	}
	switch pc.BodyBudget {
	case "", "lean":
		return leandomain.DefaultBudget(), ""
	case "narrative":
		return leandomain.NarrativeBudget(), ""
	default:
		return leandomain.DefaultBudget(), fmt.Sprintf("warning: unknown body_budget %q in %s; expected \"lean\" or \"narrative\"; using default", pc.BodyBudget, path)
	}
}

// budgetFor loads the per-project body budget for the resolved model root, printing any
// config warning to the command's stderr. Commands call this immediately before
// leandomain.ValidateWithBudget.
func budgetFor(cmd *cobra.Command, root string) leandomain.Budget {
	b, w := loadBudget(root)
	if w != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), w)
	}
	return b
}
