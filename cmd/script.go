package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var scriptCmd = &cobra.Command{
	Use:   "script [groovy-script-or-file]",
	Short: "Execute a Groovy script in the Jenkins Script Console",
	Long:  `Run a raw Groovy script or a .groovy file against the Jenkins master and see the text output.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		script := args[0]
		// Check if it's a file
		if _, err := os.Stat(script); err == nil {
			content, err := os.ReadFile(script)
			if err != nil {
				return fmt.Errorf("failed to read script file: %w", err)
			}
			script = string(content)
		}

		fmt.Fprintln(os.Stderr, "Executing script...")
		output, err := client.ExecuteGroovy(script)
		if err != nil {
			return fmt.Errorf("execution failed: %w", err)
		}

		fmt.Fprintln(os.Stderr, "--- Output ---")
		fmt.Print(output)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(scriptCmd)
}
