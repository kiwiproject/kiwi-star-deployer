package plan

import (
	"fmt"
	"io"
	"path/filepath"
	"text/tabwriter"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/graph"
	"github.com/kiwiproject/kiwi-star-deployer/internal/pom"
	"github.com/kiwiproject/kiwi-star-deployer/internal/version"
	"github.com/kiwiproject/kiwi-star-deployer/internal/workspace"
)

// Entry holds the version plan for one library within a release stage.
type Entry struct {
	Name        string
	Repo        string   // GitHub repository, e.g. "kiwiproject/kiwi"
	Type        string   // library type, e.g. "library-bom"
	DependsOn   []string // names of libraries this one depends on
	Stage       int
	VersionPlan version.Plan
}

// IsLibraryBOM reports whether this entry is a library-bom.
func (e Entry) IsLibraryBOM() bool {
	return e.Type == config.TypeLibraryBOM
}

// Build constructs the full release plan. It clones any repos not yet present
// in the workspace, reads current POM versions, and computes release and next
// development versions for every library in stage order.
func Build(cfg *config.Config, ws *workspace.Workspace) ([][]Entry, error) {
	stages, err := graph.New(cfg.Libraries).Stages()
	if err != nil {
		return nil, fmt.Errorf("computing release stages: %w", err)
	}

	result := make([][]Entry, len(stages))
	for i, stage := range stages {
		for _, name := range stage {
			lib := cfg.Libraries[name]
			if err := ws.EnsureCloned(name, lib); err != nil {
				return nil, err
			}
			pomVersion, err := pom.ReadVersion(filepath.Join(ws.RepoDir(name), "pom.xml"))
			if err != nil {
				return nil, err
			}
			vp, err := version.Compute(name, pomVersion, cfg.Release.Overrides[name])
			if err != nil {
				return nil, err
			}
			result[i] = append(result[i], Entry{
				Name:        name,
				Repo:        lib.Repo,
				Type:        lib.Type,
				DependsOn:   lib.DependsOn,
				Stage:       i + 1,
				VersionPlan: vp,
			})
		}
	}
	return result, nil
}

// Print writes the plan to w in a columnar format, one library per line.
func Print(w io.Writer, stages [][]Entry) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for i, stage := range stages {
		for j, entry := range stage {
			label := ""
			if j == 0 {
				label = fmt.Sprintf("Stage %d:", i+1)
			}
			vp := entry.VersionPlan
			override := ""
			if vp.OverrideApplied {
				override = " [OVERRIDE]"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s -> %s%s\t(next: %s)\n",
				label,
				entry.Name,
				vp.CurrentVersion,
				vp.ReleaseVersion,
				override,
				vp.NextDevVersion,
			)
		}
	}
	_ = tw.Flush()
}
