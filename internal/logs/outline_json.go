package logs

import "time"

// NodeJSON is the wire format for --json output.
// Times are RFC3339; zero times are omitted.
// "kind": "step" | "group" | "line"
// "severity": "none" | "debug" | "command" | "notice" | "warning" | "error"
type NodeJSON struct {
	Kind         string      `json:"kind"`
	Title        string      `json:"title,omitempty"`
	Severity     string      `json:"severity"`
	ErrorCount   int         `json:"error_count,omitempty"`
	WarningCount int         `json:"warning_count,omitempty"`
	NoticeCount  int         `json:"notice_count,omitempty"`
	StartLine    int         `json:"start_line,omitempty"` // 1-based
	EndLine      int         `json:"end_line,omitempty"`   // 1-based, inclusive
	StartTime    string      `json:"start_time,omitempty"`
	EndTime      string      `json:"end_time,omitempty"`
	Text         string      `json:"text,omitempty"`        // NodeLine only, ANSI-stripped
	LineNumber   int         `json:"line_number,omitempty"` // NodeLine only, 1-based
	Children     []*NodeJSON `json:"children,omitempty"`
}

// OutlineToJSON converts an Outline to its JSON-safe representation.
// b is used to resolve line text for NodeLine nodes.
func OutlineToJSON(o *Outline, b *Buffer) []*NodeJSON {
	if o == nil {
		return nil
	}
	result := make([]*NodeJSON, 0, len(o.Roots))
	for _, n := range o.Roots {
		result = append(result, nodeToJSON(n, b))
	}
	return result
}

func nodeToJSON(n *Node, b *Buffer) *NodeJSON {
	j := &NodeJSON{
		Kind:     kindName(n.Kind),
		Severity: sevJSONName(n.Sev),
	}

	switch n.Kind {
	case NodeLine:
		j.LineNumber = n.StartIdx + 1
		plain := b.PlainLines()
		if n.StartIdx >= 0 && n.StartIdx < len(plain) {
			j.Text = plain[n.StartIdx]
		}
		// NodeLine nodes have no children, title, counts, or times.

	case NodeStep, NodeGroup:
		j.Title = n.Title
		j.ErrorCount = n.ErrorCount
		j.WarningCount = n.WarningCount
		j.NoticeCount = n.NoticeCount
		j.StartLine = n.StartIdx + 1
		j.EndLine = n.EndIdx // EndIdx is exclusive 0-based = 1-based inclusive
		if !n.StartTime.IsZero() {
			j.StartTime = n.StartTime.UTC().Format(time.RFC3339)
		}
		if !n.EndTime.IsZero() {
			j.EndTime = n.EndTime.UTC().Format(time.RFC3339)
		}
		if len(n.Children) > 0 {
			j.Children = make([]*NodeJSON, 0, len(n.Children))
			for _, c := range n.Children {
				j.Children = append(j.Children, nodeToJSON(c, b))
			}
		}
	}

	return j
}

// kindName maps NodeKind to its JSON string.
func kindName(k NodeKind) string {
	switch k {
	case NodeStep:
		return "step"
	case NodeGroup:
		return "group"
	case NodeLine:
		return "line"
	default:
		return "unknown"
	}
}

// sevJSONName maps Severity to its JSON string.
func sevJSONName(s Severity) string {
	switch s {
	case SevPlain:
		return "none"
	case SevDebug:
		return "debug"
	case SevCommand:
		return "command"
	case SevNotice:
		return "notice"
	case SevWarning:
		return "warning"
	case SevError:
		return "error"
	default:
		return "none"
	}
}
