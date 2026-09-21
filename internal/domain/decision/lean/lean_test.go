package lean

import (
	"github.com/d3i-infra/adg/internal/domain/decision/madr"
	"strings"
	"testing"
)

func rec(id, filename, status, category string, body string, supersedes, amends []string) Record {
	return Record{
		ID:       id,
		Filename: filename,
		Body:     body,
		D:        madr.Decision{Status: status, Category: category, Supersedes: supersedes, Amends: amends},
	}
}

func acceptedBody(title string) string {
	return "# " + title + "\n\n## Decision\n\nWe do X.\n\n## Implication\n\n- New code must do Y.\n\n## Why\n\nWithout it, Z drifts and later code can't tell right from wrong.\n"
}

func TestParseBody_SectionsAndTitle(t *testing.T) {
	p := ParseBody(acceptedBody("Use X"))
	if p.Title != "Use X" {
		t.Errorf("title = %q, want %q", p.Title, "Use X")
	}
	if got := p.Sections["decision"]; got != "We do X." {
		t.Errorf("decision = %q", got)
	}
	if got := p.Sections["implication"]; got != "- New code must do Y." {
		t.Errorf("implication = %q", got)
	}
}

func TestValidate_AcceptedHappyPath(t *testing.T) {
	issues := Validate([]Record{rec("0001", "0001-x.md", "accepted", "Meta", acceptedBody("Use X"), nil, nil)})
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got: %+v", issues)
	}
}

func hasIssue(issues []Issue, substr string) bool {
	for _, i := range issues {
		if strings.Contains(i.Message, substr) {
			return true
		}
	}
	return false
}

// hasHardIssue reports a non-warning (blocking) issue matching substr.
func hasHardIssue(issues []Issue, substr string) bool {
	for _, i := range issues {
		if !i.Warning && strings.Contains(i.Message, substr) {
			return true
		}
	}
	return false
}

// leanRec builds a record with a priority set (rec() does not), for the leanness lints.
func leanRec(id, status, priority, body string) Record {
	return Record{ID: id, Filename: id + "-x.md", Body: body,
		D: madr.Decision{Status: status, Priority: priority, Category: "Meta"}}
}

func TestValidate_DecisionContainsListWarns(t *testing.T) {
	body := "# T\n\n## Decision\n\n- one rule\n- another rule\n\n## Guidance\n\n- do x\n"
	issues := Validate([]Record{leanRec("0001", "accepted", "default", body)})
	if !hasIssue(issues, "Decision contains a list") {
		t.Errorf("expected a Decision-contains-a-list warning; got: %+v", issues)
	}
}

func TestValidate_DecisionTooLongWarns(t *testing.T) {
	body := "# T\n\n## Decision\n\n" + strings.Repeat("word ", 70) + "\n\n## Guidance\n\n- do x\n"
	issues := Validate([]Record{leanRec("0001", "accepted", "default", body)})
	if !hasIssue(issues, "words (>") {
		t.Errorf("expected a Decision-too-long warning; got: %+v", issues)
	}
}

func TestValidate_DecisionWordCountIgnoresCodeSpans(t *testing.T) {
	// A long inline code span must not count toward the Decision word limit.
	body := "# T\n\n## Decision\n\nUse `" + strings.Repeat("x", 400) + "` for the thing.\n\n## Guidance\n\n- do x\n"
	if issues := Validate([]Record{leanRec("0001", "accepted", "default", body)}); hasIssue(issues, "words (>") {
		t.Errorf("inline code spans must not count toward the Decision word limit; got: %+v", issues)
	}
}

func TestValidate_GuidanceNoListItemWarns(t *testing.T) {
	body := "# T\n\n## Decision\n\nWe do X.\n\n## Guidance\n\nJust prose, no bullets here.\n"
	issues := Validate([]Record{leanRec("0001", "accepted", "default", body)})
	if !hasIssue(issues, "Guidance has no list item") {
		t.Errorf("expected a Guidance-no-list-item warning; got: %+v", issues)
	}
}

