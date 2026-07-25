package checkversions_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/checkversions"
	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

func pomResponse(t *testing.T, version string) *runner.Result {
	t.Helper()
	pom := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <version>` + version + `</version>
</project>`
	content := base64.StdEncoding.EncodeToString([]byte(pom))
	b, err := json.Marshal(map[string]string{"content": content, "encoding": "base64"})
	if err != nil {
		t.Fatal(err)
	}
	return &runner.Result{Stdout: string(b)}
}

func milestonesResponse(t *testing.T, titles ...string) *runner.Result {
	t.Helper()
	type milestone struct {
		Title string `json:"title"`
	}
	ms := make([]milestone, len(titles))
	for i, title := range titles {
		ms[i] = milestone{Title: title}
	}
	b, err := json.Marshal(ms)
	if err != nil {
		t.Fatal(err)
	}
	return &runner.Result{Stdout: string(b)}
}

func singleLib(repo string) map[string]config.Library {
	return map[string]config.Library{"kiwi": {Repo: repo}}
}

func TestRunAll_pass(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(pomResponse(t, "2.5.1-SNAPSHOT"), nil)
	fr.AddResponse(milestonesResponse(t, "2.5.1"), nil)

	results := checkversions.RunAll(fr, singleLib("kiwiproject/kiwi"))

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if !r.OK {
		t.Errorf("expected OK=true, got false: %s", r.Message)
	}
	if r.WantVersion != "2.5.1" {
		t.Errorf("WantVersion: got %q, want 2.5.1", r.WantVersion)
	}
}

func TestRunAll_versionMismatch(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(pomResponse(t, "2.5.1-SNAPSHOT"), nil)
	fr.AddResponse(milestonesResponse(t, "2.6.0"), nil)

	results := checkversions.RunAll(fr, singleLib("kiwiproject/kiwi"))

	r := results[0]
	if r.OK {
		t.Error("expected OK=false, got true")
	}
	if !strings.Contains(r.Message, "2.5.1") || !strings.Contains(r.Message, "2.6.0") {
		t.Errorf("message should mention both versions: %q", r.Message)
	}
}

func TestRunAll_noMilestones(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(pomResponse(t, "2.5.1-SNAPSHOT"), nil)
	fr.AddResponse(milestonesResponse(t), nil)

	results := checkversions.RunAll(fr, singleLib("kiwiproject/kiwi"))

	r := results[0]
	if r.OK {
		t.Error("expected OK=false, got true")
	}
	if !strings.Contains(r.Message, "no open milestones") {
		t.Errorf("expected 'no open milestones' in message: %q", r.Message)
	}
}

func TestRunAll_multipleMilestonesOneMatches(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(pomResponse(t, "2.5.1-SNAPSHOT"), nil)
	fr.AddResponse(milestonesResponse(t, "2.5.1", "3.0.0"), nil)

	results := checkversions.RunAll(fr, singleLib("kiwiproject/kiwi"))

	if !results[0].OK {
		t.Errorf("expected OK=true when one of multiple milestones matches: %s", results[0].Message)
	}
}

func TestRunAll_fetchPomError(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(nil, errors.New("exit status 1"))

	results := checkversions.RunAll(fr, singleLib("kiwiproject/kiwi"))

	r := results[0]
	if r.OK {
		t.Error("expected OK=false on pom fetch error")
	}
	if !strings.Contains(r.Message, "could not fetch pom.xml") {
		t.Errorf("expected pom fetch error in message: %q", r.Message)
	}
}

func TestRunAll_fetchMilestonesError(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(pomResponse(t, "2.5.1-SNAPSHOT"), nil)
	fr.AddResponse(nil, errors.New("exit status 1"))

	results := checkversions.RunAll(fr, singleLib("kiwiproject/kiwi"))

	r := results[0]
	if r.OK {
		t.Error("expected OK=false on milestones fetch error")
	}
	if !strings.Contains(r.Message, "could not fetch milestones") {
		t.Errorf("expected milestones error in message: %q", r.Message)
	}
}

// apiDispatchRunner satisfies runner.Runner and returns responses keyed on
// the joined argument string, so concurrent checks each get the response for
// the repo they actually asked about regardless of call ordering. RunAll
// checks libraries concurrently, so ordered-queue fakes are only safe for
// single-library tests.
type apiDispatchRunner struct {
	responses map[string]*runner.Result
}

func (r *apiDispatchRunner) Run(opts runner.Options) (*runner.Result, error) {
	key := strings.Join(opts.Args, " ")
	if res, ok := r.responses[key]; ok {
		return res, nil
	}
	return nil, fmt.Errorf("apiDispatchRunner: unexpected call: %s %s", opts.Command, key)
}

func dispatchFor(t *testing.T, repoVersions map[string]string) *apiDispatchRunner {
	t.Helper()
	responses := make(map[string]*runner.Result, 2*len(repoVersions))
	for repo, ver := range repoVersions {
		responses["api repos/"+repo+"/contents/pom.xml"] = pomResponse(t, ver+"-SNAPSHOT")
		responses["api repos/"+repo+"/milestones?state=open"] = milestonesResponse(t, ver)
	}
	return &apiDispatchRunner{responses: responses}
}

func TestRunAll_sortedByName(t *testing.T) {
	libs := map[string]config.Library{
		"kiwi-test": {Repo: "kiwiproject/kiwi-test"},
		"kiwi":      {Repo: "kiwiproject/kiwi"},
	}
	fr := dispatchFor(t, map[string]string{
		"kiwiproject/kiwi":      "2.5.1",
		"kiwiproject/kiwi-test": "3.0.1",
	})

	results := checkversions.RunAll(fr, libs)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "kiwi" || results[0].WantVersion != "2.5.1" {
		t.Errorf("expected first result kiwi 2.5.1, got %q %q", results[0].Name, results[0].WantVersion)
	}
	if results[1].Name != "kiwi-test" || results[1].WantVersion != "3.0.1" {
		t.Errorf("expected second result kiwi-test 3.0.1, got %q %q", results[1].Name, results[1].WantVersion)
	}
}

// TestRunAll_manyLibrariesConcurrently exercises the bounded-concurrency path
// with more libraries than the concurrency limit; each result must match its
// own library's responses, and the race detector guards the implementation.
func TestRunAll_manyLibrariesConcurrently(t *testing.T) {
	libs := make(map[string]config.Library, 20)
	repoVersions := make(map[string]string, 20)
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("lib-%02d", i)
		repo := "kiwiproject/" + name
		libs[name] = config.Library{Repo: repo}
		repoVersions[repo] = fmt.Sprintf("1.0.%d", i)
	}
	fr := dispatchFor(t, repoVersions)

	results := checkversions.RunAll(fr, libs)

	if len(results) != 20 {
		t.Fatalf("expected 20 results, got %d", len(results))
	}
	for i, r := range results {
		wantName := fmt.Sprintf("lib-%02d", i)
		wantVersion := fmt.Sprintf("1.0.%d", i)
		if r.Name != wantName || !r.OK || r.WantVersion != wantVersion {
			t.Errorf("result %d: got name=%q ok=%v version=%q, want %q true %q",
				i, r.Name, r.OK, r.WantVersion, wantName, wantVersion)
		}
	}
}

func TestAllPassed_trueWhenAllOK(t *testing.T) {
	results := []checkversions.Result{
		{Name: "kiwi", OK: true},
		{Name: "kiwi-test", OK: true},
	}
	if !checkversions.AllPassed(results) {
		t.Error("expected AllPassed=true, got false")
	}
}

func TestAllPassed_falseWhenAnyFails(t *testing.T) {
	results := []checkversions.Result{
		{Name: "kiwi", OK: true},
		{Name: "kiwi-test", OK: false, Message: "pom says 3.0.1 but open milestones are: 3.1.0"},
	}
	if checkversions.AllPassed(results) {
		t.Error("expected AllPassed=false, got true")
	}
}

func TestPrint_showsPassAndFail(t *testing.T) {
	results := []checkversions.Result{
		{Name: "kiwi", OK: true, WantVersion: "2.5.1"},
		{Name: "kiwi-test", OK: false, WantVersion: "3.0.1", Message: "pom says 3.0.1 but open milestones are: 3.1.0"},
	}
	var buf bytes.Buffer
	checkversions.Print(&buf, results)
	out := buf.String()

	if !strings.Contains(out, "[PASS]") {
		t.Errorf("expected [PASS] in output:\n%s", out)
	}
	if !strings.Contains(out, "[FAIL]") {
		t.Errorf("expected [FAIL] in output:\n%s", out)
	}
	if !strings.Contains(out, "3.1.0") {
		t.Errorf("expected failure detail in output:\n%s", out)
	}
}
