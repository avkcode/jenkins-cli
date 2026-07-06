package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate completion script",
	Long: `To load completions:

Bash:

  $ source <(jc completion bash)

  # To load completions for each session, add to your ~/.bashrc:
  # jc completion bash > /etc/bash_completion.d/jc

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, add to your ~/.zshrc:
  # jc completion zsh > "${fpath[1]}/_jc"

Fish:

  $ jc completion fish | source

  # To load completions for each session, add to your ~/.config/fish/completions/jc.fish:
  # jc completion fish > ~/.config/fish/completions/jc.fish

PowerShell:

  PS> jc completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> jc completion powershell > jc.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.ExactValidArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
