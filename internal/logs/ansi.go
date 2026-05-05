package logs

import "strings"

// stripANSI removes ANSI escape sequences (CSI and OSC) and lone
// escape characters from s. It is intentionally small and regex-free.
//
// Sequences handled:
//   - CSI:  ESC '[' <params/intermediates> <final 0x40..0x7E>
//   - OSC:  ESC ']' ... BEL (0x07) or ESC '\\' (ST)
//   - Lone: a stray ESC followed by a single byte (e.g. ESC '7')
func stripANSI(s string) string {
	if !strings.ContainsRune(s, 0x1b) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		c := s[i]
		if c != 0x1b {
			b.WriteByte(c)
			i++
			continue
		}
		// At ESC. Need at least one more byte.
		if i+1 >= len(s) {
			i++
			continue
		}
		next := s[i+1]
		switch next {
		case '[':
			// CSI: skip until a final byte in 0x40..0x7E.
			j := i + 2
			for j < len(s) {
				bb := s[j]
				if bb >= 0x40 && bb <= 0x7E {
					j++
					break
				}
				j++
			}
			i = j
		case ']':
			// OSC: skip until BEL (0x07) or ESC '\\' (ST).
			j := i + 2
			for j < len(s) {
				bb := s[j]
				if bb == 0x07 {
					j++
					break
				}
				if bb == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
					j += 2
					break
				}
				j++
			}
			i = j
		default:
			// Lone ESC + single byte: drop both.
			i += 2
		}
	}
	return b.String()
}
