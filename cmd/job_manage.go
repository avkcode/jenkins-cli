package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var jobDisableCmd = &cobra.Command{
	Use:   "disable [job name]",
	Short: "Disable a Jenkins job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		job, err := client.Client.GetJob(ctx, args[0])
		if err != nil {
			return err
		}

		_, err = job.Disable(ctx)
		if err == nil {
			fmt.Fprintf(os.Stderr, "Job %s disabled.\n", args[0])
		}
		return err
	},
}

var jobEnableCmd = &cobra.Command{
	Use:   "enable [job name]",
	Short: "Enable a Jenkins job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		job, err := client.Client.GetJob(ctx, args[0])
		if err != nil {
			return err
		}

		_, err = job.Enable(ctx)
		if err == nil {
			fmt.Fprintf(os.Stderr, "Job %s enabled.\n", args[0])
		}
		return err
	},
}

var jobRenameCmd = &cobra.Command{
	Use:   "rename [old-name] [new-name]",
	Short: "Rename a Jenkins job",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		job, err := client.Client.GetJob(ctx, args[0])
		if err != nil {
			return err
		}

		_, err = job.Rename(ctx, args[1])
		if err == nil {
			fmt.Fprintf(os.Stderr, "Job %s renamed to %s.\n", args[0], args[1])
		}
		return err
	},
}

var jobDeleteCmd = &cobra.Command{
	Use:   "delete [job name]",
	Short: "Delete a Jenkins job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if isDryRun() {
			dryRunMsg("Would delete job %s", args[0])
			return nil
		}
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "Deleting job %s...\n", args[0])
		_, err = client.Client.DeleteJob(ctx, args[0])
		if err == nil {
			fmt.Fprintln(os.Stderr, "Job deleted successfully.")
		}
		return err
	},
}

func init() {
	jobCmd.AddCommand(jobDisableCmd)
	jobCmd.AddCommand(jobEnableCmd)
	jobCmd.AddCommand(jobRenameCmd)
	jobCmd.AddCommand(jobDeleteCmd)
}
