package plan

import (
	"fmt"
	"io"
	"strings"
	"sync"
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
// and next development versions for every library in stage order. Libraries
// within a stage are independent by construction and are read concurrently.
func Build(cfg *config.Config, ws *workspace.Workspace) ([][]Entry, error) {
	stages, err := graph.New(cfg.Libraries).Stages()
	if err != nil {
		return nil, fmt.Errorf("computing release stages: %w", err)
	}

	result := make([][]Entry, len(stages))
	var propErrs []string
	for i, stage := range stages {
		entries, errs, err := buildStage(cfg, ws, stage, i+1)
		if err != nil {
			return nil, err
		}
		result[i] = entries
		propErrs = append(propErrs, errs...)
	}
	if len(propErrs) > 0 {
		return nil, fmt.Errorf("POM validation failed:\n%s", strings.Join(propErrs, "\n"))
	}
	return result, nil
}

// entryResult holds the outcome of building one library's Entry.
type entryResult struct {
	entry Entry
	errs  []string
	err   error
}

// buildStage reads pom.xml and computes the version plan for every library in
// a stage concurrently, then returns the entries in the same order as stage.
// Every library in the stage is always attempted; if any fail, all of their
// errors are combined into one so a user fixing problems doesn't have to
// discover them one at a time across repeated runs.
func buildStage(cfg *config.Config, ws *workspace.Workspace, stage []string, stageNum int) ([]Entry, []string, error) {
	results := make([]entryResult, len(stage))
	var wg sync.WaitGroup

	for i, name := range stage {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			results[idx] = buildEntry(cfg, ws, name, stageNum)
		}(i, name)
	}
	wg.Wait()

	entries := make([]Entry, len(stage))
	var propErrs []string
	var buildErrs []string
	for i, res := range results {
		if res.err != nil {
			buildErrs = append(buildErrs, fmt.Sprintf("  %s: %v", stage[i], res.err))
			continue
		}
		entries[i] = res.entry
		propErrs = append(propErrs, res.errs...)
	}
	if len(buildErrs) > 0 {
		return nil, nil, fmt.Errorf("stage %d failed:\n%s", stageNum, strings.Join(buildErrs, "\n"))
	}
	return entries, propErrs, nil
}

// buildEntry clones name's repo if missing, reads its pom.xml from
// origin/main, and computes its version plan and property-validation errors.
func buildEntry(cfg *config.Config, ws *workspace.Workspace, name string, stageNum int) entryResult {
	lib := cfg.Libraries[name]
	if err := ws.EnsureCloned(name, lib); err != nil {
		return entryResult{err: err}
	}
	pomContent, err := ws.ReadFile(name, "pom.xml")
	if err != nil {
		return entryResult{err: err}
	}
	pomVersion, err := pom.ParseVersion(strings.NewReader(pomContent))
	if err != nil {
		return entryResult{err: err}
	}
	vp, err := version.Compute(name, pomVersion)
	if err != nil {
		return entryResult{err: err}
	}
	entry := Entry{
		Name:        name,
		Repo:        lib.Repo,
		Type:        lib.Type,
		DependsOn:   lib.DependsOn,
		Stage:       stageNum,
		VersionPlan: vp,
	}
	errs, err := validatePOMProperties(name, lib, cfg, pomContent)
	if err != nil {
		return entryResult{err: err}
	}
	dependsOnErrs, err := validateDependsOn(name, lib, cfg, pomContent)
	if err != nil {
		return entryResult{err: err}
	}
	errs = append(errs, dependsOnErrs...)
	return entryResult{entry: entry, errs: errs}
}

// validateDependsOn checks the config against the POM's actual dependencies:
// every artifact the POM references (parent, dependencies, and
// dependencyManagement entries) whose groupId matches the configured group
// and whose artifactId names another configured library must be listed in
// lib's depends_on. A missing edge silently corrupts release ordering, which
// is exactly what the explicit config exists to prevent. The library-bom
// type is exempt: its POM intentionally references every library, and
// synthetic graph edges already force it to release last. The reverse
// direction — an edge declared in depends_on but absent from the POM — only
// makes staging more conservative and is not flagged.
func validateDependsOn(name string, lib config.Library, cfg *config.Config, pomContent string) ([]string, error) {
	if lib.Type == config.TypeLibraryBOM {
		return nil, nil
	}
	deps, err := pom.ParseDependencies(strings.NewReader(pomContent))
	if err != nil {
		return nil, fmt.Errorf("reading POM dependencies for %s: %w", name, err)
	}
	declared := make(map[string]bool, len(lib.DependsOn))
	for _, d := range lib.DependsOn {
		declared[d] = true
	}
	var errs []string
	seen := make(map[string]bool)
	for _, dep := range deps {
		// ${project.groupId} resolves to the configured group for every
		// library this tool releases, so treat it as a match rather than
		// missing a sibling dependency declared that way.
		groupMatches := dep.GroupID == cfg.Settings.GroupID || dep.GroupID == "${project.groupId}"
		if !groupMatches || dep.ArtifactID == name || seen[dep.ArtifactID] {
			continue
		}
		if _, configured := cfg.Libraries[dep.ArtifactID]; !configured {
			continue
		}
		seen[dep.ArtifactID] = true
		if !declared[dep.ArtifactID] {
			errs = append(errs, fmt.Sprintf("  %s: pom.xml depends on %s but depends_on does not list it", name, dep.ArtifactID))
		}
	}
	return errs, nil
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
