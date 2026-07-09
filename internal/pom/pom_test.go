package pom_test

import (
	"os"
	"strings"
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/pom"
)

func open(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestParseVersion_snapshotVersion(t *testing.T) {
	v, err := pom.ParseVersion(open(t, "testdata/snapshot.xml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "2.5.1-SNAPSHOT" {
		t.Errorf("got %q, want 2.5.1-SNAPSHOT", v)
	}
}

func TestParseVersion_ignoresParentVersion(t *testing.T) {
	// snapshot.xml has <parent><version>3.0.0</version> and
	// <version>2.5.1-SNAPSHOT</version>; we must return the project version.
	v, err := pom.ParseVersion(open(t, "testdata/snapshot.xml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v == "3.0.0" {
		t.Error("returned parent version instead of project version")
	}
}

func TestParseVersion_releaseVersionIsError(t *testing.T) {
	_, err := pom.ParseVersion(open(t, "testdata/release.xml"))
	if err == nil {
		t.Fatal("expected error for non-SNAPSHOT version, got nil")
	}
	if !strings.Contains(err.Error(), "not a SNAPSHOT") {
		t.Errorf("expected 'not a SNAPSHOT' in error, got: %v", err)
	}
}

func TestParseVersion_missingVersionIsError(t *testing.T) {
	_, err := pom.ParseVersion(open(t, "testdata/no-version.xml"))
	if err == nil {
		t.Fatal("expected error for missing version, got nil")
	}
	if !strings.Contains(err.Error(), "no <version>") {
		t.Errorf("expected 'no <version>' in error, got: %v", err)
	}
}

func TestParseVersion_parentVersionOnlyIsError(t *testing.T) {
	// Version exists only inside <parent>, not as a direct child of <project>.
	_, err := pom.ParseVersion(open(t, "testdata/parent-version-only.xml"))
	if err == nil {
		t.Fatal("expected error when project has no own version, got nil")
	}
	if !strings.Contains(err.Error(), "no <version>") {
		t.Errorf("expected 'no <version>' in error, got: %v", err)
	}
}

func TestParseVersion_invalidXMLIsError(t *testing.T) {
	_, err := pom.ParseVersion(open(t, "testdata/invalid.xml"))
	if err == nil {
		t.Fatal("expected error for invalid XML, got nil")
	}
}

func TestParseProperties_returnsProperties(t *testing.T) {
	props, err := pom.ParseProperties(open(t, "testdata/with-properties.xml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{
		"kiwi-bom.version":  "3.2.0",
		"kiwi-test.version": "4.1.0",
		"sonar.projectKey":  "kiwiproject_kiwi",
	}
	for k, v := range want {
		if got := props[k]; got != v {
			t.Errorf("property %q: got %q, want %q", k, got, v)
		}
	}
	if len(props) != len(want) {
		t.Errorf("property count: got %d, want %d", len(props), len(want))
	}
}

func TestParseProperties_noPropertiesReturnsEmptyMap(t *testing.T) {
	props, err := pom.ParseProperties(open(t, "testdata/snapshot.xml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(props) != 0 {
		t.Errorf("expected empty map, got %v", props)
	}
}

func TestParseProperties_invalidXMLIsError(t *testing.T) {
	_, err := pom.ParseProperties(open(t, "testdata/invalid.xml"))
	if err == nil {
		t.Fatal("expected error for invalid XML, got nil")
	}
}
