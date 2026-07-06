package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var jobGetCmd = &cobra.Command{
	Use:   "get [job name]",
	Short: "Get job configuration XML",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		jobName := args[0]
		config, err := client.GetJobConfig(jobName)
		if err != nil {
			return fmt.Errorf("failed to get job config: %w", err)
		}

		fmt.Println(config)
		return nil
	},
}

func init() {
	jobCmd.AddCommand(jobGetCmd)
}
