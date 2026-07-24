package ci

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

// GHChecker verifies GitHub Actions CI passes after a POM update push.
// It polls gh run list to discover runs triggered by the push commit, then
// polls gh run view until all discovered runs complete successfully.
type GHChecker struct {
	Runner runner.Runner
}

type ghRun struct {
	DatabaseID int64  `json:"databaseId"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

// Wait blocks until all push-triggered CI runs for commitSHA on repo succeed,
// or until maxWait is exceeded. An error is returned if any run fails or times out.
//
// listRuns is called on every poll cycle so that runs which appear shortly after
// the push (due to GitHub queuing delay) are not missed.
func (c *GHChecker) Wait(w io.Writer, repo, commitSHA string, maxWait, interval time.Duration) error {
	deadline := time.Now().Add(maxWait)
	shortSHA := commitSHA
	if len(shortSHA) > 8 {
		shortSHA = shortSHA[:8]
	}

	// known tracks all run IDs ever seen; pending tracks those not yet completed.
	// Separating them prevents a completed run from being re-queued if listRuns
	// keeps returning it in subsequent cycles.
	known := make(map[int64]struct{})
	pending := make(map[int64]struct{})

	// checkedWorkflows ensures the active-workflow query runs at most once,
	// the first time a poll finds no runs.
	checkedWorkflows := false

	for {
		if time.Now().After(deadline) {
			if len(known) == 0 {
				return fmt.Errorf("timed out waiting for CI run to appear for %s@%s", repo, shortSHA)
			}
			return fmt.Errorf("timed out waiting for CI to complete for %s", repo)
		}

		ids, err := c.listRuns(repo, commitSHA)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, exists := known[id]; !exists {
				known[id] = struct{}{}
				pending[id] = struct{}{}
			}
		}

		if len(known) == 0 {
			if !checkedWorkflows {
				checkedWorkflows = true
				active, err := c.hasActiveWorkflows(repo)
				switch {
				case err != nil:
					fmt.Fprintf(w, "    CI: could not check workflows for %s (%v); continuing to wait\n", repo, err)
				case !active:
					fmt.Fprintf(w, "    CI: %s has no active workflows; skipping CI verification\n", repo)
					return nil
				}
			}
			fmt.Fprintf(w, "    CI: waiting for run to appear for %s...\n", repo)
			time.Sleep(interval)
			continue
		}

		for id := range pending {
			run, err := c.viewRun(repo, id)
			if err != nil {
				return err
			}
			if run.Status != "completed" {
				fmt.Fprintf(w, "    CI: run %d status: %s\n", id, run.Status)
				continue
			}
			if run.Conclusion != "success" {
				return fmt.Errorf("CI failed for %s (run %d: https://github.com/%s/actions/runs/%d conclusion: %s)",
					repo, id, repo, id, run.Conclusion)
			}
			delete(pending, id)
		}

		if len(pending) == 0 {
			return nil
		}

		time.Sleep(interval)
	}
}

// hasActiveWorkflows reports whether repo has any active GitHub Actions
// workflows. It distinguishes a repo with no CI at all — where there is
// nothing to wait for and the POM update push will never produce a run —
// from one whose runs simply have not appeared yet.
func (c *GHChecker) hasActiveWorkflows(repo string) (bool, error) {
	result, err := c.Runner.Run(runner.Options{
		Command: "gh",
		Args: []string{
			"api",
			"repos/" + repo + "/actions/workflows",
			"--jq", `[.workflows[] | select(.state == "active")] | length`,
		},
	})
	if err != nil {
		return false, fmt.Errorf("gh api workflows: %w", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(result.Stdout))
	if err != nil {
		return false, fmt.Errorf("parsing workflow count %q: %w", strings.TrimSpace(result.Stdout), err)
	}
	return count > 0, nil
}

func (c *GHChecker) listRuns(repo, commitSHA string) ([]int64, error) {
	result, err := c.Runner.Run(runner.Options{
		Command: "gh",
		Args: []string{
			"run", "list",
			"--repo", repo,
			"--commit", commitSHA,
			"--event", "push",
			"--json", "databaseId,status",
			"--limit", "10",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("gh run list: %w", err)
	}
	var runs []ghRun
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &runs); err != nil {
		return nil, fmt.Errorf("parsing gh run list output: %w", err)
	}
	ids := make([]int64, len(runs))
	for i, r := range runs {
		ids[i] = r.DatabaseID
	}
	return ids, nil
}

func (c *GHChecker) viewRun(repo string, runID int64) (*ghRun, error) {
	result, err := c.Runner.Run(runner.Options{
		Command: "gh",
		Args: []string{
			"run", "view",
			fmt.Sprintf("%d", runID),
			"--repo", repo,
			"--json", "status,conclusion",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("gh run view %d: %w", runID, err)
	}
	var run ghRun
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &run); err != nil {
		return nil, fmt.Errorf("parsing gh run view output: %w", err)
	}
	return &run, nil
}
