package logs

import (
	"strings"
	"testing"
)

func TestBuffer_AppendAndLen(t *testing.T) {
	b := New()
	b.Append([]byte("hello\nworld\n"))
	if got := b.Len(); got != len("hello\nworld\n") {
		t.Errorf("Len = %d, want %d", got, len("hello\nworld\n"))
	}
	if got := b.Lines(); len(got) != 2 || got[0] != "hello" || got[1] != "world" {
		t.Errorf("Lines = %#v", got)
	}
	if b.Truncated() {
		t.Errorf("Truncated should be false")
	}
}

func TestBuffer_LinesAndPlainLinesParallel(t *testing.T) {
	b := New()
	b.Append([]byte("\x1b[31mred line\x1b[0m\nplain\n"))
	lines := b.Lines()
	plain := b.PlainLines()
	if len(lines) != len(plain) {
		t.Fatalf("len mismatch: lines=%d plain=%d", len(lines), len(plain))
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "\x1b[31m") {
		t.Errorf("Lines[0] should preserve ANSI, got %q", lines[0])
	}
	if plain[0] != "red line" {
		t.Errorf("PlainLines[0] = %q, want %q", plain[0], "red line")
	}
	if plain[1] != "plain" {
		t.Errorf("PlainLines[1] = %q", plain[1])
	}
}

func TestBuffer_Truncation(t *testing.T) {
	b := New()
	// Build content that exceeds MaxBytes.
	// 1024 lines of 11 KiB each → > MaxBytes.
	var sb strings.Builder
	for i := 0; i < 1024; i++ {
		sb.WriteString(strings.Repeat("x", 11*1024))
		sb.WriteByte('\n')
	}
	b.Append([]byte(sb.String()))
	if !b.Truncated() {
		t.Fatalf("expected Truncated true")
	}
	if b.Len() > MaxBytes {
		t.Errorf("Len = %d > MaxBytes = %d", b.Len(), MaxBytes)
	}
	// First byte must be the start of a fresh line — no half-line at the top.
	// Since each original line was 11 KiB of 'x', after dropping the
	// leading partial line the buffer must start with 'x' (or be empty
	// in pathological cases).
	raw := b.Raw()
	if len(raw) == 0 {
		t.Fatal("raw is empty after truncation")
	}
	// Every line in PlainLines must be 11*1024 'x' (no half-line at the top).
	for i, ln := range b.PlainLines() {
		if len(ln) != 11*1024 {
			t.Fatalf("line %d length = %d, want %d (leading half-line was not dropped)", i, len(ln), 11*1024)
		}
	}
}

func TestBuffer_TruncationStaysTrue(t *testing.T) {
	b := New()
	b.Append([]byte(strings.Repeat("a", MaxBytes+10)))
	if !b.Truncated() {
		t.Fatal("expected Truncated true")
	}
	// A subsequent small append must not reset Truncated.
	b.Append([]byte("\nshort\n"))
	if !b.Truncated() {
		t.Fatal("Truncated must remain true after later appends")
	}
}

func TestBuffer_Search(t *testing.T) {
	b := New()
	b.Append([]byte("alpha\nFooBar baz\nfoo again\nNothing\n"))
	got := b.Search("foo")
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("Search(foo) = %v, want [1 2]", got)
	}
	if got := b.Search(""); got != nil {
		t.Errorf("Search(\"\") = %v, want nil", got)
	}
	if got := b.Search("nope"); got != nil {
		t.Errorf("Search(no-match) = %v, want nil", got)
	}
}

func TestBuffer_FirstFailureLine(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    int
	}{
		{"none", "ok\nstill ok\n", -1},
		{"error_marker", "ok\n##[error] something\nmore\n", 1},
		{"exit_code", "step 1\nProcess completed with exit code 1\n", 1},
		{"case_insensitive_error", "ok\n##[ERROR] yikes\n", 1},
		{"case_insensitive_exit", "ok\nprocess completed With Exit Code 2\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := New()
			b.Append([]byte(tc.content))
			if got := b.FirstFailureLine(); got != tc.want {
				t.Errorf("FirstFailureLine = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestBuffer_StripANSI(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b]0;title\x07tail", "tail"},
		{"\x1b]0;title\x1b\\tail", "tail"},
		{"plain", "plain"},
		{"\x1b7stray", "stray"},
		{"a\x1b[1;31mB\x1b[0mc", "aBc"},
	}
	for _, tc := range cases {
		if got := stripANSI(tc.in); got != tc.want {
			t.Errorf("stripANSI(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuffer_Set(t *testing.T) {
	b := New()
	b.Set([]byte("first\nsecond\n"))
	if b.Truncated() {
		t.Fatal("Truncated should be false after small Set")
	}
	if got := b.Lines(); len(got) != 2 {
		t.Errorf("expected 2 lines, got %d (%v)", len(got), got)
	}
	// Set with overflow flips Truncated true.
	b.Set([]byte(strings.Repeat("a", MaxBytes+100)))
	if !b.Truncated() {
		t.Fatal("Truncated should be true after overflowing Set")
	}
}

// TestBuffer_CRLF verifies that Windows-style \r\n line endings are handled
// transparently. GitHub Actions job logs commonly use \r\n. The trailing \r
// must be stripped so that:
//   - Lines() / PlainLines() return content without trailing \r
//   - BuildOutline can match ##[group] titles against API step names
func TestBuffer_CRLF(t *testing.T) {
	b := New()
	b.Set([]byte("line one\r\nline two\r\nline three\r\n"))

	lines := b.Lines()
	if len(lines) != 3 {
		t.Fatalf("Lines() = %d, want 3", len(lines))
	}
	for i, l := range lines {
		if strings.HasSuffix(l, "\r") {
			t.Errorf("Lines()[%d] has trailing \\r: %q", i, l)
		}
	}

	plain := b.PlainLines()
	for i, l := range plain {
		if strings.HasSuffix(l, "\r") {
			t.Errorf("PlainLines()[%d] has trailing \\r: %q", i, l)
		}
	}
}

// TestBuffer_CRLF_GroupRecognition verifies that ##[group] markers in logs
// with \r\n endings are correctly recognized by BuildOutline. Without the
// trailing-\r strip, group titles would carry a stray \r that breaks marker
// classification and prevents downstream API-step header enrichment from
// matching API step names.
func TestBuffer_CRLF_GroupRecognition(t *testing.T) {
	src := "##[group]Set up job\r\nsome content\r\n##[endgroup]\r\n" +
		"##[group]Run tests\r\ndone\r\n##[endgroup]\r\n"
	b := New()
	b.Set([]byte(src))

	o := BuildOutline(b)
	if len(o.Roots) != 2 {
		t.Fatalf("Roots = %d, want 2; titles: %v", len(o.Roots), rootTitles(o))
	}
	if got := o.Roots[0].Title; got != "Set up job" {
		t.Errorf("Roots[0].Title = %q, want %q", got, "Set up job")
	}
	if got := o.Roots[1].Title; got != "Run tests" {
		t.Errorf("Roots[1].Title = %q, want %q", got, "Run tests")
	}
}

func rootTitles(o *Outline) []string {
	titles := make([]string, len(o.Roots))
	for i, r := range o.Roots {
		titles[i] = r.Title
	}
	return titles
}
