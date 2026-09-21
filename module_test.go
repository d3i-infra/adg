package main

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Go only accepts a vN tag (N >= 2) for a module whose path ends in /vN. The release
// tag equals plugin.json's version (ADR-0013), so the two majors must agree, or
// `go install github.com/d3i-infra/adg/vN@latest` cannot resolve the release.
func TestModuleMajorMatchesPluginVersion(t *testing.T) {
	mod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^module\s+(\S+)$`).FindSubmatch(mod)
	if m == nil {
		t.Fatal("no module line in go.mod")
	}
	path := string(m[1])

	raw, err := os.ReadFile("tools/adr-plugin/.claude-plugin/plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var p struct{ Version string }
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	major, err := strconv.Atoi(strings.SplitN(p.Version, ".", 2)[0])
	if err != nil {
		t.Fatalf("plugin.json version %q: %v", p.Version, err)
	}

	suffix := regexp.MustCompile(`/v(\d+)$`).FindStringSubmatch(path)
	switch {
	case major >= 2 && (suffix == nil || suffix[1] != strconv.Itoa(major)):
		t.Errorf("plugin.json is at major %d, so the module path must end in /v%d; go.mod says %q", major, major, path)
	case major < 2 && suffix != nil:
		t.Errorf("plugin.json is at major %d, so the module path must not carry a /vN suffix; go.mod says %q", major, path)
	}
}
