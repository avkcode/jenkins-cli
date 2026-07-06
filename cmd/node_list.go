package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var nodeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all Jenkins nodes",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		nodes, err := client.Client.GetAllNodes(ctx)
		if err != nil {
			return fmt.Errorf("failed to get nodes: %w", err)
		}

		out := getOutput()

		type nodeRow struct {
			Name      string `json:"name" yaml:"name"`
			Status    string `json:"status" yaml:"status"`
			Idle      string `json:"idle" yaml:"idle"`
			Executors int    `json:"executors" yaml:"executors"`
		}
		rows := make([]nodeRow, len(nodes))
		for i, node := range nodes {
			status := "Online"
			if node.Raw.Offline {
				status = "Offline"
			}
			idle := "Yes"
			if !node.Raw.Idle {
				idle = "No"
			}
			name := node.GetName()
			if name == "" {
				name = "(master)"
			}
			rows[i] = nodeRow{Name: name, Status: status, Idle: idle, Executors: len(node.Raw.Executors)}
		}

		switch format := viper.GetString("output"); format {
		case "json":
			return out.PrintJSON(rows)
		case "yaml":
			return out.PrintYAML(rows)
		default:
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NAME\tSTATUS\tIDLE\tEXECUTORS")
			for _, r := range rows {
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", r.Name, r.Status, r.Idle, r.Executors)
			}
			return w.Flush()
		}
	},
}

func init() {
	nodeCmd.AddCommand(nodeListCmd)
}
