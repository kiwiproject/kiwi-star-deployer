// Package changes reports what has changed in each library since its last
// GitHub release, so a person can judge which libraries are worth releasing.
package changes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

// maxConcurrentChecks bounds how many libraries are checked at once. Each
// check makes two gh api calls, so this caps concurrent GitHub requests at
// twice this value, the same bound the version check uses.
const maxConcurrentChecks = 8

// compareCommitLimit is the page size requested from the compare endpoint,
// which is the most it returns per page. A library with more commits than
// this since its last release has its breakdown computed from the first page
// only (see Result.Truncated), although the total stays exact.
const compareCommitLimit = 100

// ghTimeout bounds each gh api call so one stalled request cannot hang the
// whole command.
const ghTimeout = time.Minute

// releasePluginPrefix starts the message of the commits that the Maven
// Release Plugin makes. The "prepare for next development iteration" commit
// always follows the release tag, so every library is at least one commit
// ahead of its last release even when nothing else changed.
const releasePluginPrefix = "[maven-release-plugin]"

// Kind says what sort of change a commit looks like. It is a heuristic based
// on the commit's author and the start of its message.
type Kind string

const (
	// Bookkeeping commits are made by the release itself and are not counted.
	Bookkeeping Kind = "bookkeeping"
	// Dependency commits bump a dependency or a GitHub Action.
	Dependency Kind = "dependency"
	// Maintenance commits change the build, CI, docs or tests.
	Maintenance Kind = "maintenance"
	// Other commits are everything else, which includes the real changes.
	Other Kind = "other"
)

var (
	dependencyPrefix  = regexp.MustCompile(`^(chore\(deps|deps[:(]|bump )`)
	maintenancePrefix = regexp.MustCompile(`^(ci|build|chore|docs|test|style|refactor)[:(!]`)
)

// Commit is a commit made after a library's last release.
type Commit struct {
	Message string // first line of the commit message
	Author  string
	Kind    Kind
}

// Result holds what has changed in one library since its last release.
type Result struct {
	Name        string
	LastTag     string // empty when the repo has no release
	NoRelease   bool
	Total       int // commits since the last release, excluding release bookkeeping
	Dependency  int
	Maintenance int
	Other       int
	Commits     []Commit // commits behind the counts, excluding release bookkeeping
	Truncated   bool     // true when only the first Listed of Total commits were classified
	Listed      int      // commits classified, excluding release bookkeeping
	Message     string   // non-empty when the lookup failed
}

// RunAll looks up every library in libs concurrently (bounded by
// maxConcurrentChecks) and returns results sorted by library name.
func RunAll(r runner.Runner, libs map[string]config.Library) []Result {
	names := make([]string, 0, len(libs))
	for name := range libs {
		names = append(names, name)
	}
	sort.Strings(names)

	results := make([]Result, len(names))
	sem := make(chan struct{}, maxConcurrentChecks)
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = check(r, name, libs[name].Repo)
		}(i, name)
	}
	wg.Wait()
	return results
}

// AnyFailed returns true if the lookup failed for any result.
func AnyFailed(results []Result) bool {
	for _, r := range results {
		if r.Message != "" {
			return true
		}
	}
	return false
}

// Print writes a table with one row per library to w. With verbose it then
// lists the commits behind each library's counts.
func Print(w io.Writer, results []Result, verbose bool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "LIBRARY\tLAST RELEASE\tCOMMITS\tDEPS\tMAINT\tOTHER")
	for _, r := range results {
		switch {
		case r.Message != "":
			fmt.Fprintf(tw, "%s\t-\t-\t-\t-\t%s\n", r.Name, r.Message)
		case r.NoRelease:
			fmt.Fprintf(tw, "%s\t(none)\t-\t-\t-\t-\n", r.Name)
		default:
			fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%d\n", r.Name, r.LastTag, r.Total, r.Dependency, r.Maintenance, r.Other)
		}
	}
	_ = tw.Flush()

	for _, r := range results {
		if r.Truncated {
			fmt.Fprintf(w, "\n%s: the breakdown covers only the first %d of %d commits\n", r.Name, r.Listed, r.Total)
		}
	}

	if !verbose {
		return
	}
	for _, r := range results {
		if len(r.Commits) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n%s since %s:\n", r.Name, r.LastTag)
		for _, c := range r.Commits {
			fmt.Fprintf(w, "  %-11s %s (%s)\n", c.Kind, c.Message, c.Author)
		}
	}
}

