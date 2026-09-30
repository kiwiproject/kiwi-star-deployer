// Package ghissue creates GitHub issues to track dependency-version bumps
// performed by a downstream POM update. kiwiproject-changelog builds every
// changelog entry from GitHub issues (including merged PRs) tied to a
// milestone and never looks at commit content, so a plain commit bumping a
// dependency version is otherwise invisible to it.
package ghissue

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

const (
	maxCreateAttempts = 3
	initialBackoff    = 2 * time.Second
)

// Creator creates and looks up GitHub issues via the gh CLI.
type Creator struct {
	Runner runner.Runner
	// MaxAttempts is how many times Create retries a failed gh issue create
	// call before giving up. Zero means maxCreateAttempts (3).
	MaxAttempts int
	// InitialBackoff is the delay before the first retry, doubling on each
	// subsequent attempt. Zero means initialBackoff (2s).
	InitialBackoff time.Duration
}

type ghIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

// ListOpenByMilestone returns title -> issue number for every open issue in
// milestone on repo. Callers use this both to confirm the milestone exists
// (an unknown milestone is an error, not silently treated as "no issues") and
// to detect issues already created by a prior failed/retried attempt, so
// Create is not called again for those titles.
func (c *Creator) ListOpenByMilestone(repo, milestone string) (map[string]int, error) {
	exists, err := c.milestoneExists(repo, milestone)
	if err != nil {
		return nil, fmt.Errorf("checking milestone %q on %s: %w", milestone, repo, err)
	}
	if !exists {
		return nil, fmt.Errorf("milestone %q does not exist as an open milestone on %s; create it before releasing", milestone, repo)
	}

	result, err := c.Runner.Run(runner.Options{
		Command: "gh",
		Args: []string{
			"issue", "list",
			"--repo", repo,
			"--state", "open",
			"--milestone", milestone,
			"--json", "number,title",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("gh issue list --milestone %q on %s: %w", milestone, repo, err)
	}
	var issues []ghIssue
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &issues); err != nil {
		return nil, fmt.Errorf("parsing gh issue list output: %w", err)
	}
	byTitle := make(map[string]int, len(issues))
	for _, i := range issues {
		byTitle[i.Title] = i.Number
	}
	return byTitle, nil
}

// milestoneExists reports whether repo has an open milestone titled exactly
// milestone. gh issue list --milestone silently returns an empty result for
// an unknown milestone title rather than erroring, so existence has to be
// checked separately against the milestones themselves.
func (c *Creator) milestoneExists(repo, milestone string) (bool, error) {
	result, err := c.Runner.Run(runner.Options{
		Command: "gh",
		Args: []string{
			"api",
			"repos/" + repo + "/milestones?state=open&per_page=100",
			"--jq", ".[].title",
		},
	})
	if err != nil {
		return false, err
	}
	for _, title := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
		if strings.TrimSpace(title) == milestone {
			return true, nil
		}
	}
	return false, nil
}

// Create creates one issue on repo and returns its number, retrying up to
// maxCreateAttempts times with linear-doubling backoff on failure. gh issue
// create has no --json/--jq output support; on success it prints only the new
// issue's URL, so the number is parsed from the URL's final path segment.
func (c *Creator) Create(repo, title, body, label, milestone string) (int, error) {
	maxAttempts := c.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = maxCreateAttempts
	}
	backoff := c.InitialBackoff
	if backoff == 0 {
		backoff = initialBackoff
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			time.Sleep(backoff)
			backoff *= 2
		}
		result, err := c.Runner.Run(runner.Options{
			Command: "gh",
			Args: []string{
				"issue", "create",
				"--repo", repo,
				"--title", title,
				"--body", body,
				"--label", label,
				"--milestone", milestone,
			},
		})
		if err == nil {
			n, parseErr := parseIssueNumber(result.Stdout)
			if parseErr != nil {
				return 0, fmt.Errorf("parsing gh issue create output %q: %w", result.Stdout, parseErr)
			}
			return n, nil
		}
		lastErr = err
	}
	return 0, fmt.Errorf("gh issue create %q on %s failed after %d attempts: %w", title, repo, maxAttempts, lastErr)
}

// parseIssueNumber extracts the issue number from the URL gh issue create
// prints on success, e.g. "https://github.com/owner/repo/issues/123\n".
func parseIssueNumber(output string) (int, error) {
	url := strings.TrimSpace(output)
	idx := strings.LastIndex(url, "/")
	if idx == -1 {
		return 0, fmt.Errorf("no / found in %q", url)
	}
	return strconv.Atoi(url[idx+1:])
}
