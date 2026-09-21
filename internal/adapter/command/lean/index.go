package lean

import (
	"errors"
	"fmt"
	util "github.com/d3i-infra/adg/v4/internal/adapter/command"
	leandomain "github.com/d3i-infra/adg/v4/internal/domain/decision/lean"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// ErrLeanValidationIssues is returned when the lean validator (or scope lint)
// reports a hard failure, so main.go exits non-zero. Warnings do not trigger it.
var ErrLeanValidationIssues = errors.New("lean validation issues found")

// NewIndexCommand wires `adg index`. It validates every lean ADR and prints (or
// writes) the category-grouped README; with --root it also runs scope lint, and
// with --overlaps it prints the opt-in default-vs-default overlap diagnostic.
func NewIndexCommand() *cobra.Command {
	var modelPath, root, overlaps string
	var write bool

	cmd := &cobra.Command{
		Use:   "index",
		Short: "Validate lean ADRs and generate the grouped README index",
		Long: `Index validates every lean ADR and prints (or writes, with --write) the
category-grouped README index. Validation hard-fails on a duplicate ID or an
unsupported brace glob ({a,b}) and warns on glob-hygiene nits. With --root <tree>
it also runs scope lint: stale applies_to/excludes globs (matching no file under
the tree) and forbids globs that now match a file. Warnings are advisory; a hard
validation failure exits non-zero.

Default-vs-default scope overlap is an opt-in diagnostic (overlap between defaults
is usually benign), printed as an [info] block — never a failure. Pass --overlaps
(grouped per-hub summary) or --overlaps=pairs (unaggregated per-pair detail); both
require --root.`,
		SilenceErrors: true, // issues already printed to stderr
		SilenceUsage:  true, // failure is data, not user error
		RunE: func(cmd *cobra.Command, args []string) error {
			overlapMode, oerr := parseOverlapMode(overlaps)
			if oerr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", oerr)
				return oerr
			}
			if overlapMode != leandomain.OverlapOff && root == "" {
				err := fmt.Errorf("--overlaps requires --root <tree> (scope overlap is computed against the source tree)")
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
				return err
			}
			resolved := util.ResolveModelPath(modelPath)
			records, err := leandomain.LoadDir(resolved)
			if err != nil {
				err = util.ModelLoadHint(resolved, err)
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
				return err
			}

			issues := leandomain.ValidateWithBudget(records, budgetFor(cmd, resolved))
			if root != "" {
				lintIssues, lerr := leandomain.LintTree(records, root)
				if lerr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error: scope lint: %v\n", lerr)
					return lerr
				}
				issues = append(issues, lintIssues...)
			}

			errCount := 0
			for _, is := range issues {
				kind := "FAIL"
				if is.Warning {
					kind = "warn"
				} else {
					errCount++
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "[%s] %s: %s\n", kind, is.ID, is.Message)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "\nvalidated %d ADR(s): %d failure(s), %d warning(s)\n", len(records), errCount, len(issues)-errCount)

			// Overlap is an opt-in diagnostic, printed as an [info] block separate
			// from the validation issues so it never affects the failure/warning count.
			if block, lerr := leandomain.Overlaps(records, root, overlapMode); lerr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: overlap diagnostic: %v\n", lerr)
				return lerr
			} else if block != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "\n%s", block)
			}

			index := leandomain.RenderIndex(records)
			if write {
				out := filepath.Join(resolved, "README.md")
				if werr := os.WriteFile(out, []byte(index), 0o644); werr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error: writing index: %v\n", werr)
					return werr
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s\n", out)
			} else {
				fmt.Fprint(cmd.OutOrStdout(), index)
			}

			if errCount > 0 {
				return ErrLeanValidationIssues
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&modelPath, "model", "", "Path to the lean ADR directory (default: docs/decisions)")
	cmd.Flags().StringVar(&root, "root", "", "Source tree root for scope lint (stale applies_to/excludes, forbids-has-files); skipped if empty")
	cmd.Flags().BoolVar(&write, "write", false, "Write the generated index to <model>/README.md instead of stdout")
	cmd.Flags().StringVar(&overlaps, "overlaps", "", "Default-vs-default overlap diagnostic (requires --root): summary (default) | pairs")
	// --overlaps with no value means the grouped summary; --overlaps=pairs is the
	// per-pair detail; omitting the flag leaves it off.
	cmd.Flags().Lookup("overlaps").NoOptDefVal = "summary"
	return cmd
}

// parseOverlapMode maps the --overlaps flag string to an OverlapMode: "" is off,
// "summary" is the grouped per-hub view, "pairs" is the unaggregated detail.
func parseOverlapMode(s string) (leandomain.OverlapMode, error) {
	switch s {
	case "":
		return leandomain.OverlapOff, nil
	case "summary":
		return leandomain.OverlapSummary, nil
	case "pairs":
		return leandomain.OverlapPairs, nil
	default:
		return leandomain.OverlapOff, fmt.Errorf("invalid --overlaps %q (want: summary | pairs)", s)
	}
}
