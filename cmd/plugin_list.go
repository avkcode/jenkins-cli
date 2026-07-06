package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	pluginRestart bool
)

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed Jenkins plugins",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		plugins, err := client.Client.GetPlugins(ctx, 1)
		if err != nil {
			return fmt.Errorf("failed to get plugins: %w", err)
		}

		out := getOutput()
		type pluginRow struct {
			ShortName       string `json:"short_name" yaml:"short_name"`
			Version         string `json:"version" yaml:"version"`
			Active          string `json:"active" yaml:"active"`
			UpdateAvailable string `json:"update_available" yaml:"update_available"`
		}
		rows := make([]pluginRow, len(plugins.Raw.Plugins))
		for i, p := range plugins.Raw.Plugins {
			a, u := "Yes", "No"
			if !p.Active {
				a = "No"
			}
			if p.HasUpdate {
				u = "Yes"
			}
			rows[i] = pluginRow{ShortName: p.ShortName, Version: p.Version, Active: a, UpdateAvailable: u}
		}
		switch format := viper.GetString("output"); format {
		case "json":
			return out.PrintJSON(rows)
		case "yaml":
			return out.PrintYAML(rows)
		default:
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "SHORT_NAME\tVERSION\tACTIVE\tUPDATE_AVAILABLE")
			for _, r := range rows {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.ShortName, r.Version, r.Active, r.UpdateAvailable)
			}
			return w.Flush()
		}
	},
}

var pluginInstallCmd = &cobra.Command{
	Use:   "install [plugin-id]",
	Short: "Install a plugin by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if isDryRun() {
			dryRunMsg("Would install plugin %s", id)
			return nil
		}
		// Plugin downloads (with dependencies) routinely exceed the default
		// global timeout, so give installs at least a few minutes.
		timeout := viper.GetDuration("timeout")
		if timeout < 5*time.Minute {
			timeout = 5 * time.Minute
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		plugins, err := client.Client.GetPlugins(ctx, 1)
		if err == nil {
			for _, p := range plugins.Raw.Plugins {
				if p.ShortName == id {
					fmt.Fprintf(os.Stderr, "Plugin %s already installed (v%s).\n", id, p.Version)
					return nil
				}
			}
		}
		fmt.Fprintf(os.Stderr, "Installing plugin %s...\n", id)
		return client.InstallPlugin(ctx, id, "latest")
	},
}

var pluginUninstallCmd = &cobra.Command{
	Use:   "uninstall [plugin-id]",
	Short: "Uninstall a plugin by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if isDryRun() {
			dryRunMsg("Would uninstall plugin %s", args[0])
			return nil
		}
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Uninstalling plugin %s...\n", args[0])
		return client.Client.UninstallPlugin(ctx, args[0])
	},
}

var pluginUploadCmd = &cobra.Command{
	Use:   "upload [hpi-file]",
	Short: "Upload and install a local .hpi plugin file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		filePath := args[0]
		fmt.Fprintf(os.Stderr, "Uploading plugin %s...\n", filePath)
		err = client.UploadPlugin(filePath)
		if err != nil {
			return err
		}

		if pluginRestart {
			fmt.Fprintln(os.Stderr, "Triggering safe restart...")
			return client.SafeRestart()
		}

		fmt.Fprintln(os.Stderr, "Plugin uploaded successfully. You may need to restart Jenkins for changes to take effect.")
		return nil
	},
}

var pluginDisableCmd = &cobra.Command{
	Use:   "disable [plugin-id]",
	Short: "Disable a plugin by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		return client.DisablePlugin(args[0])
	},
}

var pluginEnableCmd = &cobra.Command{
	Use:   "enable [plugin-id]",
	Short: "Enable a plugin by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		return client.EnablePlugin(args[0])
	},
}

func init() {
	pluginUploadCmd.Flags().BoolVarP(&pluginRestart, "restart", "r", false, "Restart Jenkins after installation")

	pluginCmd.AddCommand(pluginListCmd)
	pluginCmd.AddCommand(pluginInstallCmd)
	pluginCmd.AddCommand(pluginUninstallCmd)
	pluginCmd.AddCommand(pluginUploadCmd)
	pluginCmd.AddCommand(pluginDisableCmd)
	pluginCmd.AddCommand(pluginEnableCmd)
}
