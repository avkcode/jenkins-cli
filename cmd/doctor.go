package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	doctorBundle       bool
	doctorBundleOutput string
)

var doctorCmd = &cobra.Command{
	Use:   "doctor [job-name] [build-number]",
	Short: "Explain a build failure from pipeline, logs, agents, and artifacts",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getTimeoutContext(cmd.Context())
		defer cancel()
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

		fmt.Fprintf(os.Stderr, "Analyzing %s #%d with pipeline model, stage timing, logs, agents, and artifacts...\n", jobName, buildNumber)
		if doctorBundle {
			result, err := client.CreateDoctorBundle(jobName, buildNumber, doctorBundleOutput)
			if err != nil {
				return err
			}
			switch viper.GetString("output") {
			case "json":
				return getOutput().PrintJSON(result)
			case "yaml":
				return getOutput().PrintYAML(result)
			default:
				fmt.Print(result.Report.FormatText())
				fmt.Fprintf(os.Stderr, "Doctor bundle written: %s\n", result.Path)
				fmt.Fprintf(os.Stderr, "Bundle files: %s\n", strings.Join(result.Files, ", "))
				return nil
			}
		}

		report, err := client.DoctorReport(jobName, buildNumber)
		if err != nil {
			return err
		}
		switch viper.GetString("output") {
		case "json":
			return getOutput().PrintJSON(report)
		case "yaml":
			return getOutput().PrintYAML(report)
		default:
			fmt.Print(report.FormatText())
			return nil
		}
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorBundle, "bundle", false, "Create a portable doctor evidence zip")
	doctorCmd.Flags().StringVar(&doctorBundleOutput, "bundle-output", "", "Path for --bundle output zip")
	rootCmd.AddCommand(doctorCmd)
}
