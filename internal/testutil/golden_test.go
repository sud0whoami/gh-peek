package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeTimestamps(t *testing.T) {
	LockColorProfile(t)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"rfc3339_z", "started 2024-05-12T10:34:56Z", "started <TS>"},
		{"rfc3339_offset", "at 2024-05-12T10:34:56+02:00 done", "at <TS> done"},
		{"seconds_ago", "ran 5s ago", "ran <TS>"},
		{"minutes_ago", "queued 12m ago", "queued <TS>"},
		{"hours_ago", "finished 3h ago", "finished <TS>"},
		{"days_ago", "created 7d ago", "created <TS>"},
		{"mixed", "2024-05-12T10:34:56Z and 5m ago", "<TS> and <TS>"},
		{"plain", "no timestamps here", "no timestamps here"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeTimestamps(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizeTimestamps(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestAssertGoldenInDir_Match(t *testing.T) {
	dir := t.TempDir()
	const name = "sample"
	const content = "hello golden\n"

	if err := os.MkdirAll(filepath.Join(dir, "testdata"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "testdata", name+".golden"), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fake := &fakeT{}
	assertGoldenInDir(fake, dir, name, content)
	if fake.failed {
		t.Fatalf("expected match to pass, got failure: %s", fake.msg)
	}
}

func TestAssertGoldenInDir_Mismatch(t *testing.T) {
	dir := t.TempDir()
	const name = "sample"
	if err := os.MkdirAll(filepath.Join(dir, "testdata"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "testdata", name+".golden"), []byte("expected\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fake := &fakeT{}
	assertGoldenInDir(fake, dir, name, "actual\n")
	if !fake.failed {
		t.Fatal("expected mismatch to fail the test")
	}
	if !strings.Contains(fake.msg, "golden mismatch") {
		t.Fatalf("expected error message to mention mismatch, got %q", fake.msg)
	}
}

func TestAssertGoldenInDir_UpdateWritesFile(t *testing.T) {
	dir := t.TempDir()
	const name = "sample"
	const content = "fresh\n"

	t.Setenv("UPDATE_GOLDEN", "1")
	fake := &fakeT{}
	assertGoldenInDir(fake, dir, name, content)
	if fake.failed {
		t.Fatalf("update mode should not fail, got: %s", fake.msg)
	}

	got, err := os.ReadFile(filepath.Join(dir, "testdata", name+".golden"))
	if err != nil {
		t.Fatalf("read written golden: %v", err)
	}
	if string(got) != content {
		t.Fatalf("written golden = %q, want %q", got, content)
	}
}

// fakeT implements the small subset of testing.TB used by
// assertGoldenInDir so the helper can be exercised without aborting
// the parent test.
type fakeT struct {
	failed bool
	msg    string
}

func (f *fakeT) Helper() {}
func (f *fakeT) Fatalf(format string, args ...any) {
	f.failed = true
	f.msg = fmt.Sprintf(format, args...)
}
func (f *fakeT) Errorf(format string, args ...any) {
	f.failed = true
	f.msg = fmt.Sprintf(format, args...)
}
