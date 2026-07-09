package plan

import (
	"fmt"
	"io"
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
// in the workspace, then reads each library's pom.xml directly from
// origin/main (never touching the local working copy), and computes release
// and next development versions for every library in stage order.
func Build(cfg *config.Config, ws *workspace.Workspace) ([][]Entry, error) {
	stages, err := graph.New(cfg.Libraries).Stages()
	if err != nil {
		return nil, fmt.Errorf("computing release stages: %w", err)
	}

	result := make([][]Entry, len(stages))
	var propErrs []string
	for i, stage := range stages {
		for _, name := range stage {
			lib := cfg.Libraries[name]
			if err := ws.EnsureCloned(name, lib); err != nil {
				return nil, err
			}
			pomContent, err := ws.ReadFile(name, "pom.xml")
			if err != nil {
				return nil, err
			}
			pomVersion, err := pom.ParseVersion(strings.NewReader(pomContent))
			if err != nil {
				return nil, err
			}
			vp, err := version.Compute(name, pomVersion)
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

			errs, err := validatePOMProperties(name, lib, cfg, pomContent)
			if err != nil {
				return nil, err
			}
			propErrs = append(propErrs, errs...)
		}
	}
	if len(propErrs) > 0 {
		return nil, fmt.Errorf("POM property validation failed:\n%s", strings.Join(propErrs, "\n"))
	}
	return result, nil
}

// validatePOMProperties checks that pomContent contains a property named
// <dep>.version for each of lib's non-parent-pom dependencies, returning one
// violation message per missing property.
func validatePOMProperties(name string, lib config.Library, cfg *config.Config, pomContent string) ([]string, error) {
	var needed []string
	for _, depName := range lib.DependsOn {
		if cfg.Libraries[depName].Type != config.TypeParentPOM {
			needed = append(needed, depName)
		}
	}
	if len(needed) == 0 {
		return nil, nil
	}
	props, err := pom.ParseProperties(strings.NewReader(pomContent))
	if err != nil {
		return nil, fmt.Errorf("reading POM properties for %s: %w", name, err)
	}
	var errs []string
	for _, depName := range needed {
		propName := depName + ".version"
		if _, ok := props[propName]; !ok {
			errs = append(errs, fmt.Sprintf("  %s: missing property %q for dependency %s", name, propName, depName))
		}
	}
	return errs, nil
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
			fmt.Fprintf(tw, "%s\t%s\t%s -> %s\t(next: %s)\n",
				label,
				entry.Name,
				vp.CurrentVersion,
				vp.ReleaseVersion,
				vp.NextDevVersion,
			)
		}
	}
	_ = tw.Flush()
}
