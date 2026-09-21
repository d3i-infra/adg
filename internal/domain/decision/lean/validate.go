package lean

import (
	"fmt"
	"github.com/d3i-infra/adg/internal/domain/decision/madr"
	"regexp"
	"slices"
	"strings"
)

// Record bundles everything the validator and index generator need about one
// lean ADR: its ID, on-disk filename (for index links), parsed frontmatter, and
// raw body.
type Record struct {
	ID       string
	Filename string
	D        madr.Decision
	Body     string
}

// Issue is one validation finding. Warning issues are advisory (e.g. the
// one-screen length nudge); non-warning issues are failures.
type Issue struct {
	ID      string
	Message string
	Warning bool
}

var (
	statusRe       = regexp.MustCompile(`^(proposed|accepted|rejected|deprecated|superseded by ADR-\d{4}|amended by ADR-\d{4})$`)
	supersededByRe = regexp.MustCompile(`^superseded by ADR-(\d{4})$`)
	amendedByRe    = regexp.MustCompile(`^amended by ADR-(\d{4})$`)
	// listItemRe matches a Markdown list item (-, *, +, or ordered "1.").
	listItemRe = regexp.MustCompile(`(?m)^\s*([-*+]|\d+\.)\s+`)
	// codeSpanRe matches an inline code span, so word counts ignore identifiers.
	codeSpanRe = regexp.MustCompile("`[^`]*`")
)

// Validate runs lean checks under the default (one-screen) body budget.
func Validate(records []Record) []Issue {
	return ValidateWithBudget(records, DefaultBudget())
}

// ValidateWithBudget runs lean-shape and integrity checks under an explicit body
// budget. The budget governs only the whole-body one-screen length nudge; every other
// check is budget-independent. Body checks (required sections, leftover scaffolding)
// apply only to accepted records; relationship integrity is checked across the set.
func ValidateWithBudget(records []Record, budget Budget) []Issue {
	var issues []Issue
	issues = append(issues, duplicateIDIssues(records)...)

	byID := make(map[string]Record, len(records))
	for _, r := range records {
		byID[r.ID] = r
	}
	for _, r := range records {
		issues = append(issues, validateOne(r, byID, budget)...)
	}
	return issues
}

// duplicateIDIssues flags ADRs that resolve to the same NNNN. The ID model is a
// flat global NNNN across the whole model — `category` frontmatter (not a
// subfolder) does the grouping — so a collision is always a hard error. The
// relationship checks below read a last-wins byID map and so would mis-report
// under duplicates, but the run is already non-zero and the author must fix the
// ID before those findings can be trusted.
func duplicateIDIssues(records []Record) []Issue {
	files := map[string][]string{}
	var order []string
	for _, r := range records {
		if _, seen := files[r.ID]; !seen {
			order = append(order, r.ID)
		}
		files[r.ID] = append(files[r.ID], r.Filename)
	}
	var issues []Issue
	for _, id := range order {
		if fs := files[id]; len(fs) > 1 {
			issues = append(issues, Issue{
				ID:      id,
				Message: fmt.Sprintf("duplicate ID %s used by %d files (%s); IDs must be a unique flat global NNNN across the whole model", id, len(fs), strings.Join(fs, ", ")),
			})
		}
	}
	return issues
}

