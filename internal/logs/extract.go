package logs

import "sort"

// Snippet is a contiguous range of log lines extracted for agent consumption.
type Snippet struct {
	StepTitle string   // enclosing top-level NodeStep title, or "(no error markers)"
	StartIdx  int      // 0-based index into Buffer.PlainLines()
	EndIdx    int      // exclusive
	Lines     []string // ANSI-stripped content
	Severity  Severity
}

// Extract walks the outline and returns one Snippet per cluster of SevError
// nodes, padded to ±contextLines and merged when overlapping. Lines are
// always ANSI-stripped (PlainLines).
//
// Heuristic fallback: if no error-severity nodes are found and the buffer
// has no severity markers at all (no ##[command], ##[notice], ##[warning], etc.),
// a final snippet with the last min(contextLines*2, totalLines) lines is
// returned with Severity=SevError and StepTitle="(no error markers)".
// This catches plain go test / npm failures that emit no ##[error] markers.
// Logs that have non-error severity markers (passing logs with ##[notice] etc.)
// return nil rather than triggering the fallback.
func Extract(b *Buffer, o *Outline, contextLines int) []Snippet {
	if b == nil || len(b.Lines()) == 0 {
		return nil
	}

	totalLines := len(b.PlainLines())
	plain := b.PlainLines()

	// Collect (lineIdx, enclosingStepTitle) for every SevError NodeLine,
	// and track whether any non-plain severity markers exist.
	type errorEntry struct {
		lineIdx   int
		stepTitle string
	}

	var entries []errorEntry
	seen := map[int]bool{}
	hasSeverityMarkers := false

	var walkNodes func(nodes []*Node)
	walkNodes = func(nodes []*Node) {
		for _, n := range nodes {
			if n.Kind == NodeLine {
				if n.Sev > SevPlain {
					hasSeverityMarkers = true
				}
				if n.Sev == SevError && !seen[n.StartIdx] {
					seen[n.StartIdx] = true
					entries = append(entries, errorEntry{
						lineIdx:   n.StartIdx,
						stepTitle: enclosingStepTitle(n),
					})
				}
			}
			walkNodes(n.Children)
		}
	}
	walkNodes(o.Roots)

	if len(entries) == 0 {
		if hasSeverityMarkers {
			// Log has non-error severity markers — it is a passing/clean log.
			// Do not trigger the fallback.
			return nil
		}
		// No severity markers at all — plain output from go test / npm etc.
		// Return a fallback snippet of the last min(contextLines*2, totalLines) lines.
		tailLen := contextLines * 2
		if tailLen > totalLines {
			tailLen = totalLines
		}
		start := totalLines - tailLen
		return []Snippet{{
			StepTitle: "(no error markers)",
			StartIdx:  start,
			EndIdx:    totalLines,
			Lines:     plain[start:totalLines],
			Severity:  SevError,
		}}
	}

	// Sort by line index.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].lineIdx < entries[j].lineIdx
	})

	// Build padded ranges.
	type rangeEntry struct {
		start, end int
		stepTitle  string
	}
	ranges := make([]rangeEntry, len(entries))
	for i, e := range entries {
		start := e.lineIdx - contextLines
		if start < 0 {
			start = 0
		}
		end := e.lineIdx + contextLines + 1
		if end > totalLines {
			end = totalLines
		}
		ranges[i] = rangeEntry{start: start, end: end, stepTitle: e.stepTitle}
	}

	// Merge overlapping/adjacent ranges.
	merged := []rangeEntry{ranges[0]}
	for i := 1; i < len(ranges); i++ {
		last := &merged[len(merged)-1]
		cur := ranges[i]
		if cur.start <= last.end+2 {
			// Merge.
			if cur.end > last.end {
				last.end = cur.end
			}
		} else {
			merged = append(merged, cur)
		}
	}

	snippets := make([]Snippet, len(merged))
	for i, r := range merged {
		snippets[i] = Snippet{
			StepTitle: r.stepTitle,
			StartIdx:  r.start,
			EndIdx:    r.end,
			Lines:     plain[r.start:r.end],
			Severity:  SevError,
		}
	}
	return snippets
}

// enclosingStepTitle walks n.Parent chain upward to find the nearest NodeStep.
// Returns "(root)" if none is found.
func enclosingStepTitle(n *Node) string {
	cur := n.Parent
	for cur != nil {
		if cur.Kind == NodeStep {
			return cur.Title
		}
		cur = cur.Parent
	}
	return "(root)"
}
