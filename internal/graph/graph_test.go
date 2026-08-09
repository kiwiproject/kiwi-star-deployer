package graph_test

import (
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/graph"
)

func TestStages_singleNode(t *testing.T) {
	libs := map[string]config.Library{
		"kiwi": {Repo: "kiwiproject/kiwi"},
	}
	stages := mustStages(t, libs)
	assertStages(t, stages, [][]string{{"kiwi"}})
}

func TestStages_emptyGraph(t *testing.T) {
	stages := mustStages(t, map[string]config.Library{})
	if len(stages) != 0 {
		t.Errorf("expected no stages, got %v", stages)
	}
}

func TestStages_linearChain(t *testing.T) {
	libs := map[string]config.Library{
		"a": {Repo: "org/a"},
		"b": {Repo: "org/b", DependsOn: []string{"a"}},
		"c": {Repo: "org/c", DependsOn: []string{"b"}},
	}
	stages := mustStages(t, libs)
	assertStages(t, stages, [][]string{{"a"}, {"b"}, {"c"}})
}

func TestStages_parallelLibsInSameStage(t *testing.T) {
	// Diamond: a is root; b and c both depend only on a; d depends on b and c.
	libs := map[string]config.Library{
		"a": {Repo: "org/a"},
		"b": {Repo: "org/b", DependsOn: []string{"a"}},
		"c": {Repo: "org/c", DependsOn: []string{"a"}},
		"d": {Repo: "org/d", DependsOn: []string{"b", "c"}},
	}
	stages := mustStages(t, libs)
	assertStages(t, stages, [][]string{{"a"}, {"b", "c"}, {"d"}})
}

func TestStages_cycleDetected(t *testing.T) {
	libs := map[string]config.Library{
		"a": {Repo: "org/a", DependsOn: []string{"b"}},
		"b": {Repo: "org/b", DependsOn: []string{"a"}},
	}
	g := graph.New(libs)
	_, err := g.Stages()
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected 'cycle' in error message, got: %v", err)
	}
}

func TestStages_libraryBOMLastAmongCoreLibs(t *testing.T) {
	// kiwi-libraries-bom declares only kiwi in depends_on; synthetic edges
	// must pull it after kiwi-parent and kiwi-bom as well.
	libs := map[string]config.Library{
		"kiwi-parent":        {Repo: "kiwiproject/kiwi-parent", Type: "parent-pom"},
		"kiwi-bom":           {Repo: "kiwiproject/kiwi-bom", Type: "bom", DependsOn: []string{"kiwi-parent"}},
		"kiwi":               {Repo: "kiwiproject/kiwi", DependsOn: []string{"kiwi-parent", "kiwi-bom"}},
		"kiwi-libraries-bom": {Repo: "kiwiproject/kiwi-libraries-bom", Type: "library-bom", DependsOn: []string{"kiwi"}},
	}
	stages := mustStages(t, libs)
	assertStages(t, stages, [][]string{
		{"kiwi-parent"},
		{"kiwi-bom"},
		{"kiwi"},
		{"kiwi-libraries-bom"},
	})
}

func TestStages_libraryDependingOnBOMIsCycle(t *testing.T) {
	// Libraries downstream of the library-bom are rejected by config
	// validation; if such a config somehow reaches the graph, the synthetic
	// edge to the BOM plus the declared edge from it form a cycle, so the
	// sort fails loudly rather than producing a wrong order.
	libs := map[string]config.Library{
		"kiwi-parent":        {Repo: "kiwiproject/kiwi-parent", Type: "parent-pom"},
		"kiwi":               {Repo: "kiwiproject/kiwi", DependsOn: []string{"kiwi-parent"}},
		"kiwi-libraries-bom": {Repo: "kiwiproject/kiwi-libraries-bom", Type: "library-bom", DependsOn: []string{"kiwi"}},
		"elucidation":        {Repo: "elucidation-project/elucidation", DependsOn: []string{"kiwi-libraries-bom"}},
	}
	_, err := graph.New(libs).Stages()
	if err == nil {
		t.Fatal("expected cycle error for library depending on the BOM, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

func TestStages_libraryBOMWithNoDeclaredDeps(t *testing.T) {
	// library-bom declares no depends_on at all; synthetic edges must be
	// added to every other library.
	libs := map[string]config.Library{
		"a":   {Repo: "org/a"},
		"b":   {Repo: "org/b", DependsOn: []string{"a"}},
		"agg": {Repo: "org/agg", Type: "library-bom"},
	}
	stages := mustStages(t, libs)
	assertStages(t, stages, [][]string{{"a"}, {"b"}, {"agg"}})
}

func TestStages_noLibraryBOM(t *testing.T) {
	// No library-bom means no synthetic edges; plain topo sort.
	libs := map[string]config.Library{
		"a": {Repo: "org/a"},
		"b": {Repo: "org/b", DependsOn: []string{"a"}},
		"c": {Repo: "org/c", DependsOn: []string{"a"}},
	}
	stages := mustStages(t, libs)
	assertStages(t, stages, [][]string{{"a"}, {"b", "c"}})
}

// mustStages calls graph.New and Stages, failing the test on any error.
func mustStages(t *testing.T, libs map[string]config.Library) [][]string {
	t.Helper()
	g := graph.New(libs)
	stages, err := g.Stages()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return stages
}

// assertStages checks that got and want have the same number of stages and that
// each stage contains the same library names (order within a stage is ignored).
func assertStages(t *testing.T, got, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("stage count: got %d, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i, wantStage := range want {
		gotSorted := sorted(got[i])
		wantSorted := sorted(wantStage)
		if !slices.Equal(gotSorted, wantSorted) {
			t.Errorf("stage %d: got %v, want %v", i+1, got[i], wantStage)
		}
	}
}

func sorted(ss []string) []string {
	out := make([]string, len(ss))
	copy(out, ss)
	sort.Strings(out)
	return out
}
