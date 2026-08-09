package mavencentral

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultBaseURL = "https://repo1.maven.org/maven2"

type Checker struct {
	baseURL string
	client  *http.Client
}

func New() *Checker {
	return NewWithBaseURL(defaultBaseURL)
}

func NewWithBaseURL(baseURL string) *Checker {
	return &Checker{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Checker) ArtifactURL(groupID, artifactID, version string) string {
	groupPath := strings.ReplaceAll(groupID, ".", "/")
	return fmt.Sprintf("%s/%s/%s/%s/", c.baseURL, groupPath, artifactID, version)
}

// Available performs a single existence check (no polling). found reports
// whether the artifact version is present (HTTP 200) or definitely absent
// (HTTP 404); any other outcome — transport error, non-404 status — is
// returned as an error, meaning presence could not be determined. The body is
// drained before closing so the keep-alive connection is reusable across
// Wait's polling iterations.
func (c *Checker) Available(groupID, artifactID, version string) (found bool, err error) {
	url := c.ArtifactURL(groupID, artifactID, version)
	resp, err := c.client.Get(url) //nolint:noctx
	if err != nil {
		return false, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
}

// Wait polls Maven Central until the artifact is available or maxWait is exceeded.
// Progress messages are written to w.
func (c *Checker) Wait(w io.Writer, groupID, artifactID, version string, maxWait, interval time.Duration) error {
	fmt.Fprintf(w, "  checking Maven Central: %s %s (max wait: %v, retry every: %v)\n", artifactID, version, maxWait, interval)
	deadline := time.Now().Add(maxWait)
	for {
		found, err := c.Available(groupID, artifactID, version)
		if found {
			return nil
		}
		if err == nil {
			err = fmt.Errorf("HTTP %d", http.StatusNotFound)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s %s not available in Maven Central after %v: %w", artifactID, version, maxWait, err)
		}
		fmt.Fprintf(w, "  not yet available, retrying in %v\n", interval)
		time.Sleep(interval)
	}
}
