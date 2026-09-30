package ghissue_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/ghissue"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

func TestListOpenByMilestone_success(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: "1.1.0\nother-milestone\n"}, nil)                                                                                 // gh api milestones
	fr.AddResponse(&runner.Result{Stdout: `[{"number":501,"title":"Update kiwi-parent to 3.0.0"},{"number":502,"title":"Update kiwi-bom to 3.3.3"}]`}, nil) // gh issue list

	c := &ghissue.Creator{Runner: fr}
	got, err := c.ListOpenByMilestone("kiwiproject/kiwi", "1.1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]int{"Update kiwi-parent to 3.0.0": 501, "Update kiwi-bom to 3.3.3": 502}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for title, num := range want {
		if got[title] != num {
			t.Errorf("expected %s -> %d, got %d", title, num, got[title])
		}
	}
	if fr.CallCount() != 2 {
		t.Errorf("expected 2 calls, got %d", fr.CallCount())
	}
}

func TestListOpenByMilestone_missingMilestoneErrors(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: "1.0.0\nother-milestone\n"}, nil) // gh api milestones (no 1.1.0)

	c := &ghissue.Creator{Runner: fr}
	_, err := c.ListOpenByMilestone("kiwiproject/kiwi", "1.1.0")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `milestone "1.1.0" does not exist`) {
		t.Errorf("unexpected error: %v", err)
	}
	// the issue-list call must never happen once the milestone check fails
	if fr.CallCount() != 1 {
		t.Errorf("expected 1 call, got %d", fr.CallCount())
	}
}

func TestListOpenByMilestone_milestoneCheckFailurePropagates(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(nil, errors.New("gh: authentication failed"))

	c := &ghissue.Creator{Runner: fr}
	_, err := c.ListOpenByMilestone("kiwiproject/kiwi", "1.1.0")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("expected underlying error wrapped, got: %v", err)
	}
}

func TestCreate_successFirstAttempt(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: "https://github.com/kiwiproject/kiwi/issues/123\n"}, nil)

	c := &ghissue.Creator{Runner: fr}
	n, err := c.Create("kiwiproject/kiwi", "Update kiwi-parent to 3.0.0", "body", "dependencies", "1.1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 123 {
		t.Errorf("expected issue number 123, got %d", n)
	}
	if fr.CallCount() != 1 {
		t.Errorf("expected 1 call, got %d", fr.CallCount())
	}
	args := strings.Join(fr.Calls[0].Args, " ")
	for _, want := range []string{
		"issue create", "--repo kiwiproject/kiwi", "--title Update kiwi-parent to 3.0.0",
		"--label dependencies", "--milestone 1.1.0",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("expected %q in args: %s", want, args)
		}
	}
}

func TestCreate_retriesThenSucceeds(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(nil, errors.New("gh: network error"))
	fr.AddResponse(&runner.Result{Stdout: "https://github.com/kiwiproject/kiwi/issues/124\n"}, nil)

	c := &ghissue.Creator{Runner: fr, InitialBackoff: time.Millisecond}
	n, err := c.Create("kiwiproject/kiwi", "Update kiwi-bom to 3.3.3", "body", "dependencies", "1.1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 124 {
		t.Errorf("expected issue number 124, got %d", n)
	}
	if fr.CallCount() != 2 {
		t.Errorf("expected 2 calls, got %d", fr.CallCount())
	}
}

func TestCreate_exhaustsRetriesAndFails(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(nil, errors.New("gh: network error 1"))
	fr.AddResponse(nil, errors.New("gh: network error 2"))
	fr.AddResponse(nil, errors.New("gh: network error 3"))

	c := &ghissue.Creator{Runner: fr, MaxAttempts: 3, InitialBackoff: time.Millisecond}
	_, err := c.Create("kiwiproject/kiwi", "Update kiwi-bom to 3.3.3", "body", "dependencies", "1.1.0")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "network error 3") {
		t.Errorf("expected last underlying error wrapped, got: %v", err)
	}
	if fr.CallCount() != 3 {
		t.Errorf("expected 3 calls, got %d", fr.CallCount())
	}
}
