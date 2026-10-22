package changes_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/changes"
	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

type commit struct{ author, message string }

func releaseResponse(t *testing.T, tag string) *runner.Result {
	t.Helper()
	b, err := json.Marshal(map[string]string{"tag_name": tag})
	if err != nil {
		t.Fatal(err)
	}
	return &runner.Result{Stdout: string(b)}
}

func compareResponse(t *testing.T, aheadBy int, commits ...commit) *runner.Result {
	t.Helper()
	type author struct {
		Login string `json:"login"`
	}
	type inner struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
	}
	type entry struct {
		Author *author `json:"author"`
		Commit inner   `json:"commit"`
	}
	entries := make([]entry, len(commits))
	for i, c := range commits {
		entries[i] = entry{Author: &author{Login: c.author}, Commit: inner{Message: c.message}}
	}
	b, err := json.Marshal(map[string]any{"ahead_by": aheadBy, "commits": entries})
	if err != nil {
		t.Fatal(err)
	}
	return &runner.Result{Stdout: string(b)}
}

// dispatchRunner returns responses keyed on the joined gh arguments, so
// concurrent checks each get the response for the repo they asked about.
type dispatchRunner struct {
	responses map[string]*runner.Result
	errs      map[string]error
}

func (d *dispatchRunner) Run(opts runner.Options) (*runner.Result, error) {
	key := strings.Join(opts.Args, " ")
	if err, ok := d.errs[key]; ok {
		return d.responses[key], err
	}
	if res, ok := d.responses[key]; ok {
		return res, nil
	}
	return nil, fmt.Errorf("dispatchRunner: unexpected call: %s %s", opts.Command, key)
}

func releaseKey(repo string) string { return "api repos/" + repo + "/releases/latest" }
func compareKey(repo, tag string) string {
	return "api repos/" + repo + "/compare/" + tag + "...main?per_page=100"
}

func singleLib(repo string) map[string]config.Library {
	return map[string]config.Library{"kiwi": {Repo: repo}}
}

func TestRunAll_countsExcludeReleaseBookkeeping(t *testing.T) {
	d := &dispatchRunner{responses: map[string]*runner.Result{
		releaseKey("kiwiproject/kiwi"): releaseResponse(t, "v5.4.0"),
		compareKey("kiwiproject/kiwi", "v5.4.0"): compareResponse(t, 6,
			commit{"someone", "[maven-release-plugin] prepare for next development iteration"},
			commit{"dependabot[bot]", "chore(deps): bump foo from 1 to 2"},
			commit{"someone", "deps: Bump bar to 3"},
			commit{"someone", "ci: pin actions"},
			commit{"someone", "Add a feature"},
			commit{"someone", "Fix a bug"},
		),
	}}

	r := changes.RunAll(d, singleLib("kiwiproject/kiwi"))[0]

	if r.Message != "" {
		t.Fatalf("unexpected failure: %s", r.Message)
	}
	if r.LastTag != "v5.4.0" || r.Total != 5 || r.Dependency != 2 || r.Maintenance != 1 || r.Other != 2 {
		t.Errorf("got %+v", r)
	}
	if r.Truncated {
		t.Error("did not expect truncation")
	}
	if len(r.Commits) != 5 {
		t.Errorf("expected 5 commits behind the counts, got %d", len(r.Commits))
	}
}

func TestRunAll_unchangedWhenOnlyReleaseBookkeeping(t *testing.T) {
	d := &dispatchRunner{responses: map[string]*runner.Result{
		releaseKey("kiwiproject/kiwi"): releaseResponse(t, "v1.0.0"),
		compareKey("kiwiproject/kiwi", "v1.0.0"): compareResponse(t, 1,
			commit{"someone", "[maven-release-plugin] prepare for next development iteration"}),
	}}

	r := changes.RunAll(d, singleLib("kiwiproject/kiwi"))[0]

	if r.Total != 0 || len(r.Commits) != 0 {
		t.Errorf("expected no changes, got %+v", r)
	}
}

func TestRunAll_noRelease(t *testing.T) {
	d := &dispatchRunner{
		responses: map[string]*runner.Result{releaseKey("kiwiproject/kiwi"): {Stderr: "gh: Not Found (HTTP 404)"}},
		errs:      map[string]error{releaseKey("kiwiproject/kiwi"): errors.New("exit status 1")},
	}

	results := changes.RunAll(d, singleLib("kiwiproject/kiwi"))

	if !results[0].NoRelease || results[0].Message != "" {
		t.Errorf("expected NoRelease without a failure, got %+v", results[0])
	}
	if changes.AnyFailed(results) {
		t.Error("a repo with no release is not a failure")
	}
}

