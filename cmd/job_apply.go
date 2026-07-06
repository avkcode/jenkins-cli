package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	applyFile     string
	applyShowDiff bool
)

var jobApplyCmd = &cobra.Command{
	Use:   "apply [job name]",
	Short: "Create or update a job from XML configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		jobName := args[0]

		data, err := os.ReadFile(applyFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		desired := string(data)

		// Determine the action by comparing against the live config so apply is
		// idempotent and can preview changes.
		action := "create"
		var diff string
		if existing, getErr := client.GetJobConfig(jobName); getErr == nil {
			changed, d := renderDiff(existing, desired)
			if !changed {
				action = "unchanged"
			} else {
				action = "update"
				diff = d
			}
		}

		if applyShowDiff && diff != "" {
			fmt.Fprintf(os.Stderr, "diff for job %s:\n%s", jobName, diff)
		}

		if action == "unchanged" {
			fmt.Fprintf(os.Stderr, "job %s is already up to date\n", jobName)
			return nil
		}

		if isDryRun() {
			dryRunMsg("would %s job %s", action, jobName)
			if diff != "" && !applyShowDiff {
				fmt.Fprintf(os.Stderr, "%s", diff)
			}
			return nil
		}

		if err := client.CreateOrUpdateJob(jobName, desired); err != nil {
			return fmt.Errorf("failed to apply job: %w", err)
		}
		audit("job.apply", fmt.Sprintf("%s (%s)", jobName, action))

		fmt.Fprintf(os.Stderr, "Successfully applied configuration to job %s (%s)\n", jobName, action)
		return nil
	},
}

func init() {
	jobApplyCmd.Flags().StringVarP(&applyFile, "file", "f", "", "XML configuration file")
	jobApplyCmd.Flags().BoolVar(&applyShowDiff, "diff", false, "Show a diff of the changes before applying")
	jobApplyCmd.MarkFlagRequired("file")
	jobCmd.AddCommand(jobApplyCmd)
}