func check(r runner.Runner, name, repo string) Result {
	result := Result{Name: name}

	tag, found, err := latestTag(r, repo)
	if err != nil {
		result.Message = fmt.Sprintf("could not find the latest release: %v", err)
		return result
	}
	if !found {
		result.NoRelease = true
		return result
	}
	result.LastTag = tag

	aheadBy, commits, err := fetchCommits(r, repo, tag)
	if err != nil {
		result.Message = fmt.Sprintf("could not compare with %s: %v", tag, err)
		return result
	}

	bookkeeping := 0
	for _, c := range commits {
		switch c.Kind {
		case Bookkeeping:
			bookkeeping++
			continue
		case Dependency:
			result.Dependency++
		case Maintenance:
			result.Maintenance++
		default:
			result.Other++
		}
		result.Commits = append(result.Commits, c)
	}
	result.Listed = len(result.Commits)
	result.Total = aheadBy - bookkeeping
	result.Truncated = len(commits) < aheadBy
	return result
}

// latestTag returns the tag of repo's latest GitHub release. found is false,
// with no error, when the repo has no release.
func latestTag(r runner.Runner, repo string) (tag string, found bool, err error) {
	res, err := r.Run(runner.Options{
		Command: "gh",
		Args:    []string{"api", "repos/" + repo + "/releases/latest"},
		Timeout: ghTimeout,
	})
	if err != nil {
		// gh reports a missing release as "gh: Not Found (HTTP 404)".
		if res != nil && strings.Contains(res.Stderr, "(HTTP 404)") {
			return "", false, nil
		}
		return "", false, withStderr(err, res)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &payload); err != nil {
		return "", false, fmt.Errorf("parsing response: %w", err)
	}
	if payload.TagName == "" {
		return "", false, fmt.Errorf("response has no tag_name")
	}
	return payload.TagName, true, nil
}

// fetchCommits returns how many commits main is ahead of tag, and the first
// page of those commits, classified.
func fetchCommits(r runner.Runner, repo, tag string) (aheadBy int, commits []Commit, err error) {
	path := fmt.Sprintf("repos/%s/compare/%s...main?per_page=%d", repo, url.PathEscape(tag), compareCommitLimit)
	res, err := r.Run(runner.Options{
		Command: "gh",
		Args:    []string{"api", path},
		Timeout: ghTimeout,
	})
	if err != nil {
		return 0, nil, withStderr(err, res)
	}

	var payload struct {
		AheadBy int `json:"ahead_by"`
		Commits []struct {
			Author *struct {
				Login string `json:"login"`
			} `json:"author"`
			Commit struct {
				Message string `json:"message"`
				Author  struct {
					Name string `json:"name"`
				} `json:"author"`
			} `json:"commit"`
		} `json:"commits"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &payload); err != nil {
		return 0, nil, fmt.Errorf("parsing response: %w", err)
	}

	commits = make([]Commit, len(payload.Commits))
	for i, c := range payload.Commits {
		author := c.Commit.Author.Name
		if c.Author != nil && c.Author.Login != "" {
			author = c.Author.Login
		}
		message, _, _ := strings.Cut(c.Commit.Message, "\n")
		commits[i] = Commit{Message: message, Author: author, Kind: classify(message, author)}
	}
	return payload.AheadBy, commits, nil
}

// classify says what sort of change a commit looks like. The first match
// wins, and only the first line of the message is used.
func classify(message, author string) Kind {
	lower := strings.ToLower(message)
	switch {
	case strings.HasPrefix(lower, releasePluginPrefix):
		return Bookkeeping
	case author == "dependabot[bot]", dependencyPrefix.MatchString(lower):
		return Dependency
	case maintenancePrefix.MatchString(lower):
		return Maintenance
	default:
		return Other
	}
}

// withStderr adds gh's stderr, collapsed to one line so it cannot break the
// table, to err, since the exit error alone only says "exit status 1".
func withStderr(err error, res *runner.Result) error {
	if res == nil {
		return err
	}
	detail := strings.Join(strings.Fields(res.Stderr), " ")
	if detail == "" {
		return err
	}
	return fmt.Errorf("%w (%s)", err, detail)
}
