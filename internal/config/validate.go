package config

import (
	"fmt"
	"strings"
)

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

	if cfg.Settings.CIVerify != nil && *cfg.Settings.CIVerify {
		if cfg.Settings.CIMaxWait <= 0 {
			errs = append(errs, "ci_max_wait must be positive when ci_verify is true")
		}
		if cfg.Settings.CIPollInterval <= 0 {
			errs = append(errs, "ci_poll_interval must be positive when ci_verify is true")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}
