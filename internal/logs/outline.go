package logs

import (
	"strconv"
	"strings"
	"time"
)

// NodeKind identifies the kind of an outline node.
type NodeKind int

// Node kinds.
const (
	NodeStep NodeKind = iota
	NodeGroup
	NodeLine
)

// Severity is the semantic level of a log line (higher = more severe).
type Severity int

// Severity levels (ordered from least to most severe).
const (
	SevPlain Severity = iota
	SevDebug
	SevCommand
	SevNotice
	SevWarning
	SevError
)

// Node is a step, group, or line in the outline tree.
type Node struct {
	Kind         NodeKind
	Title        string
	Sev          Severity
	StartIdx     int       // index into Buffer.Lines() — header line, or content line
	EndIdx       int       // exclusive; for groups, the matching ##[endgroup]
	StartTime    time.Time // zero if no timestamp on first child
	EndTime      time.Time
	ErrorCount   int
	WarningCount int
	NoticeCount  int
	Children     []*Node
	Parent       *Node // nil for roots
	Synthetic    bool  // pseudo-steps (ungrouped, beginning-truncated)
}

// Outline is the structural projection of a job log buffer.
type Outline struct {
	Roots       []*Node
	HeadDropped bool // true when the source Buffer.Truncated() is true
}

// stripTimestamp returns the parsed timestamp (zero on no/invalid match) and
// the body of the line with the timestamp prefix removed.
//
// Uses a fast manual scanner + parser instead of regexp and time.Parse to
// avoid per-line allocations. Handles both "YYYY-MM-DDTHH:MM:SSZ " and
// "YYYY-MM-DDTHH:MM:SS.fracZ " forms.
func stripTimestamp(line string) (time.Time, string) {
	n := len(line)
	// Minimum: "2024-01-01T10:00:00Z " = 21 chars.
	if n < 21 {
		return time.Time{}, line
	}
	// Validate the fixed-position digits and delimiters.
	if !isDigits(line, 0, 4) || line[4] != '-' ||
		!isDigits(line, 5, 7) || line[7] != '-' ||
		!isDigits(line, 8, 10) || line[10] != 'T' ||
		!isDigits(line, 11, 13) || line[13] != ':' ||
		!isDigits(line, 14, 16) || line[16] != ':' ||
		!isDigits(line, 17, 19) {
		return time.Time{}, line
	}
	pos := 19
	// Optional fractional seconds.
	nsec := 0
	if pos < n && line[pos] == '.' {
		pos++
		fracStart := pos
		for pos < n && line[pos] >= '0' && line[pos] <= '9' {
			pos++
		}
		frac := line[fracStart:pos]
		nf := len(frac)
		if nf > 9 {
			nf = 9
		}
		v := 0
		for i := 0; i < nf; i++ {
			v = v*10 + int(frac[i]-'0')
		}
		for i := nf; i < 9; i++ {
			v *= 10
		}
		nsec = v
	}
	if pos >= n || line[pos] != 'Z' {
		return time.Time{}, line
	}
	pos++ // skip 'Z'
	// Require at least one whitespace separator.
	bodyStart := pos
	for bodyStart < n && (line[bodyStart] == ' ' || line[bodyStart] == '\t') {
		bodyStart++
	}
	if bodyStart == pos {
		return time.Time{}, line
	}
	// Parse timestamp fields directly — avoids time.Parse overhead.
	year := parseDigits(line, 0, 4)
	month := parseDigits(line, 5, 7)
	day := parseDigits(line, 8, 10)
	hour := parseDigits(line, 11, 13)
	min := parseDigits(line, 14, 16)
	sec := parseDigits(line, 17, 19)
	t := time.Date(year, time.Month(month), day, hour, min, sec, nsec, time.UTC)
	return t, line[bodyStart:]
}

// isDigits reports whether all characters in line[from:to] are ASCII digits.
func isDigits(line string, from, to int) bool {
	if to > len(line) {
		return false
	}
	for i := from; i < to; i++ {
		if line[i] < '0' || line[i] > '9' {
			return false
		}
	}
	return true
}

// parseDigits parses the decimal integer in line[from:to]. No bounds checking
// — the caller must have validated the range via isDigits first.
func parseDigits(line string, from, to int) int {
	n := 0
	for i := from; i < to; i++ {
		n = n*10 + int(line[i]-'0')
	}
	return n
}

