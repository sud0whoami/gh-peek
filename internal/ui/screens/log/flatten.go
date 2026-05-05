package log

import (
	"fmt"

	"github.com/sud0whoami/gh-peek/internal/logs"
)

// ViewMode selects the rendering style for the log body.
type ViewMode int

// ViewMode values.
const (
	// ViewModeOutline (default) shows a collapsible step/group tree.
	ViewModeOutline ViewMode = iota
	// ViewModeCompact shows headers and severity lines (SevNotice and worse)
	// only — plain and command lines are hidden even in expanded nodes.
	ViewModeCompact
	// ViewModeRaw shows the unstructured log, identical to the M5 renderer.
	ViewModeRaw
)

// row is one entry in the visible-row slice used by outline and compact modes.
// In raw mode the caller uses the existing flat renderer and does not
// populate visibleRows.
type row struct {
	Node      *logs.Node // nil for raw rows
	Depth     int
	IsHeader  bool
	LineIdx   int    // -1 for headers; index into Buffer.Lines() for line rows
	Collapsed bool   // header-only field; true when the node is not expanded
	Key       string // expansion key for header rows; empty for line rows
}

// flatten projects an outline and expanded map into a flat []row.
// For ViewModeRaw it returns nil; the caller should use the existing
// flat renderer instead.
//
// expanded maps stable node path keys ("0", "0/2", "0/2/1" …) to true when
// that node is expanded. Keys absent from the map are treated as collapsed.
func flatten(outline *logs.Outline, expanded map[string]bool, viewMode ViewMode) []row {
	if viewMode == ViewModeRaw {
		return nil
	}
	if outline == nil {
		return nil
	}
	var rows []row
	for i, root := range outline.Roots {
		key := fmt.Sprintf("%d", i)
		// Roots may be NodeLine when BuildOutline lifted orphan lines
		// out of a synthetic "(ungrouped)" wrapper. Render them as
		// content lines at depth 0 rather than collapsible headers.
		if root.Kind == logs.NodeLine {
			if viewMode == ViewModeCompact && root.Sev < logs.SevNotice {
				continue
			}
			rows = append(rows, row{
				Node:     root,
				Depth:    0,
				IsHeader: false,
				LineIdx:  root.StartIdx,
			})
			continue
		}
		isExpanded := expanded[key]
		rows = append(rows, row{
			Node:      root,
			Depth:     0,
			IsHeader:  true,
			LineIdx:   -1,
			Collapsed: !isExpanded,
			Key:       key,
		})
		if isExpanded {
			rows = append(rows, flattenChildren(root, 1, key, expanded, viewMode)...)
		}
	}
	return rows
}

// flattenChildren recurses into a node's children, producing rows for each
// visible child. depth is the indentation level for the children. parentKey
// is the parent node's expansion key.
func flattenChildren(parent *logs.Node, depth int, parentKey string, expanded map[string]bool, viewMode ViewMode) []row {
	var rows []row
	for i, child := range parent.Children {
		childKey := parentKey + "/" + fmt.Sprintf("%d", i)
		switch child.Kind {
		case logs.NodeGroup:
			isExpanded := expanded[childKey]
			rows = append(rows, row{
				Node:      child,
				Depth:     depth,
				IsHeader:  true,
				LineIdx:   -1,
				Collapsed: !isExpanded,
				Key:       childKey,
			})
			if isExpanded {
				rows = append(rows, flattenChildren(child, depth+1, childKey, expanded, viewMode)...)
			} else {
				// Collapsed group: bleed through severity lines from all descendants.
				rows = append(rows, flattenSeverityLines(child, depth+1)...)
			}
		case logs.NodeLine:
			if viewMode == ViewModeCompact && child.Sev < logs.SevNotice {
				continue
			}
			rows = append(rows, row{
				Node:     child,
				Depth:    depth,
				IsHeader: false,
				LineIdx:  child.StartIdx,
			})
		}
	}
	return rows
}

// flattenSeverityLines walks a collapsed node's entire subtree and emits
// rows for severity lines at or above SevNotice (Notice, Warning, Error).
// All such lines are emitted at depth regardless of nesting. Plain, Debug,
// and Command lines are suppressed.
func flattenSeverityLines(parent *logs.Node, depth int) []row {
	var rows []row
	for _, child := range parent.Children {
		switch child.Kind {
		case logs.NodeGroup:
			// Recurse into nested (also collapsed) groups at the same depth.
			rows = append(rows, flattenSeverityLines(child, depth)...)
		case logs.NodeLine:
			if child.Sev >= logs.SevNotice {
				rows = append(rows, row{
					Node:     child,
					Depth:    depth,
					IsHeader: false,
					LineIdx:  child.StartIdx,
				})
			}
		}
	}
	return rows
}
