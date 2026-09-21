package lean

import (
	"github.com/d3i-infra/adg/internal/domain/decision/madr"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
		full := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func lintRec(id, priority, status string, appliesTo []string) Record {
	return lintRecX(id, priority, status, appliesTo, nil, nil)
}

func lintRecX(id, priority, status string, appliesTo, excludes, forbids []string) Record {
	return Record{
		ID: id,
		D:  madr.Decision{Status: status, Priority: priority, AppliesTo: appliesTo, Excludes: excludes, Forbids: forbids},
	}
}

func TestLintTree_StaleNoOverlap(t *testing.T) {
	root := writeTree(t, "port/script.py", "port/helpers/flow_builder.py")

	records := []Record{
		lintRec("0002", "default", "accepted", []string{"port/**/*.py"}),
		lintRec("0003", "default", "accepted", []string{"**/flow_builder.py", "**/uploads.py"}),
		lintRec("0004", "invariant", "accepted", []string{"**/*.py"}),
	}

	issues, err := LintTree(records, root)
	if err != nil {
		t.Fatalf("LintTree errored: %v", err)
	}

	var stale bool
	for _, is := range issues {
		if is.ID == "0003" && strings.Contains(is.Message, "uploads.py") && strings.Contains(is.Message, "stale") {
			stale = true
		}
		// Overlap moved to the opt-in Overlaps diagnostic — LintTree must never
		// emit an overlap message (even though 0002/0003 share flow_builder.py).
		if strings.Contains(is.Message, "overlap") {
			t.Errorf("LintTree should no longer report overlap; got: %s", is.Message)
		}
	}
	if !stale {
		t.Errorf("expected stale-scope warning for 0003 **/uploads.py; got: %+v", issues)
	}
}

func TestOverlaps_SummaryGroupsByADRSet(t *testing.T) {
	root := writeTree(t, "port/a.py", "port/b.py", "port/c.py")
	records := []Record{
		lintRec("0002", "default", "accepted", []string{"port/**/*.py"}),
		lintRec("0003", "default", "accepted", []string{"port/**/*.py"}),
		lintRec("0004", "invariant", "accepted", []string{"port/**/*.py"}),
	}
	out, err := Overlaps(records, root, OverlapSummary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[info] scope hubs:") {
		t.Errorf("expected the scope-hubs header:\n%s", out)
	}
	// All three files are governed by exactly {0002,0003}, so they collapse into a
	// single grouped line rather than one line per file.
	if !strings.Contains(out, "3 files: 2 defaults apply: ADR-0002, ADR-0003") {
		t.Errorf("expected files grouped by identical default set:\n%s", out)
	}
	if strings.Contains(out, "ADR-0004") {
		t.Errorf("invariants must be excluded from the overlap summary:\n%s", out)
	}
	if !strings.Contains(out, "examples:") {
		t.Errorf("expected example files in the group:\n%s", out)
	}
}

func TestOverlaps_OffReturnsEmpty(t *testing.T) {
	root := writeTree(t, "port/a.py")
	records := []Record{
		lintRec("0002", "default", "accepted", []string{"port/**/*.py"}),
		lintRec("0003", "default", "accepted", []string{"port/**/*.py"}),
	}
	out, err := Overlaps(records, root, OverlapOff)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("OverlapOff should produce no output; got:\n%s", out)
	}
}

func TestOverlaps_PairsReportsUnrelated(t *testing.T) {
	root := writeTree(t, "port/helpers/flow_builder.py")
	records := []Record{
		lintRec("0002", "default", "accepted", []string{"port/**/*.py"}),
		lintRec("0003", "default", "accepted", []string{"**/flow_builder.py"}),
	}
	out, err := Overlaps(records, root, OverlapPairs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ADR-0002 overlaps ADR-0003") {
		t.Errorf("expected a pairwise overlap between 0002 and 0003; got:\n%s", out)
	}
}

func TestOverlaps_PairsExcludesRelated(t *testing.T) {
	root := writeTree(t, "lib/a.py")
	records := []Record{
		lintRec("0001", "default", "superseded by ADR-0002", []string{"lib/**/*.py"}),
		lintRec("0002", "default", "accepted", []string{"lib/**/*.py"}),
	}
	// 0002 supersedes 0001 (declared relationship) and 0001 is superseded
	// (inactive), so no overlap should be reported.
	records[1].D.Supersedes = []string{"0001"}
	out, err := Overlaps(records, root, OverlapPairs)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "overlaps") {
		t.Errorf("superseded/related records should not be flagged as overlap; got:\n%s", out)
	}
}

func TestLintTree_ExcludesStale(t *testing.T) {
	root := writeTree(t, "port/a.py")
	records := []Record{
		lintRecX("0008", "default", "accepted", []string{"port/**/*.py"}, []string{"port/**/nonexistent.py"}, nil),
	}
	issues, err := LintTree(records, root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, is := range issues {
		if is.ID == "0008" && is.Warning && strings.Contains(is.Message, "excludes") && strings.Contains(is.Message, "stale") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a stale excludes warning; got: %+v", issues)
	}
}

func TestOverlaps_ExcludesSubtractionPreventsPhantom(t *testing.T) {
	root := writeTree(t, "port/gen/x.py")
	records := []Record{
		// 0001 governs port/** but disclaims port/gen/**; 0002 governs port/gen/**.
		// Their only common file is excluded from 0001, so there is no real overlap.
		lintRecX("0001", "default", "accepted", []string{"port/**/*.py"}, []string{"port/gen/**/*.py"}, nil),
		lintRecX("0002", "default", "accepted", []string{"port/gen/**/*.py"}, nil, nil),
	}
	out, err := Overlaps(records, root, OverlapPairs)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "overlaps") {
		t.Errorf("an excluded file must not produce a phantom overlap; got:\n%s", out)
	}
}

func TestLintTree_ForbidsHasFilesWarn(t *testing.T) {
	root := writeTree(t, "port/extraction/old.py")
	records := []Record{
		lintRecX("0011", "invariant", "accepted", nil, nil, []string{"port/extraction/**/*.py"}),
	}
	issues, err := LintTree(records, root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, is := range issues {
		if is.ID == "0011" && is.Warning && strings.Contains(is.Message, "forbidden path now has files") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a forbids-has-files warning; got: %+v", issues)
	}
}

func TestLintTree_ForbidsNoFilesNoWarnAndNotStale(t *testing.T) {
	root := writeTree(t, "port/keep.py")
	records := []Record{
		lintRecX("0011", "invariant", "accepted", nil, nil, []string{"port/extraction/**/*.py"}),
	}
	issues, err := LintTree(records, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range issues {
		if is.ID == "0011" {
			t.Errorf("forbids matching nothing is healthy — no stale and no has-files; got: %s", is.Message)
		}
	}
}
