package lean

import (
	"fmt"
	"github.com/d3i-infra/adg/v4/internal/domain/decision/madr"
	"strings"
	"testing"
)

func briefRec(id, filename, priority string, appliesTo []string, body string) Record {
	return briefRecX(id, filename, priority, appliesTo, nil, nil, nil, body)
}

func briefRecX(id, filename, priority string, appliesTo, excludes, forbids, companions []string, body string) Record {
	return Record{
		ID:       id,
		Filename: filename,
		Body:     body,
		D: madr.Decision{
			Status: "accepted", Priority: priority,
			AppliesTo: appliesTo, Excludes: excludes, Forbids: forbids, Companions: companions,
		},
	}
}

func TestBrief_RoutesGroupsAndChecks(t *testing.T) {
	records := []Record{
		briefRec("0001", "0001-meta.md", "default", nil,
			"# Meta\n\n## Decision\n\nProcess only.\n\n## Implication\n\nN/A.\n"),
		briefRec("0002", "0002-imports.md", "default", []string{"port/**/*.py"},
			"# No cross-layer private imports\n\n## Decision\n\nNo underscore imports across layers.\n\n## Implication\n\n- Review rejects `from x import _y` across layers.\n"),
		briefRec("0003", "0003-uploads.md", "default", []string{"**/flow_builder.py"},
			"# Reject unsafe uploads\n\n## Decision\n\nSafety before validation.\n\n## Guidance\n\n- Call check_file_safety first.\n\n## Checks\n\n- Confirm safety runs before extraction.\n"),
		briefRec("0004", "0004-pii.md", "invariant", []string{"**/*.py"},
			"# PII safety boundary\n\n## Decision\n\nCatch all exceptions at the boundary.\n\n## Implication\n\n- Do not remove the handler.\n\n## Checks\n\n- grep for new bridge.sendLogs paths.\n"),
	}

	out := Brief(records, []string{"port/helpers/flow_builder.py"}, BriefFull)

	// 0001 (no applies_to) must not route.
	if strings.Contains(out, "ADR-0001") {
		t.Errorf("meta ADR with no applies_to should not route:\n%s", out)
	}
	// Invariant section precedes defaults; 0004 is the invariant.
	iHard := strings.Index(out, "## Hard constraints")
	iDef := strings.Index(out, "## Defaults & conventions")
	i4 := strings.Index(out, "ADR-0004")
	i2 := strings.Index(out, "ADR-0002")
	if !(iHard >= 0 && iHard < iDef) {
		t.Errorf("invariants section should precede defaults:\n%s", out)
	}
	if !(iHard < i4 && i4 < iDef && iDef < i2) {
		t.Errorf("0004 should be under invariants, 0002 under defaults:\n%s", out)
	}
	// Matched-pattern explainability.
	if !strings.Contains(out, "matched: **/flow_builder.py") {
		t.Errorf("expected matched-pattern annotation for 0003:\n%s", out)
	}
	// Guidance synonym: 0002 uses ## Implication, surfaced as Guidance bullets.
	if !strings.Contains(out, "Review rejects") {
		t.Errorf("expected 0002 Implication surfaced as guidance:\n%s", out)
	}
	// The post-edit footer re-runs the gate and consolidates checks from 0003 and 0004.
	if !strings.Contains(out, "## Before you finish") ||
		!strings.Contains(out, "adg lean index --root .") ||
		!strings.Contains(out, "(ADR-0003) Confirm safety runs before extraction.") ||
		!strings.Contains(out, "(ADR-0004) grep for new bridge.sendLogs paths.") {
		t.Errorf("expected a post-edit footer with consolidated checks:\n%s", out)
	}
}

func TestBrief_PathFiltersOutNonMatches(t *testing.T) {
	records := []Record{
		briefRec("0003", "0003-uploads.md", "default", []string{"**/flow_builder.py"},
			"# Uploads\n\n## Decision\n\nx\n\n## Implication\n\n- y\n"),
		briefRec("0004", "0004-pii.md", "invariant", []string{"**/*.py"},
			"# PII\n\n## Decision\n\nx\n\n## Implication\n\n- y\n"),
	}
	out := Brief(records, []string{"port/script.py"}, BriefFull)
	if strings.Contains(out, "ADR-0003") {
		t.Errorf("0003 should not match port/script.py:\n%s", out)
	}
	if !strings.Contains(out, "ADR-0004") {
		t.Errorf("0004 (**/*.py) should match port/script.py:\n%s", out)
	}
}

