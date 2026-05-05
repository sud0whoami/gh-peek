package main

import (
	"bytes"
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
