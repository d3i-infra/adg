package lean

import (
	"encoding/json"
	"fmt"
	util "github.com/d3i-infra/adg/v4/internal/adapter/command"
	leandomain "github.com/d3i-infra/adg/v4/internal/domain/decision/lean"
	"io"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// NewVerifyCommand wires `adg lean verify`: the post-edit half of the contract. It
// re-runs validation (and, with --root, scope lint) and re-renders the brief — with
// its "Before you finish" footer — for the changed files, so a finished change is
// checked against the ADRs that govern it. With no path arguments it derives the
// changed files from git (working tree vs HEAD, plus untracked).
//
// --hook runs it as a Claude Code Stop hook: advisory output to stderr, always exit
// 0 (fail-open, never blocks stopping). Without --hook it is CI/manual: the brief
// prints to stdout and a hard validation failure exits non-zero.
func NewVerifyCommand() *cobra.Command {
	var modelPath, root string
	var hook bool

	cmd := &cobra.Command{
		Use:   "verify [changed-path...]",
		Short: "Re-validate the model and re-show the brief for changed files (Stop-hook friendly)",
		Long: `Verify re-runs the lean model validation (and --root scope lint) and re-renders
the architecture brief — with its "Before you finish" footer — for the changed files,
so a finished change is checked against the ADRs that govern it.

With no path arguments it derives the changed files from git (working tree vs HEAD,
plus untracked files). With --hook it runs as a Claude Code Stop hook: advisory output
to stderr, always exit 0 (fail-open, never blocks stopping). Without --hook it is
CI/manual: the brief prints to stdout and a hard validation failure exits non-zero.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := util.ResolveModelPath(modelPath)
			records, err := leandomain.LoadDir(resolved)
			if err != nil {
				if hook {
					return nil
				}
				err = util.ModelLoadHint(resolved, err)
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
				return err
			}

			paths := args
			if len(paths) == 0 {
				dir := "."
				if hook {
					dir = stopHookCWD(cmd.InOrStdin())
				}
				paths = gitChangedPaths(dir)
			}

			// Validation issues always go to stderr (the advisory channel).
			issues := leandomain.ValidateWithBudget(records, budgetFor(cmd, resolved))
			if root != "" {
				if li, lerr := leandomain.LintTree(records, root); lerr == nil {
					issues = append(issues, li...)
				}
			}
			hard := 0
			for _, is := range issues {
				kind := "warn"
				if !is.Warning {
					kind = "FAIL"
					hard++
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "[%s] %s: %s\n", kind, is.ID, is.Message)
			}

			// Re-render the brief (footer included) for the changed files. In hook
			// mode it goes to stderr so a Stop hook surfaces it to the user; otherwise
			// to stdout.
			briefW := cmd.OutOrStdout()
			if hook {
				briefW = cmd.ErrOrStderr()
			}
			if len(paths) > 0 && leandomain.Matches(records, paths) {
				fmt.Fprint(briefW, leandomain.Brief(records, paths, leandomain.BriefAuto))
			}

			// Run the executable checks (scoped to the changed files) and report
			// failures. Advisory in hook mode; gating otherwise.
			checkFailed := 0
			if root != "" {
				if results, cerr := leandomain.RunChecks(records, root, paths); cerr == nil {
					for _, c := range results {
						if c.Failed {
							checkFailed++
							fmt.Fprintf(cmd.ErrOrStderr(), "[FAIL] ADR-%s: %s — %s\n", c.ID, c.Desc, c.Detail)
						}
					}
				}
			}

			if hook {
				return nil // advisory only — never block the stop
			}
			if hard > 0 || checkFailed > 0 {
				return ErrLeanValidationIssues
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&modelPath, "model", "", "Path to the lean ADR directory (default: docs/decisions)")
	cmd.Flags().StringVar(&root, "root", ".", "Source tree root for scope lint; empty to skip")
	cmd.Flags().BoolVar(&hook, "hook", false, "Claude Code Stop hook mode: advisory output to stderr, always exit 0 (fail-open)")
	return cmd
}

// stopHookCWD reads the project root from a Claude Code Stop hook payload on stdin,
// falling back to "." (fail-open) when the payload is absent or malformed.
func stopHookCWD(r io.Reader) string {
	b, err := io.ReadAll(r)
	if err != nil {
		return "."
	}
	var in struct {
		CWD string `json:"cwd"`
	}
	if json.Unmarshal(b, &in) == nil && strings.TrimSpace(in.CWD) != "" {
		return in.CWD
	}
	return "."
}

// gitChangedPaths returns the repo-root-relative paths changed under dir: tracked
// changes against HEAD plus untracked files. It is fail-open — any git error yields
// no paths rather than an error.
func gitChangedPaths(dir string) []string {
	seen := map[string]bool{}
	var out []string
	for _, argv := range [][]string{
		{"diff", "--name-only", "HEAD"},
		{"ls-files", "--others", "--exclude-standard"},
	} {
		c := exec.Command("git", argv...)
		c.Dir = dir
		b, err := c.Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			p := strings.TrimSpace(line)
			if p != "" && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	sort.Strings(out)
	return out
}
