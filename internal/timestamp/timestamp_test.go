package timestamp_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/timestamp"
)

// clock returns a function that reports the given times in order, repeating
// the last one when it runs out.
func clock(times ...time.Time) func() time.Time {
	i := 0
	return func() time.Time {
		t := times[min(i, len(times)-1)]
		i++
		return t
	}
}

func at(day, clock string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", day+" "+clock, time.UTC)
	if err != nil {
		panic(err)
	}
	return t
}

func TestWrite_StampsEachLine(t *testing.T) {
	var out bytes.Buffer
	w := timestamp.NewWithClock(&out, clock(at("2026-10-22", "21:40:21"), at("2026-10-22", "21:40:22")))

	fmt.Fprintln(w, "first")
	fmt.Fprintln(w, "second")

	want := "21:40:21  first\n21:40:22  second\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestNew_StampsWithTheCurrentTime(t *testing.T) {
	var out bytes.Buffer
	w := timestamp.New(&out)

	before := time.Now().Format("15:04:05")
	fmt.Fprintln(w, "hello")
	after := time.Now().Format("15:04:05")

	// The clock may tick between reading the time and writing, so the stamp
	// can be either of the two readings.
	got := out.String()
	if got != before+"  hello\n" && got != after+"  hello\n" {
		t.Errorf("got %q, want a stamp of %s or %s", got, before, after)
	}
}

func TestWrite_ReportsLengthOfInput(t *testing.T) {
	var out bytes.Buffer
	w := timestamp.NewWithClock(&out, clock(at("2026-10-22", "21:40:21")))

	n, err := w.Write([]byte("hello\n"))
	if err != nil || n != 6 {
		t.Errorf("Write = (%d, %v), want (6, nil)", n, err)
	}
}

func TestWrite_SeveralLinesInOneWrite(t *testing.T) {
	var out bytes.Buffer
	w := timestamp.NewWithClock(&out, clock(at("2026-10-22", "21:40:21"), at("2026-10-22", "21:40:22")))

	_, _ = w.Write([]byte("a\nb\n"))

	want := "21:40:21  a\n21:40:22  b\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestWrite_LineInPiecesIsStampedOnce(t *testing.T) {
	var out bytes.Buffer
	w := timestamp.NewWithClock(&out, clock(at("2026-10-22", "21:40:21"), at("2026-10-22", "21:40:59")))

	_, _ = w.Write([]byte("par"))
	_, _ = w.Write([]byte("tial"))
	_, _ = w.Write([]byte(" line\n"))
	_, _ = w.Write([]byte("next\n"))

	want := "21:40:21  partial line\n21:40:59  next\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestWrite_BlankLinesAreNotStamped(t *testing.T) {
	var out bytes.Buffer
	w := timestamp.NewWithClock(&out, clock(at("2026-10-22", "21:40:21"), at("2026-10-22", "21:40:22")))

	_, _ = w.Write([]byte("a\n\n\nb\n"))

	want := "21:40:21  a\n\n\n21:40:22  b\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestWrite_NoNewlineAtEndStaysOpen(t *testing.T) {
	var out bytes.Buffer
	w := timestamp.NewWithClock(&out, clock(at("2026-10-22", "21:40:21")))

	_, _ = w.Write([]byte("no newline"))

	if want := "21:40:21  no newline"; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestWrite_NoDateLineOnTheFirstDay(t *testing.T) {
	var out bytes.Buffer
	w := timestamp.NewWithClock(&out, clock(at("2026-10-22", "09:00:00"), at("2026-10-22", "23:59:59")))

	fmt.Fprintln(w, "a")
	fmt.Fprintln(w, "b")

	if strings.Contains(out.String(), "---") {
		t.Errorf("unexpected date line in %q", out.String())
	}
}

func TestWrite_DateLineAfterMidnight(t *testing.T) {
	var out bytes.Buffer
	w := timestamp.NewWithClock(&out, clock(
		at("2026-10-22", "23:59:58"),
		at("2026-10-23", "00:00:03"),
		at("2026-10-23", "00:00:04"),
	))

	fmt.Fprintln(w, "before")
	fmt.Fprintln(w, "after")
	fmt.Fprintln(w, "later")

	want := "23:59:58  before\n--- 2026-10-23 ---\n00:00:03  after\n00:00:04  later\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestWrite_ReturnsUnderlyingError(t *testing.T) {
	w := timestamp.NewWithClock(failingWriter{}, clock(at("2026-10-22", "21:40:21")))

	if _, err := w.Write([]byte("x\n")); err == nil {
		t.Error("expected an error from the underlying writer")
	}
}

func TestWrite_ConcurrentLinesStayWhole(t *testing.T) {
	var out bytes.Buffer
	fixed := at("2026-10-22", "21:40:21")
	w := timestamp.NewWithClock(&out, func() time.Time { return fixed })

	const writers, lines = 8, 50
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range lines {
				fmt.Fprintf(w, "writer %d line %d\n", i, j)
			}
		}()
	}
	wg.Wait()

	got := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(got) != writers*lines {
		t.Fatalf("got %d lines, want %d", len(got), writers*lines)
	}
	for _, line := range got {
		// "HH:MM:SS  writer N line M": a stamp followed by one whole line.
		if len(line) < 10 || line[8:10] != "  " || !strings.HasPrefix(line[10:], "writer ") {
			t.Fatalf("mixed up or unstamped line: %q", line)
		}
	}
}

type flakyWriter struct {
	out   bytes.Buffer
	fails bool
}

func (f *flakyWriter) Write(p []byte) (int, error) {
	if f.fails {
		return 0, errors.New("disk full")
	}
	return f.out.Write(p)
}

func TestWrite_FailedWriteDoesNotUseUpTheStamp(t *testing.T) {
	fw := &flakyWriter{fails: true}
	w := timestamp.NewWithClock(fw, clock(
		at("2026-10-22", "23:59:58"),
		at("2026-10-23", "00:00:01"),
		at("2026-10-23", "00:00:02"),
	))

	fmt.Fprintln(w, "first")
	fw.fails = false
	fmt.Fprintln(w, "first")
	fmt.Fprintln(w, "second")

	// The failed write did not count, so it is the successful one that is
	// treated as the first line of the run: no date line is due yet.
	want := "00:00:01  first\n00:00:02  second\n"
	if fw.out.String() != want {
		t.Errorf("got %q, want %q", fw.out.String(), want)
	}
}
