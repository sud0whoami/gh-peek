package table

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

// Align controls how cell content is padded within its column.
type Align int

const (
	AlignLeft  Align = iota
	AlignRight       // right-align: pad on the left
)

// Col describes a single table column.
type Col struct {
	Title   string // header text
	Min     int    // minimum width (hard floor; must be ≥ 1 in practice)
	Max     int    // maximum width (0 = unlimited)
	Ideal   int    // static preferred width; 0 = derive entirely from rows / title
	Elastic bool   // absorbs remaining slack; exactly one Col should set this
	Align   Align
}

// Table is the table descriptor. Create one per screen, reuse across renders.
type Table struct {
	Cols []Col
	// Sep is the column separator string (default " ").
	Sep string
}

// sep returns the effective separator.
func (t Table) sep() string {
	if t.Sep != "" {
		return t.Sep
	}
	return " "
}

// Layout returns the realized column widths for the given available pixel
// budget. If rows are provided, the Ideal for each column is computed as:
//
//	max(col.Ideal, titleWidth, max cell width across rows), capped at Max
//
// Rows should contain the plain (unformatted) text values to measure; pass
// styled strings for pre-formatted fixed-width cells only when you know the
// visual width matches the raw length (e.g. badge columns with a static Max).
//
// Algorithm:
//  1. natural[i] = clamp(ideal[i], Min[i], Max[i])
//  2. If sum + seps < budget  → grow elastic toward Max
//  3. If sum + seps > budget  → shrink elastic toward Min first, then others
func (t Table) Layout(width int, rows ...[]string) []int {
	n := len(t.Cols)
	if n == 0 {
		return nil
	}

	sep := t.sep()
	sepW := lipgloss.Width(sep)
	totalSepW := (n - 1) * sepW
	budget := width - totalSepW
	if budget < n {
		budget = n // at least 1 per col
	}

	// Step 1: compute ideals (possibly auto-measured from rows).
	ideals := make([]int, n)
	for i, c := range t.Cols {
		base := max3(c.Min, c.Ideal, lipgloss.Width(c.Title))
		if c.Max > 0 && base > c.Max {
			base = c.Max
		}
		ideals[i] = base
	}
	for _, row := range rows {
		for i := 0; i < n && i < len(row); i++ {
			w := lipgloss.Width(row[i])
			if t.Cols[i].Max > 0 && w > t.Cols[i].Max {
				w = t.Cols[i].Max
			}
			if w > ideals[i] {
				ideals[i] = w
			}
		}
	}

	// Step 2: find elastic column (first one declared elastic, or col 0).
	elasticIdx := 0
	for i, c := range t.Cols {
		if c.Elastic {
			elasticIdx = i
			break
		}
	}

	// Step 3: start at ideal widths.
	widths := make([]int, n)
	copy(widths, ideals)
	total := sumInts(widths)

	if total < budget {
		// Grow elastic column toward its Max (or budget if unlimited).
		elasticMax := t.Cols[elasticIdx].Max
		slack := budget - total
		if elasticMax > 0 {
			canGrow := elasticMax - widths[elasticIdx]
			if canGrow < 0 {
				canGrow = 0
			}
			if slack > canGrow {
				slack = canGrow
			}
		}
		widths[elasticIdx] += slack
	} else if total > budget {
		// 1. Drain elastic toward its Min first.
		eFloor := t.Cols[elasticIdx].Min
		if eFloor < 1 {
			eFloor = 1
		}
		for total > budget && widths[elasticIdx] > eFloor {
			widths[elasticIdx]--
			total--
		}
		// 2. Still over? Drain other cols in declared order until budget met.
		for i := 0; i < n && total > budget; i++ {
			if i == elasticIdx {
				continue
			}
			floor := t.Cols[i].Min
			if floor < 1 {
				floor = 1
			}
			for total > budget && widths[i] > floor {
				widths[i]--
				total--
			}
		}
	}

	return widths
}

// Header returns a formatted header line for the given realized widths.
// The style function (if non-nil) wraps the entire line, e.g. SectionLabel.
func (t Table) Header(widths []int, style func(string) string) string {
	cells := make([]string, len(t.Cols))
	for i, c := range t.Cols {
		w := colWidth(widths, i)
		cells[i] = padCell(truncRune(c.Title, w), w, c.Align)
	}
	line := strings.Join(cells, t.sep())
	if style != nil {
		return style(line)
	}
	return line
}

// Row returns a formatted data row for the given realized widths and cell
// values. Cells may be pre-styled strings; their visual width is measured
// with lipgloss.Width so ANSI sequences are not counted as characters.
func (t Table) Row(widths []int, cells []string) string {
	out := make([]string, len(t.Cols))
	for i, c := range t.Cols {
		w := colWidth(widths, i)
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		out[i] = padCell(cell, w, c.Align)
	}
	return strings.Join(out, t.sep())
}

// colWidth safely indexes widths, returning 0 when out of bounds.
func colWidth(widths []int, i int) int {
	if i < len(widths) {
		return widths[i]
	}
	return 0
}

// padCell pads/truncates cell to exactly w visible columns.
func padCell(cell string, w int, align Align) string {
	if w <= 0 {
		return ""
	}
	// Truncate if too wide.
	if lipgloss.Width(cell) > w {
		cell = truncRune(cell, w)
	}
	vw := lipgloss.Width(cell)
	pad := w - vw
	if pad <= 0 {
		return cell
	}
	spaces := strings.Repeat(" ", pad)
	if align == AlignRight {
		return spaces + cell
	}
	return cell + spaces
}

// truncRune truncates s to n display columns, appending "…" if cut.
func truncRune(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if w+rw > n-1 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	b.WriteRune('…')
	return b.String()
}

func sumInts(s []int) int {
	t := 0
	for _, v := range s {
		t += v
	}
	return t
}

func max3(a, b, c int) int {
	if b > a {
		a = b
	}
	if c > a {
		a = c
	}
	return a
}
