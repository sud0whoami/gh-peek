package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// renderOutlineForGolden produces a deterministic, indented-tree
// text representation of an Outline for golden-file diffing.
//
// Format (each line is one node; children are indented by two spaces):
//
//	HeadDropped: <true|false>
//	[<kind>] <title> start=<n> end=<n> sev=<name> errors=<n> warnings=<n> notices=<n>[ dur=<humanized>][ SYNTHETIC]
//	  [<kind>] ...
//
// Where <kind> is one of "step", "group", "line:<sevname>".
// For NodeLine entries, the title is omitted and only "idx=<n>" is
// shown, with the severity name appearing in the kind tag.
// Duration is shown only when both StartTime and EndTime are non-zero
// and EndTime >= StartTime; format uses humanizeDuration below.
func renderOutlineForGolden(o *Outline) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "HeadDropped: %t\n", o.HeadDropped)
	for _, r := range o.Roots {
		writeNode(&sb, r, 0)
	}
	return sb.String()
}

func writeNode(sb *strings.Builder, n *Node, depth int) {
	indent := strings.Repeat("  ", depth)
	switch n.Kind {
	case NodeLine:
		fmt.Fprintf(sb, "%s[line:%s] idx=%d\n", indent, sevName(n.Sev), n.StartIdx)
	case NodeStep, NodeGroup:
		kind := "step"
		if n.Kind == NodeGroup {
			kind = "group"
		}
		fmt.Fprintf(sb, "%s[%s] %s start=%d end=%d sev=%s errors=%d warnings=%d notices=%d",
			indent, kind, n.Title, n.StartIdx, n.EndIdx, sevName(n.Sev),
			n.ErrorCount, n.WarningCount, n.NoticeCount)
		if !n.StartTime.IsZero() && !n.EndTime.IsZero() && !n.EndTime.Before(n.StartTime) {
			fmt.Fprintf(sb, " dur=%s", humanizeDuration(n.EndTime.Sub(n.StartTime)))
		}
		if n.Synthetic {
			sb.WriteString(" SYNTHETIC")
		}
		sb.WriteByte('\n')
	}
	for _, c := range n.Children {
		writeNode(sb, c, depth+1)
	}
}

func sevName(s Severity) string {
	switch s {
	case SevPlain:
		return "plain"
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
		return "unknown"
	}
}

func humanizeDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm%ds", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

func bufFrom(s string) *Buffer {
	b := New()
	b.Set([]byte(s))
	return b
}

func TestBuildOutline_Empty(t *testing.T) {
	o := BuildOutline(New())
	if o == nil {
		t.Fatal("BuildOutline returned nil")
	}
	if len(o.Roots) != 0 {
		t.Errorf("Roots = %d, want 0", len(o.Roots))
	}
	if o.HeadDropped {
		t.Error("HeadDropped should be false on empty buffer")
	}
}

func TestBuildOutline_NoGroups_LinesLiftedToRoots(t *testing.T) {
	o := BuildOutline(bufFrom("alpha\nbeta\ngamma\n"))
	// Synthetic "(ungrouped)" wrapper is spliced out; orphan lines are
	// lifted to top-level roots so they render directly without an extra
	// collapse layer that adds no information.
	if len(o.Roots) != 3 {
		t.Fatalf("Roots = %d, want 3", len(o.Roots))
	}
	for i, r := range o.Roots {
		if r.Kind != NodeLine {
			t.Errorf("root %d kind = %v, want NodeLine", i, r.Kind)
		}
		if r.StartIdx != i {
			t.Errorf("root %d StartIdx = %d, want %d", i, r.StartIdx, i)
		}
		if r.Parent != nil {
			t.Errorf("root %d Parent should be nil", i)
		}
	}
}

