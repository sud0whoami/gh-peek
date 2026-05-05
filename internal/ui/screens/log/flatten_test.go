package log

import (
	"testing"

	"github.com/sud0whoami/gh-peek/internal/logs"
)

// ---------------------------------------------------------------------------
// Helpers: build synthetic Outline nodes without a Buffer.
// ---------------------------------------------------------------------------

func makeStep(title string, children ...*logs.Node) *logs.Node {
	step := &logs.Node{Kind: logs.NodeStep, Title: title}
	for _, c := range children {
		c.Parent = step
		step.Children = append(step.Children, c)
		if c.Sev > step.Sev {
			step.Sev = c.Sev
		}
		step.ErrorCount += c.ErrorCount
		step.WarningCount += c.WarningCount
		step.NoticeCount += c.NoticeCount
	}
	return step
}

func makeGroup(title string, sev logs.Severity, errCnt, warnCnt, noticeCnt int, children ...*logs.Node) *logs.Node {
	g := &logs.Node{
		Kind:         logs.NodeGroup,
		Title:        title,
		Sev:          sev,
		ErrorCount:   errCnt,
		WarningCount: warnCnt,
		NoticeCount:  noticeCnt,
	}
	for _, c := range children {
		c.Parent = g
		g.Children = append(g.Children, c)
	}
	return g
}

func makeLine(lineIdx int, sev logs.Severity) *logs.Node {
	return &logs.Node{Kind: logs.NodeLine, StartIdx: lineIdx, Sev: sev}
}

func makeOutline(roots ...*logs.Node) *logs.Outline {
	return &logs.Outline{Roots: roots}
}

// expanded builds a map[string]bool from a list of keys.
func expandedMap(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// ---------------------------------------------------------------------------
// Tests: ViewModeRaw
// ---------------------------------------------------------------------------

// TestFlatten_Raw returns nil (raw mode bypasses outline).
func TestFlatten_Raw(t *testing.T) {
	outline := makeOutline(makeStep("step A", makeLine(0, logs.SevPlain)))
	rows := flatten(outline, nil, ViewModeRaw)
	if rows != nil {
		t.Errorf("expected nil rows for ViewModeRaw, got %d rows", len(rows))
	}
}

// ---------------------------------------------------------------------------
// Tests: ViewModeOutline
// ---------------------------------------------------------------------------

// TestFlatten_Outline_NilOutline returns nil.
func TestFlatten_Outline_NilOutline(t *testing.T) {
	rows := flatten(nil, nil, ViewModeOutline)
	if rows != nil {
		t.Errorf("expected nil rows for nil outline, got %v", rows)
	}
}

// TestFlatten_Outline_CollapsedStepShowsOnlyHeader verifies that a collapsed
// step produces exactly one header row.
func TestFlatten_Outline_CollapsedStepShowsOnlyHeader(t *testing.T) {
	outline := makeOutline(
		makeStep("step A",
			makeLine(0, logs.SevPlain),
			makeLine(1, logs.SevError),
		),
	)
	rows := flatten(outline, expandedMap() /*nothing expanded*/, ViewModeOutline)
	if len(rows) != 1 {
		t.Fatalf("want 1 row (header), got %d", len(rows))
	}
	if !rows[0].IsHeader {
		t.Error("row 0 should be a header")
	}
	if !rows[0].Collapsed {
		t.Error("row 0 should be collapsed")
	}
	if rows[0].Depth != 0 {
		t.Errorf("step row depth = %d, want 0", rows[0].Depth)
	}
	if rows[0].Key != "0" {
		t.Errorf("step row key = %q, want %q", rows[0].Key, "0")
	}
}

// TestFlatten_Outline_ExpandedStepShowsChildren verifies that expanding a
// step shows its direct children.
func TestFlatten_Outline_ExpandedStepShowsChildren(t *testing.T) {
	outline := makeOutline(
		makeStep("step A",
			makeLine(0, logs.SevPlain), // child 0
			makeLine(1, logs.SevError), // child 1
		),
	)
	// Expand step "0".
	rows := flatten(outline, expandedMap("0"), ViewModeOutline)
	// Expect: header + 2 line rows.
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d: %+v", len(rows), rows)
	}
	if !rows[0].IsHeader {
		t.Error("row 0 should be header")
	}
	if rows[1].IsHeader || rows[1].LineIdx != 0 {
		t.Errorf("row 1 should be line 0, got %+v", rows[1])
	}
	if rows[2].IsHeader || rows[2].LineIdx != 1 {
		t.Errorf("row 2 should be line 1, got %+v", rows[2])
	}
	// Lines inside a step are at depth 1.
	if rows[1].Depth != 1 {
		t.Errorf("line row depth = %d, want 1", rows[1].Depth)
	}
}

