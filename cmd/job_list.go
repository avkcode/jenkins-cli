package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var jobListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all Jenkins jobs",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		jobs, err := client.Client.GetAllJobs(ctx)
		if err != nil {
			return fmt.Errorf("failed to get jobs: %w", err)
		}

		out := getOutput()

		type jobRow struct {
			Name  string `json:"name" yaml:"name"`
			URL   string `json:"url" yaml:"url"`
			Color string `json:"color" yaml:"color"`
		}
		rows := make([]jobRow, len(jobs))
		for i, job := range jobs {
			rows[i] = jobRow{Name: job.Raw.Name, URL: job.Raw.URL, Color: job.Raw.Color}
		}

		switch format := viper.GetString("output"); format {
		case "json":
			return out.PrintJSON(rows)
		case "yaml":
			return out.PrintYAML(rows)
		default:
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NAME\tURL\tCOLOR")
			for _, r := range rows {
				fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.URL, r.Color)
			}
			return w.Flush()
		}
	},
}

func init() {
	jobCmd.AddCommand(jobListCmd)
}
