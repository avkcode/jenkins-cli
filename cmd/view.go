package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var viewCmd = &cobra.Command{
	Use:     "view",
	Short:   "Manage Jenkins views",
	GroupID: GroupCore,
}

var viewListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all views",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		views, err := client.Client.GetAllViews(ctx)
		if err != nil {
			return err
		}

		out := getOutput()

		type viewRow struct {
			Name string `json:"name" yaml:"name"`
			URL  string `json:"url" yaml:"url"`
		}
		rows := make([]viewRow, len(views))
		for i, v := range views {
			rows[i] = viewRow{Name: v.Raw.Name, URL: v.Raw.URL}
		}

		switch format := viper.GetString("output"); format {
		case "json":
			return out.PrintJSON(rows)
		case "yaml":
			return out.PrintYAML(rows)
		default:
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tURL")
			for _, r := range rows {
				fmt.Fprintf(w, "%s\t%s\n", r.Name, r.URL)
			}
			return w.Flush()
		}
	},
}

func init() {
	viewCmd.AddCommand(viewListCmd)
	rootCmd.AddCommand(viewCmd)
}
