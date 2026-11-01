// Package colortest helps tests check which color a printed line came out in.
// Its escape codes are written out here and not taken from package color, so
// a test fails if the codes there change by mistake.
package colortest

import (
	"regexp"
	"strings"
	"testing"
)

// ANSI escape codes, as package color should be producing them.
const (
	Reset    = "\x1b[0m"
	Dim      = "\x1b[2m"
	Red      = "\x1b[31m"
	Green    = "\x1b[32m"
	Yellow   = "\x1b[33m"
	BoldCyan = "\x1b[1;36m"
)

// stamp is the dimmed time of day that starts a stamped line.
var stamp = regexp.MustCompile(`^\x1b\[2m\d{2}:\d{2}:\d{2}\x1b\[0m  `)

// Want says what color the lines containing a piece of text should have.
type Want struct {
	Contains string // text that appears in the line
	Code     string // the color code the line is wrapped in, or "" for a plain line
}

// Check runs one subtest per Want, so every line that has drifted is reported
// in one run.
func Check(t *testing.T, out string, wants ...Want) {
	t.Helper()
	for _, w := range wants {
		t.Run(w.Contains, func(t *testing.T) {
			if got := CodeOf(t, out, w.Contains); got != w.Code {
				t.Errorf("color %q, want %q", got, w.Code)
			}
		})
	}
}

// CodeOf returns the color code that the lines of out containing substr are
// wrapped in, or "" if they are plain. It fails the test if no line contains
// substr, if the lines containing it differ in color (so substr is not
// specific enough), or if a colored line is not closed with a reset.
func CodeOf(t *testing.T, out, substr string) string {
	t.Helper()
	var code string
	found := false
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, substr) {
			continue
		}
		lineCode := codeOfLine(t, line, substr)
		if found && lineCode != code {
			t.Fatalf("lines containing %q have different colors (%q and %q); use more specific text", substr, code, lineCode)
		}
		code, found = lineCode, true
	}
	if !found {
		t.Fatalf("no line contains %q in:\n%s", substr, out)
	}
	return code
}

func codeOfLine(t *testing.T, line, substr string) string {
	t.Helper()
	body := stamp.ReplaceAllString(line, "")
	if !strings.HasPrefix(body, "\x1b[") {
		return ""
	}
	if !strings.HasSuffix(body, Reset) {
		t.Fatalf("line containing %q starts with a color but does not end with a reset: %q", substr, line)
	}
	return body[:strings.IndexByte(body, 'm')+1]
}