func TestBuildOutline_TwoSteps_NestedGroup(t *testing.T) {
	src := strings.Join([]string{
		"##[group]Step One",
		"hello",
		"##[group]Inner",
		"##[error]boom",
		"##[endgroup]",
		"##[endgroup]",
		"##[group]Step Two",
		"done",
		"##[endgroup]",
		"",
	}, "\n")
	o := BuildOutline(bufFrom(src))
	if len(o.Roots) != 2 {
		t.Fatalf("Roots = %d, want 2", len(o.Roots))
	}
	one := o.Roots[0]
	if one.Kind != NodeStep || one.Title != "Step One" || one.Synthetic {
		t.Errorf("Step One mismatch: %+v", one)
	}
	if one.StartIdx != 0 || one.EndIdx != 5 {
		t.Errorf("Step One span = [%d,%d), want [0,5)", one.StartIdx, one.EndIdx)
	}
	if len(one.Children) != 2 {
		t.Fatalf("Step One has %d children, want 2", len(one.Children))
	}
	if one.Children[0].Kind != NodeLine {
		t.Errorf("Step One child 0 kind = %v", one.Children[0].Kind)
	}
	inner := one.Children[1]
	if inner.Kind != NodeGroup || inner.Title != "Inner" {
		t.Errorf("Inner mismatch: %+v", inner)
	}
	if inner.StartIdx != 2 || inner.EndIdx != 4 {
		t.Errorf("Inner span = [%d,%d), want [2,4)", inner.StartIdx, inner.EndIdx)
	}
	two := o.Roots[1]
	if two.Title != "Step Two" || two.Synthetic {
		t.Errorf("Step Two mismatch: %+v", two)
	}
	if two.StartIdx != 6 || two.EndIdx != 8 {
		t.Errorf("Step Two span = [%d,%d), want [6,8)", two.StartIdx, two.EndIdx)
	}
}

func TestBuildOutline_StrayEndgroup_Ignored(t *testing.T) {
	o := BuildOutline(bufFrom("##[endgroup]\nplain\n"))
	// The stray endgroup is silently dropped; the plain line is lifted
	// to a top-level root.
	if len(o.Roots) != 1 {
		t.Fatalf("Roots = %d, want 1", len(o.Roots))
	}
	root := o.Roots[0]
	if root.Kind != NodeLine {
		t.Errorf("root.Kind = %v, want NodeLine", root.Kind)
	}
	if root.StartIdx != 1 {
		t.Errorf("root.StartIdx = %d, want 1", root.StartIdx)
	}
}

func TestBuildOutline_UnclosedGroupAtEOF_Closed(t *testing.T) {
	src := "##[group]Build\nfoo\nbar\n"
	o := BuildOutline(bufFrom(src))
	if len(o.Roots) != 1 {
		t.Fatalf("Roots = %d, want 1", len(o.Roots))
	}
	build := o.Roots[0]
	if build.Title != "Build" {
		t.Errorf("Title = %q", build.Title)
	}
	if build.EndIdx != 3 {
		t.Errorf("EndIdx = %d, want 3 (= total line count)", build.EndIdx)
	}
}

func TestBuildOutline_SeverityBubblesUp(t *testing.T) {
	src := strings.Join([]string{
		"##[group]Step",
		"##[group]Inner",
		"##[warning]careful",
		"##[error]oops",
		"##[endgroup]",
		"##[endgroup]",
		"",
	}, "\n")
	o := BuildOutline(bufFrom(src))
	step := o.Roots[0]
	if step.Sev != SevError {
		t.Errorf("step.Sev = %v, want SevError", step.Sev)
	}
	if step.ErrorCount != 1 || step.WarningCount != 1 {
		t.Errorf("step counts: errors=%d warnings=%d", step.ErrorCount, step.WarningCount)
	}
	inner := step.Children[0]
	if inner.Sev != SevError {
		t.Errorf("inner.Sev = %v", inner.Sev)
	}
	if inner.ErrorCount != 1 || inner.WarningCount != 1 {
		t.Errorf("inner counts: errors=%d warnings=%d", inner.ErrorCount, inner.WarningCount)
	}
}

func TestBuildOutline_DurationFromTimestamps(t *testing.T) {
	src := strings.Join([]string{
		"2024-01-01T00:00:00Z ##[group]Build",
		"2024-01-01T00:00:05Z compiling...",
		"2024-01-01T00:00:10Z ##[endgroup]",
		"",
	}, "\n")
	o := BuildOutline(bufFrom(src))
	if len(o.Roots) != 1 {
		t.Fatalf("roots=%d", len(o.Roots))
	}
	build := o.Roots[0]
	if build.Title != "Build" {
		t.Errorf("Title=%q", build.Title)
	}
	if got := build.EndTime.Sub(build.StartTime); got != 10*time.Second {
		t.Errorf("duration = %v, want 10s", got)
	}
}

