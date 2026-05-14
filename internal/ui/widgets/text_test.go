package widgets

import (
	"testing"
	"time"
)

func TestTruncRune_NoTrunc(t *testing.T) {
	if got := TruncRune("hello", 10); got != "hello" {
		t.Fatalf("want hello got %q", got)
	}
}

func TestTruncRune_Truncated(t *testing.T) {
	got := TruncRune("hello world", 6)
	// "hello…" is 6 display cols
	if got != "hello…" {
		t.Fatalf("want %q got %q", "hello…", got)
	}
}

func TestTruncRune_Zero(t *testing.T) {
	if got := TruncRune("hello", 0); got != "" {
		t.Fatalf("want empty got %q", got)
	}
}

func TestTruncRune_One(t *testing.T) {
	if got := TruncRune("hello", 1); got != "…" {
		t.Fatalf("want '…' got %q", got)
	}
}

func TestPadRight(t *testing.T) {
	got := PadRight("hi", 5)
	if got != "hi   " {
		t.Fatalf("want %q got %q", "hi   ", got)
	}
}

func TestPadRight_NoOp(t *testing.T) {
	got := PadRight("hello", 3)
	if got != "hello" {
		t.Fatalf("already wide: want hello got %q", got)
	}
}

func TestClamp(t *testing.T) {
	if Clamp(5, 8, 20) != 8 {
		t.Fatal("below lo")
	}
	if Clamp(25, 8, 20) != 20 {
		t.Fatal("above hi")
	}
	if Clamp(12, 8, 20) != 12 {
		t.Fatal("in range")
	}
	if Clamp(3, 5, 0) != 5 {
		t.Fatal("hi=0 should only enforce lo")
	}
}

func TestHumanizeAgo(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s ago"},
		{2*time.Minute + 10*time.Second, "2m ago"},
		{3 * time.Hour, "3h ago"},
		{50 * time.Hour, "2d ago"},
		{-5 * time.Second, "0s ago"},
	}
	for _, tc := range cases {
		if got := HumanizeAgo(tc.d); got != tc.want {
			t.Errorf("HumanizeAgo(%v): want %q got %q", tc.d, tc.want, got)
		}
	}
}
