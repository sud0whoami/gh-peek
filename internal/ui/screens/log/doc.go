// Package log implements the job log viewer screen.
//
// Scope (Milestone 5):
//   - Initial fetch via ActionsClient.DownloadJobLog.
//   - States: loading | ready | error. Refresh failures keep prior data.
//   - Storage / search / failure-marker scanning live in internal/logs.
//   - Auto-refresh tick (default 7s, injectable) only fires while
//     RunActive is true and the search input is not focused.
//   - Emits OpenInBrowserMsg / BackMsg for the parent to route.
package log