func TestBuildOutline_ProcessExitCodeIsErrorWhenNonZero(t *testing.T) {
	o := BuildOutline(bufFrom("Process completed with exit code 1.\n"))
	if o.Roots[0].Sev != SevError {
		t.Errorf("Sev = %v, want SevError", o.Roots[0].Sev)
	}
}

func TestBuildOutline_ProcessExitCodeZeroIsPlain(t *testing.T) {
	o := BuildOutline(bufFrom("Process completed with exit code 0.\n"))
	if o.Roots[0].Sev != SevPlain {
		t.Errorf("Sev = %v, want SevPlain (zero exit)", o.Roots[0].Sev)
	}
}

func TestBuildOutline_HeadDroppedFromTruncatedBuffer(t *testing.T) {
	b := New()
	// Fill past MaxBytes with simple plain content (no markers).
	chunk := strings.Repeat("x", 11*1024) + "\n"
	for i := 0; i < 1024; i++ {
		b.Append([]byte(chunk))
	}
	if !b.Truncated() {
		t.Fatal("buffer was not truncated; test setup bug")
	}
	o := BuildOutline(b)
	if !o.HeadDropped {
		t.Error("HeadDropped should be true")
	}
	if len(o.Roots) < 1 {
		t.Fatal("expected at least one root")
	}
	leader := o.Roots[0]
	if !leader.Synthetic {
		t.Error("leader should be Synthetic")
	}
	if leader.Title != "(beginning of log not retained)" {
		t.Errorf("leader.Title = %q", leader.Title)
	}
}

func TestBuildOutline_GoldenFixture_Passing(t *testing.T) {
	assertOutlineGolden(t, "passing")
}

func TestBuildOutline_GoldenFixture_Failing(t *testing.T) {
	assertOutlineGolden(t, "failing")
}

func assertOutlineGolden(t *testing.T, name string) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", name+".log"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	b := New()
	b.Set(src)
	got := renderOutlineForGolden(BuildOutline(b))

	goldenPath := filepath.Join("testdata", name+".outline.txt")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_GOLDEN=1 to create): %v", err)
	}
	if string(want) != got {
		t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", goldenPath, want, got)
	}
}

// BenchmarkBuildOutline_10MB measures BuildOutline against a synthetic log
// that approaches the 10 MB tail-cap size. The target is < 5 ms.
//
// Run with:
//
//	go test -bench=BenchmarkBuildOutline_10MB -benchtime=5s ./internal/logs/...
func BenchmarkBuildOutline_10MB(b *testing.B) {
	// Build a realistic synthetic log: alternating group/endgroup pairs with
	// a mix of plain, command, error, and warning lines.
	const approxSize = 10 * 1024 * 1024 // 10 MB
	var sb strings.Builder
	lineN := 0
	for sb.Len() < approxSize {
		stepN := lineN / 200
		groupN := (lineN / 20) % 10
		fmt.Fprintf(&sb, "2024-01-01T10:%02d:%02dZ ##[group]Step %d group %d\n", stepN%60, groupN%60, stepN, groupN)
		for j := 0; j < 18; j++ {
			lineN++
			switch j % 4 {
			case 0:
				fmt.Fprintf(&sb, "2024-01-01T10:%02d:%02dZ plain output line %d\n", stepN%60, j, lineN)
			case 1:
				fmt.Fprintf(&sb, "2024-01-01T10:%02d:%02dZ ##[command]run step %d\n", stepN%60, j, lineN)
			case 2:
				fmt.Fprintf(&sb, "2024-01-01T10:%02d:%02dZ ##[warning]minor warning %d\n", stepN%60, j, lineN)
			case 3:
				fmt.Fprintf(&sb, "2024-01-01T10:%02d:%02dZ normal line %d\n", stepN%60, j, lineN)
			}
		}
		fmt.Fprintf(&sb, "2024-01-01T10:%02d:%02dZ ##[endgroup]\n", stepN%60, groupN%60)
		lineN++
	}
	src := []byte(sb.String())

	buf := New()
	buf.Set(src)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildOutline(buf)
	}
}
