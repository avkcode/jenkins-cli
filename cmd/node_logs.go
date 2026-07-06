package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var nodeLogsCmd = &cobra.Command{
	Use:   "logs [node name]",
	Short: "Stream agent connection logs",
	Long:  `View the connection logs for a specific node/agent. Useful for debugging Firecracker boot failures.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		nodeName := args[0]
		if nodeName == "(master)" || nodeName == "built-in" {
			nodeName = "(master)"
		}

		fmt.Printf("Fetching connection logs for node %s...\n", nodeName)

		// The connection log is available at /computer/{name}/logText/progressiveText
		endpoint := fmt.Sprintf("/computer/%s/logText/progressiveText?start=0", nodeName)

		// We reuse our raw API logic to get the text
		var logContent string
		_, err = client.Client.Requester.Get(ctx, endpoint, &logContent, nil)
		if err != nil {
			if strings.Contains(err.Error(), "404") {
				return fmt.Errorf("node %s has no connection log (it might be the built-in node)", nodeName)
			}
			return err
		}

		fmt.Println("--- Agent Connection Log ---")
		fmt.Print(logContent)
		return nil
	},
}

func init() {
	nodeCmd.AddCommand(nodeLogsCmd)
}
