package version_test

import (
	"strings"
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/version"
)

func TestCompute_defaultDerivation(t *testing.T) {
	plan, err := version.Compute("kiwi", "2.5.1-SNAPSHOT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Name != "kiwi" {
		t.Errorf("name: got %q, want kiwi", plan.Name)
	}
	if plan.CurrentVersion != "2.5.1-SNAPSHOT" {
		t.Errorf("current: got %q, want 2.5.1-SNAPSHOT", plan.CurrentVersion)
	}
	if plan.ReleaseVersion != "2.5.1" {
		t.Errorf("release: got %q, want 2.5.1", plan.ReleaseVersion)
	}
	if plan.NextDevVersion != "2.5.2-SNAPSHOT" {
		t.Errorf("next dev: got %q, want 2.5.2-SNAPSHOT", plan.NextDevVersion)
	}
}

func TestCompute_tableVariants(t *testing.T) {
	tests := []struct {
		name       string
		pomVersion string
		wantRel    string
		wantNext   string
	}{
		{"patch bump", "1.2.3-SNAPSHOT", "1.2.3", "1.2.4-SNAPSHOT"},
		{"patch zero", "3.0.0-SNAPSHOT", "3.0.0", "3.0.1-SNAPSHOT"},
		{"minor version", "1.1.0-SNAPSHOT", "1.1.0", "1.1.1-SNAPSHOT"},
		{"major version", "1.0.0-SNAPSHOT", "1.0.0", "1.0.1-SNAPSHOT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := version.Compute("lib", tt.pomVersion)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if plan.ReleaseVersion != tt.wantRel {
				t.Errorf("release: got %q, want %q", plan.ReleaseVersion, tt.wantRel)
			}
			if plan.NextDevVersion != tt.wantNext {
				t.Errorf("next dev: got %q, want %q", plan.NextDevVersion, tt.wantNext)
			}
		})
	}
}

func TestCompute_nonSnapshotIsError(t *testing.T) {
	_, err := version.Compute("kiwi", "2.5.1")
	if err == nil {
		t.Fatal("expected error for non-SNAPSHOT version, got nil")
	}
	if !strings.Contains(err.Error(), "not a SNAPSHOT") {
		t.Errorf("expected 'not a SNAPSHOT' in error, got: %v", err)
	}
}

func TestCompute_malformedVersionIsError(t *testing.T) {
	_, err := version.Compute("kiwi", "2.5-SNAPSHOT")
	if err == nil {
		t.Fatal("expected error for malformed version, got nil")
	}
	if !strings.Contains(err.Error(), "X.Y.Z") {
		t.Errorf("expected 'X.Y.Z' in error, got: %v", err)
	}
}

func TestCompute_leadingZeroInPOMVersionIsError(t *testing.T) {
	tests := []struct {
		name       string
		pomVersion string
	}{
		{"leading zero major", "01.2.3-SNAPSHOT"},
		{"leading zero minor", "1.02.3-SNAPSHOT"},
		{"leading zero patch", "1.2.03-SNAPSHOT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := version.Compute("kiwi", tt.pomVersion)
			if err == nil {
				t.Fatalf("expected error for leading zero in POM version %q, got nil", tt.pomVersion)
			}
			if !strings.Contains(err.Error(), "leading zero") {
				t.Errorf("expected 'leading zero' in error, got: %v", err)
			}
		})
	}
}