func TestValidate_AcceptedWithoutWhyHardFails(t *testing.T) {
	// Reasoning is now co-equal with Decision/Guidance: an accepted record with no
	// Why is a hard failure, regardless of priority or routing.
	body := "# T\n\n## Decision\n\nWe do X.\n\n## Guidance\n\n- do x\n"
	for _, priority := range []string{"invariant", "default"} {
		issues := Validate([]Record{leanRec("0001", "accepted", priority, body)})
		if !hasHardIssue(issues, "missing or empty required section: Why") {
			t.Errorf("accepted %s without Why should hard-fail; got: %+v", priority, issues)
		}
	}
	withWhy := body + "\n## Why\n\nRemoving it silently breaks the privacy boundary.\n"
	if issues := Validate([]Record{leanRec("0001", "accepted", "invariant", withWhy)}); hasIssue(issues, "required section: Why") {
		t.Errorf("a populated ## Why should clear the failure; got: %+v", issues)
	}
}

func TestValidate_ProposedWithoutWhyWarnsOnly(t *testing.T) {
	body := "# T\n\n## Decision\n\nWe do X.\n\n## Guidance\n\n- do x\n"
	issues := Validate([]Record{leanRec("0001", "proposed", "default", body)})
	if !hasIssue(issues, "no Why yet") {
		t.Errorf("proposed without Why should warn; got: %+v", issues)
	}
	if hasHardIssue(issues, "Why") {
		t.Errorf("proposed without Why must not hard-fail; got: %+v", issues)
	}
}

func TestValidate_TerminalWithoutWhyExempt(t *testing.T) {
	body := "# T\n\n## Decision\n\nWe do X.\n\n## Guidance\n\n- do x\n"
	if issues := Validate([]Record{leanRec("0001", "deprecated", "default", body)}); hasIssue(issues, "Why") {
		t.Errorf("a terminal record must not be nudged for a Why; got: %+v", issues)
	}
}

func TestValidate_LeanLintsSkipPlaceholders(t *testing.T) {
	// A fresh default-priority scaffold is all {...} placeholders — nothing to lint.
	scaffold := RenderNewBodyFor("T", "")
	for _, i := range Validate([]Record{leanRec("0001", "proposed", "default", scaffold)}) {
		if strings.Contains(i.Message, "Decision ") || strings.Contains(i.Message, "Guidance has no list") {
			t.Errorf("scaffold placeholders must not trip leanness lints; got: %s", i.Message)
		}
	}
}

func TestValidate_LeanLintsSkipTerminalRecords(t *testing.T) {
	// A deprecated record with a listy Decision is frozen history, not a nudge target.
	body := "# T\n\n## Decision\n\n- a\n- b\n\n## Guidance\n\nprose\n"
	for _, i := range Validate([]Record{leanRec("0001", "deprecated", "default", body)}) {
		if strings.Contains(i.Message, "Decision contains a list") || strings.Contains(i.Message, "Guidance has no list") {
			t.Errorf("terminal records should be exempt from leanness lints; got: %s", i.Message)
		}
	}
}

func TestValidate_AcceptedMissingImplicationAndToken(t *testing.T) {
	body := "# T\n\n## Decision\n\n{...}\n"
	issues := Validate([]Record{rec("0001", "0001-t.md", "accepted", "Meta", body, nil, nil)})
	var msgs []string
	for _, i := range issues {
		msgs = append(msgs, i.Message)
	}
	joined := strings.Join(msgs, "\n")
	for _, want := range []string{"Guidance", "template placeholder"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected an issue mentioning %q; got:\n%s", want, joined)
		}
	}
}

func TestValidate_ProposedDraftExemptFromBodyChecks(t *testing.T) {
	body := "# T\n\n## Decision\n\n{...}\n\n## Implication\n\n{...}\n"
	issues := Validate([]Record{rec("0001", "0001-t.md", "proposed", "", body, nil, nil)})
	for _, i := range issues {
		if !i.Warning {
			t.Errorf("proposed draft should not hard-fail; got: %+v", i)
		}
	}
}

