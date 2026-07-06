package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var approvalCmd = &cobra.Command{
	Use:   "approval",
	Short: "Manage In-process Script Approvals",
	Long:  `List and approve pending Groovy scripts and method signatures blocked by the Script Security plugin.`,
}

var approvalListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending script and signature approvals",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		out, err := client.ListPendingApprovals()
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	},
}

var approvalApproveCmd = &cobra.Command{
	Use:   "approve [hash-or-signature]",
	Short: "Approve a pending script or signature",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		target := args[0]
		err = client.ApproveScript(target)
		if err != nil {
			return err
		}
		fmt.Printf("Approved: %s\n", target)
		return nil
	},
}

var pipelineCmd = &cobra.Command{
	Use:   "pipeline",
	Short: "Pipeline utility commands",
}

var pipelineLintCmd = &cobra.Command{
	Use:   "lint [jenkinsfile-path]",
	Short: "Lint and validate a Declarative Pipeline (Jenkinsfile)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		path := args[0]
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		out, err := client.LintPipeline(string(content))
		if err != nil {
			return err
		}

		fmt.Print(out)
		if strings.Contains(out, "ERROR:") {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	approvalCmd.AddCommand(approvalListCmd)
	approvalCmd.AddCommand(approvalApproveCmd)
	rootCmd.AddCommand(approvalCmd)

	pipelineCmd.AddCommand(pipelineLintCmd)
	rootCmd.AddCommand(pipelineCmd)
}
