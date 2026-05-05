package browser

import (
	"context"
	"errors"
	"runtime"
	"testing"
)

func TestCommandFor_PerOS(t *testing.T) {
	tests := []struct {
		goos     string
		url      string
		wantName string
		wantArgs []string
	}{
		{"darwin", "https://example.com", "open", []string{"https://example.com"}},
		{"linux", "https://example.com", "xdg-open", []string{"https://example.com"}},
		{"windows", "https://example.com", "rundll32", []string{"url.dll,FileProtocolHandler", "https://example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			name, args, err := commandFor(tt.goos, tt.url)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args = %v, want %v", args, tt.wantArgs)
			}
			for i := range args {
				if args[i] != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, args[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestCommandFor_UnsupportedOS(t *testing.T) {
	if _, _, err := commandFor("plan9", "https://example.com"); err == nil {
		t.Fatal("expected error for unsupported GOOS")
	}
}

func TestCommandOpener_Records(t *testing.T) {
	var gotName string
	var gotArgs []string
	called := 0
	c := CommandOpener{
		Run: func(_ context.Context, name string, args ...string) error {
			called++
			gotName = name
			gotArgs = args
			return nil
		},
	}
	if err := c.Open(context.Background(), "https://example.com/x"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if called != 1 {
		t.Fatalf("Run called %d times, want 1", called)
	}
	wantName, wantArgs, _ := commandFor(runtime.GOOS, "https://example.com/x")
	if gotName != wantName {
		t.Errorf("name = %q, want %q", gotName, wantName)
	}
	if len(gotArgs) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestOpener_RejectsNonHTTPScheme(t *testing.T) {
	called := 0
	c := CommandOpener{
		Run: func(context.Context, string, ...string) error {
			called++
			return nil
		},
	}
	err := c.Open(context.Background(), "javascript:alert(1)")
	if err == nil {
		t.Fatal("expected error for javascript: URL")
	}
	if called != 0 {
		t.Errorf("Run was called for rejected URL")
	}
}

func TestOpener_RejectsEmptyURL(t *testing.T) {
	called := 0
	c := CommandOpener{
		Run: func(context.Context, string, ...string) error {
			called++
			return nil
		},
	}
	if err := c.Open(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty URL")
	}
	if called != 0 {
		t.Errorf("Run was called for empty URL")
	}
}

func TestCommandOpener_PropagatesRunError(t *testing.T) {
	want := errors.New("boom")
	c := CommandOpener{
		Run: func(context.Context, string, ...string) error { return want },
	}
	err := c.Open(context.Background(), "https://example.com")
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want wraps %v", err, want)
	}
}
