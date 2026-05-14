package table

import (
	"strings"
	"testing"
)

// helpers for tests -------------------------------------------------------

func cols3(elastic int) []Col {
	// Three cols: elastic at index `elastic`, others fixed 10.
	c := []Col{
		{Title: "A", Min: 4, Max: 30, Ideal: 10},
		{Title: "B", Min: 4, Max: 15, Ideal: 10},
		{Title: "C", Min: 4, Max: 12, Ideal: 10},
	}
	c[elastic].Elastic = true
	return c
}

// Layout tests -------------------------------------------------------------

func TestLayout_SlackGoesToElastic(t *testing.T) {
	tbl := Table{Cols: cols3(0)}
	// width=40 → budget = 40 - 2 seps = 38; naturals = 10+10+10=30; slack=8
	// elastic (col 0) grows from 10 to 18 (still ≤ Max=30)
	w := tbl.Layout(40)
	if w[0] != 18 {
		t.Fatalf("col0 want 18 got %d", w[0])
	}
	if w[1] != 10 || w[2] != 10 {
		t.Fatalf("non-elastic cols want 10,10 got %d,%d", w[1], w[2])
	}
}

func TestLayout_SlackCapAtMax(t *testing.T) {
	// elastic max is 30; even if budget allows more, cap at 30.
	tbl := Table{Cols: cols3(0)}
	// width=200 → budget=198; naturals=30; slack=168; elastic max=30, can grow by 20 → 30
	w := tbl.Layout(200)
	if w[0] != 30 {
		t.Fatalf("elastic should be capped at Max=30, got %d", w[0])
	}
}

func TestLayout_ElasticLast(t *testing.T) {
	tbl := Table{Cols: cols3(2)} // elastic is last
	w := tbl.Layout(200)
	if w[2] != 12 { // Max=12 for col2
		t.Fatalf("col2 (elastic) should cap at Max=12, got %d", w[2])
	}
}

func TestLayout_ShrinkElasticFirst(t *testing.T) {
	// width=20 → budget=18; naturals=30; need to shed 12.
	// elastic (col0, Max=30, Min=4) shrinks from 10 toward 4; need shed 12:
	//   col0: 10→4 = shed 6 → total now 24 still > 18
	//   col1: 10→4 = shed 6 → total now 18 = budget
	// So col2 untouched at 10.
	tbl := Table{Cols: cols3(0)}
	w := tbl.Layout(20)
	if w[0] != 4 {
		t.Fatalf("elastic col0 want 4 got %d", w[0])
	}
	if w[1] != 4 {
		t.Fatalf("col1 want 4 got %d", w[1])
	}
	if w[2] != 10 {
		t.Fatalf("col2 want 10 got %d", w[2])
	}
}

func TestLayout_AutoIdealFromRows(t *testing.T) {
	tbl := Table{Cols: []Col{
		{Title: "NAME", Min: 4, Max: 40, Elastic: true},
		{Title: "TYPE", Min: 4, Max: 12},
		{Title: "UPDATED", Min: 6, Max: 12},
	}}
	rows := [][]string{
		{"short", "container", "3d ago"},
		{"a-very-long-package-name-here", "npm", "1h ago"},
	}
	// ideals after auto: NAME=max(4,0,28→capped40)=28, TYPE=max(4,0,9)=9, UPDATED=max(6,0,7)=7
	// budget for width=100 = 100-2=98; natural=28+9+7=44; slack=54
	// elastic grows from 28 toward Max=40: +12 → 40
	w := tbl.Layout(100, rows...)
	if w[0] != 40 {
		t.Fatalf("elastic NAME want 40 got %d", w[0])
	}
	if w[1] != 9 {
		t.Fatalf("TYPE want 9 got %d", w[1])
	}
	if w[2] != 7 {
		t.Fatalf("UPDATED want 7 got %d", w[2])
	}
}

func TestLayout_AutoIdealRespectsTitleWidth(t *testing.T) {
	tbl := Table{Cols: []Col{
		{Title: "PUBLISHED", Min: 6, Max: 12},
		{Title: "X", Min: 4, Max: 40, Elastic: true},
	}}
	// No rows; ideal for PUBLISHED = max(6, 0, 9) = 9 (title width=9)
	w := tbl.Layout(50)
	if w[0] != 9 {
		t.Fatalf("PUBLISHED want 9 (from title width) got %d", w[0])
	}
}

func TestLayout_NoElasticUsesFirst(t *testing.T) {
	// No col has Elastic=true → col 0 is treated as elastic by default.
	tbl := Table{Cols: []Col{
		{Title: "A", Min: 4, Max: 30, Ideal: 10},
		{Title: "B", Min: 4, Max: 10, Ideal: 10},
	}}
	w := tbl.Layout(40) // budget=39; naturals=20; slack=19; col0 grows +19 → min(29,30)=29
	if w[0] != 29 {
		t.Fatalf("col0 want 29 got %d", w[0])
	}
}

func TestLayout_MaxZeroMeansUnlimited(t *testing.T) {
	tbl := Table{Cols: []Col{
		{Title: "A", Min: 4, Max: 0, Ideal: 10, Elastic: true},
		{Title: "B", Min: 4, Max: 10, Ideal: 10},
	}}
	// Max=0 on elastic means unlimited; all slack goes there.
	w := tbl.Layout(200) // budget=199; naturals=20; slack=179; elastic gets all
	if w[0] != 10+179 {
		t.Fatalf("col0 want %d got %d", 10+179, w[0])
	}
}

// Header/Row tests ---------------------------------------------------------

func TestHeader_LeftAlign(t *testing.T) {
	tbl := Table{Cols: []Col{
		{Title: "NAME"},
		{Title: "TYPE"},
	}}
	h := tbl.Header([]int{8, 6}, nil)
	// "NAME    " + " " + "TYPE  "
	if !strings.HasPrefix(h, "NAME    ") {
		t.Fatalf("unexpected header: %q", h)
	}
}

func TestHeader_RightAlign(t *testing.T) {
	tbl := Table{Cols: []Col{
		{Title: "COUNT", Align: AlignRight},
	}}
	h := tbl.Header([]int{8}, nil)
	// "   COUNT"
	if h != "   COUNT" {
		t.Fatalf("want %q got %q", "   COUNT", h)
	}
}

func TestRow_Truncation(t *testing.T) {
	tbl := Table{Cols: []Col{{Title: "T", Min: 4, Max: 8}}}
	// cell wider than width → truncated with ellipsis
	r := tbl.Row([]int{6}, []string{"hello world"})
	if len([]rune(r)) > 6 {
		t.Fatalf("row should be truncated to 6 runes, got %q", r)
	}
}

func TestRow_CustomSep(t *testing.T) {
	tbl := Table{
		Cols: []Col{{Title: "A"}, {Title: "B"}},
		Sep:  " | ",
	}
	r := tbl.Row([]int{3, 3}, []string{"foo", "bar"})
	if !strings.Contains(r, " | ") {
		t.Fatalf("want ' | ' separator in row, got %q", r)
	}
}

func TestLayout_ZeroWidth(t *testing.T) {
	tbl := Table{Cols: []Col{{Title: "A", Min: 4, Max: 10, Elastic: true}}}
	w := tbl.Layout(0)
	// budget = 0 < 1 col → clamped to 1; elastic starts at max(4,0,1)=4
	if len(w) != 1 {
		t.Fatalf("want 1 width, got %d", len(w))
	}
}
