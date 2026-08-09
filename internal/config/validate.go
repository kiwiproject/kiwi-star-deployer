package config

import (
	"fmt"
	"math"
	"strings"
	"time"
)

var validTypes = map[string]bool{
	"":             true,
	TypeParentPOM:  true,
	TypeBOM:        true,
	TypeLibraryBOM: true,
}

// maxLogRetentionDays is the largest value that keeps
// LogRetentionDays*24*time.Hour from overflowing time.Duration's int64 range.
const maxLogRetentionDays = math.MaxInt64 / int64(24*time.Hour)

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
			depLib, ok := cfg.Libraries[dep]
			if !ok {
				errs = append(errs, fmt.Sprintf("library %q: unknown dependency %q", name, dep))
				continue
			}
			// The library-bom is always released last via synthetic graph
			// edges; libraries downstream of it are outside this tool's scope.
			if depLib.Type == TypeLibraryBOM {
				errs = append(errs, fmt.Sprintf("library %q: depends on the library-bom %q; libraries that depend on the BOM are not supported", name, dep))
			}
		}
	}

	if libraryBOMCount > 1 {
		errs = append(errs, fmt.Sprintf("at most one library may have type %q, found %d", TypeLibraryBOM, libraryBOMCount))
	}

	if cfg.Settings.MavenReleaseTimeout <= 0 {
		errs = append(errs, "maven_release_timeout must be positive")
	}
	if cfg.Settings.MavenCentralMaxWait <= 0 {
		errs = append(errs, "maven_central_max_wait must be positive")
	}
	if cfg.Settings.MavenCentralPollInterval <= 0 {
		errs = append(errs, "maven_central_poll_interval must be positive")
	}
	if cfg.Settings.CIMaxWait <= 0 {
		errs = append(errs, "ci_max_wait must be positive")
	}
	if cfg.Settings.CIPollInterval <= 0 {
		errs = append(errs, "ci_poll_interval must be positive")
	}
	if cfg.Settings.LogRetentionDays < 0 {
		errs = append(errs, "log_retention_days must not be negative")
	}
	if int64(cfg.Settings.LogRetentionDays) > maxLogRetentionDays {
		errs = append(errs, fmt.Sprintf("log_retention_days must be at most %d", maxLogRetentionDays))
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}
