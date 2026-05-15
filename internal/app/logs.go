package app

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/sud0whoami/gh-peek/internal/bootstrap"
	"github.com/sud0whoami/gh-peek/internal/clipboard"
	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/ghctx"
	"github.com/sud0whoami/gh-peek/internal/gitctx"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
	"github.com/sud0whoami/gh-peek/internal/logs"
)

// logsDeps bundles the swappable dependencies of RunLogs for testing.
type logsDeps struct {
	bootstrap func(ctx context.Context) (domain.StartupContext, error)
	client    githubapi.ActionsClient
	copier    clipboard.Copier // nil means --copy is not used; set by defaultLogsDeps
}

// RunLogs is the entrypoint for the non-interactive `gh peek logs` subcommand.
func RunLogs(args []string, stdout, stderr io.Writer) int {
	return runLogsWithDeps(args, stdout, stderr, defaultLogsDeps())
}

func defaultLogsDeps() logsDeps {
	return logsDeps{
		bootstrap: func(ctx context.Context) (domain.StartupContext, error) {
			boot := bootstrap.New(gitctx.New(""), ghctx.New())
			return boot.Resolve(ctx)
		},
		client: githubapi.New(),
		copier: clipboard.OSCopier{},
	}
}

func runLogsWithDeps(args []string, stdout, stderr io.Writer, deps logsDeps) int {
	fs := flag.NewFlagSet("gh-peek logs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "gh-peek logs \u2014 download job logs to stdout (non-interactive)\n\nUSAGE\n  gh peek logs [flags]\n\nDESCRIPTION\n  Resolves the current repo context (same logic as the TUI), picks the most\n  recent run, and writes logs for failed jobs to stdout. Use --all to include\n  passing jobs.\n\n  Auto-pick order for the run: PR head SHA \u2192 current branch \u2192 repo-wide.\n\nOUTPUT MODES\n  (default)  ANSI-stripped text, one job per section\n  --errors   Failure snippets only \u2014 ideal for pasting into an AI agent\n  --json     Structured JSON (outline tree + flat error list)\n\nFLAGS\n") //nolint:errcheck
		fs.PrintDefaults()
		fmt.Fprintf(stderr, "\nEXIT CODES\n  0  success\n  1  bootstrap or API failure; no jobs matched the filter\n  2  flag error or mutually exclusive flags\n\nEXAMPLES\n  gh peek logs                        # failed jobs on current branch/PR\n  gh peek logs --all                  # all jobs\n  gh peek logs --errors               # failure snippets (best for AI agents)\n  gh peek logs --errors --copy        # copy failure snippets to clipboard\n  gh peek logs --json | jq .jobs[0].errors\n  gh peek logs --run 12345 --job build\n") //nolint:errcheck
	}

	runID := fs.Int64("run", 0, "specific run ID; 0 = auto-pick")
	job := fs.String("job", "", "numeric → exact job ID; otherwise substring match on job name")
	failedOnly := fs.Bool("failed-only", true, "restrict to failed jobs")
	all := fs.Bool("all", false, "include all jobs regardless of conclusion (sets failed-only=false)")
	errorsFlag := fs.Bool("errors", false, "extract failure snippets with surrounding context")
	contextLines := fs.Int("context", 20, "lines of context for --errors")
	jsonFlag := fs.Bool("json", false, "structured JSON output")
	noStripANSI := fs.Bool("no-strip-ansi", false, "preserve ANSI escape codes in text output")
	copyFlag := fs.Bool("copy", false, "copy output to the clipboard instead of writing to stdout")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Validate mutual exclusion.
	if *errorsFlag && *jsonFlag {
		fmt.Fprintln(stderr, "gh-peek logs: --errors and --json are mutually exclusive") //nolint:errcheck
		return 2
	}

	// --all overrides --failed-only.
	if *all {
		*failedOnly = false
	}

	ctx := context.Background()
	startup, err := deps.bootstrap(ctx)
	if err != nil {
		writeBootstrapError(stderr, err)
		return 1
	}

	run, selectedJobs, err := selectJobs(ctx, deps.client, startup.Repo.Repo, startup, *runID, *job, *failedOnly)
	if err != nil {
		fmt.Fprintln(stderr, "gh-peek logs:", err) //nolint:errcheck
		return 1
	}

	if len(selectedJobs) == 0 {
		hint := fmt.Sprintf("gh-peek logs: run %d (%s) has no jobs matching the current filter", run.ID, run.Conclusion)
		if *failedOnly {
			hint += "\n  hint: run succeeded or is still in progress — use --all to see all jobs"
		}
		fmt.Fprintln(stderr, hint) //nolint:errcheck
		return 1
	}

	// Set up the output sink. When --copy is active, capture into a
	// buffer so we can write it to the clipboard instead of stdout.
	var sink = stdout // io.Writer inferred
	var clipBuf *bytes.Buffer
	if *copyFlag {
		clipBuf = &bytes.Buffer{}
		sink = clipBuf
	}

	// build the run summary used by the JSON formatter
	rj := runJSON{
		ID:         run.ID,
		Name:       run.Name,
		Status:     run.Status,
		Conclusion: run.Conclusion,
		Branch:     run.HeadBranch,
		HeadSHA:    run.HeadSHA,
		HTMLURL:    run.URL,
	}
	if !run.CreatedAt.IsZero() {
		rj.CreatedAt = run.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}

	// Download logs and build entries.
	var entries []jobLogEntry
	for _, j := range selectedJobs {
		raw, dlErr := deps.client.DownloadJobLog(ctx, startup.Repo.Repo, j.ID)

		// ErrLogTooLarge is a partial success: use the returned bytes.
		truncated := errors.Is(dlErr, githubapi.ErrLogTooLarge)
		if dlErr != nil && !truncated {
			fmt.Fprintf(stderr, "gh-peek logs: download job %d (%s): %v\n", j.ID, j.Name, dlErr) //nolint:errcheck
			return 1
		}

		buf := logs.New()
		buf.Set(raw)
		outline := logs.BuildOutline(buf)

		entries = append(entries, jobLogEntry{
			Job:       j,
			Buffer:    buf,
			Outline:   outline,
			Truncated: truncated,
		})
	}

	if *jsonFlag {
		envelope := logsEnvelope{
			Repo: startup.Repo.Repo,
			Run:  rj,
			Jobs: make([]jobJSON, 0, len(entries)),
		}
		for _, e := range entries {
			envelope.Jobs = append(envelope.Jobs, buildJobJSON(e))
		}
		if err := formatJSON(envelope, sink); err != nil {
			fmt.Fprintln(stderr, "gh-peek logs:", err) //nolint:errcheck
			return 1
		}
	} else if *errorsFlag {
		if err := formatErrors(entries, *contextLines, sink); err != nil {
			fmt.Fprintln(stderr, "gh-peek logs:", err) //nolint:errcheck
			return 1
		}
	} else {
		// Default: text output.
		for _, e := range entries {
			fmt.Fprintf(sink, "=== %s (%s) ===\n", e.Job.Name, e.Job.Conclusion) //nolint:errcheck

			if e.Truncated {
				fmt.Fprintln(sink, "[LOG TRUNCATED — showing tail only]") //nolint:errcheck
			}

			var lines []string
			if *noStripANSI {
				lines = e.Buffer.Lines()
			} else {
				lines = e.Buffer.PlainLines()
			}
			for _, line := range lines {
				fmt.Fprintln(sink, line) //nolint:errcheck
			}
		}
	}

	if *copyFlag {
		ctxCopy, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := deps.copier.Copy(ctxCopy, clipBuf.Bytes()); err != nil {
			if errors.Is(err, clipboard.ErrNoClipboardTool) {
				fmt.Fprintln(stderr, "gh-peek logs: no clipboard tool found; install xclip, xsel, or wl-clipboard") //nolint:errcheck
			} else {
				fmt.Fprintln(stderr, "gh-peek logs: failed to copy to clipboard:", err) //nolint:errcheck
			}
			return 1
		}
		fmt.Fprintf(stderr, "gh-peek logs: copied %d bytes to clipboard\n", clipBuf.Len()) //nolint:errcheck
	}

	return 0
}
