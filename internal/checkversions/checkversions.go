package checkversions

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/pom"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

// Result holds the outcome of a single version check.
type Result struct {
	Name        string
	OK          bool
	WantVersion string // release version derived from pom (e.g. "2.5.1")
	Message     string // non-empty on failure
}

// RunAll checks every library in libs and returns results sorted by library name.
func RunAll(r runner.Runner, libs map[string]config.Library) []Result {
	names := make([]string, 0, len(libs))
	for name := range libs {
		names = append(names, name)
	}
	sort.Strings(names)

	results := make([]Result, 0, len(names))
	for _, name := range names {
		results = append(results, check(r, name, libs[name].Repo))
	}
	return results
}

// AllPassed returns true if every result passed.
func AllPassed(results []Result) bool {
	for _, r := range results {
		if !r.OK {
			return false
		}
	}
	return true
}

// Print writes a formatted pass/fail table to w.
func Print(w io.Writer, results []Result) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range results {
		status := "[PASS]"
		msg := ""
		if !r.OK {
			status = "[FAIL]"
			msg = r.Message
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", status, r.Name, r.WantVersion, msg)
	}
	_ = tw.Flush()
}

func check(r runner.Runner, name, repo string) Result {
	result := Result{Name: name}

	pomVersion, err := fetchPomVersion(r, repo)
	if err != nil {
		result.Message = fmt.Sprintf("could not fetch pom.xml: %v", err)
		return result
	}

	if !strings.HasSuffix(pomVersion, "-SNAPSHOT") {
		result.Message = fmt.Sprintf("pom version %q is not a SNAPSHOT", pomVersion)
		return result
	}

	wantVersion := strings.TrimSuffix(pomVersion, "-SNAPSHOT")
	result.WantVersion = wantVersion

	milestones, err := fetchOpenMilestones(r, repo)
	if err != nil {
		result.Message = fmt.Sprintf("could not fetch milestones: %v", err)
		return result
	}

	for _, m := range milestones {
		if m == wantVersion {
			result.OK = true
			return result
		}
	}

	if len(milestones) == 0 {
		result.Message = fmt.Sprintf("pom says %s but no open milestones found", wantVersion)
	} else {
		result.Message = fmt.Sprintf("pom says %s but open milestones are: %s", wantVersion, strings.Join(milestones, ", "))
	}
	return result
}

func fetchPomVersion(r runner.Runner, repo string) (string, error) {
	res, err := r.Run(runner.Options{
		Command: "gh",
		Args:    []string{"api", "repos/" + repo + "/contents/pom.xml"},
	})
	if err != nil {
		return "", err
	}

	var payload struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &payload); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if payload.Encoding != "base64" {
		return "", fmt.Errorf("unexpected encoding %q", payload.Encoding)
	}

	// GitHub includes newlines in the base64 content.
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(payload.Content, "\n", ""))
	if err != nil {
		return "", fmt.Errorf("decoding pom.xml: %w", err)
	}

	version, err := pom.ParseVersion(bytes.NewReader(decoded))
	if err != nil {
		return "", fmt.Errorf("reading version: %w", err)
	}
	return version, nil
}

func fetchOpenMilestones(r runner.Runner, repo string) ([]string, error) {
	res, err := r.Run(runner.Options{
		Command: "gh",
		Args:    []string{"api", "repos/" + repo + "/milestones?state=open"},
	})
	if err != nil {
		return nil, err
	}

	var milestones []struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &milestones); err != nil {
		return nil, fmt.Errorf("parsing milestones: %w", err)
	}

	titles := make([]string, len(milestones))
	for i, m := range milestones {
		titles[i] = m.Title
	}
	return titles, nil
}
