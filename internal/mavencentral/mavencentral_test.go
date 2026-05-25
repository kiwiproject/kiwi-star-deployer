package mavencentral_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/mavencentral"
)

func TestArtifactURL(t *testing.T) {
	c := mavencentral.New()
	got := c.ArtifactURL("org.kiwiproject", "kiwi", "2.5.1")
	want := "https://repo1.maven.org/maven2/org/kiwiproject/kiwi/2.5.1/"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWait_availableImmediately(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := mavencentral.NewWithBaseURL(ts.URL)
	var buf bytes.Buffer
	err := c.Wait(&buf, "org.kiwiproject", "kiwi", "2.5.1", time.Minute, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWait_availableAfterRetry(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	c := mavencentral.NewWithBaseURL(ts.URL)
	var buf bytes.Buffer
	err := c.Wait(&buf, "org.kiwiproject", "kiwi", "2.5.1", time.Minute, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls < 3 {
		t.Errorf("expected at least 3 calls, got %d", calls)
	}
}

func TestWait_timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	c := mavencentral.NewWithBaseURL(ts.URL)
	var buf bytes.Buffer
	err := c.Wait(&buf, "org.kiwiproject", "kiwi", "2.5.1", 50*time.Millisecond, time.Millisecond)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
