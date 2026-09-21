package lean

import (
	"github.com/d3i-infra/adg/internal/domain/decision/madr"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// maxCheckExamples caps the offending files listed in a failed check's detail.
const maxCheckExamples = 5

// CheckResult is the outcome of one executable check (a record's grep assertion).
type CheckResult struct {
	ID     string // the governing ADR's ID
	Desc   string // the check's human-readable statement
	Failed bool
	Detail string // why it failed (offending files, or "no file matched"); "" when passing
}

// RunChecks runs every record's frontmatter `checks` against the tree at root. When
// scopePaths is non-empty, absence checks search only those files (the "check what
// changed" lens); `expect: present` checks always evaluate their full declared
// in/except scope, because existence is a global invariant that narrowing would
// invert. Without scopePaths, everything runs against the whole tree under root.
// Returns one result per check, in record-then-declaration order. An unparseable
// grep or glob surfaces as a failed result (the index validator catches these
// earlier as hard errors).
func RunChecks(records []Record, root string, scopePaths []string) ([]CheckResult, error) {
	files, err := listFiles(root)
	if err != nil {
		return nil, err
	}
	var scope map[string]bool
	if len(scopePaths) > 0 {
		scope = make(map[string]bool, len(scopePaths))
		for _, p := range scopePaths {
			scope[filepath.ToSlash(p)] = true
		}
	}

	var results []CheckResult
	for _, r := range records {
		for _, c := range r.D.Checks {
			results = append(results, runOneCheck(r, c, files, root, scope))
		}
	}
	return results, nil
}

func runOneCheck(r Record, c madr.Check, files []string, root string, scope map[string]bool) CheckResult {
	res := CheckResult{ID: r.ID, Desc: strings.TrimSpace(c.Desc)}

	grep, err := regexp.Compile(c.Grep)
	if err != nil {
		res.Failed = true
		res.Detail = "invalid grep regexp: " + err.Error()
		return res
	}
	expect := strings.TrimSpace(c.Expect)
	if expect == "" {
		expect = "absent"
	}

	in := compileGlobs(c.In)
	except := compileGlobs(c.Except)
	inScope := func(f string) bool {
		// The changed-files lens narrows only absence checks: unchanged files were
		// compliant at the last commit, so searching the change set is exact and
		// cheap. A "present" check asserts existence — a global invariant — and
		// must always evaluate its full declared in/except scope; narrowed to a
		// change set that doesn't contain its file, it would fail unconditionally
		// (and a commit deleting the required file would slip through unchecked).
		if expect != "present" && scope != nil && !scope[f] {
			return false
		}
		if len(in) > 0 && !anyMatch(in, f) {
			return false
		}
		return !anyMatch(except, f)
	}

	var hits []string
	for _, f := range files {
		if !inScope(f) {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if rerr != nil {
			continue
		}
		if grep.Match(b) {
			hits = append(hits, f)
		}
	}
	sort.Strings(hits)

	switch expect {
	case "present":
		if len(hits) == 0 {
			res.Failed = true
			res.Detail = "pattern found in no file in scope"
		}
	default: // absent
		if len(hits) > 0 {
			res.Failed = true
			res.Detail = "pattern found in: " + examples(hits, maxCheckExamples)
		}
	}
	return res
}

func anyMatch(globs []compiledGlob, f string) bool {
	for _, g := range globs {
		if g.re.MatchString(f) {
			return true
		}
	}
	return false
}
