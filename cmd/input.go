package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	inputAbort bool
	inputId    string
)

var inputCmd = &cobra.Command{
	Use:   "input",
	Short: "Manage pending Pipeline input steps",
}

var inputListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all builds currently waiting for input",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		output, err := client.ListPendingInputs()
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

var inputProceedCmd = &cobra.Command{
	Use:   "proceed [job-name] [build-number]",
	Short: "Approve/Proceed a pending input",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		buildNumber, _ := strconv.ParseInt(args[1], 10, 64)
		return client.SignalInput(args[0], buildNumber, inputId, false)
	},
}

var inputAbortCmd = &cobra.Command{
	Use:   "abort [job-name] [build-number]",
	Short: "Abort/Reject a pending input",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		buildNumber, _ := strconv.ParseInt(args[1], 10, 64)
		return client.SignalInput(args[0], buildNumber, inputId, true)
	},
}

func init() {
	inputProceedCmd.Flags().StringVar(&inputId, "id", "", "Specific Input ID to target")
	inputAbortCmd.Flags().StringVar(&inputId, "id", "", "Specific Input ID to target")

	inputCmd.AddCommand(inputListCmd)
	inputCmd.AddCommand(inputProceedCmd)
	inputCmd.AddCommand(inputAbortCmd)
	rootCmd.AddCommand(inputCmd)
}
