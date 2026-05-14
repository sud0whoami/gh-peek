package layout

import (
	"strings"
	"testing"
)

func TestCompute_Zero(t *testing.T) {
	f := Compute(0)
	if f.TooNarrow {
		t.Fatal("expected TooNarrow=false for width 0")
	}
	if f.Content != DefaultMaxWidth {
		t.Fatalf("want Content=%d got %d", DefaultMaxWidth, f.Content)
	}
	if f.LeftPad != 0 {
		t.Fatalf("want LeftPad=0 got %d", f.LeftPad)
	}
}

func TestCompute_TooNarrow(t *testing.T) {
	f := Compute(60)
	if !f.TooNarrow {
		t.Fatal("expected TooNarrow=true for width 60")
	}
	if f.Content != 60 {
		t.Fatalf("want Content=60 got %d", f.Content)
	}
}

func TestCompute_AtMinWidth(t *testing.T) {
	f := Compute(MinWidth)
	if f.TooNarrow {
		t.Fatal("expected TooNarrow=false for MinWidth")
	}
	if f.Content != MinWidth {
		t.Fatalf("want Content=%d got %d", MinWidth, f.Content)
	}
	if f.LeftPad != 0 {
		t.Fatalf("want LeftPad=0 got %d", f.LeftPad)
	}
}

func TestCompute_BelowCap(t *testing.T) {
	f := Compute(100)
	if f.TooNarrow {
		t.Fatal("unexpected TooNarrow")
	}
	if f.Content != 100 {
		t.Fatalf("want Content=100 got %d", f.Content)
	}
	if f.LeftPad != 0 {
		t.Fatalf("want LeftPad=0 for width below cap, got %d", f.LeftPad)
	}
}

func TestCompute_AtCap(t *testing.T) {
	f := Compute(DefaultMaxWidth)
	if f.Content != DefaultMaxWidth {
		t.Fatalf("want Content=%d got %d", DefaultMaxWidth, f.Content)
	}
	if f.LeftPad != 0 {
		t.Fatalf("want LeftPad=0 got %d", f.LeftPad)
	}
}

func TestCompute_WideTerminal(t *testing.T) {
	f := Compute(200)
	if f.Content != DefaultMaxWidth {
		t.Fatalf("want Content=%d got %d", DefaultMaxWidth, f.Content)
	}
	want := (200 - DefaultMaxWidth) / 2
	if f.LeftPad != want {
		t.Fatalf("want LeftPad=%d got %d", want, f.LeftPad)
	}
	if f.TooNarrow {
		t.Fatal("unexpected TooNarrow")
	}
}

func TestCompute_EnvOverride(t *testing.T) {
	t.Setenv("GHPEEK_MAX_WIDTH", "120")
	f := Compute(200)
	if f.Content != 120 {
		t.Fatalf("want Content=120 got %d", f.Content)
	}
	want := (200 - 120) / 2
	if f.LeftPad != want {
		t.Fatalf("want LeftPad=%d got %d", want, f.LeftPad)
	}
}

func TestCompute_EnvOverrideBelowMin(t *testing.T) {
	// Invalid values (below MinWidth) are ignored.
	t.Setenv("GHPEEK_MAX_WIDTH", "50")
	f := Compute(200)
	if f.Content != DefaultMaxWidth {
		t.Fatalf("env value below MinWidth should be ignored; want Content=%d got %d", DefaultMaxWidth, f.Content)
	}
}

func TestFrame_Center_NoPad(t *testing.T) {
	f := Frame{LeftPad: 0}
	in := "hello\nworld"
	if got := f.Center(in); got != in {
		t.Fatalf("want %q got %q", in, got)
	}
}

func TestFrame_Center_WithPad(t *testing.T) {
	f := Frame{LeftPad: 4}
	got := f.Center("a\nbb\n\nc")
	lines := strings.Split(got, "\n")
	// non-empty lines get padded; empty stays empty
	if lines[0] != "    a" {
		t.Fatalf("line 0: want %q got %q", "    a", lines[0])
	}
	if lines[1] != "    bb" {
		t.Fatalf("line 1: want %q got %q", "    bb", lines[1])
	}
	if lines[2] != "" {
		t.Fatalf("line 2 (empty): want empty got %q", lines[2])
	}
	if lines[3] != "    c" {
		t.Fatalf("line 3: want %q got %q", "    c", lines[3])
	}
}