// TestFlatten_Outline_ExpandedStepWithGroup verifies group headers appear
// when the step is expanded.
func TestFlatten_Outline_ExpandedStepWithGroup(t *testing.T) {
	grp := makeGroup("Run tests", logs.SevPlain, 0, 0, 0,
		makeLine(1, logs.SevPlain),
		makeLine(2, logs.SevPlain),
	)
	outline := makeOutline(makeStep("step A", makeLine(0, logs.SevPlain), grp))
	// Only expand step; leave group collapsed.
	rows := flatten(outline, expandedMap("0"), ViewModeOutline)
	// header(step) + line(0) + header(group) = 3 rows.
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	if !rows[2].IsHeader {
		t.Error("row 2 should be group header")
	}
	if rows[2].Collapsed != true {
		t.Error("group header should be collapsed")
	}
	if rows[2].Key != "0/1" {
		t.Errorf("group key = %q, want %q", rows[2].Key, "0/1")
	}
}

// TestFlatten_Outline_ExpandedGroup shows lines inside the group.
func TestFlatten_Outline_ExpandedGroup(t *testing.T) {
	grp := makeGroup("Run tests", logs.SevPlain, 0, 0, 0,
		makeLine(1, logs.SevPlain),
		makeLine(2, logs.SevError),
	)
	outline := makeOutline(makeStep("step A", grp))
	// Expand step and group.
	rows := flatten(outline, expandedMap("0", "0/0"), ViewModeOutline)
	// header(step) + header(group) + line(1) + line(2) = 4 rows.
	if len(rows) != 4 {
		t.Fatalf("want 4 rows, got %d: %+v", len(rows), rows)
	}
	if rows[2].LineIdx != 1 || rows[3].LineIdx != 2 {
		t.Errorf("lines mismatch: rows[2].LineIdx=%d rows[3].LineIdx=%d", rows[2].LineIdx, rows[3].LineIdx)
	}
	// Group header at depth 1; lines at depth 2.
	if rows[1].Depth != 1 {
		t.Errorf("group header depth = %d, want 1", rows[1].Depth)
	}
	if rows[2].Depth != 2 {
		t.Errorf("line depth inside group = %d, want 2", rows[2].Depth)
	}
}

// TestFlatten_Outline_CollapsedGroupShowsSeverityLines verifies that error,
// warning, and notice lines bleed through a collapsed group.
func TestFlatten_Outline_CollapsedGroupShowsSeverityLines(t *testing.T) {
	grp := makeGroup("Run tests", logs.SevError, 1, 1, 1,
		makeLine(1, logs.SevPlain),   // hidden
		makeLine(2, logs.SevDebug),   // hidden
		makeLine(3, logs.SevCommand), // hidden
		makeLine(4, logs.SevNotice),  // visible
		makeLine(5, logs.SevWarning), // visible
		makeLine(6, logs.SevError),   // visible
	)
	outline := makeOutline(makeStep("step A", grp))
	// Only expand step; leave group collapsed.
	rows := flatten(outline, expandedMap("0"), ViewModeOutline)
	// header(step) + header(group) + 3 severity lines = 5 rows.
	if len(rows) != 5 {
		t.Fatalf("want 5 rows, got %d: %+v", len(rows), rows)
	}
	// rows[0] = step header, rows[1] = group header (collapsed)
	// rows[2..4] = notice, warning, error lines
	lineIdxs := []int{4, 5, 6}
	for i, want := range lineIdxs {
		got := rows[2+i].LineIdx
		if got != want {
			t.Errorf("severity line[%d].LineIdx = %d, want %d", i, got, want)
		}
		if rows[2+i].IsHeader {
			t.Errorf("severity line[%d] should not be a header", i)
		}
		// Depth should be group.Depth+1 = 2.
		if rows[2+i].Depth != 2 {
			t.Errorf("severity line[%d].Depth = %d, want 2", i, rows[2+i].Depth)
		}
	}
}

// TestFlatten_Outline_CollapsedGroupHidesPlainLines verifies plain lines are
// hidden in a collapsed group.
func TestFlatten_Outline_CollapsedGroupHidesPlainLines(t *testing.T) {
	grp := makeGroup("Setup", logs.SevPlain, 0, 0, 0,
		makeLine(1, logs.SevPlain),
		makeLine(2, logs.SevPlain),
		makeLine(3, logs.SevPlain),
	)
	outline := makeOutline(makeStep("step A", grp))
	rows := flatten(outline, expandedMap("0"), ViewModeOutline)
	// header(step) + header(group) = 2 rows; no plain bleed-through.
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
}