func TestBulletLines_FoldsWrappedContinuations(t *testing.T) {
	section := "## Implication\n\n- first bullet that wraps\n  onto a second line\n- second bullet\n"
	got := bulletLines(section)
	want := []string{"first bullet that wraps onto a second line", "second bullet"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("bulletLines folding wrong:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestBrief_NoMatches(t *testing.T) {
	records := []Record{
		briefRec("0004", "0004-pii.md", "invariant", []string{"**/*.py"},
			"# PII\n\n## Decision\n\nx\n\n## Implication\n\n- y\n"),
	}
	out := Brief(records, []string{"docs/notes.md"}, BriefFull)
	if !strings.Contains(out, "No ADRs match these paths") {
		t.Errorf("expected no-match message:\n%s", out)
	}
}

func TestRouteMatch_ExcludesSuppressesGovernedPath(t *testing.T) {
	// 0008: govern helpers under port, but the sanctioned construction home
	// port_helpers.py must not fire (the real worked-around case).
	r := briefRecX("0008", "0008-x.md", "default",
		[]string{"port/**/*.py"}, []string{"**/port_helpers.py"}, nil, nil, "")
	got := routeMatch(r, []string{"port/helpers/flow_builder.py", "port/helpers/port_helpers.py"})
	if len(got.matched) != 1 || got.matched[0] != "port/**/*.py" {
		t.Errorf("matched = %v, want [port/**/*.py]", got.matched)
	}
	if len(got.excluded) != 1 || got.excluded[0] != "**/port_helpers.py" {
		t.Errorf("excluded = %v, want [**/port_helpers.py]", got.excluded)
	}
	if len(got.governed) != 1 || got.governed[0] != "port/helpers/flow_builder.py" {
		t.Errorf("governed = %v, want [port/helpers/flow_builder.py]", got.governed)
	}
}

func TestRouteMatch_DedupAcrossPaths(t *testing.T) {
	r := briefRecX("0002", "0002-x.md", "default", []string{"port/**/*.py"}, nil, nil, nil, "")
	got := routeMatch(r, []string{"port/a.py", "port/b.py"})
	if len(got.matched) != 1 {
		t.Errorf("matched should dedup to one pattern; got %v", got.matched)
	}
	if len(got.governed) != 2 {
		t.Errorf("expected both paths governed; got %v", got.governed)
	}
}

func TestRouteMatch_ExcludesOnNonGovernedPathNotReported(t *testing.T) {
	r := briefRecX("0008", "0008-x.md", "default",
		[]string{"port/**/*.py"}, []string{"docs/**"}, nil, nil, "")
	got := routeMatch(r, []string{"docs/x.md"})
	if len(got.matched) != 0 || len(got.excluded) != 0 || len(got.governed) != 0 {
		t.Errorf("excludes must not be reported on a path applies_to never governs; got %+v", got)
	}
}

func TestRouteMatch_ForbidsIndependentOfApplies(t *testing.T) {
	r := briefRecX("0011", "0011-x.md", "invariant",
		nil, nil, []string{"port/extraction/**/*.py"}, nil, "")
	got := routeMatch(r, []string{"port/extraction/new.py"})
	if len(got.forbidden) != 1 || got.forbidden[0] != "port/extraction/**/*.py" {
		t.Errorf("forbidden = %v, want [port/extraction/**/*.py]", got.forbidden)
	}
	if len(got.matched) != 0 {
		t.Errorf("no applies_to, so matched should be empty; got %v", got.matched)
	}
}

func TestBrief_ExcludedAnnotation(t *testing.T) {
	r := briefRecX("0008", "0008-x.md", "default",
		[]string{"port/**/*.py"}, []string{"**/port_helpers.py"}, nil, nil,
		"# Pages\n\n## Decision\n\nBuild via helpers.\n\n## Guidance\n\n- x\n")
	out := Brief([]Record{r}, []string{"port/helpers/flow_builder.py", "port/helpers/port_helpers.py"}, BriefFull)
	if !strings.Contains(out, "excluded: **/port_helpers.py") {
		t.Errorf("expected excluded annotation in scope line:\n%s", out)
	}
}

func TestBrief_ForbiddenWarningRendered(t *testing.T) {
	r := briefRecX("0011", "0011-x.md", "invariant",
		nil, nil, []string{"port/extraction/**/*.py"}, nil,
		"# Single architecture\n\n## Decision\n\nOne extraction path.\n\n## Guidance\n\n- x\n")
	out := Brief([]Record{r}, []string{"port/extraction/new.py"}, BriefFull)
	if !strings.Contains(out, "Forbidden scope matched") || !strings.Contains(out, "ADR-0011") {
		t.Errorf("expected forbidden-scope warning:\n%s", out)
	}
}

func TestBrief_ForbiddenOnlyNoEmptyMatched(t *testing.T) {
	r := briefRecX("0011", "0011-x.md", "invariant",
		nil, nil, []string{"port/extraction/**/*.py"}, nil,
		"# Single architecture\n\n## Decision\n\nx\n\n## Guidance\n\n- x\n")
	out := Brief([]Record{r}, []string{"port/extraction/new.py"}, BriefFull)
	// The scope-line annotation (`· matched: …`) must be absent for a forbids-only
	// hit; "Forbidden scope matched:" is the separate warning line and is expected.
	if strings.Contains(out, "· matched:") {
		t.Errorf("forbidden-only entry must not render an empty matched: annotation:\n%s", out)
	}
	if !strings.Contains(out, "scope: forbids port/extraction/**/*.py") {
		t.Errorf("expected a forbids scope line:\n%s", out)
	}
}

func TestBrief_CompanionsRenderedWithSoftNote(t *testing.T) {
	r := briefRecX("0009", "0009-x.md", "default",
		[]string{"api/d3i_props.py"}, nil, nil, []string{"packages/data-collector/src/App.tsx"},
		"# Props\n\n## Decision\n\nx\n\n## Guidance\n\n- x\n")
	// Touch the governed file but not the companion -> soft note appears.
	out := Brief([]Record{r}, []string{"api/d3i_props.py"}, BriefFull)
	if !strings.Contains(out, "Related files to consider:") ||
		!strings.Contains(out, "packages/data-collector/src/App.tsx") {
		t.Errorf("expected companions line:\n%s", out)
	}
	if !strings.Contains(out, "none of the changed paths are among these") {
		t.Errorf("expected soft note when the companion is untouched:\n%s", out)
	}
	// Touch the companion too -> no soft note.
	out2 := Brief([]Record{r}, []string{"api/d3i_props.py", "packages/data-collector/src/App.tsx"}, BriefFull)
	if strings.Contains(out2, "none of the changed paths are among these") {
		t.Errorf("did not expect the soft note when the companion is touched:\n%s", out2)
	}
}

func TestBrief_CompanionsOnlyDoesNotRoute(t *testing.T) {
	r := briefRecX("0009", "0009-x.md", "default",
		nil, nil, nil, []string{"src/App.tsx"},
		"# Props\n\n## Decision\n\nx\n\n## Guidance\n\n- x\n")
	out := Brief([]Record{r}, []string{"src/App.tsx"}, BriefFull)
	if !strings.Contains(out, "No ADRs match these paths") {
		t.Errorf("a companions-only record must never route:\n%s", out)
	}
}

func TestBrief_AutoCompactsOverCeiling(t *testing.T) {
	// Ten defaults all governing the same hub path: rendered full, the brief blows
	// the one-screen budget, so BriefAuto must re-render the defaults compact.
	var records []Record
	for i := 1; i <= 10; i++ {
		id := fmt.Sprintf("%04d", i)
		records = append(records, briefRec(id, id+"-x.md", "default", []string{"port/**/*.py"},
			fmt.Sprintf("# Rule %s\n\n## Decision\n\nDecision text for %s.\n\n## Guidance\n\n- Do rule %s.\n", id, id, id)))
	}
	paths := []string{"port/x.py"}

	full := Brief(records, paths, BriefFull)
	if briefLineCount(full) <= MaxBriefLines {
		t.Fatalf("setup: full brief should exceed MaxBriefLines (%d); got %d lines", MaxBriefLines, briefLineCount(full))
	}

	auto := Brief(records, paths, BriefAuto)
	if !strings.Contains(auto, "## Defaults & conventions (condensed") {
		t.Errorf("auto brief over the ceiling should use the condensed defaults header:\n%s", auto)
	}
	if strings.Contains(auto, "**Decision:**") {
		t.Errorf("condensed defaults must not render full Decision blocks:\n%s", auto)
	}
	if !strings.Contains(auto, "- ADR-0005 Rule 0005: Do rule 0005. → 0005-x.md") {
		t.Errorf("expected a compact checklist line for a default:\n%s", auto)
	}
	// A small brief (one default) must stay full under auto.
	small := Brief(records[:1], paths, BriefAuto)
	if !strings.Contains(small, "**Decision:**") {
		t.Errorf("a brief under the ceiling should stay full under auto:\n%s", small)
	}
}

func TestBrief_CompactDefaultLineSummary(t *testing.T) {
	withGuidance := briefRec("0001", "0001-x.md", "default", []string{"port/**/*.py"},
		"# Layering\n\n## Decision\n\nA very long decision paragraph we do not want in the compact line.\n\n## Guidance\n\n- No upward `port` imports; same-layer imports are OK.\n")
	onlyDecision := briefRec("0002", "0002-x.md", "default", []string{"port/**/*.py"},
		"# Naming\n\n## Decision\n\nUse snake_case for module files.\n")
	longBullet := briefRec("0003", "0003-x.md", "default", []string{"port/**/*.py"},
		"# Big\n\n## Guidance\n\n- "+strings.Repeat("x", 400)+"\n")

	out := Brief([]Record{withGuidance, onlyDecision, longBullet}, []string{"port/x.py"}, BriefCompact)

	if !strings.Contains(out, "- ADR-0001 Layering: No upward `port` imports; same-layer imports are OK. → 0001-x.md") {
		t.Errorf("compact line should prefer the first Guidance bullet:\n%s", out)
	}
	if strings.Contains(out, "long decision paragraph") {
		t.Errorf("compact line must not fall back to Decision when Guidance exists:\n%s", out)
	}
	if !strings.Contains(out, "- ADR-0002 Naming: Use snake_case for module files. → 0002-x.md") {
		t.Errorf("compact line should fall back to Decision when there is no Guidance:\n%s", out)
	}
	if !strings.Contains(out, "…") {
		t.Errorf("an over-long summary should be truncated with an ellipsis:\n%s", out)
	}
}

func TestBrief_CompactKeepsForbiddenDefaultFull(t *testing.T) {
	// A default-priority record (not an invariant) whose forbids glob is violated
	// must still render in full under compact mode — negative space stays prominent.
	r := briefRecX("0007", "0007-x.md", "default",
		nil, nil, []string{"port/legacy/**/*.py"}, nil,
		"# No legacy pipeline\n\n## Decision\n\nOne pipeline only.\n\n## Guidance\n\n- Do not add a second pipeline.\n")
	out := Brief([]Record{r}, []string{"port/legacy/new.py"}, BriefCompact)
	if !strings.Contains(out, "Forbidden scope matched") {
		t.Errorf("a forbidden-path hit must stay full even for a default in compact mode:\n%s", out)
	}
	if !strings.Contains(out, "### ADR-0007") {
		t.Errorf("a forbidden default should render as a full entry, not a compact line:\n%s", out)
	}
}

func TestBrief_CompactAggregatesCompanions(t *testing.T) {
	r := briefRecX("0009", "0009-x.md", "default",
		[]string{"api/d3i_props.py"}, nil, nil, []string{"packages/data-collector/src/App.tsx"},
		"# Props\n\n## Decision\n\nx\n\n## Guidance\n\n- Register the prop.\n")
	out := Brief([]Record{r}, []string{"api/d3i_props.py"}, BriefCompact)
	if !strings.Contains(out, "## Related files to consider") {
		t.Errorf("compact mode should aggregate companions into a section:\n%s", out)
	}
	if !strings.Contains(out, "- ADR-0009: packages/data-collector/src/App.tsx") {
		t.Errorf("aggregated companions should list the ADR and its globs:\n%s", out)
	}
	if strings.Contains(out, "**Related files to consider:**") {
		t.Errorf("compact mode should not also inline companions inside the entry:\n%s", out)
	}
}

func TestBrief_FooterNamesTestsAndReRun(t *testing.T) {
	// A test in applies_to (the test enforces the rule) + a companion test (partner
	// edit); the non-test applies_to glob must not be listed as a test.
	r := briefRecX("0016", "0016-x.md", "invariant",
		[]string{"port/uploads.py", "packages/python/tests/test_uploads.py"}, nil, nil,
		[]string{"packages/python/tests/test_flow_builder.py"},
		"# Streaming invariant\n\n## Decision\n\nStream uploads; never read the whole file.\n\n## Guidance\n\n- Pass the adapter to zipfile.\n")
	out := Brief([]Record{r}, []string{"port/uploads.py"}, BriefFull)

	if !strings.Contains(out, "## Before you finish") || !strings.Contains(out, "Re-run `adg lean index --root .`") {
		t.Errorf("expected the post-edit re-run line:\n%s", out)
	}
	if !strings.Contains(out, "Run the tests these ADRs name") ||
		!strings.Contains(out, "packages/python/tests/test_uploads.py") ||
		!strings.Contains(out, "packages/python/tests/test_flow_builder.py") {
		t.Errorf("expected named tests (from applies_to and companions) in the footer:\n%s", out)
	}
}

func TestBrief_TerminalRecordsDoNotRoute(t *testing.T) {
	rec := func(id, status string) Record {
		return Record{ID: id, Filename: id + "-x.md",
			D:    madr.Decision{Status: status, AppliesTo: []string{"port/**/*.py"}},
			Body: "# Rule " + id + "\n\n## Decision\n\nx.\n\n## Guidance\n\n- y.\n"}
	}
	records := []Record{
		rec("0001", "superseded by ADR-0004"),
		rec("0002", "deprecated"),
		rec("0003", "rejected"),
		rec("0004", "accepted"), // the in-force replacement
	}
	out := Brief(records, []string{"port/x.py"}, BriefFull)
	for _, gone := range []string{"ADR-0001", "ADR-0002", "ADR-0003"} {
		if strings.Contains(out, gone) {
			t.Errorf("terminal record %s must not route into the brief:\n%s", gone, out)
		}
	}
	if !strings.Contains(out, "ADR-0004") {
		t.Errorf("the in-force record should still route:\n%s", out)
	}
}

func TestMatches_TerminalForbidsDoesNotTrigger(t *testing.T) {
	r := Record{ID: "0001", Filename: "0001-x.md",
		D: madr.Decision{Status: "superseded by ADR-0002", Forbids: []string{"port/legacy/**/*.py"}}}
	if Matches([]Record{r}, []string{"port/legacy/new.py"}) {
		t.Errorf("a superseded record's forbids must not trip the hook gate")
	}
	r.D.Status = "accepted"
	if !Matches([]Record{r}, []string{"port/legacy/new.py"}) {
		t.Errorf("an in-force forbids should trip the gate")
	}
}

func TestMatches_ForbiddenTriggersGate(t *testing.T) {
	r := briefRecX("0011", "0011-x.md", "invariant",
		nil, nil, []string{"port/extraction/**/*.py"}, nil, "")
	if !Matches([]Record{r}, []string{"port/extraction/new.py"}) {
		t.Errorf("a forbidden-path edit should trigger the hook gate")
	}
	if Matches([]Record{r}, []string{"port/other.py"}) {
		t.Errorf("a non-matching path should not trigger the gate")
	}
}
