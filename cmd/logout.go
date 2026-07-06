package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var logoutCmd = &cobra.Command{
	Use:     "logout",
	Short:   "Clear saved credentials and config",
	GroupID: GroupConfig,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}

		configPath := filepath.Join(home, ".jenkins-cli.yaml")
		if cfgFile != "" {
			configPath = cfgFile
		}

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			fmt.Println("No saved configuration found.")
			return nil
		}

		if err := os.Remove(configPath); err != nil {
			return fmt.Errorf("failed to remove config: %w", err)
		}

		viper.Reset()
		fmt.Fprintf(os.Stderr, "Config removed: %s\n", configPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}
