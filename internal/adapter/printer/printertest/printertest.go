// Package printertest is a tiny helper for presenter tests. It returns a
// Streams value backed by fresh buffers so tests can assert separately
// against stdout and stderr without monkey-patching os.Stdout/os.Stderr.
package printertest

import (
	"bytes"

	printer "github.com/d3i-infra/adg/v4/internal/adapter/printer"
)

// Capture builds a Streams writing to fresh buffers. The returned
// buffers grow as the presenter writes; assert against `out.String()`
// for machine-readable stdout and `err.String()` for human-readable
// stderr. Set quiet=true to test --quiet behavior.
func Capture(quiet bool) (streams printer.Streams, out, err *bytes.Buffer) {
	out, err = &bytes.Buffer{}, &bytes.Buffer{}
	q := quiet
	return printer.Streams{Out: out, Err: err, Quiet: &q}, out, err
}
