// This file is derived from github.com/adr/ad-guidance-tool (Apache-2.0) and has
// been modified by the adg authors; see the Acknowledgements section of README.md.

package commands

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveModelPath_FlagWins(t *testing.T) {
	assert.Equal(t, "/explicit/path", ResolveModelPath("/explicit/path"))
}

func TestResolveModelPath_DefaultsToDocsDecisions(t *testing.T) {
	assert.Equal(t, "docs/decisions", ResolveModelPath(""))
}

func TestModelLoadHint_NamesPathAndFlag(t *testing.T) {
	err := ModelLoadHint(DefaultModelPath, errors.New("open docs/decisions: no such file or directory"))
	assert.ErrorContains(t, err, `"docs/decisions"`)
	assert.ErrorContains(t, err, "--model")
}

func TestModelLoadHint_ExplicitPathOmitsFlagHint(t *testing.T) {
	err := ModelLoadHint("elsewhere/adrs", errors.New("open elsewhere/adrs: no such file or directory"))
	assert.ErrorContains(t, err, `"elsewhere/adrs"`)
	assert.NotContains(t, err.Error(), "--model")
}

func TestNormalizeID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "short form zero-padded", input: "5", want: "0005"},
		{name: "already zero-padded", input: "0022", want: "0022"},
		{name: "zero is reserved", input: "0", wantErr: true},
		{name: "above range", input: "10000", wantErr: true},
		{name: "non-numeric", input: "abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeID(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
