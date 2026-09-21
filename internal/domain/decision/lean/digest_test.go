package lean

import (
	"encoding/json"
	"fmt"
	"github.com/d3i-infra/adg/internal/domain/decision/madr"
	"strings"
	"testing"
	"unicode/utf8"
)

func digestRec(id, status, priority, category, title string, appliesTo []string) Record {
	return Record{
		ID:       id,
		Filename: id + "-x.md",
		Body: "# " + title + "\n\n## Decision\n\nThe rule.\n\n## Guidance\n\n- Do the thing.\n\n" +
			"## Why\n\nWhySentinelNeverRendered.\n",
		D: madr.Decision{Status: status, Priority: priority, Category: category, AppliesTo: appliesTo},
	}
}

// digestCorpus builds n records, alternating invariant/default, spread over two
// categories, with realistic ~40-char titles.
func digestCorpus(n int) []Record {
	recs := make([]Record, 0, n)
	for i := 1; i <= n; i++ {
		prio, cat := "default", "State machine"
		if i%2 == 0 {
			prio, cat = "invariant", "Architecture"
		}
		recs = append(recs, digestRec(fmt.Sprintf("%04d", i), "accepted", prio, cat,
			fmt.Sprintf("Keep rule %04d short and imperative always", i),
			[]string{fmt.Sprintf("src/area%d/**/*.go", i%3)}))
	}
	return recs
}

func TestBriefDigest_FullRungGroupsMarkersAndLifecycle(t *testing.T) {
	recs := []Record{
		digestRec("0001", "accepted", "default", "State machine", "Return row-change counts from mutators", []string{"src/state/**/*.rs"}),
		digestRec("0002", "accepted", "invariant", "Architecture", "Persist artifacts before mark_succeeded", []string{"src/output/**/*.rs"}),
		digestRec("0003", "superseded by ADR-0004", "invariant", "Architecture", "Old retired rule", []string{"src/old/**"}),
		digestRec("0004", "accepted", "invariant", "Architecture", "Cancel per-request via a polled AtomicBool", []string{"src/output/lib.rs"}),
		digestRec("0005", "accepted", "default", "", "Split plans into per-task files", nil),
	}

	out := BriefDigest(recs)
	if out == "" || len(out) > MaxDigestBytes {
		t.Fatalf("digest empty or over budget (%d bytes)", len(out))
	}
	// Contract line counts in-force records only (4, of which 2 invariants in force).
	if !strings.Contains(out, "ADR digest — 4 records (2 invariants)") {
		t.Errorf("contract line wrong; got:\n%s", out)
	}
	// Markers: invariants get !, defaults do not.
	for _, want := range []string{" !0002 ", " !0004 ", " 0001 ", " 0005 "} {
		if !strings.Contains(out, want) {
			t.Errorf("missing line prefix %q in:\n%s", want, out)
		}
	}
	// Terminal record governs nothing (ADR-0012): absent from every rung.
	if strings.Contains(out, "0003") || strings.Contains(out, "Old retired rule") {
		t.Errorf("superseded record leaked into the digest:\n%s", out)
	}
	// Group order mirrors RenderIndex: lowest contained ID first, Uncategorized last.
	state := strings.Index(out, "STATE MACHINE")
	arch := strings.Index(out, "ARCHITECTURE")
	uncat := strings.Index(out, strings.ToUpper(uncategorized))
	if state < 0 || arch < 0 || uncat < 0 || !(state < arch && arch < uncat) {
		t.Errorf("group order wrong (state=%d arch=%d uncat=%d):\n%s", state, arch, uncat, out)
	}
	// Scope hints: literal directory prefixes, subsumed and deduped.
	if !strings.Contains(out, "STATE MACHINE  (src/state/)") {
		t.Errorf("missing scope hint for state machine group:\n%s", out)
	}
	if !strings.Contains(out, "ARCHITECTURE  (src/output/)") {
		t.Errorf("missing deduped scope hint for architecture group:\n%s", out)
	}
	// Recall, not instruction: no filenames, no section prose, no Why.
	for _, banned := range []string{"-x.md", "**Decision:**", "The rule.", "Do the thing.", "WhySentinelNeverRendered"} {
		if strings.Contains(out, banned) {
			t.Errorf("digest must not contain %q:\n%s", banned, out)
		}
	}
}

func TestBriefDigest_AlwaysUnderCeiling(t *testing.T) {
	for _, n := range []int{1, 5, 20, 40, 200, 500} {
		out := BriefDigest(digestCorpus(n))
		if out == "" {
			t.Errorf("n=%d: digest is empty", n)
		}
		if len(out) > MaxDigestBytes {
			t.Errorf("n=%d: digest is %d bytes (> %d)", n, len(out), MaxDigestBytes)
		}
	}
	// A single record with a pathologically long title still fits.
	giant := []Record{digestRec("0001", "accepted", "invariant", "Meta",
		strings.Repeat("verylongtitle ", 400), nil)}
	if out := BriefDigest(giant); out == "" || len(out) > MaxDigestBytes {
		t.Errorf("giant-title corpus: %d bytes", len(BriefDigest(giant)))
	}
}

