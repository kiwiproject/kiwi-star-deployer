package plan

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
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
	if err := validatePOMProperties(result, cfg, ws); err != nil {
		return nil, err
	}
	return result, nil
}

// validatePOMProperties checks that every library's pom.xml contains a property
// named <dep>.version for each non-parent-pom dependency. All violations are
// collected and returned as a single error so the user can fix them all at once.
func validatePOMProperties(stages [][]Entry, cfg *config.Config, ws *workspace.Workspace) error {
	var errs []string
	for _, stage := range stages {
		for _, entry := range stage {
			var needed []string
			for _, depName := range entry.DependsOn {
				if cfg.Libraries[depName].Type != config.TypeParentPOM {
					needed = append(needed, depName)
				}
			}
			if len(needed) == 0 {
				continue
			}
			pomPath := filepath.Join(ws.RepoDir(entry.Name), "pom.xml")
			props, err := pom.ReadProperties(pomPath)
			if err != nil {
				return fmt.Errorf("reading POM properties for %s: %w", entry.Name, err)
			}
			for _, depName := range needed {
				propName := depName + ".version"
				if _, ok := props[propName]; !ok {
					errs = append(errs, fmt.Sprintf("  %s: missing property %q for dependency %s", entry.Name, propName, depName))
				}
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("POM property validation failed:\n%s", strings.Join(errs, "\n"))
	}
	return nil
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
