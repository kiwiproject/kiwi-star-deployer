package pom

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
)

// ReadProperties reads the <properties> section from the pom.xml at path and
// returns a map of property name to value. If the POM has no <properties>
// element, an empty map is returned.
func ReadProperties(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	props, err := parseProperties(xml.NewDecoder(f))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return props, nil
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

// ReadVersion reads the current version from the pom.xml at path. It returns
// an error if the file cannot be read, the XML is malformed, no <version>
// element exists as a direct child of <project>, or the version is not a
// SNAPSHOT (which would indicate an unexpected post-release state).
func ReadVersion(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	version, err := parseVersion(xml.NewDecoder(f))
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return version, nil
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
