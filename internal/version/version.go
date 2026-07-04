package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Plan holds the computed version information for a single library's release.
type Plan struct {
	Name           string
	CurrentVersion string // POM version, e.g. "2.5.1-SNAPSHOT"
	ReleaseVersion string // e.g. "2.5.1"
	NextDevVersion string // e.g. "2.5.2-SNAPSHOT"
}

// Compute derives the release and next development versions for name.
// pomVersion must end with "-SNAPSHOT".
func Compute(name, pomVersion string) (Plan, error) {
	if !strings.HasSuffix(pomVersion, "-SNAPSHOT") {
		return Plan{}, fmt.Errorf("%s: POM version %q is not a SNAPSHOT", name, pomVersion)
	}

	releaseVersion := strings.TrimSuffix(pomVersion, "-SNAPSHOT")

	nextDev, err := nextPatchSnapshot(releaseVersion)
	if err != nil {
		return Plan{}, fmt.Errorf("%s: %w", name, err)
	}

	return Plan{
		Name:           name,
		CurrentVersion: pomVersion,
		ReleaseVersion: releaseVersion,
		NextDevVersion: nextDev,
	}, nil
}

// nextPatchSnapshot parses a "X.Y.Z" version string and returns "X.Y.(Z+1)-SNAPSHOT".
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
