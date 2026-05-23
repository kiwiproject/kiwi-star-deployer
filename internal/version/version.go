package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Plan holds the computed version information for a single library's release.
type Plan struct {
	Name            string
	CurrentVersion  string // POM version, e.g. "2.5.1-SNAPSHOT"
	ReleaseVersion  string // e.g. "2.5.1", or override like "3.0.0"
	NextDevVersion  string // e.g. "2.5.2-SNAPSHOT"
	OverrideApplied bool
}

// Compute derives the release and next development versions for name.
// pomVersion must end with "-SNAPSHOT". If override is non-empty it replaces
// the POM-derived release version (used for major/minor bumps configured in
// [release.overrides]). The next development version is always patch+1-SNAPSHOT
// of the release version, regardless of whether an override was applied.
func Compute(name, pomVersion, override string) (Plan, error) {
	if !strings.HasSuffix(pomVersion, "-SNAPSHOT") {
		return Plan{}, fmt.Errorf("%s: POM version %q is not a SNAPSHOT", name, pomVersion)
	}

	derived := strings.TrimSuffix(pomVersion, "-SNAPSHOT")

	releaseVersion := derived
	overrideApplied := false
	if override != "" {
		releaseVersion = override
		overrideApplied = true
	}

	nextDev, err := nextPatchSnapshot(releaseVersion)
	if err != nil {
		return Plan{}, fmt.Errorf("%s: %w", name, err)
	}

	return Plan{
		Name:            name,
		CurrentVersion:  pomVersion,
		ReleaseVersion:  releaseVersion,
		NextDevVersion:  nextDev,
		OverrideApplied: overrideApplied,
	}, nil
}

// nextPatchSnapshot parses a "X.Y.Z" version string and returns "X.Y.(Z+1)-SNAPSHOT".
// Pre-release suffixes (e.g. "1.8.0-alpha.1") are not supported; use a
// [release.overrides] entry for libraries that use non-standard versioning.
func nextPatchSnapshot(version string) (string, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("version %q is not X.Y.Z format", version)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", fmt.Errorf("invalid patch number in version %q: %w", version, err)
	}
	return fmt.Sprintf("%s.%s.%d-SNAPSHOT", parts[0], parts[1], patch+1), nil
}
