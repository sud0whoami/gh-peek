package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, name string) *Buffer {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("load fixture %q: %v", name, err)
	}
	b := New()
	b.Set(data)
	return b
}

func TestExtract_PassingLogReturnsEmpty(t *testing.T) {
	b := loadFixture(t, "passing.log")
	o := BuildOutline(b)
	snippets := Extract(b, o, 5)
	if len(snippets) != 0 {
		t.Errorf("passing log: want 0 snippets, got %d", len(snippets))
		for i, s := range snippets {
			t.Logf("  snippet[%d]: step=%q lines=%v", i, s.StepTitle, s.Lines)
		}
	}
}

func TestExtract_FailingLogReturnsSnippets(t *testing.T) {
	b := loadFixture(t, "failing.log")
	o := BuildOutline(b)
	snippets := Extract(b, o, 5)
	if len(snippets) == 0 {
		t.Fatal("failing log: want ≥1 snippet, got 0")
	}
	// At least one snippet should contain the ##[error] line text.
	found := false
	for _, s := range snippets {
		for _, line := range s.Lines {
			if strings.Contains(line, "Process completed with exit code") {
				found = true
			}
		}
	}
	if !found {
		t.Error("no snippet contained the ##[error] line")
	}
	// Severity must be SevError.
	for i, s := range snippets {
		if s.Severity != SevError {
			t.Errorf("snippet[%d]: severity = %v, want SevError", i, s.Severity)
		}
	}
}

func TestExtract_TwoErrorsClose_Merged(t *testing.T) {
	// Two errors 2 lines apart — with contextLines=3 they should merge.
	raw := "" +
		"2024-01-01T00:00:00Z ##[group]step\n" +
		"2024-01-01T00:00:00Z line0\n" +
		"2024-01-01T00:00:00Z ##[error]error one\n" + // line 2
		"2024-01-01T00:00:00Z between\n" +
		"2024-01-01T00:00:00Z ##[error]error two\n" + // line 4
		"2024-01-01T00:00:00Z line5\n" +
		"2024-01-01T00:00:00Z ##[endgroup]\n"
	b := New()
	b.Set([]byte(raw))
	o := BuildOutline(b)
	snippets := Extract(b, o, 3)
	if len(snippets) != 1 {
		t.Errorf("want 1 merged snippet, got %d", len(snippets))
	}
}

func TestExtract_TwoErrorsFar_Separate(t *testing.T) {
	// Two errors far apart (>2*contextLines gap) — should produce 2 snippets.
	var lines []string
	lines = append(lines, "2024-01-01T00:00:00Z ##[group]step")
	lines = append(lines, "2024-01-01T00:00:00Z ##[error]error one") // line 1
	// 30 plain lines of padding
	for i := 0; i < 30; i++ {
		lines = append(lines, "2024-01-01T00:00:00Z plain line")
	}
	lines = append(lines, "2024-01-01T00:00:00Z ##[error]error two") // line 32
	lines = append(lines, "2024-01-01T00:00:00Z ##[endgroup]")

	raw := strings.Join(lines, "\n") + "\n"
	b := New()
	b.Set([]byte(raw))
	o := BuildOutline(b)
	snippets := Extract(b, o, 2)
	if len(snippets) != 2 {
		t.Errorf("want 2 separate snippets, got %d", len(snippets))
	}
}

func TestExtract_ProcessExitCode_NoErrorMarker(t *testing.T) {
	// Log with "Process completed with exit code 1" but no ##[error].
	raw := "" +
		"2024-01-01T00:00:00Z ##[group]run tests\n" +
		"2024-01-01T00:00:00Z some output\n" +
		"2024-01-01T00:00:00Z Process completed with exit code 1\n" +
		"2024-01-01T00:00:00Z ##[endgroup]\n"
	b := New()
	b.Set([]byte(raw))
	o := BuildOutline(b)
	snippets := Extract(b, o, 2)
	if len(snippets) == 0 {
		t.Fatal("want ≥1 snippet for exit code line, got 0")
	}
	found := false
	for _, s := range snippets {
		for _, line := range s.Lines {
			if strings.Contains(line, "exit code 1") {
				found = true
			}
		}
	}
	if !found {
		t.Error("snippet does not contain 'exit code 1' line")
	}
}

func TestExtract_EmptyBuffer_NoFallback(t *testing.T) {
	b := New()
	o := BuildOutline(b)
	snippets := Extract(b, o, 5)
	if len(snippets) != 0 {
		t.Errorf("empty buffer: want 0 snippets, got %d", len(snippets))
	}
}

func TestExtract_NoMarkersWithContent_FallbackSnippet(t *testing.T) {
	// Content with no ##[error] markers and no "exit code N" → fallback snippet.
	raw := "" +
		"2024-01-01T00:00:00Z ##[group]run tests\n" +
		"2024-01-01T00:00:00Z some output\n" +
		"2024-01-01T00:00:00Z more output\n" +
		"2024-01-01T00:00:00Z final line\n" +
		"2024-01-01T00:00:00Z ##[endgroup]\n"
	b := New()
	b.Set([]byte(raw))
	o := BuildOutline(b)
	snippets := Extract(b, o, 3)
	if len(snippets) != 1 {
		t.Errorf("want 1 fallback snippet, got %d", len(snippets))
	}
	if len(snippets) > 0 {
		if snippets[0].StepTitle != "(no error markers)" {
			t.Errorf("fallback step title = %q, want %q", snippets[0].StepTitle, "(no error markers)")
		}
		if snippets[0].Severity != SevError {
			t.Errorf("fallback severity = %v, want SevError", snippets[0].Severity)
		}
	}
}

func TestExtract_SnippetLinesANSIStripped(t *testing.T) {
	// Lines with ANSI should be stripped in snippet Lines.
	raw := "2024-01-01T00:00:00Z ##[group]step\n" +
		"2024-01-01T00:00:00Z \x1b[31m##[error]red error\x1b[0m\n" +
		"2024-01-01T00:00:00Z ##[endgroup]\n"
	b := New()
	b.Set([]byte(raw))
	o := BuildOutline(b)
	snippets := Extract(b, o, 1)
	if len(snippets) == 0 {
		t.Fatal("want ≥1 snippet")
	}
	for _, s := range snippets {
		for _, line := range s.Lines {
			if strings.Contains(line, "\x1b") {
				t.Errorf("snippet line contains ANSI escape: %q", line)
			}
		}
	}
}

func TestExtract_AdjacentRangesNoDuplicateLines(t *testing.T) {
	// Two errors at consecutive lines — merged range should not duplicate them.
	raw := "" +
		"2024-01-01T00:00:00Z ##[group]step\n" +
		"2024-01-01T00:00:00Z ##[error]first\n" +
		"2024-01-01T00:00:00Z ##[error]second\n" +
		"2024-01-01T00:00:00Z ##[endgroup]\n"
	b := New()
	b.Set([]byte(raw))
	o := BuildOutline(b)
	snippets := Extract(b, o, 1)
	if len(snippets) != 1 {
		t.Errorf("want 1 merged snippet, got %d", len(snippets))
	}
	if len(snippets) > 0 {
		// Check no duplicate lines.
		seen := map[string]int{}
		for _, line := range snippets[0].Lines {
			seen[line]++
		}
		for line, count := range seen {
			if count > 1 {
				t.Errorf("duplicate line in snippet: %q (count=%d)", line, count)
			}
		}
	}
}
