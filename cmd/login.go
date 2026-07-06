package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	contextName string
)

var loginCmd = &cobra.Command{
	Use:     "login",
	Short:   "Authenticate with Jenkins server and save credentials",
	GroupID: GroupConfig,
	RunE: func(cmd *cobra.Command, args []string) error {
		url := viper.GetString("url")
		user := viper.GetString("user")
		tok := viper.GetString("token")

		// Interactive prompts if values are missing
		if url == "" {
			prompt := promptui.Prompt{
				Label: "Jenkins URL",
				Validate: func(input string) error {
					if !strings.HasPrefix(input, "http") {
						return fmt.Errorf("URL must start with http:// or https://")
					}
					return nil
				},
			}
			var err error
			url, err = prompt.Run()
			if err != nil {
				return err
			}
			viper.Set("url", url)
		}

		if user == "" {
			prompt := promptui.Prompt{
				Label: "Username",
			}
			var err error
			user, err = prompt.Run()
			if err != nil {
				return err
			}
			viper.Set("user", user)
		}

		if tok == "" {
			prompt := promptui.Prompt{
				Label: "API Token/Password",
				Mask:  '*',
			}
			var err error
			tok, err = prompt.Run()
			if err != nil {
				return err
			}
			viper.Set("token", tok)
		}

		// Verify connection
		fmt.Fprintf(os.Stderr, "Verifying connection to %s...\n", url)
		_, err := getClient(context.Background())
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		if contextName == "" {
			// Auto-generate name from URL if not provided
			contextName = strings.TrimPrefix(url, "http://")
			contextName = strings.TrimPrefix(contextName, "https://")
			contextName = strings.ReplaceAll(contextName, ".", "-")
			contextName = strings.ReplaceAll(contextName, ":", "-")
			contextName = strings.ReplaceAll(contextName, "/", "")
		}

		// Save to config under contexts map
		contexts := viper.GetStringMap("contexts")
		if contexts == nil {
			contexts = make(map[string]interface{})
		}

		contexts[contextName] = map[string]string{
			"url":   url,
			"user":  user,
			"token": tok,
		}

		viper.Set("contexts", contexts)
		viper.Set("current-context", contextName)

		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		configPath := filepath.Join(home, ".jenkins-cli.yaml")
		err = viper.WriteConfigAs(configPath)
		if err != nil {
			_ = viper.WriteConfig()
		}

		fmt.Printf("Successfully authenticated and saved context '%s'!\n", contextName)
		return nil
	},
}

func init() {
	loginCmd.Flags().StringVar(&contextName, "name", "", "Name for this context (e.g. 'prod', 'lab')")
	rootCmd.AddCommand(loginCmd)
}
