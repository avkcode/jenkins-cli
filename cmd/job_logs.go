package cmd

import (
	"fmt"
	"os"
	"strconv"
	"time"

	clientpkg "github.com/avkcode/jenkins-cli/pkg/client"
	"github.com/spf13/cobra"
)

var jobLogsRaw bool
var jobLogsFollow bool

var jobLogsCmd = &cobra.Command{
	Use:   "logs [job name] [build number]",
	Short: "Print or follow logs for a specific build",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		jobName := args[0]

		var buildNumber int64
		if len(args) > 1 {
			bn, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid build number: %s", args[1])
			}
			buildNumber = bn
		} else {
			// Get latest build
			job, err := client.Client.GetJob(ctx, jobName)
			if err != nil {
				return fmt.Errorf("failed to get job %s: %w", jobName, err)
			}
			lastBuild, err := job.GetLastBuild(ctx)
			if err != nil {
				return fmt.Errorf("failed to get last build for %s: %w", jobName, err)
			}
			buildNumber = lastBuild.GetBuildNumber()
			if jobLogsFollow {
				fmt.Fprintf(os.Stderr, "Streaming logs for latest build (#%d)\n", buildNumber)
			} else {
				fmt.Fprintf(os.Stderr, "Printing logs for latest build (#%d)\n", buildNumber)
			}
		}

		// Resolve the build to get the node name for NATS subject routing.
		build, err := client.Client.GetBuild(ctx, jobName, buildNumber)
		if err != nil {
			return fmt.Errorf("failed to get build #%d: %w", buildNumber, err)
		}
		if build.Raw.BuiltOn != "" {
			client.NodeName = build.Raw.BuiltOn
		}

		err = client.StreamLogsWithOptions(jobName, buildNumber, os.Stdout, clientpkg.LogStreamOptions{
			PollInterval: 2 * time.Second,
			Raw:          jobLogsRaw,
			Follow:       jobLogsFollow,
		})
		if err != nil {
			return fmt.Errorf("error streaming logs: %w", err)
		}

		return nil
	},
}

func init() {
	jobLogsCmd.Flags().BoolVar(&jobLogsRaw, "raw", false, "Stream raw Jenkins console output including hidden annotations")
	jobLogsCmd.Flags().BoolVarP(&jobLogsFollow, "follow", "f", false, "Continue streaming until Jenkins finishes the build")
	jobCmd.AddCommand(jobLogsCmd)
}
