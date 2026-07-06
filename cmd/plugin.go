package cmd

import (
	"github.com/spf13/cobra"
)

var pluginCmd = &cobra.Command{
	Use:     "plugin",
	Short:   "Manage Jenkins plugins",
	Long:    `List and manage Jenkins plugins installed on the server.`,
	GroupID: GroupAdmin,
}

func init() {
	rootCmd.AddCommand(pluginCmd)
}
