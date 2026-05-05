package log

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sud0whoami/gh-peek/internal/githubapi"
	"github.com/sud0whoami/gh-peek/internal/logs"
)

// defaultTickInterval is the default auto-refresh polling cadence.
const defaultTickInterval = 7 * time.Second

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return m.fetchCmd()
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampTop()
		return m, nil

	case logLoadedMsg:
		return m.handleLoaded(msg), m.scheduleTick()

	case tickMsg:
		return m.handleTick()

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleLoaded(msg logLoadedMsg) *Model {
	// ErrLogTooLarge is partial-success: bytes are present.
	if msg.Err != nil && !errors.Is(msg.Err, githubapi.ErrLogTooLarge) {
		if m.state == stateLoading {
			m.loadErr = msg.Err
			m.state = stateError
			return m
		}
		m.refreshErr = msg.Err
		return m
	}
	isFirstLoad := m.state == stateLoading
	m.refreshErr = nil
	m.truncatedFromAPI = errors.Is(msg.Err, githubapi.ErrLogTooLarge)
	m.buf.Set(msg.Bytes)
	m.state = stateReady
	m.lastRefreshed = m.params.Now()

	// Rebuild the structural outline directly from the log buffer's native
	// ##[group] markers. The outline mirrors what the runner emitted —
	// no API-step partitioning is attempted because the GitHub Actions API
	// reports step boundaries at second precision only, which produces
	// unstable / incorrect groupings when consecutive steps share a
	// second. Native groups are stable and accurate; API step metadata is
	// still surfaced as a header badge / duration when a top-level group's
	// title happens to match an API step name (see renderOutlineHeader).
	m.outline = logs.BuildOutline(m.buf)
	// On first load, initialise the expansion map and apply auto-expand policy.
	// On refresh, preserve the existing expanded map so user choices survive.
	if isFirstLoad {
		m.expanded = make(map[string]bool)
		m.applyAutoExpand()
	}
	m.rebuildRows()
	m.recomputeMatches()
	m.clampTop()
	return m
}

