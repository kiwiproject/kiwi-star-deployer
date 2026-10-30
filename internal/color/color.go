// Package color colors the lines a release prints on the console so stages,
// results and problems stand out. Lines are colored from how they start, which
// keeps the print sites free of escape codes and lets the same text go
// uncolored to the session log.
package color

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// Mode says when to use color.
type Mode string

const (
	// Auto uses color when the output is a terminal, unless NO_COLOR is set or
	// the terminal is dumb.
	Auto Mode = "auto"
	// Always uses color whatever the output is, such as when it is piped to a
	// pager that understands it.
	Always Mode = "always"
	// Never does not use color.
	Never Mode = "never"
)

// ParseMode returns the Mode named by s.
func ParseMode(s string) (Mode, error) {
	switch m := Mode(s); m {
	case Auto, Always, Never:
		return m, nil
	}
	return "", fmt.Errorf("invalid --color %q: must be auto, always or never", s)
}

// Enabled says whether output to f should be colored in mode m. getenv reads
// the environment, so os.Getenv in real use.
func Enabled(m Mode, f *os.File, getenv func(string) string) bool {
	switch m {
	case Always:
		return true
	case Never:
		return false
	}
	// https://no-color.org: any non-empty value turns color off.
	if getenv("NO_COLOR") != "" || getenv("TERM") == "dumb" {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

const (
	reset      = "\x1b[0m"
	dim        = "\x1b[2m"
	red        = "\x1b[31m"
	green      = "\x1b[32m"
	yellow     = "\x1b[33m"
	boldCyan   = "\x1b[1;36m"
	stampWidth = len("15:04:05  ")
)

var (
	// stamp is the time of day that the timestamp package puts at the start of
	// a line. It is dimmed on its own so the rest of the line can have a color.
	stamp = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}  `)

	stage    = regexp.MustCompile(`^Stage \d+:`)
	failure  = regexp.MustCompile(`^(\[FAIL\]|FAILED|error:)`)
	success  = regexp.MustCompile(`^(\[PASS\]|done\b|CI passed|Released )`)
	skipped  = regexp.MustCompile(`^skip\b`)
	retry    = regexp.MustCompile(`; retrying in |will retry|continuing to wait|^could not |^CI: (poll failed|could not) `)
	polling  = regexp.MustCompile(`^CI: run \d+ status:`)
	dateLine = regexp.MustCompile(`^--- \d{4}-\d{2}-\d{2} ---$`)
)

// colorFor returns the color of a line, or "" for none. The line has its
// leading stamp and indentation removed. The first match wins, so a failure
// stays red even if it also mentions a retry.
func colorFor(line string) string {
	switch {
	case failure.MatchString(line):
		return red
	case success.MatchString(line):
		return green
	case skipped.MatchString(line), retry.MatchString(line):
		return yellow
	case polling.MatchString(line), dateLine.MatchString(line):
		return dim
	case stage.MatchString(line):
		return boldCyan
	}
	return ""
}

// Writer colors each line written to it and passes it on to the underlying
// writer, which should be the console. A line is held back until its newline
// arrives, since its color depends on how it starts, so call Flush at the end
// to write a final line that has no newline.
type Writer struct {
	mu      sync.Mutex
	w       io.Writer
	pending []byte
}

// NewWriter returns a Writer that writes colored lines to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// Write colors every complete line in p. It reports len(p) on success, not
// counting the escape codes that were added, as io.Writer requires.
func (c *Writer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	data := append(slices.Clone(c.pending), p...)
	var out strings.Builder
	for {
		end := bytes.IndexByte(data, '\n')
		if end < 0 {
			break
		}
		out.WriteString(colorLine(string(data[:end])))
		out.WriteByte('\n')
		data = data[end+1:]
	}
	if out.Len() > 0 {
		if _, err := io.WriteString(c.w, out.String()); err != nil {
			// Keep what was held before this call, so the caller can try p again.
			return 0, err
		}
	}
	c.pending = data
	return len(p), nil
}

// Flush writes any line that is still waiting for its newline.
func (c *Writer) Flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.pending) == 0 {
		return nil
	}
	if _, err := io.WriteString(c.w, colorLine(string(c.pending))); err != nil {
		return err
	}
	c.pending = nil
	return nil
}

// colorLine returns line, without its newline, with escape codes added.
func colorLine(line string) string {
	prefix := ""
	if stamp.MatchString(line) {
		prefix = dim + line[:stampWidth-2] + reset + "  "
		line = line[stampWidth:]
	}
	color := colorFor(strings.TrimLeft(line, " "))
	if color == "" || line == "" {
		return prefix + line
	}
	return prefix + color + line + reset
}
