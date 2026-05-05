// Package testutil holds shared test fakes, fixtures, and helpers
// for gh-peek (golden helpers, color-profile locking, and
// timestamp normalization).
package testutil

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// FixedWidth is the constant terminal width used by golden tests.
const FixedWidth = 100

// FixedWidthValue returns FixedWidth. Provided as a function for tests
// that prefer a call expression.
func FixedWidthValue() int { return FixedWidth }

// LockColorProfile pins Lip Gloss's default writer to TrueColor for
// the duration of the test. The previous profile is restored via
// t.Cleanup.
func LockColorProfile(t *testing.T) {
	t.Helper()
	prev := lipgloss.Writer.Profile
	lipgloss.Writer.Profile = colorprofile.TrueColor
	t.Cleanup(func() {
		lipgloss.Writer.Profile = prev
	})
}

// tb is the small subset of testing.TB used by the golden helper, so
// the helper can be exercised with a fake in unit tests.
type tb interface {
	Helper()
	Fatalf(format string, args ...any)
}

var (
	rfc3339Re = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})`)
	relTimeRe = regexp.MustCompile(`\b\d+[smhd] ago\b`)
)

// NormalizeTimestamps replaces RFC3339 timestamps and common relative
// time strings (Xs ago, Xm ago, Xh ago, Xd ago) with the literal "<TS>".
func NormalizeTimestamps(s string) string {
	s = rfc3339Re.ReplaceAllString(s, "<TS>")
	s = relTimeRe.ReplaceAllString(s, "<TS>")
	return s
}

// AssertGolden compares actual against the golden file at
// testdata/<name>.golden in the calling test's package directory.
//
// If the env var UPDATE_GOLDEN=1 is set, the file is written instead.
func AssertGolden(t *testing.T, name, actual string) {
	t.Helper()
	dir, err := callerDir()
	if err != nil {
		t.Fatalf("AssertGolden: locate caller dir: %v", err)
		return
	}
	assertGoldenInDir(t, dir, name, actual)
}

// assertGoldenInDir is the unit-testable core of AssertGolden. It
// resolves the golden file as <baseDir>/testdata/<name>.golden.
func assertGoldenInDir(t tb, baseDir, name, actual string) {
	t.Helper()
	path := filepath.Join(baseDir, "testdata", name+".golden")

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("AssertGolden: mkdir %s: %v", filepath.Dir(path), err)
			return
		}
		if err := os.WriteFile(path, []byte(actual), 0o644); err != nil {
			t.Fatalf("AssertGolden: write %s: %v", path, err)
			return
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Fatalf("AssertGolden: golden file %s does not exist (run with UPDATE_GOLDEN=1 to create)", path)
			return
		}
		t.Fatalf("AssertGolden: read %s: %v", path, err)
		return
	}
	if !bytes.Equal(want, []byte(actual)) {
		t.Fatalf("golden mismatch for %s:\n--- want ---\n%s\n--- got ---\n%s", path, want, actual)
	}
}

// callerDir returns the directory of the file that called AssertGolden.
func callerDir() (string, error) {
	// Skip: callerDir, AssertGolden, calling test.
	_, file, _, ok := runtime.Caller(2)
	if !ok {
		return "", fmt.Errorf("runtime.Caller failed")
	}
	return filepath.Dir(file), nil
}
