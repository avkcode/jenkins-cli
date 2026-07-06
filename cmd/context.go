package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var contextCmd = &cobra.Command{
	Use:     "context",
	Short:   "Manage Jenkins connection contexts (profiles)",
	Long:    `Switch between multiple Jenkins server profiles stored in ~/.jenkins-cli.yaml.`,
	GroupID: GroupConfig,
}

var contextListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all saved contexts",
	RunE: func(cmd *cobra.Command, args []string) error {
		current := viper.GetString("current-context")
		contexts := viper.GetStringMap("contexts")
		if len(contexts) == 0 {
			fmt.Fprintln(os.Stderr, "No contexts configured. Use 'jc context set' to create one.")
			return nil
		}
		for name, v := range contexts {
			marker := " "
			if name == current {
				marker = "*"
			}
			c, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			url := c["url"].(string)
			fmt.Printf("%s %s\t%s\n", marker, name, url)
		}
		return nil
	},
}

var contextUseCmd = &cobra.Command{
	Use:   "use [name]",
	Short: "Switch to a saved context",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		contexts := viper.GetStringMap("contexts")
		if _, ok := contexts[name]; !ok {
			return fmt.Errorf("context %q not found. Available: %v", name, mapKeys(contexts))
		}
		viper.Set("current-context", name)
		if err := saveConfig(); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Switched to context %q\n", name)
		return nil
	},
}

var contextSetCmd = &cobra.Command{
	Use:   "set [name]",
	Short: "Create or update a connection context",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		url, _ := cmd.Flags().GetString("url")
		user, _ := cmd.Flags().GetString("user")
		tok, _ := cmd.Flags().GetString("token")

		if url == "" && user == "" && tok == "" {
			return fmt.Errorf("at least one of --url, --user, or --token is required")
		}

		contexts := viper.GetStringMap("contexts")
		if existing, ok := contexts[name].(map[string]interface{}); ok {
			if url != "" {
				existing["url"] = url
			}
			if user != "" {
				existing["user"] = user
			}
			if tok != "" {
				existing["token"] = tok
			}
			contexts[name] = existing
		} else {
			contexts[name] = map[string]interface{}{
				"url":   url,
				"user":  user,
				"token": tok,
			}
		}
		viper.Set("contexts", contexts)

		if viper.GetString("current-context") == "" {
			viper.Set("current-context", name)
		}

		if err := saveConfig(); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Context %q saved.\n", name)
		return nil
	},
}

var contextDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a saved context",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		contexts := viper.GetStringMap("contexts")
		if _, ok := contexts[name]; !ok {
			return fmt.Errorf("context %q not found", name)
		}
		if isDryRun() {
			dryRunMsg("would delete context %s", name)
			return nil
		}
		delete(contexts, name)
		viper.Set("contexts", contexts)
		if viper.GetString("current-context") == name {
			viper.Set("current-context", "")
			fmt.Fprintf(os.Stderr, "Cleared current-context (was %q)\n", name)
		}
		if err := saveConfig(); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Context %q deleted.\n", name)
		return nil
	},
}

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func saveConfig() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(home, ".jenkins-cli.yaml")
	if cfgFile != "" {
		configPath = cfgFile
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}
	if err := viper.WriteConfigAs(configPath); err != nil {
		return viper.WriteConfig()
	}
	return nil
}

func init() {
	contextSetCmd.Flags().String("url", "", "Jenkins server URL")
	contextSetCmd.Flags().String("user", "", "Jenkins username")
	contextSetCmd.Flags().String("token", "", "Jenkins API token")

	contextCmd.AddCommand(contextListCmd)
	contextCmd.AddCommand(contextUseCmd)
	contextCmd.AddCommand(contextSetCmd)
	contextCmd.AddCommand(contextDeleteCmd)
	rootCmd.AddCommand(contextCmd)
}
