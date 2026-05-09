package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Print release stages, ordering, and resolved versions",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("not yet implemented")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(planCmd)
}
