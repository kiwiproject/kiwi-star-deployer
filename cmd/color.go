package cmd

import (
	"io"
	"os"

	"github.com/kiwiproject/kiwi-star-deployer/internal/color"
	"github.com/spf13/cobra"
)

// colorMode is the value of the --color flag. Only one command runs per
// process, so the commands that have the flag can share it.
var colorMode string

// addColorFlag gives cmd a --color flag. It is added per command rather than
// on the root so that only commands that color their output accept it.
func addColorFlag(cmd *cobra.Command) {
	cmd.Flags().StringVar(&colorMode, "color", "auto", "color the output: auto, always or never (auto uses color on a terminal unless NO_COLOR is set)")
}

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
		return f, func() {
			// Intentionally empty: f is written to directly, so no line is held back.
		}, nil
	}
	w := color.NewWriter(f)
	return w, func() { _ = w.Flush() }, nil
}
