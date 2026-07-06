package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Verify Jenkins connectivity",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getTimeoutContext(cmd.Context())
		defer cancel()

		allOk := true
		fmt.Fprintf(os.Stderr, "jc health check - %s\n\n", time.Now().Format(time.RFC3339))

		// Jenkins
		fmt.Fprintf(os.Stderr, "Jenkins:  ")
		jc, err := getClient(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL (%v)\n", err)
			allOk = false
		} else {
			if _, err := jc.GetInfo(); err != nil {
				fmt.Fprintf(os.Stderr, "FAIL (%v)\n", err)
				allOk = false
			} else {
				fmt.Fprintf(os.Stderr, "OK (%s)\n", jc.Client.Server)
			}
		}

		// Config
		fmt.Fprintf(os.Stderr, "\nConfig:   %s", viper.ConfigFileUsed())
		if viper.ConfigFileUsed() == "" {
			fmt.Fprintf(os.Stderr, "not found (using flags/env only)")
		}
		fmt.Fprintln(os.Stderr)

		context := viper.GetString("current-context")
		if context != "" {
			fmt.Fprintf(os.Stderr, "Context:  %s\n", context)
		}

		if !allOk {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(healthCmd)
}
