package cmd

import (
	"io"
	"os"

	"github.com/kiwiproject/kiwi-star-deployer/internal/color"
)

// consoleOutput returns where a command should print its progress: f itself,
// or a writer that colors the lines on their way to f when mode and the
// environment call for color. The returned flush writes any last unfinished
// line and must be called when the command is done.
func consoleOutput(f *os.File, mode string) (out io.Writer, flush func(), err error) {
	m, err := color.ParseMode(mode)
	if err != nil {
		return nil, nil, err
	}
	if !color.Enabled(m, f, os.Getenv) {
		return f, func() {}, nil
	}
	w := color.NewWriter(f)
	return w, func() { _ = w.Flush() }, nil
}
