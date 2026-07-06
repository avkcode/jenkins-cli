package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var systemCmd = &cobra.Command{
	Use:     "system",
	Short:   "General Jenkins system management",
	GroupID: GroupAdmin,
}

var systemInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show Jenkins server information",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		info, err := client.GetInfo()
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "PROPERTY\tVALUE\n")
		fmt.Fprintf(w, "URL\t%s\n", client.Client.Server)
		fmt.Fprintf(w, "Node Description\t%s\n", info.NodeDescription)
		fmt.Fprintf(w, "Mode\t%s\n", info.Mode)
		fmt.Fprintf(w, "Num Executors\t%d\n", info.NumExecutors)
		w.Flush()
		return nil
	},
}

var systemRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Safely restart the Jenkins server",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		fmt.Fprintln(os.Stderr, "Triggering safe restart...")
		err = client.SafeRestart()
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Restart command sent successfully.")
		return nil
	},
}

var systemToolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List global tool installations (JDK, Maven, etc.)",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		output, err := client.GetGlobalTools()
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

var systemGetConfigCmd = &cobra.Command{
	Use:   "get-config",
	Short: "Get global Jenkins config.xml",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		config, err := client.GetSystemConfig()
		if err != nil {
			return err
		}
		fmt.Println(config)
		return nil
	},
}

var systemApplyConfigCmd = &cobra.Command{
	Use:   "apply-config [config-file]",
	Short: "Update global Jenkins config.xml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Applying global configuration...")
		return client.UpdateSystemConfig(string(data))
	},
}

var systemLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View global Jenkins system logs",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		logs, err := client.GetSystemLogs()
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "--- Jenkins System Log ---")
		fmt.Println(logs)
		return nil
	},
}

var systemEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage Global Environment Variables",
}

var systemEnvListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all global environment variables",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		out, err := client.ListEnvVars()
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	},
}

var systemEnvSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a global environment variable",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		return client.SetEnvVar(args[0], args[1])
	},
}

var systemEnvDeleteCmd = &cobra.Command{
	Use:   "delete [key]",
	Short: "Delete a global environment variable",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		return client.DeleteEnvVar(args[0])
	},
}

func init() {
	systemEnvCmd.AddCommand(systemEnvListCmd)
	systemEnvCmd.AddCommand(systemEnvSetCmd)
	systemEnvCmd.AddCommand(systemEnvDeleteCmd)
	systemCmd.AddCommand(systemEnvCmd)

	systemCmd.AddCommand(systemInfoCmd)

	systemCmd.AddCommand(systemRestartCmd)
	systemCmd.AddCommand(systemGetConfigCmd)
	systemCmd.AddCommand(systemApplyConfigCmd)
	systemCmd.AddCommand(systemLogsCmd)
	systemCmd.AddCommand(systemToolsCmd)
	rootCmd.AddCommand(systemCmd)
}
