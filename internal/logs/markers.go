package logs

import "strings"

// failureMarkers are matched case-insensitively against the
// ANSI-stripped lines.
var failureMarkers = []string{
	"##[error]",
	"process completed with exit code",
}

// FirstFailureLine returns the 0-based index of the first line
// matching one of the failure markers, or -1 if none.
//
// Markers (case-insensitive):
//   - "##[error]" anywhere in the line
//   - "Process completed with exit code"
func (b *Buffer) FirstFailureLine() int {
	for i, ln := range b.plainLines {
		l := strings.ToLower(ln)
		for _, m := range failureMarkers {
			if strings.Contains(l, m) {
				return i
			}
		}
	}
	return -1
}
