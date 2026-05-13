// Package domain defines the shared types used across gh-peek:
// repo and PR context, workflow runs, jobs, steps, and logs.
//
// Types here are pure data with no behavior beyond trivial helpers.
// They are deliberately decoupled from the GitHub REST payloads and
// from the Bubble Tea UI layer.
package domain

import "time"

// RepoRef identifies a GitHub repository on a particular host.
type RepoRef struct {
	Host  string
	Owner string
	Name  string
}

// RepoContext bundles the local + remote facts about the repository
// the user invoked gh-peek inside of.
type RepoContext struct {
	Repo          RepoRef
	RootDir       string
	CurrentBranch string
	DefaultBranch string
	HeadSHA       string
	IsDefault     bool
}

// StartContextKind enumerates the startup screens.
type StartContextKind string

const (
	// StartContextPR opens the PR-runs view for the current branch's PR.
	StartContextPR StartContextKind = "pr"
	// StartContextAll opens the repo-wide all-runs view.
	StartContextAll StartContextKind = "all"
	// StartContextBranch opens the branch-runs view.
	StartContextBranch StartContextKind = "branch"
)

// PullRequestContext is the subset of PR data the app needs to drive
// the PR-runs screen.
type PullRequestContext struct {
	Number      int
	Title       string
	URL         string
	HeadRefName string
	HeadRefOID  string
	BaseRefName string
}

// StartupContext is the resolved bundle returned by the bootstrap
// service before the main UI loads.
type StartupContext struct {
	Kind StartContextKind
	Repo RepoContext
	PR   *PullRequestContext
}

// WorkflowRun is a single GitHub Actions workflow run.
type WorkflowRun struct {
	ID           int64
	Name         string
	WorkflowName string
	DisplayTitle string
	Event        string
	HeadBranch   string
	HeadSHA      string
	Status       string
	Conclusion   string
	Attempt      int
	CreatedAt    time.Time
	StartedAt    *time.Time
	UpdatedAt    time.Time
	URL          string
}

// WorkflowJob is a single job within a workflow run.
type WorkflowJob struct {
	ID           int64
	RunID        int64
	Name         string
	Status       string
	Conclusion   string
	StartedAt    *time.Time
	CompletedAt  *time.Time
	WorkflowName string
	HeadBranch   string
	RunnerName   string
	RunnerGroup  string
	Labels       []string
	Steps        []WorkflowStep
	URL          string
}

// WorkflowStep is a single step within a workflow job.
type WorkflowStep struct {
	Number      int
	Name        string
	Status      string
	Conclusion  string
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// JobLog is the captured log content for a single job.
type JobLog struct {
	JobID    int64
	Raw      string
	Lines    []string
	LoadedAt time.Time
}

// Release is a single GitHub Release.
type Release struct {
	ID          int64
	TagName     string
	Name        string
	Body        string
	Draft       bool
	Prerelease  bool
	CreatedAt   time.Time
	PublishedAt *time.Time
	Author      ReleaseAuthor
	URL         string
	TarballURL  string
	ZipballURL  string
	Assets      []ReleaseAsset
}

// ReleaseAuthor is the user who published the release.
type ReleaseAuthor struct {
	Login     string
	AvatarURL string
	HTMLURL   string
}

// ReleaseAsset is a file attached to a release.
type ReleaseAsset struct {
	ID            int64
	Name          string
	Label         string
	ContentType   string
	Size          int64
	DownloadCount int
	State         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	BrowserURL    string
}

// SemanticStatus is the UI-facing status for a run, job, or step.
// It collapses GitHub's status + conclusion fields into a small set
// of values that map to theme colors and icons.
type SemanticStatus string

const (
	StatusUnknown   SemanticStatus = "unknown"
	StatusPending   SemanticStatus = "pending"
	StatusRunning   SemanticStatus = "running"
	StatusSuccess   SemanticStatus = "success"
	StatusFailure   SemanticStatus = "failure"
	StatusCancelled SemanticStatus = "cancelled"
	StatusSkipped   SemanticStatus = "skipped"
)
