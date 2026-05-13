package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sud0whoami/gh-peek/internal/app"
)

func TestRun_NonTTYExitsWithCode2(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := app.Run(&stdout, &stderr, false)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	const want = "gh-peek is interactive; run it from a terminal\n"
	if got := stderr.String(); got != want {
		t.Fatalf("unexpected stderr:\n  got:  %q\n  want: %q", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
}

// TestRun_UnknownFlagExitsTwo confirms the flag parser is wired up and
// rejects unknown flags with exit 2.
func TestRun_UnknownFlagExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--definitely-not-a-flag"}, &stdout, &stderr, false)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr=%q", code, stderr.String())
	}
}

// TestRun_HelpFlagListsWeb confirms the --web flag is registered with
// the parser and appears in the help output.
func TestRun_HelpFlagListsWeb(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// -h triggers flag.ErrHelp; ContinueOnError surfaces it as a non-zero
	// exit from run. We only care that the registered flag name appears
	// in the help output written to stderr.
	_ = run([]string{"-h"}, &stdout, &stderr, false)
	if !strings.Contains(stderr.String(), "-web") {
		t.Fatalf("help output missing --web flag; stderr=%q", stderr.String())
	}
}

// TestRun_NoArgsNonTTYDelegatesToApp confirms that without --web, the
// non-TTY guard fires
func TestRun_NoArgsNonTTYDelegatesToApp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr, false)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "interactive") {
		t.Fatalf("stderr = %q, want contains %q", stderr.String(), "interactive")
	}
}
