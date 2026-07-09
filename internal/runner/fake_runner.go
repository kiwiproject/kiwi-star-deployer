package runner

import (
	"fmt"
	"sync"
)

// FakeRunner is a test double for Runner. It records every call made and
// returns pre-configured responses in order. Panics if called more times
// than responses have been configured, so tests fail loudly on mismatches.
//
// Responses are handed out strictly in enqueue order regardless of which
// caller asks, so FakeRunner is only safe for concurrent Run calls (e.g.
// plan.Build's or release.Execute's per-stage goroutines) when the test
// truly doesn't care which queued response lands on which concurrent call —
// as internal/release/release_test.go's parallel-stage tests do. If a test
// needs a specific response matched to a specific library under concurrency,
// dispatch on Options.WorkingDir/Args instead of enqueue order, the way
// internal/plan/plan_test.go's gitShowFromDiskRunner does.
type FakeRunner struct {
	mu        sync.Mutex
	Calls     []Options
	responses []*Result
	errors    []error
	next      int
}

func (f *FakeRunner) AddResponse(result *Result, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses = append(f.responses, result)
	f.errors = append(f.errors, err)
}

func (f *FakeRunner) Run(opts Options) (*Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, opts)
	if f.next >= len(f.responses) {
		panic(fmt.Sprintf("FakeRunner: unexpected call %d: %s %v", f.next+1, opts.Command, opts.Args))
	}
	result := f.responses[f.next]
	err := f.errors[f.next]
	f.next++
	return result, err
}

func (f *FakeRunner) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Calls)
}
