package cmd

import (
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/plan"
	ver "github.com/kiwiproject/kiwi-star-deployer/internal/version"
)

func mustPlanEntry(name string) plan.Entry {
	vp, err := ver.Compute(name, "1.0.0-SNAPSHOT", "")
	if err != nil {
		panic(err)
	}
	return plan.Entry{Name: name, Stage: 1, VersionPlan: vp}
}

func TestFilterStages_keepsMatchingEntries(t *testing.T) {
	stages := [][]plan.Entry{
		{mustPlanEntry("kiwi-parent")},
		{mustPlanEntry("kiwi"), mustPlanEntry("kiwi-test")},
	}

	got := filterStages(stages, []string{"kiwi-parent", "kiwi"})

	if len(got) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(got))
	}
	if len(got[0]) != 1 || got[0][0].Name != "kiwi-parent" {
		t.Errorf("stage 1: got %v, want [kiwi-parent]", names(got[0]))
	}
	if len(got[1]) != 1 || got[1][0].Name != "kiwi" {
		t.Errorf("stage 2: got %v, want [kiwi]", names(got[1]))
	}
}

func TestFilterStages_dropsEmptyStages(t *testing.T) {
	stages := [][]plan.Entry{
		{mustPlanEntry("kiwi-parent")},
		{mustPlanEntry("kiwi")},
	}

	got := filterStages(stages, []string{"kiwi"})

	if len(got) != 1 {
		t.Fatalf("expected 1 stage (empty stage dropped), got %d", len(got))
	}
	if got[0][0].Name != "kiwi" {
		t.Errorf("expected kiwi, got %s", got[0][0].Name)
	}
}

func TestFilterStages_emptyOnlyReturnsEmpty(t *testing.T) {
	stages := [][]plan.Entry{
		{mustPlanEntry("kiwi-parent")},
		{mustPlanEntry("kiwi")},
	}

	got := filterStages(stages, []string{"nonexistent"})

	if len(got) != 0 {
		t.Errorf("expected 0 stages, got %d", len(got))
	}
}

func TestFilterStages_allMatchedPreservesAll(t *testing.T) {
	stages := [][]plan.Entry{
		{mustPlanEntry("kiwi-parent")},
		{mustPlanEntry("kiwi"), mustPlanEntry("kiwi-test")},
	}

	got := filterStages(stages, []string{"kiwi-parent", "kiwi", "kiwi-test"})

	if len(got) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(got))
	}
	if len(got[1]) != 2 {
		t.Errorf("stage 2: expected 2 entries, got %d", len(got[1]))
	}
}

func names(entries []plan.Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}
