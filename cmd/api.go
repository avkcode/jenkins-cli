package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var apiCmd = &cobra.Command{
	Use:   "api [GET|POST] [endpoint]",
	Short: "Call any Jenkins REST API endpoint",
	Long:  `Perform a raw authenticated request to any Jenkins API endpoint and see the raw response.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		method := strings.ToUpper(args[0])
		endpoint := args[1]

		fmt.Fprintf(os.Stderr, "Calling %s %s...\n", method, endpoint)

		var respBody io.ReadCloser
		if method == "GET" {
			var raw string
			_, err := client.Client.Requester.Get(ctx, endpoint, &raw, nil)
			if err != nil {
				return err
			}
			fmt.Println(raw)
		} else if method == "POST" {
			resp, err := client.Client.Requester.Post(ctx, endpoint, nil, nil, nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			fmt.Println(string(body))
		} else {
			return fmt.Errorf("unsupported method: %s", method)
		}

		_ = respBody
		return nil
	},
}

func init() {
	rootCmd.AddCommand(apiCmd)
}