func validateOne(r Record, byID map[string]Record, budget Budget) []Issue {
	var issues []Issue
	add := func(msg string) { issues = append(issues, Issue{ID: r.ID, Message: msg}) }
	warn := func(msg string) { issues = append(issues, Issue{ID: r.ID, Message: msg, Warning: true}) }

	status := strings.TrimSpace(r.D.Status)
	if status != "" && !statusRe.MatchString(status) {
		add(fmt.Sprintf("status %q is not valid lean vocabulary (proposed, accepted, rejected, deprecated, superseded by ADR-NNNN, amended by ADR-NNNN)", status))
	}

	switch strings.TrimSpace(r.D.Priority) {
	case "", "invariant", "default":
	default:
		add(fmt.Sprintf("priority %q is not valid (invariant, default, or unset)", r.D.Priority))
	}

	// Glob syntax + hygiene across every routing field. Brace expansion is a hard
	// failure (the matcher treats `{` `}` as literals, so a brace glob silently
	// routes to nothing) with a concrete fix; a single-star segment under a
	// nestable directory is an advisory nudge.
	for _, field := range []struct {
		name  string
		globs []string
	}{
		{"applies_to", r.D.AppliesTo},
		{"excludes", r.D.Excludes},
		{"forbids", r.D.Forbids},
		{"companions", r.D.Companions},
	} {
		for _, g := range field.globs {
			if strings.ContainsAny(g, "{}") {
				add(fmt.Sprintf("%s glob %q uses brace expansion, which is not supported; write separate globs (one per alternative)", field.name, g))
			}
			if sug, ok := singleStarNested(g); ok {
				warn(fmt.Sprintf("%s glob %q uses a single-star segment under a directory; it won't match nested paths — did you mean %q?", field.name, g, sug))
			}
		}
	}

	// Companions surface only when the record also routes; companions with no
	// applies_to and no forbids can never appear in a brief.
	if len(r.D.Companions) > 0 && len(r.D.AppliesTo) == 0 && len(r.D.Forbids) == 0 {
		warn("companions set but the record has no applies_to or forbids, so it never routes and the companions never surface")
	}

	// Executable checks: each must carry a compilable grep and a valid expect; a
	// malformed check would silently never run, so these are hard issues.
	for i, c := range r.D.Checks {
		if strings.TrimSpace(c.Grep) == "" {
			add(fmt.Sprintf("checks[%d] has no grep pattern", i))
		} else if _, err := regexp.Compile(c.Grep); err != nil {
			add(fmt.Sprintf("checks[%d] grep %q is not a valid regexp: %v", i, c.Grep, err))
		}
		switch strings.TrimSpace(c.Expect) {
		case "", "absent", "present":
		default:
			add(fmt.Sprintf("checks[%d] expect %q is not valid (absent | present)", i, c.Expect))
		}
		if strings.TrimSpace(c.Desc) == "" {
			warn(fmt.Sprintf("checks[%d] has no desc; add a human-readable statement of what it verifies", i))
		}
		for _, g := range append(append([]string{}, c.In...), c.Except...) {
			if strings.ContainsAny(g, "{}") {
				add(fmt.Sprintf("checks[%d] glob %q uses brace expansion, which is not supported; write separate globs", i, g))
			}
		}
	}

	p := ParseBody(r.Body)
	if strings.TrimSpace(p.Title) == "" {
		add("H1 title is missing or empty")
	}

	// An accepted ADR is a finished record: both core sections written, no
	// leftover scaffolding. Proposed drafts are exempt.
	if status == "accepted" {
		if sectionEmpty(p, "decision") {
			add("missing or empty required section: Decision")
		}
		if guidanceEmpty(p) {
			add("missing or empty required section: Guidance")
		}
		if sectionEmpty(p, "why") {
			add("missing or empty required section: Why — state why the rule exists / what it protects, so a reader without the author's context can generalize (not why it's a nice design)")
		}
		for _, tok := range PlaceholderTokens {
			if strings.Contains(r.Body, tok) {
				add(fmt.Sprintf("body still contains template placeholder %q; fill it in before accepting", tok))
			}
		}
		if strings.TrimSpace(r.D.Category) == "" {
			warn("no category set; the ADR will be grouped under \"Uncategorized\" in the index")
		}
	}

	if budget.WarnWholeBody {
		if n := bodyLineCount(r.Body); n > budget.MaxBodyLines {
			warn(fmt.Sprintf("body is %d lines (> %d); a lean ADR should fit one screen — consider splitting", n, budget.MaxBodyLines))
		}
	}

	// Leanness nudges (advisory). They run on records still in force and skip
	// sections that are empty or still hold scaffold placeholders, so a fresh draft
	// is not nagged about content it has not written yet.
	if inForce(status) {
		// Title-length nudge (ADR-0022): the session digest renders only the title,
		// so an over-long one is either clipped there or crowding out other records.
		if t := strings.TrimSpace(p.Title); t != "" {
			if n := len([]rune(t)); n > MaxTitleRunes {
				warn(fmt.Sprintf("title is %d runes (> %d); the session digest shows only the title — write it as a short imperative instruction and move detail into the body", n, MaxTitleRunes))
			}
		}
		if d, ok := filledSection(p, "decision"); ok {
			if hasListItem(d) {
				warn("Decision contains a list; a Decision is the rule in prose (1–3 sentences) — per-case detail belongs in Guidance")
			}
			if n := decisionWordCount(d); n > MaxDecisionWords {
				warn(fmt.Sprintf("Decision is %d words (> %d); a lean Decision is the rule in 1–3 sentences — move detail to Guidance", n, MaxDecisionWords))
			}
		}
		if g := strings.TrimSpace(guidanceSection(p)); g != "" && !containsPlaceholder(g) && !hasListItem(g) {
			warn("Guidance has no list item; lead with reviewable bullets (the first bullet is what a compact brief renders)")
		}
		// Nudge a not-yet-accepted draft toward its rationale before the accepted gate
		// above turns it into a hard failure. filledSection also catches the {...}
		// placeholder, so a fresh scaffold is reminded. (ADR-0005: warning tier here,
		// hard-failure tier once accepted.)
		if status != "accepted" {
			if _, ok := filledSection(p, "why"); !ok {
				warn("no Why yet; before accepting, state why the rule exists / what it protects so a reader without your context can generalize")
			}
		}
	}

	// Supersession integrity (forward + reverse), mirroring adg's MADR validator.
	if m := supersededByRe.FindStringSubmatch(status); m != nil {
		succID := m[1]
		succ, ok := byID[succID]
		if !ok {
			add(fmt.Sprintf("status references ADR-%s but no such ADR exists", succID))
		} else if !slices.Contains(succ.D.Supersedes, r.ID) {
			add(fmt.Sprintf("status says superseded by ADR-%s, but ADR-%s's supersedes list does not include %s", succID, succID, r.ID))
		}
	}
	for _, predID := range r.D.Supersedes {
		pred, ok := byID[predID]
		if !ok {
			add(fmt.Sprintf("supersedes %s but no such ADR exists", predID))
			continue
		}
		want := fmt.Sprintf("superseded by ADR-%s", r.ID)
		if strings.TrimSpace(pred.D.Status) != want {
			add(fmt.Sprintf("supersedes %s but ADR-%s status is %q, not %q", predID, predID, pred.D.Status, want))
		}
	}

	// Amendment integrity (forward + reverse) — the machine-checkable version of
	// Ferry's prose "Amended by" annotation.
	if m := amendedByRe.FindStringSubmatch(status); m != nil {
		amID := m[1]
		am, ok := byID[amID]
		if !ok {
			add(fmt.Sprintf("status references ADR-%s but no such ADR exists", amID))
		} else if !slices.Contains(am.D.Amends, r.ID) {
			add(fmt.Sprintf("status says amended by ADR-%s, but ADR-%s's amends list does not include %s", amID, amID, r.ID))
		}
	}
	for _, baseID := range r.D.Amends {
		if _, ok := byID[baseID]; !ok {
			add(fmt.Sprintf("amends %s but no such ADR exists", baseID))
		}
	}

	return issues
}

