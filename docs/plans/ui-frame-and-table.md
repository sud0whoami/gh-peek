# Implementation Plan — UI Frame + Smart Table Widget

Status: proposed
Owner: TBD
Branch: `feature/ui-frame-and-table` (suggested)

## Goal
Make gh-peek visually consistent across terminal sizes by:

1. **Capping content width** with a centered frame so wide terminals stop
   stretching list rows past readable width.
2. **Allocating column widths from content needs** instead of dumping all
   slack into a single column. Eliminates the "TAGS column eats 200 cols"
   problem the user reported on the package detail screen.

The work touches all four list screens
(`runs`, `releases`, `packages`, `pkgscreen`) plus the root model.

## Non-goals
- Decorative borders around the frame. (Trivial follow-up; intentionally
  out of scope to keep this PR focused on layout correctness.)
- Vertical centering. Top-alignment is preserved everywhere because the
  log viewer / run detail rely on viewport scroll math.
- Restyling the run-detail two-pane layout. The 100-col threshold for
  jobs/steps split stays as-is; only its inputs change to "content
  width" instead of raw terminal width.
- Per-screen custom max widths. One project-wide cap.

## Design overview

### Two new packages

#### `internal/ui/layout`
Pure-data frame computation. No Bubble Tea, no lipgloss-rendered output —
just integers and a left-pad helper.

```go
package layout

const (
    MinWidth = 80   // below this we render a "too narrow" message
    MaxWidth = 140  // content cap; slack becomes left/right padding
)

type Frame struct {
    Terminal  int  // raw terminal width
    Content   int  // capped width that screens render into
    LeftPad   int  // spaces prepended to each line for centering
    TooNarrow bool // true when Terminal < MinWidth
}

func Compute(terminalWidth int) Frame
func (f Frame) Center(s string) string  // pads each line with LeftPad
```

`Compute(0)` returns `{Content: MaxWidth}` so default-init / tests work
without a `WindowSizeMsg`.

#### `internal/ui/widgets/table`
Declarative column sizer. Sole API:

```go
package table

type Align int
const (
    AlignLeft Align = iota
    AlignRight
)

type Col struct {
    Title   string
    Min     int    // hard floor; truncation kicks in at this width
    Max     int    // hard ceiling; never grows past this
    Ideal   int    // preferred width (often "longest content seen")
    Elastic bool   // absorbs slack; exactly one column should set this
    Align   Align
}

type Table struct {
    Cols []Col
    Sep  string // default " "
}

// Layout returns realized widths summing to <= width (after sep).
// Rules:
//  1. Start each col at min(Ideal, Max).
//  2. While sum + seps < width: grow Elastic col toward Max.
//  3. While sum + seps > width: shrink Elastic toward Min, then shrink
//     other cols toward Min in declared order.
//  4. If still over after all cols are at Min: truncation at render.
func (t Table) Layout(width int) []int

// Header / Row produce a single line padded/truncated to fit. Uses
// existing widgets helpers for unicode-safe truncation.
func (t Table) Header(widths []int) string
func (t Table) Row(widths []int, cells []string) string
```

Algorithm choices:
- **Elastic = exactly one** by convention. If zero or many, the sizer
  picks the first as elastic and logs a warning in tests (not in prod).
- **Ideal defaults to Min** when zero. Saves callers from boilerplate
  for static columns like `STATUS`.
- **No row-scanning auto-Ideal in v1**. Callers pass a sensible Ideal
  (e.g. `len("v0.99.99")` for VERSION). Auto-measurement is a future
  enhancement; keeps the widget pure-data and side-effect-free.

### Root model wiring

`internal/app/model.go`:

1. Add a `frame layout.Frame` field, recomputed from `WindowSizeMsg`.
2. When propagating `WindowSizeMsg` to children, **rewrite `Width` to
   `frame.Content`**. Each screen continues to render to whatever width
   it was told — no change inside screens regarding "is there a frame".
3. In root `View()`, post-process the active screen's view by
   `frame.Center(view.String())` so each line is left-padded.
4. When `frame.TooNarrow`, render a one-line muted message
   ("terminal too narrow — needs ≥80 columns") instead of delegating.

### Helper consolidation

