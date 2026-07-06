package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Interactive Groovy shell for Jenkins",
	Long:  `Open an interactive shell to explore the Jenkins JVM. This is a powerful REPL-like environment.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		fmt.Println("Jenkins Interactive Groovy Shell (jc shell)")
		fmt.Println("Type 'exit' or 'quit' to leave. Use 'Jenkins.instance' as your entry point.")
		fmt.Println("")

		for {
			prompt := promptui.Prompt{
				Label: "jenkins>",
			}

			result, err := prompt.Run()

			if err != nil {
				if err == promptui.ErrInterrupt || err == promptui.ErrEOF {
					return nil
				}
				return err
			}

			input := strings.TrimSpace(result)
			if input == "exit" || input == "quit" {
				break
			}
			if input == "" {
				continue
			}

			output, err := client.ExecuteGroovy(input)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}

			fmt.Print(output)
			if !strings.HasSuffix(output, "\n") {
				fmt.Println("")
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(shellCmd)
}
