package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var preflightCmd = &cobra.Command{
	Use:   "preflight",
	Short: "Verify that all required tools are installed and configured",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("not yet implemented")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(preflightCmd)
}
