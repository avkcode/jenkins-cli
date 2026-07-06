package cmd

import (
	"github.com/spf13/cobra"
)

var nodeCmd = &cobra.Command{
	Use:     "node",
	Short:   "Manage Jenkins nodes (agents)",
	Long:    `List and manage Jenkins nodes and their statuses.`,
	GroupID: GroupCore,
}

func init() {
	rootCmd.AddCommand(nodeCmd)
}