func TestValidate_BadStatusVocabulary(t *testing.T) {
	issues := Validate([]Record{rec("0001", "0001-t.md", "Superseded by ADR 0002", "Meta", acceptedBody("T"), nil, nil)})
	found := false
	for _, i := range issues {
		if strings.Contains(i.Message, "not valid lean vocabulary") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected invalid-vocabulary issue; got: %+v", issues)
	}
}

func TestValidate_SupersessionIntegrity(t *testing.T) {
	// 0001 says it is superseded by 0002, and 0002 correctly lists 0001.
	old := rec("0001", "0001-old.md", "superseded by ADR-0002", "Meta", acceptedBody("Old"), nil, nil)
	newOK := rec("0002", "0002-new.md", "accepted", "Meta", acceptedBody("New"), []string{"0001"}, nil)
	if issues := Validate([]Record{old, newOK}); len(issues) != 0 {
		t.Fatalf("consistent supersession should pass; got: %+v", issues)
	}

	// Now break the reverse link: 0002 forgets to list 0001.
	newBad := rec("0002", "0002-new.md", "accepted", "Meta", acceptedBody("New"), nil, nil)
	issues := Validate([]Record{old, newBad})
	found := false
	for _, i := range issues {
		if strings.Contains(i.Message, "supersedes list does not include") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected supersession reverse-link issue; got: %+v", issues)
	}
}

func TestValidate_AmendmentIntegrity(t *testing.T) {
	base := rec("0001", "0001-base.md", "amended by ADR-0002", "Meta", acceptedBody("Base"), nil, nil)
	amender := rec("0002", "0002-amend.md", "accepted", "Meta", acceptedBody("Amend"), nil, []string{"0001"})
	if issues := Validate([]Record{base, amender}); len(issues) != 0 {
		t.Fatalf("consistent amendment should pass; got: %+v", issues)
	}

	amenderBad := rec("0002", "0002-amend.md", "accepted", "Meta", acceptedBody("Amend"), nil, nil)
	issues := Validate([]Record{base, amenderBad})
	found := false
	for _, i := range issues {
		if strings.Contains(i.Message, "amends list does not include") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected amendment reverse-link issue; got: %+v", issues)
	}
}

func TestValidate_LengthWarning(t *testing.T) {
	var b strings.Builder
	b.WriteString("# T\n\n## Decision\n\nx\n\n## Implication\n\n")
	for i := 0; i < MaxBodyLines+5; i++ {
		b.WriteString("* line\n")
	}
	issues := Validate([]Record{rec("0001", "0001-t.md", "accepted", "Meta", b.String(), nil, nil)})
	found := false
	for _, i := range issues {
		if i.Warning && strings.Contains(i.Message, "one screen") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected one-screen length warning; got: %+v", issues)
	}
}

func TestRenderIndex_GroupingAndAnnotations(t *testing.T) {
	records := []Record{
		rec("0001", "0001-framework.md", "accepted", "Meta", acceptedBody("ADR framework"), nil, nil),
		rec("0009", "0009-cqrs.md", "accepted", "CQRS + event sourcing", acceptedBody("CQRS + event sourcing"), nil, nil),
		rec("0014", "0014-audit.md", "superseded by ADR-0039", "CQRS + event sourcing", acceptedBody("Audit: projection-only"), nil, nil),
		rec("0039", "0039-audit2.md", "accepted", "Data layer", acceptedBody("Audit two-channel"), []string{"0014"}, nil),
		rec("0002", "0002-privacy.md", "amended by ADR-0054", "Invariants", acceptedBody("Privacy invariant"), nil, nil),
		rec("0054", "0054-direct.md", "accepted", "Implementation strategy", acceptedBody("Direct delivery"), nil, []string{"0002"}),
	}
	out := RenderIndex(records)

	// Meta (0001) must precede CQRS (min 0009) which must precede Data layer (0039).
	iMeta := strings.Index(out, "### Meta")
	iCQRS := strings.Index(out, "### CQRS + event sourcing")
	iData := strings.Index(out, "### Data layer")
	if !(iMeta >= 0 && iMeta < iCQRS && iCQRS < iData) {
		t.Errorf("group order wrong: meta=%d cqrs=%d data=%d\n%s", iMeta, iCQRS, iData, out)
	}
	if !strings.Contains(out, "~~[0014 — Audit: projection-only](./0014-audit.md)~~ — *superseded by ADR 0039*") {
		t.Errorf("superseded entry not struck through/annotated:\n%s", out)
	}
	if !strings.Contains(out, "*(amended by ADR 0054)*") {
		t.Errorf("amended entry not annotated:\n%s", out)
	}
}

