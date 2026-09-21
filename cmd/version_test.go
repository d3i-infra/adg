package cmd

import "testing"

func TestResolveVersion(t *testing.T) {
	cases := []struct{ name, linked, main, want string }{
		{"goreleaser link wins", "4.0.1", "v9.9.9", "4.0.1"},
		{"go install of a tag", "dev", "v4.0.1", "4.0.1"},
		{"go install of a commit", "dev", "v4.0.2-0.20260921193000-abcdef123456", "4.0.2-0.20260921193000-abcdef123456"},
		{"plain go build in a checkout", "dev", "(devel)", "dev"},
		{"no build info", "dev", "", "dev"},
	}
	for _, c := range cases {
		if got := resolveVersion(c.linked, c.main); got != c.want {
			t.Errorf("%s: resolveVersion(%q, %q) = %q, want %q", c.name, c.linked, c.main, got, c.want)
		}
	}
}
