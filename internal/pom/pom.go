package pom

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// ParseProperties reads the <properties> section from any XML source and
// returns a map of property name to value. If the POM has no <properties>
// element, an empty map is returned.
func ParseProperties(r io.Reader) (map[string]string, error) {
	return parseProperties(xml.NewDecoder(r))
}

// parseProperties scans for the <properties> element that is a direct child of
// the root <project> element (depth 2 after incrementing) and returns its
// children as a map.
func parseProperties(dec *xml.Decoder) (map[string]string, error) {
	props := make(map[string]string)
	depth := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 && t.Name.Local == "properties" {
				if err := readPropertyChildren(dec, props); err != nil {
					return nil, err
				}
				depth-- // </properties> was consumed inside readPropertyChildren
			}
		case xml.EndElement:
			depth--
		}
	}
	return props, nil
}

// readPropertyChildren reads property elements until </properties> is reached,
// storing each as name→value in props.
func readPropertyChildren(dec *xml.Decoder, props map[string]string) error {
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return fmt.Errorf("unexpected EOF inside <properties>")
		}
		if err != nil {
			return fmt.Errorf("parsing XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			var v string
			if err := dec.DecodeElement(&v, &t); err != nil {
				return fmt.Errorf("decoding <%s>: %w", t.Name.Local, err)
			}
			props[t.Name.Local] = v
		case xml.EndElement:
			return nil // </properties>
		}
	}
}

// Dependency identifies one artifact referenced by a POM.
type Dependency struct {
	GroupID    string
	ArtifactID string
}

// ParseDependencies returns every artifact the POM references that can affect
// release ordering: the <parent> artifact, entries in <dependencies>, and
// entries in <dependencyManagement> (which includes BOM imports). Version
// elements are ignored; only the coordinates are extracted.
func ParseDependencies(r io.Reader) ([]Dependency, error) {
	type coord struct {
		GroupID    string `xml:"groupId"`
		ArtifactID string `xml:"artifactId"`
	}
	var doc struct {
		Parent  coord   `xml:"parent"`
		Deps    []coord `xml:"dependencies>dependency"`
		Managed []coord `xml:"dependencyManagement>dependencies>dependency"`
	}
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("parsing XML: %w", err)
	}
	var deps []Dependency
	if doc.Parent.ArtifactID != "" {
		deps = append(deps, Dependency{GroupID: doc.Parent.GroupID, ArtifactID: doc.Parent.ArtifactID})
	}
	for _, d := range doc.Deps {
		deps = append(deps, Dependency(d))
	}
	for _, d := range doc.Managed {
		deps = append(deps, Dependency(d))
	}
	return deps, nil
}

// ParseVersion reads the project version from any XML source. It returns an
// error if the XML is malformed, no <version> element exists as a direct child
// of <project>, or the version is not a SNAPSHOT.
func ParseVersion(r io.Reader) (string, error) {
	return parseVersion(xml.NewDecoder(r))
}

// parseVersion scans the XML token stream for the first <version> element
// that is a direct child of the root <project> element (depth 1). It ignores
// <parent><version> and any deeper nesting.
func parseVersion(dec *xml.Decoder) (string, error) {
	depth := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parsing XML: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 1 && t.Name.Local == "version" {
				var v string
				if err := dec.DecodeElement(&v, &t); err != nil {
					return "", fmt.Errorf("decoding <version>: %w", err)
				}
				if !strings.HasSuffix(v, "-SNAPSHOT") {
					return "", fmt.Errorf("version %q is not a SNAPSHOT; unexpected state", v)
				}
				return v, nil
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return "", fmt.Errorf("no <version> element found in root <project>")
}
