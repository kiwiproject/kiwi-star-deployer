package checkversions_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
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

func TestRunAll_sortedByName(t *testing.T) {
	libs := map[string]config.Library{
		"kiwi-test": {Repo: "kiwiproject/kiwi-test"},
		"kiwi":      {Repo: "kiwiproject/kiwi"},
	}
	fr := &runner.FakeRunner{}
	// kiwi comes first alphabetically
	fr.AddResponse(pomResponse(t, "2.5.1-SNAPSHOT"), nil)
	fr.AddResponse(milestonesResponse(t, "2.5.1"), nil)
	fr.AddResponse(pomResponse(t, "3.0.1-SNAPSHOT"), nil)
	fr.AddResponse(milestonesResponse(t, "3.0.1"), nil)

	results := checkversions.RunAll(fr, libs)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "kiwi" {
		t.Errorf("expected first result to be kiwi, got %q", results[0].Name)
	}
	if results[1].Name != "kiwi-test" {
		t.Errorf("expected second result to be kiwi-test, got %q", results[1].Name)
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
