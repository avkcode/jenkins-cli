package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configViewCmd = &cobra.Command{
	Use:   "view",
	Short: "Show effective (merged) configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintf(os.Stderr, "Config file: %s\n\n", viper.ConfigFileUsed())

		all := viper.AllSettings()
		keys := make([]string, 0, len(all))
		for k := range all {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			v := viper.Get(k)
			switch val := v.(type) {
			case map[string]interface{}:
				fmt.Printf("%s:\n", k)
				subKeys := make([]string, 0, len(val))
				for sk := range val {
					subKeys = append(subKeys, sk)
				}
				sort.Strings(subKeys)
				for _, sk := range subKeys {
					fmt.Printf("  %s: %v\n", sk, val[sk])
				}
			default:
				fmt.Printf("%s: %v\n", k, val)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configViewCmd)
}

var configCmd = &cobra.Command{
	Use:     "config",
	Short:   "Manage CLI configuration",
	GroupID: GroupConfig,
}