// classifyMarker inspects the (post-timestamp-strip) body of a line
// and returns its semantic role.
type markerKind int

const (
	markerNone markerKind = iota
	markerGroupOpen
	markerGroupClose
	markerSeverity
)

// classifyLine returns the marker kind for body. For markerGroupOpen,
// title is the text that follows "##[group]". For markerSeverity,
// sev is the severity. Other return values are zero.
func classifyLine(body string) (kind markerKind, title string, sev Severity) {
	// Fast path: most log lines don't start with "##[". Skip all marker
	// checks for those lines and only do the exit-code scan.
	if len(body) < 3 || body[0] != '#' || body[1] != '#' || body[2] != '[' {
		if s, ok := exitCodeSeverity(body); ok {
			return markerSeverity, "", s
		}
		return markerNone, "", SevPlain
	}
	// body starts with "##[" — check specific marker types.
	// Group open: prefix "##[group]" (case-insensitive).
	if hasFoldPrefix(body, "##[group]") {
		return markerGroupOpen, body[len("##[group]"):], SevPlain
	}
	// Group close: equals "##[endgroup]" (case-insensitive, after trim).
	trimmed := strings.TrimSpace(body)
	if strings.EqualFold(trimmed, "##[endgroup]") {
		return markerGroupClose, "", SevPlain
	}
	// Severity prefixes.
	switch {
	case hasFoldPrefix(body, "##[error]"):
		return markerSeverity, "", SevError
	case hasFoldPrefix(body, "##[warning]"):
		return markerSeverity, "", SevWarning
	case hasFoldPrefix(body, "##[notice]"):
		return markerSeverity, "", SevNotice
	case hasFoldPrefix(body, "##[debug]"):
		return markerSeverity, "", SevDebug
	case hasFoldPrefix(body, "##[command]"):
		return markerSeverity, "", SevCommand
	}
	// "Process completed with exit code N" — anywhere on the line.
	if s, ok := exitCodeSeverity(body); ok {
		return markerSeverity, "", s
	}
	return markerNone, "", SevPlain
}

// hasFoldPrefix reports whether s begins with prefix under
// case-insensitive comparison.
func hasFoldPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return strings.EqualFold(s[:len(prefix)], prefix)
}

// exitCodeSeverity scans body for "Process completed with exit code
// N" (case-insensitive). When N is non-zero it returns SevError, true.
// When N is zero it returns SevPlain, false (treated as a plain line
// — exit-code-zero is not a severity event).
func exitCodeSeverity(body string) (Severity, bool) {
	const needle = "process completed with exit code "
	lower := strings.ToLower(body)
	idx := strings.Index(lower, needle)
	if idx < 0 {
		return SevPlain, false
	}
	rest := body[idx+len(needle):]
	// Read the numeric token (allow optional leading '-').
	end := 0
	if end < len(rest) && rest[end] == '-' {
		end++
	}
	digits := end
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == digits {
		return SevPlain, false
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return SevPlain, false
	}
	if n == 0 {
		return SevPlain, false
	}
	return SevError, true
}

