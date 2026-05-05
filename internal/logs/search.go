package logs

import "strings"

// Search performs a case-insensitive substring search over the
// ANSI-stripped line index and returns the 0-based line indexes of
// matches in ascending order. An empty needle returns nil.
func (b *Buffer) Search(needle string) []int {
	if needle == "" {
		return nil
	}
	q := strings.ToLower(needle)
	var out []int
	for i, ln := range b.plainLines {
		if strings.Contains(strings.ToLower(ln), q) {
			out = append(out, i)
		}
	}
	return out
}