// LoadDir + Validate end-to-end against the relocated broken example: an
// accepted record missing Guidance, carrying a leftover {...} token, and with no
// category must produce the corresponding failures and warning.
func TestLoadDir_BrokenExampleFailsValidation(t *testing.T) {
	records, err := LoadDir("testdata/broken")
	if err != nil {
		t.Fatalf("LoadDir errored: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	var msgs []string
	for _, i := range Validate(records) {
		msgs = append(msgs, i.Message)
	}
	joined := strings.Join(msgs, "\n")
	for _, want := range []string{"required section: Guidance", "template placeholder", "no category"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected a finding mentioning %q; got:\n%s", want, joined)
		}
	}
}

func TestRenderNewBodyFor_ContainsCoreSections(t *testing.T) {
	body := RenderNewBodyFor("Pick a thing", "")
	for _, want := range []string{"# Pick a thing", "## Decision", "## Guidance", "{...}"} {
		if !strings.Contains(body, want) {
			t.Errorf("template missing %q:\n%s", want, body)
		}
	}
}

func TestValidate_DuplicateID(t *testing.T) {
	records := []Record{
		rec("0008", "0008-a.md", "accepted", "Meta", acceptedBody("A"), nil, nil),
		rec("0008", "sub/0008-b.md", "accepted", "Meta", acceptedBody("B"), nil, nil),
	}
	found := false
	for _, i := range Validate(records) {
		if !i.Warning && strings.Contains(i.Message, "duplicate ID 0008") &&
			strings.Contains(i.Message, "0008-a.md") && strings.Contains(i.Message, "0008-b.md") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a hard duplicate-ID issue naming both files; got: %+v", Validate(records))
	}
}

func TestValidate_BraceGlobHardFail(t *testing.T) {
	r := briefRecX("0011", "0011-x.md", "invariant",
		[]string{"port/{donation_flows,extraction}/**"}, nil, nil, nil, acceptedBody("Single architecture"))
	found := false
	for _, i := range Validate([]Record{r}) {
		if !i.Warning && strings.Contains(i.Message, "brace expansion") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a hard brace-glob issue; got: %+v", Validate([]Record{r}))
	}
}

func TestValidate_GlobHygiene_SingleStarNested(t *testing.T) {
	r := briefRecX("0008", "0008-x.md", "default",
		[]string{"platforms/*.py"}, nil, nil, nil, acceptedBody("Pages"))
	found := false
	for _, i := range Validate([]Record{r}) {
		if i.Warning && strings.Contains(i.Message, "platforms/**/*.py") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a glob-hygiene suggestion of platforms/**/*.py; got: %+v", Validate([]Record{r}))
	}
}

func TestValidate_GlobHygiene_NoFalsePositives(t *testing.T) {
	r := briefRecX("0008", "0008-x.md", "default",
		[]string{"port/**/*.py", "*.py", "main.py"}, nil, nil, nil, acceptedBody("Pages"))
	for _, i := range Validate([]Record{r}) {
		if strings.Contains(i.Message, "single-star segment") {
			t.Errorf("recursive/root globs should not warn; got: %s", i.Message)
		}
	}
}

func TestValidate_CompanionsOrphanWarn(t *testing.T) {
	r := briefRecX("0009", "0009-x.md", "default",
		nil, nil, nil, []string{"src/App.tsx"}, acceptedBody("Props"))
	found := false
	for _, i := range Validate([]Record{r}) {
		if i.Warning && strings.Contains(i.Message, "companions set but") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a companions-orphan warning; got: %+v", Validate([]Record{r}))
	}
}
