package app

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/logs"
)

// jobLogEntry bundles a job with its downloaded log buffer and outline.
type jobLogEntry struct {
	Job       domain.WorkflowJob
	Buffer    *logs.Buffer
	Outline   *logs.Outline
	Truncated bool // true when DownloadJobLog returned ErrLogTooLarge
}

// formatErrors writes error snippets for a set of (job, buffer, outline) triples to w.
// Each snippet is preceded by a header:
//
//	--- <jobName> · <stepTitle> (lines <start+1>–<end>) ---
func formatErrors(entries []jobLogEntry, contextLines int, w io.Writer) error {
	for _, e := range entries {
		snippets := logs.Extract(e.Buffer, e.Outline, contextLines)
		for _, s := range snippets {
			header := fmt.Sprintf("--- %s · %s (lines %d–%d) ---",
				e.Job.Name, s.StepTitle, s.StartIdx+1, s.EndIdx)
			if _, err := fmt.Fprintln(w, header); err != nil {
				return err
			}
			for _, line := range s.Lines {
				if _, err := fmt.Fprintln(w, line); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ----- JSON output types -----

type logsEnvelope struct {
	Repo domain.RepoRef `json:"repo"`
	Run  runJSON        `json:"run"`
	Jobs []jobJSON      `json:"jobs"`
}

type runJSON struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Branch     string `json:"branch"`
	HeadSHA    string `json:"head_sha"`
	HTMLURL    string `json:"html_url"`
	CreatedAt  string `json:"created_at"`
}

type jobJSON struct {
	ID           int64            `json:"id"`
	Name         string           `json:"name"`
	Status       string           `json:"status"`
	Conclusion   string           `json:"conclusion"`
	StartedAt    string           `json:"started_at,omitempty"`
	CompletedAt  string           `json:"completed_at,omitempty"`
	HTMLURL      string           `json:"html_url"`
	LogTruncated bool             `json:"log_truncated"`
	HeadDropped  bool             `json:"head_dropped"`
	Outline      []*logs.NodeJSON `json:"outline"`
	Errors       []errorLine      `json:"errors"`
}

type errorLine struct {
	Line int    `json:"line"`
	Step string `json:"step"`
	Text string `json:"text"`
}

// buildJobJSON converts a jobLogEntry to a jobJSON wire object.
func buildJobJSON(e jobLogEntry) jobJSON {
	j := jobJSON{
		ID:           e.Job.ID,
		Name:         e.Job.Name,
		Status:       e.Job.Status,
		Conclusion:   e.Job.Conclusion,
		HTMLURL:      e.Job.URL,
		LogTruncated: e.Truncated,
		HeadDropped:  e.Outline.HeadDropped,
		Outline:      logs.OutlineToJSON(e.Outline, e.Buffer),
		Errors:       collectErrorLines(e.Outline, e.Buffer),
	}
	if e.Job.StartedAt != nil {
		j.StartedAt = e.Job.StartedAt.UTC().Format(time.RFC3339)
	}
	if e.Job.CompletedAt != nil {
		j.CompletedAt = e.Job.CompletedAt.UTC().Format(time.RFC3339)
	}
	return j
}

// collectErrorLines returns the flat list of SevError lines for jq-friendly consumption.
func collectErrorLines(o *logs.Outline, b *logs.Buffer) []errorLine {
	if o == nil {
		return []errorLine{}
	}
	plain := b.PlainLines()
	var result []errorLine
	var walk func([]*logs.Node)
	walk = func(nodes []*logs.Node) {
		for _, n := range nodes {
			if n.Kind == logs.NodeLine && n.Sev == logs.SevError {
				// Find enclosing step title by walking the Parent chain.
				step := "(root)"
				cur := n.Parent
				for cur != nil {
					if cur.Kind == logs.NodeStep {
						step = cur.Title
						break
					}
					cur = cur.Parent
				}
				el := errorLine{
					Line: n.StartIdx + 1,
					Step: step,
				}
				if n.StartIdx >= 0 && n.StartIdx < len(plain) {
					el.Text = plain[n.StartIdx]
				}
				result = append(result, el)
			}
			walk(n.Children)
		}
	}
	walk(o.Roots)
	if result == nil {
		return []errorLine{}
	}
	return result
}

// formatJSON encodes the envelope as pretty-printed JSON.
func formatJSON(envelope logsEnvelope, w io.Writer) error {
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}
