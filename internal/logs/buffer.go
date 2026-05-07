package logs

import (
	"bytes"
	"strings"
)

// MaxBytes is the in-memory cap for a single job log buffer (10 MB).
const MaxBytes = 10 * 1024 * 1024

// Buffer stores raw (ANSI-preserving) log bytes alongside a plain-text index
// for search. Not safe for concurrent use.
type Buffer struct {
	raw        []byte
	lines      []string
	plainLines []string
	truncated  bool
}

// New constructs an empty buffer.
func New() *Buffer {
	return &Buffer{}
}

// Set replaces the whole content. If the new content exceeds MaxBytes,
// the leading bytes are dropped (keeping the tail) and Truncated()
// flips to true. Truncated() never resets to false.
func (b *Buffer) Set(p []byte) {
	b.raw = nil
	b.appendBytes(p)
	b.rebuildLines()
}

// Append adds bytes to the buffer. If the total would exceed
// MaxBytes, only the tail is kept and Truncated() reports true on
// first overflow.
func (b *Buffer) Append(p []byte) {
	b.appendBytes(p)
	b.rebuildLines()
}

// appendBytes appends p to b.raw, applying the MaxBytes truncation
// policy. When truncating, the leading partial line (everything up to
// and including the first '\n') is dropped so callers don't see a
// half-line at the top.
func (b *Buffer) appendBytes(p []byte) {
	if len(p) == 0 {
		return
	}
	combined := append(b.raw, p...)
	if len(combined) > MaxBytes {
		b.truncated = true
		// Keep the last MaxBytes.
		tail := combined[len(combined)-MaxBytes:]
		// Drop the leading partial line, if any.
		if i := bytes.IndexByte(tail, '\n'); i >= 0 {
			tail = tail[i+1:]
		}
		// Copy to a fresh backing array so we don't pin the larger one.
		b.raw = append([]byte(nil), tail...)
		return
	}
	b.raw = combined
}

// rebuildLines refreshes b.lines and b.plainLines from b.raw.
func (b *Buffer) rebuildLines() {
	if len(b.raw) == 0 {
		b.lines = nil
		b.plainLines = nil
		return
	}
	parts := strings.Split(string(b.raw), "\n")
	// strings.Split returns a trailing empty element when raw ends
	// with '\n'; drop it so our line count matches the visible
	// number of log lines.
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	// GitHub Actions job logs use \r\n line endings. Strip trailing \r
	// so that group titles and content are clean for both rendering and
	// structural parsing (BuildOutline marker recognition).
	for i, ln := range parts {
		if len(ln) > 0 && ln[len(ln)-1] == '\r' {
			parts[i] = ln[:len(ln)-1]
		}
	}
	b.lines = parts
	b.plainLines = make([]string, len(parts))
	for i, ln := range parts {
		b.plainLines[i] = stripANSI(ln)
	}
}

// Raw returns the raw bytes (preserving ANSI). Callers MUST NOT mutate.
func (b *Buffer) Raw() []byte { return b.raw }

// Lines returns the raw bytes split on '\n' (no ANSI stripped).
// Callers MUST NOT mutate.
func (b *Buffer) Lines() []string { return b.lines }

// PlainLines returns the ANSI-stripped lines used for search.
// Callers MUST NOT mutate.
func (b *Buffer) PlainLines() []string { return b.plainLines }

// Len returns the current byte size of the buffer.
func (b *Buffer) Len() int { return len(b.raw) }

// Truncated reports whether the buffer has overflowed MaxBytes at any
// point. Once true it never returns to false.
func (b *Buffer) Truncated() bool { return b.truncated }
