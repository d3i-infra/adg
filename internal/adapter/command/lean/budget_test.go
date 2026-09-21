package lean

import (
	leandomain "github.com/d3i-infra/adg/internal/domain/decision/lean"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, projectConfigFile), []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", projectConfigFile, err)
	}
}

func TestLoadBudget_NoFile_IsDefault(t *testing.T) {
	b, w := loadBudget(t.TempDir())
	if b != leandomain.DefaultBudget() || w != "" {
		t.Fatalf("absent .adg.yaml must be default with no warning; got %+v warning=%q", b, w)
	}
}

func TestLoadBudget_Narrative(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "body_budget: narrative\n")
	b, w := loadBudget(dir)
	if b != leandomain.NarrativeBudget() || w != "" {
		t.Fatalf("body_budget: narrative must load NarrativeBudget with no warning; got %+v warning=%q", b, w)
	}
}

func TestLoadBudget_ExplicitLean(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "body_budget: lean\n")
	b, w := loadBudget(dir)
	if b != leandomain.DefaultBudget() || w != "" {
		t.Fatalf("body_budget: lean must load DefaultBudget with no warning; got %+v warning=%q", b, w)
	}
}

func TestLoadBudget_UnknownValue_DefaultsWithWarning(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "body_budget: narative\n") // typo
	b, w := loadBudget(dir)
	if b != leandomain.DefaultBudget() {
		t.Fatalf("unknown body_budget must degrade to DefaultBudget; got %+v", b)
	}
	if !strings.Contains(w, "unknown body_budget") {
		t.Fatalf("unknown body_budget must warn; got warning=%q", w)
	}
}

func TestLoadBudget_UnknownKey_Ignored(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "some_future_key: value\n") // no body_budget
	b, w := loadBudget(dir)
	if b != leandomain.DefaultBudget() || w != "" {
		t.Fatalf("unknown key with no body_budget must be default, no warning; got %+v warning=%q", b, w)
	}
}

func TestLoadBudget_MalformedYAML_DefaultsWithWarning(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "body_budget: [unterminated\n")
	b, w := loadBudget(dir)
	if b != leandomain.DefaultBudget() {
		t.Fatalf("malformed .adg.yaml must degrade to DefaultBudget; got %+v", b)
	}
	if w == "" {
		t.Fatalf("malformed .adg.yaml must warn")
	}
}
