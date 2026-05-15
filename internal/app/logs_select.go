package app

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
)

// pickRun returns the most-recent run matching the startup context.
// If runID > 0, it is validated by calling GetRun directly.
// Priority when runID == 0: PR head SHA → current branch → repo-wide.
func pickRun(ctx context.Context, c githubapi.ActionsClient, startup domain.StartupContext, runID int64) (domain.WorkflowRun, error) {
	if runID > 0 {
		return c.GetRun(ctx, startup.Repo.Repo, runID)
	}

	filter := githubapi.ListRunsFilter{PerPage: 1}
	if startup.PR != nil && startup.PR.HeadRefOID != "" {
		filter.HeadSHA = startup.PR.HeadRefOID
	} else if startup.Repo.CurrentBranch != "" {
		filter.Branch = startup.Repo.CurrentBranch
	}

	result, err := c.ListRuns(ctx, startup.Repo.Repo, filter)
	if err != nil {
		return domain.WorkflowRun{}, err
	}
	if len(result.Runs) == 0 {
		return domain.WorkflowRun{}, errors.New("no runs found")
	}
	return result.Runs[0], nil
}

// selectJobs fetches the run and returns matching jobs.
// jobSpec == "" → all jobs; jobSpec is a number → exact ID match;
// otherwise case-insensitive substring match on job name.
// failedOnly=true → only jobs with conclusion="failure".
func selectJobs(
	ctx context.Context,
	c githubapi.ActionsClient,
	repo domain.RepoRef,
	startup domain.StartupContext,
	runID int64,
	jobSpec string,
	failedOnly bool,
) (domain.WorkflowRun, []domain.WorkflowJob, error) {
	run, err := pickRun(ctx, c, startup, runID)
	if err != nil {
		return domain.WorkflowRun{}, nil, err
	}

	jobs, err := c.ListJobs(ctx, repo, run.ID)
	if err != nil {
		return domain.WorkflowRun{}, nil, err
	}

	var filtered []domain.WorkflowJob
	for _, job := range jobs {
		if jobSpec != "" {
			if id, parseErr := strconv.ParseInt(jobSpec, 10, 64); parseErr == nil {
				if job.ID != id {
					continue
				}
			} else {
				if !strings.Contains(strings.ToLower(job.Name), strings.ToLower(jobSpec)) {
					continue
				}
			}
		}
		if failedOnly && job.Conclusion != "failure" {
			continue
		}
		filtered = append(filtered, job)
	}
	return run, filtered, nil
}