// TestFlatten_Outline_MultipleSteps verifies multiple step headers appear.
func TestFlatten_Outline_MultipleSteps(t *testing.T) {
	outline := makeOutline(
		makeStep("step A", makeLine(0, logs.SevPlain)),
		makeStep("step B", makeLine(1, logs.SevPlain)),
		makeStep("step C", makeLine(2, logs.SevError)),
	)
	rows := flatten(outline, expandedMap(), ViewModeOutline)
	if len(rows) != 3 {
		t.Fatalf("want 3 header rows, got %d", len(rows))
	}
	keys := []string{"0", "1", "2"}
	for i, want := range keys {
		if rows[i].Key != want {
			t.Errorf("rows[%d].Key = %q, want %q", i, rows[i].Key, want)
		}
	}
}

// TestFlatten_Outline_NestedGroupCollapsedShowsSeverityLines verifies that
// severity lines inside a doubly-nested collapsed group bleed through.
func TestFlatten_Outline_NestedGroupCollapsedShowsSeverityLines(t *testing.T) {
	inner := makeGroup("inner", logs.SevError, 1, 0, 0,
		makeLine(5, logs.SevPlain),
		makeLine(6, logs.SevError),
	)
	outer := makeGroup("outer", logs.SevError, 1, 0, 0, inner)
	outline := makeOutline(makeStep("step A", outer))
	// Expand step only; outer and inner collapsed.
	rows := flatten(outline, expandedMap("0"), ViewModeOutline)
	// header(step) + header(outer) + error line(6) = 3 rows.
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d: %+v", len(rows), rows)
	}
	if rows[2].LineIdx != 6 {
		t.Errorf("error line LineIdx = %d, want 6", rows[2].LineIdx)
	}
	// Depth: step=0, outer=1, severity line from nested = 2 (outer depth+1).
	if rows[2].Depth != 2 {
		t.Errorf("nested severity line depth = %d, want 2", rows[2].Depth)
	}
}

// ---------------------------------------------------------------------------
// Tests: ViewModeCompact
// ---------------------------------------------------------------------------

// TestFlatten_Compact_HidesPlainLinesInExpandedGroup verifies that plain
// lines are hidden in compact mode even when the group is expanded.
func TestFlatten_Compact_HidesPlainLinesInExpandedGroup(t *testing.T) {
	grp := makeGroup("Build", logs.SevError, 1, 0, 0,
		makeLine(0, logs.SevPlain),
		makeLine(1, logs.SevCommand),
		makeLine(2, logs.SevNotice),
		makeLine(3, logs.SevWarning),
		makeLine(4, logs.SevError),
	)
	outline := makeOutline(makeStep("step A", grp))
	// Expand step and group.
	rows := flatten(outline, expandedMap("0", "0/0"), ViewModeCompact)
	// header(step) + header(group) + notice + warning + error = 5 rows.
	// SevPlain=0 and SevCommand=2 are both < SevNotice=3 → hidden.
	if len(rows) != 5 {
		t.Fatalf("want 5 rows, got %d: %+v", len(rows), rows)
	}
	lineIdxs := []int{2, 3, 4}
	for i, want := range lineIdxs {
		got := rows[2+i].LineIdx
		if got != want {
			t.Errorf("compact line[%d].LineIdx = %d, want %d", i, got, want)
		}
	}
}

// TestFlatten_Compact_ShowsHeadersAndSeverityLinesOnly verifies compact mode
// shows step/group headers plus notice/warning/error lines, but no plain or
// command lines even in expanded scope.
func TestFlatten_Compact_ShowsHeadersAndSeverityLinesOnly(t *testing.T) {
	outline := makeOutline(
		makeStep("step A",
			makeLine(0, logs.SevPlain),
			makeLine(1, logs.SevCommand),
			makeLine(2, logs.SevNotice),
		),
	)
	// Expand step.
	rows := flatten(outline, expandedMap("0"), ViewModeCompact)
	// header(step) + notice line = 2 rows. plain and command are hidden.
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d: %+v", len(rows), rows)
	}
	if rows[1].LineIdx != 2 {
		t.Errorf("visible line LineIdx = %d, want 2", rows[1].LineIdx)
	}
}
