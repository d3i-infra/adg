package lean

import (
	"fmt"
	util "github.com/d3i-infra/adg/internal/adapter/command"
	"github.com/d3i-infra/adg/internal/domain/decision"
	leandomain "github.com/d3i-infra/adg/internal/domain/decision/lean"
	"github.com/d3i-infra/adg/internal/domain/decision/madr"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// NewLeanNewCommand wires `adg lean new`: it authors a lean ADR from flags so
// records are not hand-written. It assigns the next flat-global NNNN (or an
// explicit --id), builds the frontmatter, and either scaffolds the body or reads
// it from stdin (prepending the H1 from --title). It validates the candidate
// against the model BEFORE writing and refuses on a hard failure, so an invalid
// record never lands on disk; on success it writes the record and regenerates the
// README. stdout carries only the new ID; status and warnings go to stderr.
func NewLeanNewCommand() *cobra.Command {
	var (
		modelPath, title, id, status, priority, category, source, date string
		appliesTo, excludes, forbids, companions, tags                 []string
		fromStdin                                                      bool
	)

	cmd := &cobra.Command{
		Use:   "new",
		Short: "Author a lean ADR (frontmatter + body) and validate before writing",
		Long: `New authors a lean ADR from flags instead of hand-writing the file. It assigns
the next flat-global NNNN (or --id), builds the frontmatter, and either scaffolds
the body or reads it from stdin (--from-stdin), prepending the H1 from --title. It
validates the candidate against the model and refuses to write on a hard failure,
so an invalid record never lands on disk; on success it writes the record and
regenerates the README. stdout is the new ID; status and warnings go to stderr.`,
		SilenceErrors: true, // issues already printed to stderr
		SilenceUsage:  true, // failure is data, not user error
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(title) == "" {
				err := fmt.Errorf("--title is required")
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

			// ID: explicit (collision-checked) or the next free NNNN.
			if strings.TrimSpace(id) != "" {
				norm, nerr := util.NormalizeID(id)
				if nerr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", nerr)
					return nerr
				}
				for _, r := range records {
					if r.ID == norm {
						e := fmt.Errorf("ID %s is already taken by %s", norm, r.Filename)
						fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", e)
						return e
					}
				}
				id = norm
			} else {
				id = leandomain.NextID(records)
			}

			slug, err := decision.Slugify(title)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
				return err
			}
			filename := fmt.Sprintf("%s-%s.md", id, slug)

			if strings.TrimSpace(date) == "" {
				date = time.Now().Format("2006-01-02")
			}

			// Body: provided on stdin (with the H1 injected from --title) or the scaffold.
			var body string
			if fromStdin {
				raw, rerr := io.ReadAll(cmd.InOrStdin())
				if rerr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error: reading stdin: %v\n", rerr)
					return rerr
				}
				body, err = leandomain.EnsureTitle(string(raw), title)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
					return err
				}
			} else {
				body = leandomain.RenderNewBodyFor(title, priority)
			}

			d := madr.Decision{
				Status:     status,
				Date:       date,
				Category:   category,
				Priority:   priority,
				Source:     source,
				AppliesTo:  appliesTo,
				Excludes:   excludes,
				Forbids:    forbids,
				Companions: companions,
				Tags:       tags,
			}
			candidate := leandomain.Record{ID: id, Filename: filename, D: d, Body: body}

			// Validate the candidate against the model BEFORE writing; refuse on a
			// hard failure attributable to the candidate so nothing lands on disk.
			all := append(append([]leandomain.Record{}, records...), candidate)
			hard := 0
			for _, is := range leandomain.ValidateWithBudget(all, budgetFor(cmd, resolved)) {
				if is.ID != id {
					continue // only the candidate's findings gate this write
				}
				kind := "warn"
				if !is.Warning {
					kind = "FAIL"
					hard++
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "[%s] %s: %s\n", kind, is.ID, is.Message)
			}
			if hard > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "refusing to write %s: %d hard failure(s)\n", filename, hard)
				return ErrLeanValidationIssues
			}

			content, err := madr.RenderFile(d, body)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: rendering record: %v\n", err)
				return err
			}
			out := filepath.Join(resolved, filename)
			if werr := os.WriteFile(out, []byte(content), 0o644); werr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: writing %s: %v\n", out, werr)
				return werr
			}

			// Regenerate the README — derived output, never an input (ADR 0009).
			readme := filepath.Join(resolved, "README.md")
			if werr := os.WriteFile(readme, []byte(leandomain.RenderIndex(all)), 0o644); werr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: writing %s: %v\n", readme, werr)
				return werr
			}

			fmt.Fprintln(cmd.OutOrStdout(), id)
			fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s; regenerated README.md\n", out)
			return nil
		},
	}

	cmd.Flags().StringVar(&modelPath, "model", "", "Path to the lean ADR directory (default: docs/decisions)")
	cmd.Flags().StringVar(&title, "title", "", "Decision title — sets the H1 and the filename slug (required)")
	cmd.Flags().StringVar(&id, "id", "", "Optional explicit ID (1-9999, zero-padded); fails if already taken")
	cmd.Flags().StringVar(&status, "status", "proposed", "Status (proposed | accepted | rejected | deprecated)")
	cmd.Flags().StringVar(&priority, "priority", "", "Priority (invariant | default); invariant adds a Why scaffold")
	cmd.Flags().StringVar(&category, "category", "", "Category — groups the generated index")
	cmd.Flags().StringVar(&source, "source", "", "Provenance note")
	cmd.Flags().StringArrayVar(&appliesTo, "applies-to", nil, "Routing glob (repeatable)")
	cmd.Flags().StringArrayVar(&excludes, "excludes", nil, "Exclude glob (repeatable)")
	cmd.Flags().StringArrayVar(&forbids, "forbids", nil, "Forbid glob (repeatable)")
	cmd.Flags().StringArrayVar(&companions, "companions", nil, "Companion glob (repeatable)")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Tag (repeatable)")
	cmd.Flags().BoolVar(&fromStdin, "from-stdin", false, "Read the body (Decision/Guidance/...) from stdin; the H1 is set from --title")
	cmd.Flags().StringVar(&date, "date", "", "Record date YYYY-MM-DD (default: today) — preserves the original date when migrating an ADR")
	return cmd
}
