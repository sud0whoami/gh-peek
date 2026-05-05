package githubapi

import "github.com/sud0whoami/gh-peek/internal/domain"

// MapAPIStatus collapses a GitHub Actions raw (status, conclusion)
// pair into a domain.SemanticStatus suitable for UI rendering.
//
// status is one of: queued, requested, waiting, pending, in_progress,
// completed (and any future values, which fall through to Unknown).
// conclusion is meaningful only when status == "completed" and is one
// of: success, failure, cancelled, skipped, neutral, timed_out,
// action_required, "".
func MapAPIStatus(status, conclusion string) domain.SemanticStatus {
	switch status {
	case "completed":
		switch conclusion {
		case "success":
			return domain.StatusSuccess
		case "failure", "timed_out":
			return domain.StatusFailure
		case "cancelled":
			return domain.StatusCancelled
		case "skipped":
			return domain.StatusSkipped
		case "action_required":
			return domain.StatusPending
		default:
			return domain.StatusUnknown
		}
	case "in_progress":
		return domain.StatusRunning
	case "queued", "requested", "waiting", "pending":
		return domain.StatusPending
	default:
		return domain.StatusUnknown
	}
}