`truncRune`, `padRight`, `clampInt` are duplicated in `pkgscreen`,
`packages`, `releases`. Move to `internal/ui/widgets` (a new
`widgets.go`) and import. Removes ~30 lines per screen and prevents
drift.

## Phase plan

Each phase ends with `make check` green and a focused commit.

### Phase 1 — `internal/ui/layout` package
Files:
- `internal/ui/layout/frame.go`
- `internal/ui/layout/frame_test.go`
- `internal/ui/layout/doc.go`

Tests (TDD):
- `Compute(0)` → `{Content: MaxWidth, LeftPad: 0, TooNarrow: false}`.
- `Compute(60)` → `{TooNarrow: true, Content: 60}`.
- `Compute(100)` → `{Content: 100, LeftPad: 0}`.
- `Compute(200)` → `{Content: MaxWidth, LeftPad: (200-MaxWidth)/2}`.
- `Frame{LeftPad: 4}.Center("a\nbb")` → `"    a\n    bb"`.
- ANSI-safe centering: input with SGR escapes is padded by the same
  left-pad without breaking sequences (use existing
  `lipgloss.Width`/`runewidth` for rune counting).

### Phase 2 — `internal/ui/widgets/table` package
Files:
- `internal/ui/widgets/table/table.go`
- `internal/ui/widgets/table/table_test.go`
- `internal/ui/widgets/table/doc.go`

Tests (TDD), targeting the sizer first:
- 3 cols at Ideal=10 with elastic=col0 in width=40 → `[18, 10, 10]`
  (slack of 8 minus 2 seps goes to col0).
- Same in width=20 → `[8, 5, 5]` if Min=5 (proportional shrink, elastic
  shrinks first toward Min before others).
- `Max` enforced when budget exceeds: width=200, col0 elastic Max=30 →
  col0 = 30, leftover unused.
- No elastic set: first column is treated as elastic (or fail-safe:
  give all slack to col[0]). Pick one and document.
- Row rendering preserves order, pads/truncates per realized width and
  Align; right-aligned columns space-pad on the left.
- Sep with width > 1 (`" · "`) is accounted for in budget math.

### Phase 3 — Helper consolidation
Files:
- `internal/ui/widgets/text.go` (new): `TruncRune`, `PadRight`,
  `Clamp` (renames of clampInt).
- `internal/ui/widgets/text_test.go`.
- Remove duplicates in `pkgscreen/view.go`, `packages/view.go`,
  `releases/view.go`. Re-import.

This phase has zero behavioral change; only refactor + tests for the
moved helpers.

### Phase 4 — Root model frame integration
Files:
- `internal/app/model.go`
- `internal/app/router_test.go` (extend)

Changes:
- New field `frame layout.Frame`.
- On `WindowSizeMsg{Width: w, Height: h}`: compute `frame`, then
  forward `WindowSizeMsg{Width: frame.Content, Height: h}` to children.
- In `View()`: if `frame.TooNarrow`, return a `tea.NewView(...)` with
  the muted message; otherwise call `frame.Center(child.View().String())`
  (or whatever `tea.View` exposes) and return that.
- Propagate `frame.Content` into all `Width` fields when constructing
  child models in the existing `runs.OpenRunMsg` etc. handlers.

Tests:
- Sending `WindowSizeMsg{Width: 200}` to root yields a child width of
  `MaxWidth` (140), not 200.
- Sending `WindowSizeMsg{Width: 70}` puts the root in too-narrow mode
  and child View is not invoked.
- Sending `WindowSizeMsg{Width: 100}` is a passthrough (no padding,
  no truncation).

### Phase 5 — Migrate `runs` list to Table widget
Files:
- `internal/ui/screens/runs/view.go`
- `internal/ui/screens/runs/runs_test.go` (golden may shift)
- `internal/ui/screens/runs/testdata/runs_ready.golden` (refresh)

Replace the current ad-hoc column math with:

```go
tbl := table.Table{Cols: []table.Col{
    {Title: "WORKFLOW", Min: 6,  Max: 24, Ideal: 16},
    {Title: "TITLE",    Min: 10, Max: 60, Ideal: 30, Elastic: true},
    {Title: "BRANCH",   Min: 6,  Max: 20, Ideal: 14},
    {Title: "EVENT",    Min: 4,  Max: 12, Ideal: 8},
    {Title: "STATUS",   Min: 12, Max: 14, Ideal: 14},
    {Title: "UPDATED",  Min: 8,  Max: 12, Ideal: 10},
}}
widths := tbl.Layout(m.width)
```