func TestBriefDigest_InvariantOnlyRung(t *testing.T) {
	// 15 invariants + 80 defaults: the full digest busts the budget, the
	// invariant-only digest fits.
	var recs []Record
	for i := 1; i <= 15; i++ {
		recs = append(recs, digestRec(fmt.Sprintf("%04d", i), "accepted", "invariant", "Core",
			fmt.Sprintf("Hold invariant %04d firmly", i), nil))
	}
	for i := 16; i <= 95; i++ {
		recs = append(recs, digestRec(fmt.Sprintf("%04d", i), "accepted", "default", "Conventions",
			fmt.Sprintf("Follow convention %04d as the default", i), nil))
	}
	if full := renderDigest(recs, 95, 15, false); len(full) <= MaxDigestBytes {
		t.Fatalf("setup: grouped digest should exceed the budget, got %d bytes", len(full))
	}
	if flat := renderDigestFlat(recs, 95, 15); len(flat) <= MaxDigestBytes {
		t.Fatalf("setup: flat digest should exceed the budget, got %d bytes", len(flat))
	}

	out := BriefDigest(recs)
	if len(out) > MaxDigestBytes {
		t.Fatalf("digest is %d bytes (> %d)", len(out), MaxDigestBytes)
	}
	for i := 1; i <= 15; i++ {
		if !strings.Contains(out, fmt.Sprintf("!%04d ", i)) {
			t.Errorf("invariant %04d missing from invariant-only rung:\n%s", i, out)
		}
	}
	if strings.Contains(out, "0016") || strings.Contains(out, "Follow convention") {
		t.Errorf("default records must not render on the invariant-only rung:\n%s", out)
	}
	if !strings.Contains(out, "Plus 80 defaults & conventions — see docs/decisions/README.md.") {
		t.Errorf("missing defaults-count closing line:\n%s", out)
	}
}

func TestBriefDigest_FlatRung(t *testing.T) {
	// A finely-categorized corpus: header overhead pushes the grouped rung over
	// budget while the same records rendered flat fit. Every record its own
	// category — the pathological version of what consolidation can't fully fix.
	var recs []Record
	for i := 1; i <= 30; i++ {
		recs = append(recs, digestRec(fmt.Sprintf("%04d", i), "accepted",
			map[bool]string{true: "invariant", false: "default"}[i%2 == 0],
			fmt.Sprintf("Category number %02d", i),
			fmt.Sprintf("Keep rule %04d short, imperative, and tight", i),
			[]string{fmt.Sprintf("src/area%02d/**/*.go", i)}))
	}
	if grouped := renderDigest(recs, 30, 15, false); len(grouped) <= MaxDigestBytes {
		t.Fatalf("setup: grouped digest should exceed the budget, got %d bytes", len(grouped))
	}

	out := BriefDigest(recs)
	if len(out) > MaxDigestBytes {
		t.Fatalf("digest is %d bytes (> %d)", len(out), MaxDigestBytes)
	}
	// Flat rung: every record visible, invariant markers kept, no headers/hints.
	for i := 1; i <= 30; i++ {
		if !strings.Contains(out, fmt.Sprintf("%04d ", i)) {
			t.Errorf("record %04d missing from flat rung:\n%s", i, out)
		}
	}
	if !strings.Contains(out, " !0002 ") || strings.Contains(out, " !0001 ") {
		t.Errorf("invariant markers wrong on flat rung:\n%s", out)
	}
	if strings.Contains(out, "CATEGORY NUMBER") || strings.Contains(out, "(src/area") {
		t.Errorf("flat rung must not render group headers or scope hints:\n%s", out)
	}
	if strings.Contains(out, "Plus ") {
		t.Errorf("flat rung must not carry the invariant-only closing line:\n%s", out)
	}
}

func TestDigestReport_SizesAndSelection(t *testing.T) {
	// Small corpus: grouped fits and is selected; every rung reports a size.
	small := digestCorpus(4)
	report := DigestReport(small)
	if len(report) != 4 {
		t.Fatalf("expected 4 rungs (grouped, flat, invariants-only, floor), got %+v", report)
	}
	if report[0].Name != "grouped" || !report[0].Selected {
		t.Errorf("small corpus should select the grouped rung: %+v", report)
	}
	for _, r := range report {
		if r.Bytes <= 0 {
			t.Errorf("rung %s reports no size: %+v", r.Name, report)
		}
	}
	if !report[len(report)-1].Fits {
		t.Errorf("the floor must always fit: %+v", report)
	}
	// Exactly one rung selected, and it matches what BriefDigest returns.
	selected := ""
	for _, r := range report {
		if r.Selected {
			if selected != "" {
				t.Fatalf("two rungs selected: %+v", report)
			}
			selected = r.Name
		}
	}
	if selected != "grouped" || len(BriefDigest(small)) != report[0].Bytes {
		t.Errorf("selection disagrees with BriefDigest: %+v", report)
	}
	if DigestReport(nil) != nil {
		t.Errorf("empty corpus should report nil")
	}
}

