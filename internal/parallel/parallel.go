// Package parallel runs a function for each configured library at the same
// time, with a bound on how many run at once.
package parallel

import (
	"sort"
	"sync"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
)

// ForEachLibrary calls fn for every library in libs concurrently, with at most
// limit calls running at once (at least one), and returns the results sorted
// by library name.
func ForEachLibrary[T any](libs map[string]config.Library, limit int, fn func(name string, lib config.Library) T) []T {
	names := make([]string, 0, len(libs))
	for name := range libs {
		names = append(names, name)
	}
	sort.Strings(names)

	if limit < 1 {
		limit = 1
	}
	results := make([]T, len(names))
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = fn(name, libs[name])
		}(i, name)
	}
	wg.Wait()
	return results
}
