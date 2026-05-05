package githubapi

import (
	"testing"

	"github.com/sud0whoami/gh-peek/internal/domain"
)

func TestMapAPIStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		status     string
		conclusion string
		want       domain.SemanticStatus
	}{
		{"completed_success", "completed", "success", domain.StatusSuccess},
		{"completed_failure", "completed", "failure", domain.StatusFailure},
		{"completed_cancelled", "completed", "cancelled", domain.StatusCancelled},
		{"completed_skipped", "completed", "skipped", domain.StatusSkipped},
		{"completed_timed_out", "completed", "timed_out", domain.StatusFailure},
		{"completed_action_required", "completed", "action_required", domain.StatusPending},
		{"completed_neutral", "completed", "neutral", domain.StatusUnknown},
		{"completed_empty", "completed", "", domain.StatusUnknown},
		{"completed_weird", "completed", "wat", domain.StatusUnknown},
		{"in_progress", "in_progress", "", domain.StatusRunning},
		{"queued", "queued", "", domain.StatusPending},
		{"requested", "requested", "", domain.StatusPending},
		{"waiting", "waiting", "", domain.StatusPending},
		{"pending", "pending", "", domain.StatusPending},
		{"unknown", "weird", "", domain.StatusUnknown},
		{"empty", "", "", domain.StatusUnknown},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := MapAPIStatus(tc.status, tc.conclusion)
			if got != tc.want {
				t.Fatalf("MapAPIStatus(%q,%q) = %q, want %q", tc.status, tc.conclusion, got, tc.want)
			}
		})
	}
}
