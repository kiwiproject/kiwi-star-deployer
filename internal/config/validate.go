package config

import (
	"fmt"
	"regexp"
	"strings"
)

var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

var validTypes = map[string]bool{
	"":             true,
	TypeParentPOM:  true,
	TypeBOM:        true,
	TypeLibraryBOM: true,
}

func validate(cfg *Config) error {
	var errs []string

	libraryBOMCount := 0
	for name, lib := range cfg.Libraries {
		if lib.Repo == "" {
			errs = append(errs, fmt.Sprintf("library %q: repo is required", name))
		}
		if !validTypes[lib.Type] {
			errs = append(errs, fmt.Sprintf("library %q: invalid type %q", name, lib.Type))
		}
		if lib.Type == TypeLibraryBOM {
			libraryBOMCount++
		}
		for _, dep := range lib.DependsOn {
			if dep == name {
				errs = append(errs, fmt.Sprintf("library %q: depends on itself", name))
				continue
			}
			if _, ok := cfg.Libraries[dep]; !ok {
				errs = append(errs, fmt.Sprintf("library %q: unknown dependency %q", name, dep))
			}
		}
	}

	if libraryBOMCount > 1 {
		errs = append(errs, fmt.Sprintf("at most one library may have type %q, found %d", TypeLibraryBOM, libraryBOMCount))
	}

	for name, version := range cfg.Release.Overrides {
		if _, ok := cfg.Libraries[name]; !ok {
			errs = append(errs, fmt.Sprintf("release override for unknown library %q", name))
		}
		if !semverRe.MatchString(version) {
			errs = append(errs, fmt.Sprintf("release override for %q: %q is not a valid version (expected X.Y.Z)", name, version))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}