func (m *Model) handleTick() (tea.Model, tea.Cmd) {
	if !m.autoRefresh || !m.runActive || m.input.Focused() {
		return m, m.tickCmd()
	}
	return m, m.fetchCmd()
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Quit is global.
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	}

	// While search input is focused, route most keys to it.
	if m.input.Focused() {
		switch msg.String() {
		case "esc":
			m.input.Reset()
			m.input.Blur()
			m.matches = nil
			m.matchCursor = -1
			return m, nil
		case "enter":
			m.input.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.recomputeMatches()
		return m, cmd
	}

	switch msg.String() {
	case "up", "k":
		m.moveUp(1)
		return m, nil
	case "down", "j":
		m.moveDown(1)
		return m, nil
	case "pgup", "ctrl+u":
		m.moveUp(m.pageSize())
		return m, nil
	case "pgdown", "ctrl+d":
		m.moveDown(m.pageSize())
		return m, nil
	case "g", "home":
		m.moveToTop()
		return m, nil
	case "G", "end":
		m.moveToBottom()
		return m, nil
	case "/":
		cmd := m.input.Focus()
		return m, cmd
	case "n":
		m.cycleMatch(+1)
		return m, nil
	case "N":
		m.cycleMatch(-1)
		return m, nil
	case "w":
		m.wrap = !m.wrap
		m.clampTop()
		return m, nil
	case "F":
		m.jumpToFirstFailure()
		return m, nil
	case "r":
		return m, m.fetchCmd()
	case "R":
		m.autoRefresh = !m.autoRefresh
		return m, nil
	case "o":
		url := m.jobURL()
		if url == "" {
			return m, nil
		}
		return m, func() tea.Msg { return OpenInBrowserMsg{URL: url} }
	case "esc", "b":
		return m, func() tea.Msg { return BackMsg{} }
	case "?":
		m.showHelp = !m.showHelp
		return m, nil

	// Outline-mode keys.
	case "enter", " ":
		if m.viewMode != ViewModeRaw {
			m.toggleAt(m.cursor)
		}
		return m, nil
	case "right", "l":
		if m.viewMode != ViewModeRaw {
			m.expandAt(m.cursor)
		}
		return m, nil
	case "left", "h":
		if m.viewMode != ViewModeRaw {
			m.collapseAt(m.cursor)
		}
		return m, nil
	case "E":
		if m.viewMode != ViewModeRaw {
			m.expandAll()
		}
		return m, nil
	case "O":
		if m.viewMode != ViewModeRaw {
			m.collapseAll()
		}
		return m, nil
	case "v":
		m.cycleMode()
		return m, nil
	case "t":
		m.showTimestamps = !m.showTimestamps
		return m, nil
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Outline helpers
// ---------------------------------------------------------------------------

// rebuildRows recomputes visibleRows from the current outline, expanded map,
// and view mode. In raw mode, visibleRows is set to nil.
func (m *Model) rebuildRows() {
	if m.viewMode == ViewModeRaw {
		m.visibleRows = nil
		return
	}
	m.visibleRows = flatten(m.outline, m.expanded, m.viewMode)
	m.clampCursor()
	m.scrollToCursor()
}

// applyAutoExpand seeds the expansion map on first load.
func (m *Model) applyAutoExpand() {
	if m.outline == nil {
		return
	}
	// Rule 3: truncated-head synthetic leader is always expanded.
	if m.outline.HeadDropped && len(m.outline.Roots) > 0 {
		m.expanded["0"] = true
	}
	// Rules 1 & 2: expand every step with errors and path to first error.
	for i, step := range m.outline.Roots {
		stepKey := fmt.Sprintf("%d", i)
		if step.ErrorCount > 0 {
			m.expanded[stepKey] = true
			expandToFirstError(step, stepKey, m.expanded)
		}
	}
}

// expandToFirstError walks the subtree of node looking for the first NodeLine
// with SevError. On success it marks all ancestors expanded and returns true.
func expandToFirstError(node *logs.Node, key string, expanded map[string]bool) bool {
	for i, child := range node.Children {
		childKey := key + "/" + fmt.Sprintf("%d", i)
		if child.Kind == logs.NodeLine && child.Sev == logs.SevError {
			expanded[key] = true
			return true
		}
		if child.Kind == logs.NodeGroup {
			if expandToFirstError(child, childKey, expanded) {
				expanded[key] = true
				return true
			}
		}
	}
	return false
}

// toggleAt toggles the expansion state of the header row at rowIdx.
func (m *Model) toggleAt(rowIdx int) {
	if rowIdx < 0 || rowIdx >= len(m.visibleRows) {
		return
	}
	r := m.visibleRows[rowIdx]
	if !r.IsHeader {
		return
	}
	m.expanded[r.Key] = !m.expanded[r.Key]
	m.rebuildRows()
}

// expandAt expands the header row at rowIdx.
func (m *Model) expandAt(rowIdx int) {
	if rowIdx < 0 || rowIdx >= len(m.visibleRows) {
		return
	}
	r := m.visibleRows[rowIdx]
	if !r.IsHeader {
		return
	}
	if !m.expanded[r.Key] {
		m.expanded[r.Key] = true
		m.rebuildRows()
	}
}

// collapseAt collapses the focused header or, if already collapsed, jumps to
// the nearest parent header above.
func (m *Model) collapseAt(rowIdx int) {
	if rowIdx < 0 || rowIdx >= len(m.visibleRows) {
		return
	}
	r := m.visibleRows[rowIdx]
	if r.IsHeader && m.expanded[r.Key] {
		m.expanded[r.Key] = false
		m.rebuildRows()
		return
	}
	// Already collapsed or a line row: jump to nearest ancestor header.
	for i := rowIdx - 1; i >= 0; i-- {
		if m.visibleRows[i].IsHeader && m.visibleRows[i].Depth < r.Depth {
			m.cursor = i
			m.scrollToCursor()
			return
		}
	}
}

// expandAll marks every step and group as expanded.
func (m *Model) expandAll() {
	if m.outline == nil {
		return
	}
	expandAllNodes(m.outline.Roots, "", m.expanded)
	m.rebuildRows()
}

// expandAllNodes recursively marks every step and group as expanded.
func expandAllNodes(nodes []*logs.Node, prefix string, expanded map[string]bool) {
	for i, node := range nodes {
		var key string
		if prefix == "" {
			key = fmt.Sprintf("%d", i)
		} else {
			key = prefix + "/" + fmt.Sprintf("%d", i)
		}
		if node.Kind == logs.NodeStep || node.Kind == logs.NodeGroup {
			expanded[key] = true
			expandAllNodes(node.Children, key, expanded)
		}
	}
}

// collapseAll clears all expansion state.
func (m *Model) collapseAll() {
	m.expanded = make(map[string]bool)
	m.rebuildRows()
}

// cycleMode advances the view mode: outline → compact → raw → outline.
func (m *Model) cycleMode() {
	switch m.viewMode {
	case ViewModeOutline:
		m.viewMode = ViewModeCompact
	case ViewModeCompact:
		m.viewMode = ViewModeRaw
	case ViewModeRaw:
		m.viewMode = ViewModeOutline
	}
	m.rebuildRows()
	m.clampTop()
}

// expandAncestors expands all ancestors of lineIdx so the line is visible.
// Returns the visible-row index of the line after expanding, or -1 if not
// found.
func (m *Model) expandAncestors(lineIdx int) int {
	if m.outline == nil {
		return -1
	}
	changed := false
	for i, step := range m.outline.Roots {
		stepKey := fmt.Sprintf("%d", i)
		if markAncestorsForLine(step, stepKey, lineIdx, m.expanded) {
			changed = true
			break
		}
	}
	if changed {
		m.rebuildRows()
	}
	return m.rowForLineIdx(lineIdx)
}

// markAncestorsForLine searches node's subtree for NodeLine with StartIdx ==
// lineIdx. On success it marks all ancestor keys expanded and returns true.
func markAncestorsForLine(node *logs.Node, key string, lineIdx int, expanded map[string]bool) bool {
	for i, child := range node.Children {
		childKey := key + "/" + fmt.Sprintf("%d", i)
		switch child.Kind {
		case logs.NodeLine:
			if child.StartIdx == lineIdx {
				expanded[key] = true
				return true
			}
		case logs.NodeGroup:
			if markAncestorsForLine(child, childKey, lineIdx, expanded) {
				expanded[key] = true
				return true
			}
		}
	}
	return false
}

// rowForLineIdx returns the visible-row index for the first row whose LineIdx
// matches lineIdx. Returns -1 if not found.
func (m *Model) rowForLineIdx(lineIdx int) int {
	for i, r := range m.visibleRows {
		if !r.IsHeader && r.LineIdx == lineIdx {
			return i
		}
	}
	return -1
}

// jumpToFirstFailure navigates to the first failure line, expanding ancestors
// in outline/compact mode.
func (m *Model) jumpToFirstFailure() {
	i := m.buf.FirstFailureLine()
	if i < 0 {
		return
	}
	if m.viewMode == ViewModeRaw {
		m.top = i
		m.clampTop()
		return
	}
	rowIdx := m.expandAncestors(i)
	if rowIdx >= 0 {
		m.cursor = rowIdx
		m.scrollToCursor()
	}
}

// ---------------------------------------------------------------------------
// Navigation helpers
// ---------------------------------------------------------------------------

// moveUp moves cursor (outline/compact) or viewport (raw) up by n.
func (m *Model) moveUp(n int) {
	if m.viewMode == ViewModeRaw {
		m.scrollBy(-n)
		return
	}
	m.cursor -= n
	m.clampCursor()
	m.scrollToCursor()
}

// moveDown moves cursor (outline/compact) or viewport (raw) down by n.
func (m *Model) moveDown(n int) {
	if m.viewMode == ViewModeRaw {
		m.scrollBy(n)
		return
	}
	m.cursor += n
	m.clampCursor()
	m.scrollToCursor()
}

// moveToTop moves to the first row.
func (m *Model) moveToTop() {
	if m.viewMode == ViewModeRaw {
		m.top = 0
		return
	}
	m.cursor = 0
	m.scrollToCursor()
}

// moveToBottom moves to the last row.
func (m *Model) moveToBottom() {
	if m.viewMode == ViewModeRaw {
		m.scrollToBottom()
		return
	}
	n := len(m.visibleRows)
	if n > 0 {
		m.cursor = n - 1
	}
	m.scrollToCursor()
}

// clampCursor ensures cursor is in [0, len(visibleRows)-1].
func (m *Model) clampCursor() {
	if m.cursor < 0 {
		m.cursor = 0
	}
	n := len(m.visibleRows)
	if n == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
}

// scrollToCursor adjusts m.top to keep m.cursor visible.
func (m *Model) scrollToCursor() {
	if len(m.visibleRows) == 0 {
		m.top = 0
		return
	}
	h := m.bodyHeight()
	if m.cursor < m.top {
		m.top = m.cursor
	} else if m.cursor >= m.top+h {
		m.top = m.cursor - h + 1
	}
	n := len(m.visibleRows)
	if m.top < 0 {
		m.top = 0
	}
	if m.top >= n {
		m.top = n - 1
	}
}

// fetchCmd returns a Cmd that performs DownloadJobLog and emits logLoadedMsg.
func (m *Model) fetchCmd() tea.Cmd {
	repo := m.params.Repo
	jobID := m.params.JobID
	client := m.params.Client
	return func() tea.Msg {
		bs, err := client.DownloadJobLog(context.Background(), repo, jobID)
		return logLoadedMsg{Bytes: bs, Err: err}
	}
}

// scheduleTick returns a tickCmd if auto-refresh would fire, else nil.
func (m *Model) scheduleTick() tea.Cmd {
	if !m.autoRefresh || !m.runActive {
		return nil
	}
	return m.tickCmd()
}

// tickCmd schedules a single tickMsg after the configured interval.
func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(m.tickInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// pageSize returns the body height in rows.
func (m *Model) pageSize() int {
	h := m.bodyHeight()
	if h < 1 {
		return 1
	}
	return h
}

// bodyHeight returns how many rows are available for the log body.
func (m *Model) bodyHeight() int {
	used := 1 /*header*/ + 1 /*subheader*/ + m.footerHeight()
	if m.refreshErr != nil && m.state == stateReady {
		used++
	}
	if m.input.Focused() {
		used++
	}
	h := m.height - used
	if h < 1 {
		return 1
	}
	return h
}

// footerHeight returns the number of terminal lines the footer occupies.
// When full help is shown, each help row group takes one line.
func (m *Model) footerHeight() int {
	if m.showHelp {
		return len(m.keys.FullHelp())
	}
	return 1
}

// scrollBy moves the raw-mode viewport by delta lines.
func (m *Model) scrollBy(delta int) {
	m.top += delta
	m.clampTop()
}

// scrollToBottom snaps to the last viewport-page of raw lines.
func (m *Model) scrollToBottom() {
	n := len(m.buf.Lines())
	page := m.bodyHeight()
	target := n - page
	if target < 0 {
		target = 0
	}
	m.top = target
}

// clampTop ensures top is in [0, max(0, rowCount-1)].
func (m *Model) clampTop() {
	if m.top < 0 {
		m.top = 0
	}
	n := m.rowCount()
	if n == 0 {
		m.top = 0
		return
	}
	if m.top > n-1 {
		m.top = n - 1
	}
}

// rowCount returns the total navigable row count for the current mode.
func (m *Model) rowCount() int {
	if m.viewMode == ViewModeRaw {
		return len(m.buf.Lines())
	}
	return len(m.visibleRows)
}

// recomputeMatches recomputes matches and snaps to the first match.
func (m *Model) recomputeMatches() {
	q := strings.TrimSpace(m.input.Value())
	m.matches = m.buf.Search(q)
	if len(m.matches) == 0 {
		m.matchCursor = -1
		return
	}
	m.matchCursor = 0
	m.snapToMatch(0)
}

// cycleMatch moves the match cursor by delta with wrap-around.
func (m *Model) cycleMatch(delta int) {
	n := len(m.matches)
	if n == 0 {
		return
	}
	idx := m.matchCursor + delta
	for idx < 0 {
		idx += n
	}
	idx %= n
	m.matchCursor = idx
	m.snapToMatch(idx)
}

// snapToMatch positions the view at the match at matchIdx. In raw mode this
// sets m.top; in outline/compact it expands ancestors and moves the cursor.
func (m *Model) snapToMatch(matchIdx int) {
	if matchIdx < 0 || matchIdx >= len(m.matches) {
		return
	}
	lineIdx := m.matches[matchIdx]
	if m.viewMode == ViewModeRaw {
		m.top = lineIdx
		m.clampTop()
		return
	}
	rowIdx := m.expandAncestors(lineIdx)
	if rowIdx >= 0 {
		m.cursor = rowIdx
		m.scrollToCursor()
	}
}

// jobURL returns the GitHub web URL for the current job.
func (m *Model) jobURL() string {
	r := m.params.Repo
	if r.Host == "" || r.Owner == "" || r.Name == "" {
		return ""
	}
	return fmt.Sprintf("https://%s/%s/%s/actions/runs/%d/job/%d",
		r.Host, r.Owner, r.Name, m.params.RunID, m.params.JobID)
}
