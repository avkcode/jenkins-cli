package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

var jobCmd = &cobra.Command{
	Use:     "job",
	Short:   "Manage Jenkins jobs",
	Long:    `List, build, and manage Jenkins jobs.`,
	GroupID: GroupCore,
}

func init() {
	rootCmd.AddCommand(jobCmd)
	jobCmd.AddCommand(jobHistoryCmd)
	jobCmd.AddCommand(jobWorkspaceCmd)
	jobCmd.AddCommand(jobStagesCmd)
	jobCmd.AddCommand(jobTestsCmd)
	jobCmd.AddCommand(jobSnippetCmd)
	jobCmd.AddCommand(jobReplayCmd)
	jobCmd.AddCommand(jobScanCmd)
	jobCmd.AddCommand(jobScanLogsCmd)
	jobCmd.AddCommand(jobBranchesCmd)
	jobCmd.AddCommand(jobScriptCmd)
	jobCmd.AddCommand(jobRestartCmd)
	jobCmd.AddCommand(jobCleanCmd)
}

var jobScriptCmd = &cobra.Command{
	Use:   "script [job-name] [build-number]",
	Short: "View the exact Jenkinsfile executed for a build",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		buildNumber, _ := strconv.ParseInt(args[1], 10, 64)
		out, err := client.GetExecutedScript(args[0], buildNumber)
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	},
}

var jobRestartCmd = &cobra.Command{
	Use:   "restart [job-name] [build-number] [stage-name]",
	Short: "Restart a Declarative Pipeline from a specific stage",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		buildNumber, _ := strconv.ParseInt(args[1], 10, 64)
		return client.RestartFromStage(args[0], buildNumber, args[2])
	},
}

var jobCleanCmd = &cobra.Command{
	Use:   "clean [job-name]",
	Short: "Wipe out the entire job workspace (including @libs cache)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Cleaning workspace for %s...\n", args[0])
		return client.CleanWorkspace(args[0])
	},
}

var jobScanLogsCmd = &cobra.Command{
	Use:   "scan-logs [job-name]",
	Short: "View indexing/scan logs for a multibranch project or folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		output, err := client.GetScanLogs(args[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "--- Scan Logs ---")
		fmt.Print(output)
		return nil
	},
}

var jobReplayCmd = &cobra.Command{
	Use:   "replay [job-name] [build-number] [script-file]",
	Short: "Replay a build with a modified script",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		buildNumber, _ := strconv.ParseInt(args[1], 10, 64)
		scriptData, err := os.ReadFile(args[2])
		if err != nil {
			return err
		}
		output, err := client.ReplayBuild(args[0], buildNumber, string(scriptData))
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

var jobScanCmd = &cobra.Command{
	Use:   "scan [job-name]",
	Short: "Trigger a scan/indexing for a multibranch project or folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		return client.ScanJob(args[0])
	},
}

var jobBranchesCmd = &cobra.Command{
	Use:   "branches [job-name]",
	Short: "List branches of a multibranch pipeline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		output, err := client.ListJobBranches(args[0])
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

var jobSnippetCmd = &cobra.Command{
	Use:   "snippet [step-name]",
	Short: "Generate/Help for a Pipeline DSL step",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		output, err := client.GenerateSnippet(args[0])
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

var jobStagesCmd = &cobra.Command{
	Use:   "stages [job-name] [build-number]",
	Short: "Show pipeline stages for a build",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		jobName := args[0]
		var buildNumber int64
		if len(args) > 1 {
			buildNumber, err = strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid build number: %w", err)
			}
		} else {
			job, err := client.Client.GetJob(ctx, jobName)
			if err != nil {
				return err
			}
			lb, err := job.GetLastBuild(ctx)
			if err != nil {
				return err
			}
			buildNumber = lb.GetBuildNumber()
		}

		output, err := client.GetPipelineStages(jobName, buildNumber)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "--- Stages for %s #%d ---\n", jobName, buildNumber)
		fmt.Print(output)
		return nil
	},
}

var jobTestsCmd = &cobra.Command{
	Use:   "tests [job-name] [build-number]",
	Short: "Show test results for a build",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		jobName := args[0]
		var buildNumber int64
		if len(args) > 1 {
			buildNumber, err = strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid build number: %w", err)
			}
		} else {
			job, err := client.Client.GetJob(ctx, jobName)
			if err != nil {
				return err
			}
			lb, err := job.GetLastBuild(ctx)
			if err != nil {
				return err
			}
			buildNumber = lb.GetBuildNumber()
		}

		output, err := client.GetTestResults(jobName, buildNumber)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "--- Test Summary for %s #%d ---\n", jobName, buildNumber)
		fmt.Print(output)
		return nil
	},
}

var jobHistoryCmd = &cobra.Command{
	Use:   "history [job-name]",
	Short: "Show recent build history",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		output, err := client.GetBuildHistory(args[0])
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

var jobWorkspaceCmd = &cobra.Command{
	Use:   "workspace [job-name]",
	Short: "List files in the current job workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		output, err := client.ListWorkspace(args[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "--- Workspace Contents ---")
		fmt.Print(output)
		return nil
	},
}
