package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

var jobStopCmd = &cobra.Command{
	Use:   "stop [job name] [build number]",
	Short: "Stop a running build",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		jobName := args[0]
		buildNumberStr := args[1]

		buildNumber, err := strconv.ParseInt(buildNumberStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid build number: %s", buildNumberStr)
		}

		ok, err := client.StopBuild(jobName, buildNumber)
		if err != nil {
			return fmt.Errorf("failed to stop build: %w", err)
		}

		if ok {
			fmt.Fprintf(os.Stderr, "Successfully stopped build #%d for job %s\n", buildNumber, jobName)
		} else {
			fmt.Fprintf(os.Stderr, "Could not stop build #%d for job %s (it may have already completed)\n", buildNumber, jobName)
		}

		return nil
	},
}

func init() {
	jobCmd.AddCommand(jobStopCmd)
}
