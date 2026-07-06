package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:     "edit",
	Short:   "Edit configurations interactively",
	GroupID: GroupAdmin,
}

var editJobCmd = &cobra.Command{
	Use:   "job [job-name]",
	Short: "Edit job configuration in your $EDITOR",
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
			return err
		}

		newConfig, err := openInEditor(config, "job-"+jobName+"-*.xml")
		if err != nil {
			return err
		}

		if config == newConfig {
			fmt.Fprintln(os.Stderr, "No changes detected.")
			return nil
		}

		fmt.Fprintf(os.Stderr, "Applying changes to %s...\n", jobName)
		return client.UpdateJobConfig(jobName, newConfig)
	},
}

var editSystemCmd = &cobra.Command{
	Use:   "system",
	Short: "Edit global system config.xml in your $EDITOR",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		config, err := client.GetSystemConfig()
		if err != nil {
			return err
		}

		newConfig, err := openInEditor(config, "system-config-*.xml")
		if err != nil {
			return err
		}

		if config == newConfig {
			fmt.Fprintln(os.Stderr, "No changes detected.")
			return nil
		}

		fmt.Fprintln(os.Stderr, "Applying global system configuration...")
		return client.UpdateSystemConfig(newConfig)
	},
}

func openInEditor(content string, pattern string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim" // fallback
	}

	tmpFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		return "", err
	}
	tmpFile.Close()

	cmd := exec.Command("sh", "-c", editor+" "+tmpFile.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor failed: %w", err)
	}

	newContent, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return "", err
	}

	return string(newContent), nil
}

func init() {
	editCmd.AddCommand(editJobCmd)
	editCmd.AddCommand(editSystemCmd)
	rootCmd.AddCommand(editCmd)
}
