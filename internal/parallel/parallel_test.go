package parallel_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/parallel"
)

func TestForEachLibrary_sortedByName(t *testing.T) {
	libs := map[string]config.Library{
		"c": {Repo: "o/c"}, "a": {Repo: "o/a"}, "b": {Repo: "o/b"},
	}

	got := parallel.ForEachLibrary(libs, 2, func(name string, lib config.Library) string {
		return name + "=" + lib.Repo
	})

	want := []string{"a=o/a", "b=o/b", "c=o/c"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestForEachLibrary_respectsLimit(t *testing.T) {
	libs := make(map[string]config.Library, 20)
	for i := 0; i < 20; i++ {
		libs[fmt.Sprintf("lib-%02d", i)] = config.Library{}
	}
	const limit = 3
	var running, peak atomic.Int32
	var mu sync.Mutex

	parallel.ForEachLibrary(libs, limit, func(string, config.Library) int {
		n := running.Add(1)
		mu.Lock()
		if n > peak.Load() {
			peak.Store(n)
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond) // long enough for the other calls to overlap
		running.Add(-1)
		return 0
	})

	if peak.Load() > limit {
		t.Errorf("peak concurrency %d exceeded the limit %d", peak.Load(), limit)
	}
	if peak.Load() < 2 {
		t.Errorf("expected calls to overlap, but peak concurrency was %d", peak.Load())
	}
}

func TestForEachLibrary_nonPositiveLimitStillRuns(t *testing.T) {
	libs := map[string]config.Library{"a": {}, "b": {}}

	got := parallel.ForEachLibrary(libs, 0, func(name string, _ config.Library) string { return name })

	if fmt.Sprint(got) != "[a b]" {
		t.Errorf("got %v, want [a b]", got)
	}
}

func TestForEachLibrary_empty(t *testing.T) {
	got := parallel.ForEachLibrary(nil, 2, func(string, config.Library) int { return 1 })
	if len(got) != 0 {
		t.Errorf("expected no results, got %v", got)
	}
}
