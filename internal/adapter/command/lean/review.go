package lean

import (
	"fmt"
	util "github.com/d3i-infra/adg/internal/adapter/command"
	leandomain "github.com/d3i-infra/adg/internal/domain/decision/lean"
	"github.com/d3i-infra/adg/internal/domain/decision/madr"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// NewReviewCommand wires `adg lean review`: it emits a deterministic *review packet*
// — the target ADRs plus their lint findings — for a reviewer to judge against the
// lean authoring rubric. The judging itself is not done here: `adg` makes no LLM
// calls. A Claude Code subagent (the write-lean-adr skill drives this) reads the
// packet and produces the verdict, using the session's own model access — no API key
// (see ADR-0011).
//
// Targets: explicit ADR file paths, or --since <git-ref> (records changed since that
// ref), or all records in the model when neither is given.
func NewReviewCommand() *cobra.Command {
	var modelPath, since string

	cmd := &cobra.Command{
		Use:   "review [adr-file...]",
		Short: "Emit a review packet (ADRs + findings) for a reviewer to judge against the rubric",
		Long: `Review prints a deterministic review packet — each target ADR plus the lint
findings already reported for it — for a reviewer to judge against the lean authoring
rubric (references/lean-rubric.md). It does not call an LLM: the write-lean-adr skill
has a Claude Code subagent read this packet and produce the verdict, using the session's
own model access (no API key).

Targets: explicit ADR file paths, or --since <git-ref> (records changed since that ref),
or all records in the model when neither is given.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := util.ResolveModelPath(modelPath)
			records, err := leandomain.LoadDir(resolved)
			if err != nil {
				err = util.ModelLoadHint(resolved, err)
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
				return err
			}
			targets := selectReviewTargets(records, args, since)
			if len(targets) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no ADRs to review")
				return nil
			}

			byID := map[string][]leandomain.Issue{}
			for _, is := range leandomain.ValidateWithBudget(records, budgetFor(cmd, resolved)) {
				byID[is.ID] = append(byID[is.ID], is)
			}

			out := cmd.OutOrStdout()
			fmt.Fprint(out, reviewPacketHeader(len(targets)))
			for _, rec := range targets {
				fmt.Fprint(out, reviewPacketEntry(rec, byID[rec.ID]))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&modelPath, "model", "", "Path to the lean ADR directory (default: docs/decisions)")
	cmd.Flags().StringVar(&since, "since", "", "Review only the ADR files changed since this git ref")
	return cmd
}

func reviewPacketHeader(n int) string {
	return fmt.Sprintf(`# Lean ADR review packet (%d record(s))

Judge each ADR below against the lean authoring rubric (references/lean-rubric.md in the
write-lean-adr skill). For each, return a verdict — pass | revise — and specific,
rubric-anchored, actionable fixes; do not invent problems where the record is sound.
Prefer a fresh-context subagent per ADR.

`, n)
}

func reviewPacketEntry(rec leandomain.Record, findings []leandomain.Issue) string {
	var b strings.Builder
	p := leandomain.ParseBody(rec.Body)
	fmt.Fprintf(&b, "## ADR-%s — %s\n\n", rec.ID, p.Title)
	fmt.Fprintf(&b, "_%s_\n\n", reviewRoutingLine(rec.D))
	b.WriteString(strings.TrimRight(rec.Body, "\n"))
	b.WriteString("\n\nDeterministic findings already reported by the linter:\n")
	if len(findings) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, f := range findings {
			kind := "warn"
			if !f.Warning {
				kind = "FAIL"
			}
			fmt.Fprintf(&b, "- [%s] %s\n", kind, f.Message)
		}
	}
	b.WriteString("\n")
	return b.String()
}

func reviewRoutingLine(d madr.Decision) string {
	parts := []string{"status: " + d.Status, "priority: " + d.Priority}
	for _, kv := range []struct {
		k string
		v []string
	}{
		{"applies_to", d.AppliesTo}, {"excludes", d.Excludes},
		{"forbids", d.Forbids}, {"companions", d.Companions},
	} {
		if len(kv.v) > 0 {
			parts = append(parts, kv.k+": "+strings.Join(kv.v, ", "))
		}
	}
	return strings.Join(parts, " · ")
}

// selectReviewTargets resolves which records to review: explicit ADR file paths, the
// records changed --since a git ref, or all records when neither is given. Paths are
// matched to records by filename.
func selectReviewTargets(records []leandomain.Record, args []string, since string) []leandomain.Record {
	if len(args) == 0 && since == "" {
		return records
	}
	want := map[string]bool{}
	if len(args) > 0 {
		for _, a := range args {
			want[filepath.Base(a)] = true
		}
	} else {
		for _, p := range gitDiffNames(".", since) {
			want[filepath.Base(p)] = true
		}
	}
	var out []leandomain.Record
	for _, r := range records {
		if want[r.Filename] {
			out = append(out, r)
		}
	}
	return out
}

// gitDiffNames returns the paths changed since ref (fail-open to nil).
func gitDiffNames(dir, ref string) []string {
	c := exec.Command("git", "diff", "--name-only", ref)
	c.Dir = dir
	b, err := c.Output()
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if p := strings.TrimSpace(line); p != "" {
			out = append(out, p)
		}
	}
	return out
}