Run `go test ./internal/ui/screens/runs/... -update` (or hand-edit) to
regenerate the golden, eyeball the diff, commit.

### Phase 6 — Migrate `releases` list
Same shape as Phase 5. Columns approx:
`TAG | NAME | STATUS | ASSETS | PUBLISHED` with `NAME` elastic.

### Phase 7 — Migrate `packages` list
`TYPE | NAME | VISIBILITY | VERS | UPDATED`, `NAME` elastic.
- Cap `TYPE` at 10 (`container` is the longest), `VERS` at 6, etc.

### Phase 8 — Migrate `pkgscreen` (versions)
`NAME | TAGS | CREATED` for container/docker, `NAME | CREATED` otherwise.
- For container/docker: `NAME` Min=20 Max=20 Ideal=20 (digest is
  always `sha256:` + 12 hex), `TAGS` Min=8 Max=32 Ideal=24 elastic,
  `CREATED` Min=8 Max=10 Ideal=10.
- For non-container: `NAME` elastic, `CREATED` fixed.

This phase directly fixes the reported bug.

### Phase 9 — Docs
Update:
- `.github/copilot-instructions.md`:
  - Add a "Layout & framing" section under `## UX rules`.
  - Add Milestone 11 entry summarizing the new packages and table
    widget.
  - Update `## Code map` with `internal/ui/layout` and
    `internal/ui/widgets/table`.
- `docs/architecture.md`: short "App routing" addendum noting that
  `WindowSizeMsg` is rewritten by the root before being delegated.
- `docs/code-map.md`: add the two new packages.
- `README.md` (Features): mention "centered, max-width frame so wide
  terminals stay readable".

## Test strategy
- All existing screen golden tests run at width=100, which is below
  `MaxWidth=140`, so no centering happens in them — they stay valid.
- New layout tests at width 60 / 100 / 200 cover frame branches.
- Table sizer is pure-data, so unit tests can be exhaustive
  (parameterized table-test in Go).
- Add **one** new screen test per migrated screen at width 200 to
  prove that columns no longer stretch beyond `Max`.

## Risks & mitigations
- **Risk**: `tea.View` post-processing may not be straightforward in
  Bubble Tea v2 if `View` is opaque. **Mitigation**: peek at
  `charm.land/bubbletea/v2` View API before Phase 4; if opaque, push
  the centering down by passing `Frame` into each screen's `Params` and
  having the screen call `frame.Center` on its own output (slightly
  less clean, still localized).
- **Risk**: Run detail's two-pane threshold (currently `width >= 100`)
  is now evaluated against content width, not terminal width. **Effect**:
  on a 250-col terminal it still goes two-pane (content is 140). On a
  90-col terminal it goes stacked. This matches user intent and need
  not change.
- **Risk**: Helper consolidation in Phase 3 churns 4 files; do it as a
  pure-refactor commit *before* Phase 4 to keep diffs reviewable.
- **Risk**: Goldens need regenerating in Phases 5–8. Update them per
  phase, never as a single mega-commit, so reviewers can read the
  visual diff for each screen separately.

## Open questions
- `MaxWidth = 140` — defensible default, mirrors common code-style
  guides. Should it be env-overridable (`GHPEEK_MAX_WIDTH`)? Defer to
  v1.2 unless feedback demands it.
- Centering vs. left-aligning the frame on wide terminals — both are
  valid. Plan goes with **centered** because that's what
  lazygit/k9s/glow do and the user explicitly mentioned "prettier".
  Trivial flip if we change our minds.
- Future "auto-Ideal from rows" extension to the Table widget — keep
  it for v1.1 once we see real-world tag list lengths in production.

## Acceptance criteria
- On a 250-col terminal, the package detail TAGS column no longer
  exceeds ~32 chars; total content width is ≤ 140; everything is
  visually centered.
- On an 80-col terminal, every screen renders identically to today
  (no regression).
- Below 80-col, root displays the "too narrow" message instead of a
  broken layout.
- `make check` green.
- All four list screens use `table.Table`; the duplicated
  `truncRune`/`padRight`/`clampInt` helpers exist in exactly one
  place (`internal/ui/widgets`).
