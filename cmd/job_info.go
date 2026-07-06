package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var buildInfoCmd = &cobra.Command{
	Use:   "info [job-name] [build-number]",
	Short: "Get detailed information about a specific build",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		jobName := args[0]
		var buildNumber int64
		if len(args) > 1 {
			buildNumber, _ = strconv.ParseInt(args[1], 10, 64)
		} else {
			job, err := client.Client.GetJob(ctx, jobName)
			if err != nil {
				return fmt.Errorf("get job %q: %w", jobName, err)
			}
			lb, err := job.GetLastBuild(ctx)
			if err != nil {
				return fmt.Errorf("no builds for job %q: %w", jobName, err)
			}
			if lb == nil {
				return fmt.Errorf("no builds for job %q", jobName)
			}
			buildNumber = lb.GetBuildNumber()
		}

		output, err := client.GetBuildInfo(jobName, buildNumber)
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

func init() {
	jobCmd.AddCommand(buildInfoCmd)
}