func TestBriefDigest_FloorRung(t *testing.T) {
	// Even the invariants alone bust the budget: only the floor fits.
	var recs []Record
	for i := 1; i <= 120; i++ {
		recs = append(recs, digestRec(fmt.Sprintf("%04d", i), "accepted", "invariant", "Core",
			fmt.Sprintf("Hold invariant %04d with a fairly long title attached", i), nil))
	}
	out := BriefDigest(recs)
	if out == "" || len(out) > MaxDigestBytes {
		t.Fatalf("floor empty or over budget (%d bytes)", len(out))
	}
	if !strings.Contains(out, "120 lean records (120 invariants)") || !strings.Contains(out, "too many to list inline") {
		t.Errorf("floor should carry counts and the pointer:\n%s", out)
	}
	if strings.Contains(out, "0001 Hold invariant") {
		t.Errorf("floor must not list records:\n%s", out)
	}
}

func TestBriefDigest_TitleTruncationRespectsRunes(t *testing.T) {
	// Multi-byte title over MaxTitleRunes truncates on runes with an ellipsis and
	// never splits a UTF-8 sequence.
	title := strings.Repeat("é", MaxTitleRunes+20)
	out := BriefDigest([]Record{digestRec("0001", "accepted", "invariant", "Meta", title, nil)})
	if !utf8.ValidString(out) {
		t.Fatalf("digest contains invalid UTF-8")
	}
	if !strings.Contains(out, "…") {
		t.Errorf("over-long title should truncate with an ellipsis:\n%s", out)
	}
	if strings.Contains(out, title) {
		t.Errorf("full over-long title must not survive:\n%s", out)
	}
	// A title at exactly the cap is never clipped.
	exact := strings.Repeat("x", MaxTitleRunes)
	out = BriefDigest([]Record{digestRec("0001", "accepted", "default", "Meta", exact, nil)})
	if !strings.Contains(out, exact) || strings.Contains(out, "…") {
		t.Errorf("cap-length title must render unclipped:\n%s", out)
	}
}

func TestBriefDigest_EmptyAndTerminalOnlyCorpora(t *testing.T) {
	if out := BriefDigest(nil); out != "" {
		t.Errorf("empty corpus should render nothing, got:\n%s", out)
	}
	terminal := []Record{
		digestRec("0001", "superseded by ADR-0002", "invariant", "Meta", "Old", nil),
		digestRec("0002", "deprecated", "default", "Meta", "Older", nil),
	}
	if out := BriefDigest(terminal); out != "" {
		t.Errorf("terminal-only corpus should render nothing, got:\n%s", out)
	}
}

func TestSessionDigest_ReflectsEventAndWrapsDigest(t *testing.T) {
	recs := digestCorpus(4)
	out := SessionDigest(recs, []byte(`{"hook_event_name":"SubagentStart"}`))
	var env struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not a hook envelope: %v\n%s", err, out)
	}
	if env.HookSpecificOutput.HookEventName != "SubagentStart" {
		t.Errorf("event = %q, want reflected SubagentStart", env.HookSpecificOutput.HookEventName)
	}
	if env.HookSpecificOutput.AdditionalContext != BriefDigest(recs) {
		t.Errorf("additionalContext should be exactly BriefDigest output")
	}
	if got := SessionDigest(nil, []byte(`{}`)); got != "" {
		t.Errorf("empty corpus should inject nothing, got %q", got)
	}
}

func TestValidate_LongTitleWarns(t *testing.T) {
	long := strings.Repeat("é", MaxTitleRunes+1) // runes, not bytes: 65 runes, 130 bytes
	body := "# " + long + "\n\n## Decision\n\nWe do X.\n\n## Guidance\n\n- do x\n\n## Why\n\nBecause Z drifts.\n"
	issues := Validate([]Record{leanRec("0001", "accepted", "default", body)})
	if !hasIssue(issues, "runes (>") {
		t.Errorf("expected a title-length warning; got: %+v", issues)
	}

	// Exactly at the cap: no warning (and 64 runes of multi-byte is >64 bytes,
	// proving the count is runes).
	exact := strings.Repeat("é", MaxTitleRunes)
	body = "# " + exact + "\n\n## Decision\n\nWe do X.\n\n## Guidance\n\n- do x\n\n## Why\n\nBecause Z drifts.\n"
	if issues := Validate([]Record{leanRec("0001", "accepted", "default", body)}); hasIssue(issues, "runes (>") {
		t.Errorf("cap-length title must not warn; got: %+v", issues)
	}

	// Terminal records are skipped by the leanness block.
	body = "# " + long + "\n\n## Decision\n\nWe do X.\n"
	if issues := Validate([]Record{leanRec("0001", "deprecated", "default", body)}); hasIssue(issues, "runes (>") {
		t.Errorf("terminal record must not earn the title warning; got: %+v", issues)
	}
}
