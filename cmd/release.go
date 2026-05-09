package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Release kiwiproject libraries in dependency order",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("not yet implemented")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(releaseCmd)
}