// BuildOutline returns the structural projection of b. The returned
// outline references b only by index — Buffer is not retained.
func BuildOutline(b *Buffer) *Outline {
	out := &Outline{}
	if b == nil {
		return out
	}
	lines := b.PlainLines()

	// Synthetic ungrouped root for any leading content that arrives
	// before the first ##[group]. May be discarded if the very first
	// real group arrives before any plain content has been emitted.
	syntheticRoot := &Node{Kind: NodeStep, Title: "(ungrouped)", Synthetic: true, StartIdx: 0}
	out.Roots = append(out.Roots, syntheticRoot)
	stack := []*Node{syntheticRoot}
	syntheticActive := true

	// bumpAncestors increments the appropriate counter and bubbles
	// severity / timestamps onto every node currently on the stack.
	bumpAncestors := func(sev Severity, ts time.Time) {
		for _, anc := range stack {
			switch sev {
			case SevError:
				anc.ErrorCount++
			case SevWarning:
				anc.WarningCount++
			case SevNotice:
				anc.NoticeCount++
			}
			if sev > anc.Sev {
				anc.Sev = sev
			}
			if !ts.IsZero() {
				if anc.StartTime.IsZero() {
					anc.StartTime = ts
				}
				anc.EndTime = ts
			}
		}
	}

	for i, ln := range lines {
		ts, body := stripTimestamp(ln)
		kind, title, sev := classifyLine(body)

		switch kind {
		case markerGroupOpen:
			// First real group with a still-empty synthetic root: drop
			// the synthetic and replace it with a real step.
			if syntheticActive && len(syntheticRoot.Children) == 0 && len(stack) == 1 {
				out.Roots = out.Roots[:len(out.Roots)-1]
				stack = stack[:0]
				syntheticActive = false
			} else if syntheticActive && len(syntheticRoot.Children) > 0 {
				// Synthetic has content; close it as we move on. We
				// keep it as a root but it is no longer the open node.
				if len(stack) == 1 && stack[0] == syntheticRoot {
					syntheticRoot.EndIdx = i
					stack = stack[:0]
				}
				syntheticActive = false
			}

			var n *Node
			if len(stack) == 0 {
				n = &Node{Kind: NodeStep, Title: title, StartIdx: i, StartTime: ts}
				out.Roots = append(out.Roots, n)
			} else {
				parent := stack[len(stack)-1]
				n = &Node{Kind: NodeGroup, Title: title, StartIdx: i, StartTime: ts, Parent: parent}
				parent.Children = append(parent.Children, n)
			}
			stack = append(stack, n)
			// A group-open line itself does not contribute severity; it
			// only updates ancestor timestamps.
			if !ts.IsZero() {
				for _, anc := range stack {
					if anc.StartTime.IsZero() {
						anc.StartTime = ts
					}
					anc.EndTime = ts
				}
			}

		case markerGroupClose:
			// Stray endgroup at depth 0 (only synthetic root or empty
			// stack) is ignored.
			if len(stack) == 0 {
				continue
			}
			top := stack[len(stack)-1]
			if top == syntheticRoot {
				continue
			}
			top.EndIdx = i
			if !ts.IsZero() {
				top.EndTime = ts
			}
			stack = stack[:len(stack)-1]

		default:
			// Plain or severity line.
			if len(stack) == 0 {
				// Stack is empty (a step closed and a new orphan line
				// arrived before the next group). Open a fresh
				// synthetic step to hold it.
				n := &Node{Kind: NodeStep, Title: "(ungrouped)", Synthetic: true, StartIdx: i, StartTime: ts}
				out.Roots = append(out.Roots, n)
				stack = append(stack, n)
			}
			parent := stack[len(stack)-1]
			line := &Node{
				Kind:      NodeLine,
				Sev:       sev,
				StartIdx:  i,
				EndIdx:    i + 1,
				StartTime: ts,
				EndTime:   ts,
				Parent:    parent,
			}
			parent.Children = append(parent.Children, line)
			bumpAncestors(sev, ts)
		}
	}

	// Close all still-open nodes at EOF.
	totalLines := len(lines)
	for i := len(stack) - 1; i >= 0; i-- {
		n := stack[i]
		if n.EndIdx == 0 {
			n.EndIdx = totalLines
		}
		// EndTime: bubbleAncestors already set it from the last child
		// with a timestamp; leave as-is (zero if none seen).
	}
	stack = nil

	// If the synthetic root was never displaced and ended up empty,
	// drop it entirely so an empty buffer yields no roots.
	if len(out.Roots) == 1 && out.Roots[0] == syntheticRoot && len(syntheticRoot.Children) == 0 {
		out.Roots = nil
	}

	// Splice out any synthetic "(ungrouped)" pseudo-step, lifting its
	// children to top-level roots. This keeps the outline visually flat
	// for orphan content emitted before the first ##[group] or between
	// closed groups — those lines are just shown directly at the root
	// level instead of behind an extra collapse layer that adds no
	// information.
	if len(out.Roots) > 0 {
		flattened := make([]*Node, 0, len(out.Roots))
		for _, r := range out.Roots {
			if r.Synthetic && r.Title == "(ungrouped)" {
				for _, c := range r.Children {
					c.Parent = nil
					flattened = append(flattened, c)
				}
				continue
			}
			flattened = append(flattened, r)
		}
		out.Roots = flattened
	}

	if b.Truncated() {
		out.HeadDropped = true
		leader := &Node{
			Kind:      NodeStep,
			Title:     "(beginning of log not retained)",
			Synthetic: true,
			StartIdx:  0,
			EndIdx:    0,
		}
		out.Roots = append([]*Node{leader}, out.Roots...)
	}

	return out
}
