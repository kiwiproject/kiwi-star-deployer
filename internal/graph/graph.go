package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
)

// Graph is a directed dependency graph of libraries.
// Callers must ensure all DependsOn entries reference libraries present in the
// map passed to New (config.Load guarantees this).
type Graph struct {
	nodes      []string            // all node names, sorted for deterministic output
	successors map[string][]string // successors[A] = libs that must come after A
	inDegree   map[string]int      // number of incoming edges per node
}

// New builds a dependency graph from the given library map. If a library with
// type "library-bom" is present, synthetic dependency edges are added from
// every non-library-bom, non-downstream library to it. This ensures it appears
// after all core libraries in the sort while still allowing libraries that
// depend on it (e.g. elucidation) to follow naturally.
func New(libs map[string]config.Library) *Graph {
	g := &Graph{
		successors: make(map[string][]string, len(libs)),
		inDegree:   make(map[string]int, len(libs)),
	}

	for name := range libs {
		g.nodes = append(g.nodes, name)
		g.successors[name] = nil
		g.inDegree[name] = 0
	}
	sort.Strings(g.nodes)

	for name, lib := range libs {
		for _, dep := range lib.DependsOn {
			g.successors[dep] = append(g.successors[dep], name)
			g.inDegree[name]++
		}
	}

	if agg := findLibraryBOM(libs); agg != "" {
		downstream := reachableFrom(g.successors, agg)
		aggDeps := toSet(libs[agg].DependsOn)

		for _, name := range g.nodes {
			if name == agg || downstream[name] || aggDeps[name] {
				continue
			}
			g.successors[name] = append(g.successors[name], agg)
			g.inDegree[agg]++
		}
	}

	return g
}

// Stages performs a topological sort and returns library names grouped into
// release stages. Libraries within a stage have no inter-dependencies and can
// be released in parallel. Returns an error if a dependency cycle is detected.
func (g *Graph) Stages() ([][]string, error) {
	inDeg := make(map[string]int, len(g.inDegree))
	for k, v := range g.inDegree {
		inDeg[k] = v
	}

	remaining := make(map[string]bool, len(g.nodes))
	for _, name := range g.nodes {
		remaining[name] = true
	}

	var stages [][]string
	for len(remaining) > 0 {
		var stage []string
		for name := range remaining {
			if inDeg[name] == 0 {
				stage = append(stage, name)
			}
		}
		if len(stage) == 0 {
			return nil, cycleError(remaining)
		}
		sort.Strings(stage)
		stages = append(stages, stage)

		for _, name := range stage {
			delete(remaining, name)
			for _, succ := range g.successors[name] {
				inDeg[succ]--
			}
		}
	}

	return stages, nil
}

func findLibraryBOM(libs map[string]config.Library) string {
	for name, lib := range libs {
		if lib.Type == config.TypeLibraryBOM {
			return name
		}
	}
	return ""
}

// reachableFrom returns all nodes reachable from start by following successors
// (not including start itself).
func reachableFrom(successors map[string][]string, start string) map[string]bool {
	visited := make(map[string]bool)
	queue := append([]string(nil), successors[start]...)
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if visited[node] {
			continue
		}
		visited[node] = true
		queue = append(queue, successors[node]...)
	}
	return visited
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

func cycleError(remaining map[string]bool) error {
	members := make([]string, 0, len(remaining))
	for name := range remaining {
		members = append(members, name)
	}
	sort.Strings(members)
	return fmt.Errorf("dependency cycle detected among: %s", strings.Join(members, ", "))
}
