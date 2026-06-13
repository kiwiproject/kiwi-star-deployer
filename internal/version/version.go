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
// An error is returned if the override is lower than the current POM version.
func Compute(name, pomVersion, override string) (Plan, error) {
	if !strings.HasSuffix(pomVersion, "-SNAPSHOT") {
		return Plan{}, fmt.Errorf("%s: POM version %q is not a SNAPSHOT", name, pomVersion)
	}

	derived := strings.TrimSuffix(pomVersion, "-SNAPSHOT")

	releaseVersion := derived
	overrideApplied := false
	if override != "" {
		derivedParts, err := parseXYZ(derived)
		if err != nil {
			return Plan{}, fmt.Errorf("%s: POM version: %w", name, err)
		}
		overrideParts, err := parseXYZ(override)
		if err != nil {
			return Plan{}, fmt.Errorf("%s: invalid override: %w", name, err)
		}
		if lessVersion(overrideParts, derivedParts) {
			return Plan{}, fmt.Errorf("%s: override %q is lower than current POM version %q", name, override, derived)
		}
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
func nextPatchSnapshot(v string) (string, error) {
	nums, err := parseXYZ(v)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d.%d-SNAPSHOT", nums[0], nums[1], nums[2]+1), nil
}

// parseXYZ parses an "X.Y.Z" version string into its three numeric components.
func parseXYZ(v string) ([3]int, error) {
	segs := strings.Split(v, ".")
	if len(segs) != 3 {
		return [3]int{}, fmt.Errorf("version %q is not X.Y.Z format", v)
	}
	var nums [3]int
	for i, s := range segs {
		if len(s) > 1 && s[0] == '0' {
			return [3]int{}, fmt.Errorf("version %q has a leading zero in segment %q", v, s)
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return [3]int{}, fmt.Errorf("invalid number in version %q: %w", v, err)
		}
		nums[i] = n
	}
	return nums, nil
}

// lessVersion reports whether a is strictly less than b.
func lessVersion(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
