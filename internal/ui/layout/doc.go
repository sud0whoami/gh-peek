// Package layout provides terminal frame computation for gh-peek.
//
// Compute returns a Frame that caps content at a maximum width (default 140,
// overridable via GHPEEK_MAX_WIDTH) and provides left-padding so content is
// horizontally centered on wider terminals. Terminals narrower than MinWidth
// set TooNarrow=true so callers can show a "please resize" fallback.
package layout