func TestRunAll_lookupFailureOnlyAffectsThatLibrary(t *testing.T) {
	libs := map[string]config.Library{
		"good": {Repo: "kiwiproject/good"},
		"bad":  {Repo: "kiwiproject/bad"},
	}
	d := &dispatchRunner{
		responses: map[string]*runner.Result{
			releaseKey("kiwiproject/good"):           releaseResponse(t, "v1.0.0"),
			compareKey("kiwiproject/good", "v1.0.0"): compareResponse(t, 1, commit{"someone", "Add a thing"}),
			releaseKey("kiwiproject/bad"):            {Stderr: "gh: Bad credentials (HTTP 401)"},
		},
		errs: map[string]error{releaseKey("kiwiproject/bad"): errors.New("exit status 1")},
	}

	results := changes.RunAll(d, libs)

	if results[0].Name != "bad" || results[0].Message == "" {
		t.Errorf("expected bad to fail, got %+v", results[0])
	}
	if results[1].Name != "good" || results[1].Message != "" || results[1].Total != 1 {
		t.Errorf("expected good to succeed, got %+v", results[1])
	}
	if !changes.AnyFailed(results) {
		t.Error("expected AnyFailed to be true")
	}
}

func TestRunAll_truncatedWhenMoreCommitsThanListed(t *testing.T) {
	d := &dispatchRunner{responses: map[string]*runner.Result{
		releaseKey("kiwiproject/kiwi"): releaseResponse(t, "v1.0.0"),
		compareKey("kiwiproject/kiwi", "v1.0.0"): compareResponse(t, 150,
			commit{"someone", "Add a thing"}, commit{"someone", "Add another"}),
	}}

	r := changes.RunAll(d, singleLib("kiwiproject/kiwi"))[0]

	if !r.Truncated || r.Total != 150 || r.Listed != 2 {
		t.Errorf("got %+v", r)
	}
	var out bytes.Buffer
	changes.Print(&out, []changes.Result{r}, false)
	if !strings.Contains(out.String(), "only the first 2 of 150 commits") {
		t.Errorf("expected a truncation note:\n%s", out.String())
	}
}

func TestRunAll_sortedByName(t *testing.T) {
	libs := map[string]config.Library{
		"kiwi-test": {Repo: "kiwiproject/kiwi-test"},
		"kiwi":      {Repo: "kiwiproject/kiwi"},
	}
	responses := map[string]*runner.Result{}
	for _, repo := range []string{"kiwiproject/kiwi", "kiwiproject/kiwi-test"} {
		responses[releaseKey(repo)] = releaseResponse(t, "v1.0.0")
		responses[compareKey(repo, "v1.0.0")] = compareResponse(t, 0)
	}

	results := changes.RunAll(&dispatchRunner{responses: responses}, libs)

	if results[0].Name != "kiwi" || results[1].Name != "kiwi-test" {
		t.Errorf("expected results sorted by name, got %q then %q", results[0].Name, results[1].Name)
	}
}

func TestRunAll_manyLibrariesConcurrently(t *testing.T) {
	libs := make(map[string]config.Library, 20)
	responses := map[string]*runner.Result{}
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("lib-%02d", i)
		repo := "kiwiproject/" + name
		libs[name] = config.Library{Repo: repo}
		responses[releaseKey(repo)] = releaseResponse(t, fmt.Sprintf("v1.0.%d", i))
		responses[compareKey(repo, fmt.Sprintf("v1.0.%d", i))] = compareResponse(t, i)
	}

	results := changes.RunAll(&dispatchRunner{responses: responses}, libs)

	for i, r := range results {
		if r.Name != fmt.Sprintf("lib-%02d", i) || r.LastTag != fmt.Sprintf("v1.0.%d", i) {
			t.Errorf("result %d: got %q %q", i, r.Name, r.LastTag)
		}
	}
}

func TestPrint(t *testing.T) {
	results := []changes.Result{
		{Name: "kiwi", LastTag: "v5.4.0", Total: 2, Dependency: 1, Other: 1, Listed: 2, Commits: []changes.Commit{
			{Message: "chore(deps): bump foo", Author: "dependabot[bot]", Kind: changes.Dependency},
			{Message: "Add a thing", Author: "someone", Kind: changes.Other},
		}},
		{Name: "kiwi-new", NoRelease: true},
		{Name: "kiwi-bad", Message: "could not find the latest release: boom"},
	}

	var plain, verbose bytes.Buffer
	changes.Print(&plain, results, false)
	changes.Print(&verbose, results, true)

	for _, want := range []string{"LIBRARY", "kiwi", "v5.4.0", "(none)", "could not find the latest release: boom"} {
		if !strings.Contains(plain.String(), want) {
			t.Errorf("table is missing %q:\n%s", want, plain.String())
		}
	}
	if strings.Contains(plain.String(), "Add a thing") {
		t.Errorf("non-verbose output should not list commits:\n%s", plain.String())
	}
	for _, want := range []string{"kiwi since v5.4.0:", "Add a thing (someone)", "dependency"} {
		if !strings.Contains(verbose.String(), want) {
			t.Errorf("verbose output is missing %q:\n%s", want, verbose.String())
		}
	}
}

func TestRunAll_failureMessageIncludesGhStderr(t *testing.T) {
	d := &dispatchRunner{
		responses: map[string]*runner.Result{releaseKey("kiwiproject/kiwi"): {Stderr: "gh: Bad credentials\n(HTTP 401)\n"}},
		errs:      map[string]error{releaseKey("kiwiproject/kiwi"): errors.New("exit status 1")},
	}

	r := changes.RunAll(d, singleLib("kiwiproject/kiwi"))[0]

	if !strings.Contains(r.Message, "exit status 1 (gh: Bad credentials (HTTP 401))") {
		t.Errorf("expected stderr on one line in the message, got %q", r.Message)
	}
}