func sectionEmpty(p Parsed, key string) bool {
	body, ok := p.Sections[key]
	return !ok || strings.TrimSpace(body) == ""
}

// filledSection returns a section's trimmed content when it has real content to
// lint — i.e. it is present, non-empty, and not still a scaffold placeholder.
func filledSection(p Parsed, key string) (string, bool) {
	s := strings.TrimSpace(p.Sections[key])
	if s == "" || containsPlaceholder(s) {
		return "", false
	}
	return s, true
}

// containsPlaceholder reports whether s still holds any template scaffolding token.
func containsPlaceholder(s string) bool {
	for _, tok := range PlaceholderTokens {
		if strings.Contains(s, tok) {
			return true
		}
	}
	return false
}

func hasListItem(s string) bool { return listItemRe.MatchString(s) }

// decisionWordCount counts the words of a Decision, with inline code spans removed
// so a rule that names several identifiers is not penalized for them.
func decisionWordCount(s string) int {
	return len(strings.Fields(codeSpanRe.ReplaceAllString(s, " ")))
}

// guidanceSection returns the Guidance section body, accepting the legacy
// "Implication" header as an alias. Guidance is canonical; the two are synonyms.
func guidanceSection(p Parsed) string {
	if g, ok := p.Sections["guidance"]; ok {
		return g
	}
	return p.Sections["implication"]
}

func guidanceEmpty(p Parsed) bool {
	return strings.TrimSpace(guidanceSection(p)) == ""
}

func bodyLineCount(body string) int {
	return len(strings.Split(strings.TrimRight(body, "\n"), "\n"))
}

// singleStarNested reports whether a glob uses a single-star final segment under a
// directory (e.g. platforms/*.py), which silently misses nested paths, and returns
// the recursive form (platforms/**/*.py) to suggest. It deliberately inspects only
// the final segment, so a root-level glob (*.py, main.py) and a mid-path literal
// tail (a/*/b.py) are not flagged.
func singleStarNested(pat string) (string, bool) {
	if strings.Contains(pat, "**") {
		return "", false
	}
	i := strings.LastIndex(pat, "/")
	if i < 0 {
		return "", false
	}
	if !strings.Contains(pat[i+1:], "*") {
		return "", false
	}
	return pat[:i] + "/**/" + pat[i+1:], true
}
