package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/state"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Display the state of the current or most recent release run",
	RunE:  runStatus,
}

func runStatus(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	s, err := state.Load(cfg.Settings.StatePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stdout, "No release run has been started.")
			return nil
		}
		return fmt.Errorf("loading state file: %w", err)
	}

	state.Print(os.Stdout, s)
	return nil
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
