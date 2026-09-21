package lean

import (
	"github.com/d3i-infra/adg/internal/domain/decision/madr"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeContentTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRunChecks_AbsentViolationHonorsExcept(t *testing.T) {
	root := writeContentTree(t, map[string]string{
		"port/flow.py":                 "x = CommandUIRender()\n", // violation
		"port/helpers/port_helpers.py": "CommandUIRender()\n",     // sanctioned home (except)
	})
	rec := Record{ID: "0001", D: madr.Decision{Checks: []madr.Check{{
		Desc:   "no UI render outside port_helpers",
		Grep:   `CommandUIRender\(`,
		In:     []string{"port/**/*.py"},
		Except: []string{"**/port_helpers.py"},
	}}}}

	results, err := RunChecks([]Record{rec}, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Failed {
		t.Fatalf("expected the check to fail on port/flow.py; got %+v", results)
	}
	if !strings.Contains(results[0].Detail, "port/flow.py") || strings.Contains(results[0].Detail, "port_helpers.py") {
		t.Errorf("detail should name the violating file, not the excepted one: %q", results[0].Detail)
	}
}

func TestRunChecks_PresentAndScope(t *testing.T) {
	root := writeContentTree(t, map[string]string{"a.py": "import safe\n", "b.py": "danger here\n"})
	rec := Record{ID: "0002", D: madr.Decision{Checks: []madr.Check{{
		Desc: "danger appears somewhere", Grep: "danger", Expect: "present",
	}}}}

	// Whole tree: passes (b.py has it).
	if r, _ := RunChecks([]Record{rec}, root, nil); r[0].Failed {
		t.Errorf("present check should pass against the whole tree; got %+v", r)
	}
	// A present check asserts existence — a global invariant — so the changed-files
	// lens must not narrow it: b.py still satisfies it when only a.py changed.
	if r, _ := RunChecks([]Record{rec}, root, []string{"a.py"}); r[0].Failed {
		t.Errorf("present check must ignore path narrowing (existence is global); got %+v", r)
	}
}

func TestRunChecks_PresentUnderScopeStillEvaluatesFullScope(t *testing.T) {
	// The pattern is missing from the whole tree: a present check must FAIL even
	// when the changed set doesn't touch its scope — evaluated, never skipped.
	root := writeContentTree(t, map[string]string{"release.sh": "echo build\n", "other.txt": "x\n"})
	rec := Record{ID: "0004", D: madr.Decision{Checks: []madr.Check{{
		Desc: "release.sh builds per-platform", Grep: "VITE_PLATFORM",
		In: []string{"release.sh"}, Expect: "present",
	}}}}

	r, _ := RunChecks([]Record{rec}, root, []string{"other.txt"})
	if !r[0].Failed {
		t.Errorf("present check must still evaluate (and fail) under narrowing when the pattern is gone; got %+v", r)
	}
	if r[0].Detail == "" {
		t.Errorf("failed present check should carry a detail; got %+v", r)
	}
}

func TestRunChecks_AbsentKeepsChangedFilesNarrowing(t *testing.T) {
	// Absence checks keep the lens: a violation in an UNCHANGED file is not
	// re-reported when only a.py is in the change set.
	root := writeContentTree(t, map[string]string{"a.py": "import safe\n", "b.py": "danger here\n"})
	rec := Record{ID: "0003", D: madr.Decision{Checks: []madr.Check{{
		Desc: "no danger anywhere", Grep: "danger",
	}}}}

	if r, _ := RunChecks([]Record{rec}, root, []string{"a.py"}); r[0].Failed {
		t.Errorf("absent check narrowed to a.py should pass (violation lives in unchanged b.py); got %+v", r)
	}
	if r, _ := RunChecks([]Record{rec}, root, []string{"b.py"}); !r[0].Failed {
		t.Errorf("absent check narrowed to b.py should fail; got %+v", r)
	}
}

func TestValidate_BadCheckRegexpIsHardFailure(t *testing.T) {
	r := leanRec("0001", "accepted", "default", acceptedBody("T"))
	r.D.Checks = []madr.Check{{Desc: "bad", Grep: "("}}
	hard := false
	for _, i := range Validate([]Record{r}) {
		if !i.Warning && strings.Contains(i.Message, "not a valid regexp") {
			hard = true
		}
	}
	if !hard {
		t.Errorf("a check with an invalid grep regexp should be a hard failure; got: %+v", Validate([]Record{r}))
	}
}
