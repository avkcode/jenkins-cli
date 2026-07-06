package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var queueCmd = &cobra.Command{
	Use:     "queue",
	Short:   "Manage Jenkins build queue",
	GroupID: GroupCore,
}

var queueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all items in the build queue",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		queue, err := client.Client.GetQueue(ctx)
		if err != nil {
			return err
		}

		out := getOutput()

		type queueRow struct {
			ID     int64  `json:"id" yaml:"id"`
			Job    string `json:"job" yaml:"job"`
			Reason string `json:"reason" yaml:"reason"`
			Stuck  bool   `json:"stuck" yaml:"stuck"`
		}
		rows := make([]queueRow, len(queue.Raw.Items))
		for i, item := range queue.Raw.Items {
			rows[i] = queueRow{ID: item.ID, Job: item.Task.Name, Reason: item.Why, Stuck: item.Stuck}
		}

		switch format := viper.GetString("output"); format {
		case "json":
			return out.PrintJSON(rows)
		case "yaml":
			return out.PrintYAML(rows)
		default:
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tJOB\tREASON\tSTUCK")
			for _, r := range rows {
				fmt.Fprintf(w, "%d\t%s\t%s\t%v\n", r.ID, r.Job, r.Reason, r.Stuck)
			}
			return w.Flush()
		}
	},
}

var queueCancelCmd = &cobra.Command{
	Use:   "cancel [id]",
	Short: "Cancel a queued item",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		id := args[0]
		fmt.Fprintf(os.Stderr, "Canceling queue item %s...\n", id)
		if err := client.CancelQueueItem(id); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Success.")
		return nil
	},
}

func init() {
	queueCmd.AddCommand(queueListCmd)
	queueCmd.AddCommand(queueCancelCmd)
	rootCmd.AddCommand(queueCmd)
}
