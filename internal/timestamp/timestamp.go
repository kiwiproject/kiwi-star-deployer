// Package timestamp adds the time of day to the start of every line written
// through it, so a long release can be followed after the fact.
package timestamp

import (
	"bytes"
	"io"
	"sync"
	"time"
)

const (
	timeLayout = "15:04:05"
	dayLayout  = "2006-01-02"
)

// Writer prefixes each line written to it with the time of day. A line gets
// its stamp when its first byte arrives, so a line written in several pieces
// is stamped once, and blank lines are left alone. Because only the time of
// day is shown, a line saying the date follows the first line written after
// midnight.
//
// Writes are serialized, so lines from concurrent writers are not mixed up as
// long as each is written in a single call, as fmt.Fprintf does.
type Writer struct {
	mu          sync.Mutex
	w           io.Writer
	now         func() time.Time
	atLineStart bool
	lastDay     string
}

// New returns a Writer that stamps lines with the current time and writes
// them to w.
func New(w io.Writer) *Writer {
	return NewWithClock(w, time.Now)
}

// NewWithClock is like New but reads the time from now.
func NewWithClock(w io.Writer, now func() time.Time) *Writer {
	return &Writer{w: w, now: now, atLineStart: true}
}

// Write writes p to the underlying writer with the stamps added. It reports
// len(p) on success, not counting the added bytes, as io.Writer requires.
func (t *Writer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	atLineStart, lastDay := t.atLineStart, t.lastDay
	var buf bytes.Buffer
	rest := p
	for len(rest) > 0 {
		if t.atLineStart && rest[0] != '\n' {
			t.writeStamp(&buf)
			t.atLineStart = false
		}
		end := bytes.IndexByte(rest, '\n')
		if end < 0 {
			buf.Write(rest)
			break
		}
		buf.Write(rest[:end+1])
		rest = rest[end+1:]
		t.atLineStart = true
	}

	if _, err := t.w.Write(buf.Bytes()); err != nil {
		// Forget the stamp and date line that were not written, so a retry
		// of p is stamped the same way.
		t.atLineStart, t.lastDay = atLineStart, lastDay
		return 0, err
	}
	return len(p), nil
}

func (t *Writer) writeStamp(buf *bytes.Buffer) {
	now := t.now()
	if day := now.Format(dayLayout); day != t.lastDay {
		if t.lastDay != "" {
			buf.WriteString("--- " + day + " ---\n")
		}
		t.lastDay = day
	}
	buf.WriteString(now.Format(timeLayout) + "  ")
}
