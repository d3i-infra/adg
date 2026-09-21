// Package lean holds the cobra commands for the lean ADR format (new, brief,
// index, verify, check, review). They are thin cobra adapters over the
// internal/domain/decision/lean package, which already returns finished output —
// the adapters parse flags, call the domain, and pick the output stream; the
// shared renderer is never reimplemented here.
package lean

import (
	"encoding/json"
	"fmt"
	util "github.com/d3i-infra/adg/internal/adapter/command"
	leandomain "github.com/d3i-infra/adg/internal/domain/decision/lean"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// NewBriefCommand wires `adg brief`. It routes changed paths to the ADRs that
// govern them (by applies_to) and prints the compiled guidance packet; with
// --hook it runs as a Claude Code PreToolUse hook over stdin.
func NewBriefCommand() *cobra.Command {
	var modelPath string
	var hook, full, compact, digest, whole, invariants, staged, guard bool

	cmd := &cobra.Command{
		Use:   "brief [changed-path...]",
		Short: "Compile the architecture guidance brief for changed files",
		Long: `Brief routes changed file paths to the ADRs that govern them and prints a
compact guidance packet: the matching ADRs grouped by force (invariants first),
each with its Decision and Guidance, and a "Before you finish" footer that re-runs
the index gate and lists the matched ADRs' Checks and named test files.

Routing is by frontmatter globs: applies_to selects, excludes carves out sanctioned
or out-of-scope paths, and forbids flags edits to negative-space paths; companions
are surfaced as related files. Without --hook, brief also validates the model and
prints any issues (e.g. an unsupported brace glob) to stderr, exiting non-zero on a
hard failure, so a malformed scope is never silently mis-routed.

By default the brief is rendered in auto mode: full entries, but if the brief would
exceed one screen the defaults collapse to a one-line checklist (invariants and
forbidden-path hits always stay full). --full forces every entry full with scope
detail (the debuggable form); --compact forces the condensed form.

With --hook it runs as a Claude Code hook reading the hook JSON on stdin. By default
(PreToolUse on an edit) it injects the brief for the edited file as additionalContext.
--digest injects the session-open digest — a titles-only tripwire index of every
in-force ADR, hard-capped at 2 KB so it always lands inline (SessionStart); without
--hook, --digest prints the digest and reports each ladder rung's size to stderr;
--whole injects the whole-corpus brief (SubagentStart for planner agents); --invariants
injects the invariants-only brief (SubagentStart); --staged briefs the staged files on a git commit
Bash call, blocking only on a forbidden-scope hit; --guard blocks hand-creating an ADR
record (a Write of a new NNNN-*.md under the model) and warns on editing one. Hook mode
is fail-open on errors — a malformed payload or missing model injects nothing; only
--staged (forbidden scope) and --guard (hand-created record) ever block. It always uses
auto rendering, so --full/--compact are invalid with --hook.`,
		SilenceErrors: true, // issues already printed to stderr
		SilenceUsage:  true, // failure is data, not user error
		RunE: func(cmd *cobra.Command, args []string) error {
			if hook {
				if full || compact {
					err := fmt.Errorf("--full/--compact are invalid with --hook (hook mode always uses auto rendering)")
					fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
					return err
				}
				if boolCount(digest, whole, invariants, staged, guard) > 1 {
					err := fmt.Errorf("--digest, --whole, --invariants, --staged, and --guard are mutually exclusive")
					fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
					return err
				}
				return runHook(cmd, modelPath, digest, whole, invariants, staged, guard)
			}
			// --digest without --hook is the diagnostic mode: render the digest to
			// stdout and report every ladder rung's size to stderr, so an author
			// tuning a corpus toward a richer rung can see the gap instead of
			// guessing (or re-implementing the renderer, forbidden per ADR-0002).
			if digest {
				resolved := util.ResolveModelPath(modelPath)
				records, err := leandomain.LoadDir(resolved)
				if err != nil {
					err = util.ModelLoadHint(resolved, err)
					fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
					return err
				}
				hard := reportLeanIssues(cmd.ErrOrStderr(), leandomain.ValidateWithBudget(records, budgetFor(cmd, resolved)))
				for _, r := range leandomain.DigestReport(records) {
					state := "over budget"
					if r.Selected {
						state = "selected"
					} else if r.Fits {
						state = "fits"
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "rung %-15s %5d bytes (budget %d) — %s\n", r.Name, r.Bytes, leandomain.MaxDigestBytes, state)
				}
				fmt.Fprint(cmd.OutOrStdout(), leandomain.BriefDigest(records))
				if hard > 0 {
					return ErrLeanValidationIssues
				}
				return nil
			}
			if whole || invariants || staged || guard {
				err := fmt.Errorf("--whole/--invariants/--staged/--guard require --hook")
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
				return err
			}

			mode := leandomain.BriefAuto
			if full {
				mode = leandomain.BriefFull
			} else if compact {
				mode = leandomain.BriefCompact
			}

			resolved := util.ResolveModelPath(modelPath)
			if len(args) == 0 {
				err := fmt.Errorf("provide one or more changed paths, or use --hook")
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
				return err
			}
			records, err := leandomain.LoadDir(resolved)
			if err != nil {
				err = util.ModelLoadHint(resolved, err)
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
				return err
			}
			// Non-hook brief is fail-closed: surface model problems (e.g. an
			// unsupported brace glob that would silently mis-route) to stderr rather
			// than render a wrong brief in silence. runHook stays fail-open.
			hard := reportLeanIssues(cmd.ErrOrStderr(), leandomain.ValidateWithBudget(records, budgetFor(cmd, resolved)))
			fmt.Fprint(cmd.OutOrStdout(), leandomain.Brief(records, args, mode))
			if hard > 0 {
				return ErrLeanValidationIssues
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&modelPath, "model", "", "Path to the lean ADR directory (default: docs/decisions)")
	cmd.Flags().BoolVar(&hook, "hook", false, "Claude Code hook mode: read hook JSON from stdin and inject the brief (default: the edited file, PreToolUse)")
	cmd.Flags().BoolVar(&digest, "digest", false, "With --hook (SessionStart): inject the session-open digest — titles-only tripwire index, capped at 2 KB. Without --hook: print the digest and report every ladder rung's size to stderr")
	cmd.Flags().BoolVar(&whole, "whole", false, "Hook mode (SubagentStart for planner agents): inject the whole-corpus brief — every in-force ADR")
	cmd.Flags().BoolVar(&invariants, "invariants", false, "Hook mode (SubagentStart): inject the invariants-only brief")
	cmd.Flags().BoolVar(&staged, "staged", false, "Hook mode (PreToolUse on `git commit`): brief the staged files; block only on a forbidden-scope hit")
	cmd.Flags().BoolVar(&guard, "guard", false, "Hook mode (PreToolUse on Write/Edit): guard the ADR model — block hand-creating a record, warn on editing one")
	cmd.Flags().BoolVar(&full, "full", false, "Render every governing ADR in full, with scope/matched detail (debuggable)")
	cmd.Flags().BoolVar(&compact, "compact", false, "Render defaults as a one-line checklist; invariants and forbidden-path hits stay full")
	cmd.MarkFlagsMutuallyExclusive("full", "compact")
	return cmd
}

// runHook is fully fail-open: a misconfigured model, an empty directory, or a
// malformed payload all inject nothing and exit 0, so the hook never breaks an edit.
// The mode flags select which brief the hook injects: --digest (SessionStart), --whole
// (SubagentStart for planners), --invariants (SubagentStart), --staged (commit-time
// advisory), or the default file-scoped brief (PreToolUse on an edit). Rendering and
// routing stay in the domain (ADR-0002/0001).
func runHook(cmd *cobra.Command, modelPath string, digest, whole, invariants, staged, guard bool) error {
	resolved := util.ResolveModelPath(modelPath)
	payload, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return nil
	}
	// The guard decides from the payload and the path alone; it never reads the records.
	if guard {
		if out := modelGuard(payload, resolved); out != "" {
			fmt.Fprintln(cmd.OutOrStdout(), out)
		}
		return nil
	}
	records, err := leandomain.LoadDir(resolved)
	if err != nil {
		return nil
	}
	var out string
	switch {
	case digest:
		out = leandomain.SessionDigest(records, payload)
	case whole:
		out = leandomain.SessionBrief(records, payload)
	case invariants:
		out = leandomain.SubagentBrief(records, payload)
	case staged:
		out = stagedAdvisory(records, payload)
	default:
		out = leandomain.HookContext(records, payload)
	}
	if out != "" {
		fmt.Fprintln(cmd.OutOrStdout(), out)
	}
	return nil
}

// boolCount returns how many of the flags are set — used to reject conflicting hook modes.
func boolCount(bs ...bool) int {
	n := 0
	for _, b := range bs {
		if b {
			n++
		}
	}
	return n
}

// gitCommitRe detects a `git commit` invocation in a Bash command — a heuristic,
// fail-open trigger for the advisor, tolerating git global flags between `git` and
// the subcommand (`git -c user.email=x -c user.name=y commit`, `git -C /repo
// commit`, `git --no-pager commit`). Over-matching only costs a spurious advisory;
// under-matching silently skips the commit brief, so err toward matching.
var gitCommitRe = regexp.MustCompile(`\bgit\b(\s+-[\w-]+(=\S+|\s+\S+)?)*\s+commit\b`)

// stagedAdvisory is the commit-time advisor: on a `git commit` Bash call it routes the
// staged files to the ADRs and returns the brief (or a block on a forbidden-scope hit,
// per CommitAdvisory). Fully fail-open — any parse or git error injects nothing and
// never blocks the commit. It is a thin git-plumbing shell; routing and rendering stay
// in the domain (leandomain.CommitAdvisory).
func stagedAdvisory(records []leandomain.Record, payload []byte) string {
	var in struct {
		CWD       string `json:"cwd"`
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal(payload, &in); err != nil {
		return ""
	}
	if !gitCommitRe.MatchString(in.ToolInput.Command) {
		return ""
	}
	staged := stagedFiles(in.CWD)
	if len(staged) == 0 {
		return ""
	}
	return leandomain.CommitAdvisory(records, staged)
}

// modelGuard is the ADR-model guard: on a Write/Edit whose target is a lean ADR record
// directly under the model dir, it blocks a hand-authored *new* record (Write to a
// not-yet-existing NNNN-*.md) and warns on an *edit* to an existing one. It is a thin
// path/existence shell; the deny/advisory decision is CommitAdvisory's sibling in the
// domain (leandomain.ModelGuard). Fail-open: a non-record path or any parse error yields
// nothing. It does not dedup.
func modelGuard(payload []byte, modelPath string) string {
	var in struct {
		CWD       string `json:"cwd"`
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			FilePath string `json:"file_path"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal(payload, &in); err != nil {
		return ""
	}
	file := strings.TrimSpace(in.ToolInput.FilePath)
	if file == "" {
		return ""
	}
	absModel := modelPath
	if !filepath.IsAbs(absModel) && in.CWD != "" {
		absModel = filepath.Join(in.CWD, absModel)
	}
	rel, err := filepath.Rel(absModel, file)
	// Records are flat NNNN-*.md directly in the model dir: reject "../" (outside) and
	// any nested path (a separator in rel).
	if err != nil || strings.HasPrefix(rel, "..") || strings.ContainsRune(rel, filepath.Separator) {
		return ""
	}
	_, statErr := os.Stat(file)
	return leandomain.ModelGuard(in.ToolName, leandomain.IsRecordFile(filepath.Base(file)), statErr == nil)
}

// stagedFiles returns the staged (added/copied/modified) paths in dir, or nil on any
// error — the advisor stays fail-open and never blocks a commit on a git hiccup.
func stagedFiles(dir string) []string {
	c := exec.Command("git", "diff", "--cached", "--name-only", "--diff-filter=ACM")
	if dir != "" {
		c.Dir = dir
	}
	out, err := c.Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			files = append(files, s)
		}
	}
	return files
}

// reportLeanIssues prints validation issues to w in the same [FAIL]/[warn] form as
// `adg index`, returning the number of hard failures (so a caller can exit non-zero).
func reportLeanIssues(w io.Writer, issues []leandomain.Issue) int {
	hard := 0
	for _, is := range issues {
		kind := "FAIL"
		if is.Warning {
			kind = "warn"
		} else {
			hard++
		}
		fmt.Fprintf(w, "[%s] %s: %s\n", kind, is.ID, is.Message)
	}
	return hard
}
