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

func (c *Checker) available(groupID, artifactID, version string) error {
	url := c.ArtifactURL(groupID, artifactID, version)
	resp, err := c.client.Get(url) //nolint:noctx
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// Wait polls Maven Central until the artifact is available or maxWait is exceeded.
// Progress messages are written to w.
func (c *Checker) Wait(w io.Writer, groupID, artifactID, version string, maxWait, interval time.Duration) error {
	fmt.Fprintf(w, "  checking Maven Central: %s %s (max wait: %v, retry every: %v)\n", artifactID, version, maxWait, interval)
	deadline := time.Now().Add(maxWait)
	var lastErr error
	for {
		if lastErr = c.available(groupID, artifactID, version); lastErr == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s %s not available in Maven Central after %v: %w", artifactID, version, maxWait, lastErr)
		}
		fmt.Fprintf(w, "  not yet available, retrying in %v\n", interval)
		time.Sleep(interval)
	}
}
