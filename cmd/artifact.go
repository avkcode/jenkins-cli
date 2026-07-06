package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

var artifactCmd = &cobra.Command{
	Use:     "artifact",
	Short:   "Manage build artifacts",
	GroupID: GroupCore,
}

var artifactDownloadCmd = &cobra.Command{
	Use:   "download [job] [build] [file] [output-path]",
	Short: "Download a build artifact",
	Args:  cobra.ExactArgs(4),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		jobName := args[0]
		buildNumber, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid build number: %w", err)
		}
		fileName := args[2]
		outputPath := args[3]

		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()

		fmt.Fprintf(os.Stderr, "Downloading artifact %s from %s #%d...\n", fileName, jobName, buildNumber)
		err = client.DownloadArtifact(jobName, buildNumber, fileName, f)
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "Artifact saved to %s\n", outputPath)
		return nil
	},
}

var artifactListCmd = &cobra.Command{
	Use:   "list [job] [build]",
	Short: "List artifacts for a build",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		jobName := args[0]
		buildNumber, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid build number: %w", err)
		}

		artifacts, err := client.ListArtifacts(jobName, buildNumber)
		if err != nil {
			return err
		}

		if len(artifacts) == 0 {
			fmt.Fprintln(os.Stderr, "No artifacts found.")
			return nil
		}

		for _, a := range artifacts {
			fmt.Printf("%s\t(Path: %s)\n", a.FileName, a.Path)
		}
		return nil
	},
}

var artifactDownloadAllCmd = &cobra.Command{
	Use:   "download-all [job] [build] [output-zip]",
	Short: "Download all artifacts as a zip file",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		jobName := args[0]
		buildNumber, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid build number: %w", err)
		}
		outputPath := args[2]

		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()

		fmt.Fprintf(os.Stderr, "Downloading all artifacts from %s #%d as zip...\n", jobName, buildNumber)
		err = client.DownloadAllArtifacts(jobName, buildNumber, f)
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "Archive saved to %s\n", outputPath)
		return nil
	},
}

func init() {
	artifactCmd.AddCommand(artifactListCmd)
	artifactCmd.AddCommand(artifactDownloadCmd)
	artifactCmd.AddCommand(artifactDownloadAllCmd)
	rootCmd.AddCommand(artifactCmd)
}
