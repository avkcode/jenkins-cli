package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var maintCmd = &cobra.Command{
	Use:     "maint",
	Short:   "Jenkins maintenance operations",
	GroupID: GroupAdmin,
}

var maintQuietDownCmd = &cobra.Command{
	Use:   "quiet-down",
	Short: "Put Jenkins in quiet-down mode (prepare for restart)",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		fmt.Fprintln(os.Stderr, "Putting Jenkins into Quiet Down mode...")
		_, err = client.Client.Requester.Post(ctx, "/quietDown", nil, nil, nil)
		return err
	},
}

var maintCancelQuietDownCmd = &cobra.Command{
	Use:   "cancel",
	Short: "Cancel quiet-down mode",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		fmt.Fprintln(os.Stderr, "Canceling Quiet Down mode...")
		_, err = client.Client.Requester.Post(ctx, "/cancelQuietDown", nil, nil, nil)
		return err
	},
}

func init() {
	maintCmd.AddCommand(maintQuietDownCmd)
	maintCmd.AddCommand(maintCancelQuietDownCmd)
	rootCmd.AddCommand(maintCmd)
}
