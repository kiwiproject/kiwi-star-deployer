package pom_test

import (
	"strings"
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/pom"
)

func TestReadVersion_snapshotVersion(t *testing.T) {
	v, err := pom.ReadVersion("testdata/snapshot.xml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "2.5.1-SNAPSHOT" {
		t.Errorf("got %q, want 2.5.1-SNAPSHOT", v)
	}
}

func TestReadVersion_ignoresParentVersion(t *testing.T) {
	// snapshot.xml has <parent><version>3.0.0</version> and
	// <version>2.5.1-SNAPSHOT</version>; we must return the project version.
	v, err := pom.ReadVersion("testdata/snapshot.xml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v == "3.0.0" {
		t.Error("returned parent version instead of project version")
	}
}

func TestReadVersion_releaseVersionIsError(t *testing.T) {
	_, err := pom.ReadVersion("testdata/release.xml")
	if err == nil {
		t.Fatal("expected error for non-SNAPSHOT version, got nil")
	}
	if !strings.Contains(err.Error(), "not a SNAPSHOT") {
		t.Errorf("expected 'not a SNAPSHOT' in error, got: %v", err)
	}
}

func TestReadVersion_missingVersionIsError(t *testing.T) {
	_, err := pom.ReadVersion("testdata/no-version.xml")
	if err == nil {
		t.Fatal("expected error for missing version, got nil")
	}
	if !strings.Contains(err.Error(), "no <version>") {
		t.Errorf("expected 'no <version>' in error, got: %v", err)
	}
}

func TestReadVersion_parentVersionOnlyIsError(t *testing.T) {
	// Version exists only inside <parent>, not as a direct child of <project>.
	_, err := pom.ReadVersion("testdata/parent-version-only.xml")
	if err == nil {
		t.Fatal("expected error when project has no own version, got nil")
	}
	if !strings.Contains(err.Error(), "no <version>") {
		t.Errorf("expected 'no <version>' in error, got: %v", err)
	}
}

func TestReadVersion_invalidXMLIsError(t *testing.T) {
	_, err := pom.ReadVersion("testdata/invalid.xml")
	if err == nil {
		t.Fatal("expected error for invalid XML, got nil")
	}
}

func TestReadVersion_fileNotFoundIsError(t *testing.T) {
	_, err := pom.ReadVersion("testdata/nonexistent.xml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
