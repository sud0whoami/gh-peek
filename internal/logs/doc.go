// Package logs owns the in-memory storage, ANSI handling, search
// index, and failure-marker scanning for a single job log.
//
// The package is pure data: it has no Bubble Tea, lipgloss, or other
// UI dependency. The job-log viewer screen consumes it.
package logs
