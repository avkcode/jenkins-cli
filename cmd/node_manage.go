package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var nodeDeleteCmd = &cobra.Command{
	Use:   "delete [node name]",
	Short: "Delete a Jenkins node",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if isDryRun() {
			dryRunMsg("Would delete node %s", args[0])
			return nil
		}
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		nodeName := args[0]
		fmt.Fprintf(os.Stderr, "Deleting node %s...\n", nodeName)
		_, err = client.Client.DeleteNode(ctx, nodeName)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Node deleted successfully.")
		return nil
	},
}

var nodeCreateCmd = &cobra.Command{
	Use:   "create [node name]",
	Short: "Create a new permanent agent node",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName := args[0]

		if isDryRun() {
			dryRunMsg("Would create node %s", nodeName)
			return nil
		}

		ctx, cancel := getTimeoutContext(cmd.Context())
		defer cancel()

		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		// Idempotency: check if node already exists
		nodes, err := client.Client.GetAllNodes(ctx)
		if err == nil {
			for _, n := range nodes {
				if n.GetName() == nodeName {
					fmt.Fprintf(os.Stderr, "Node %s already exists.\n", nodeName)
					return nil
				}
			}
		}

		fmt.Fprintf(os.Stderr, "Creating node %s...\n", nodeName)
		_, err = client.Client.CreateNode(ctx, nodeName, 2, "Created via jc CLI", "/home/jenkins", "jc-agent")
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Node %s created successfully.\n", nodeName)
		return nil
	},
}

func init() {
	nodeCmd.AddCommand(nodeCreateCmd)
	nodeCmd.AddCommand(nodeDeleteCmd)
}
