package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var credCmd = &cobra.Command{
	Use:     "credential",
	Short:   "Manage Jenkins credentials",
	GroupID: GroupAdmin,
}

var credListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all credentials IDs and types",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		script := `
println "ID\tTYPE\tDESCRIPTION"
def repo = com.cloudbees.plugins.credentials.CredentialsProvider.lookupCredentials(
    com.cloudbees.plugins.credentials.common.StandardCredentials.class,
    jenkins.model.Jenkins.instance,
    null,
    java.util.Collections.emptyList()
)

repo.each { c ->
    println "${c.id}\t${c.class.simpleName}\t${c.description ?: ''}"
}
`
		output, err := client.ExecuteGroovy(script)
		if err != nil {
			return err
		}

		fmt.Print(output)
		return nil
	},
}

func init() {
	credCmd.AddCommand(credListCmd)
	credCmd.AddCommand(credCreateCmd)
	credCmd.AddCommand(credDeleteCmd)
	rootCmd.AddCommand(credCmd)
}

var credDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a credential by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if isDryRun() {
			dryRunMsg("would delete credential %s", args[0])
			return nil
		}
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Deleting credential %s...\n", args[0])
		if err := client.DeleteCredential(args[0]); err != nil {
			return err
		}
		audit("credential.delete", args[0])
		return nil
	},
}
var credCreateCmd = &cobra.Command{
	Use:   "create [id] [secret-text] [description]",
	Short: "Create a new Secret Text credential",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if isDryRun() {
			dryRunMsg("would create credential %s", args[0])
			return nil
		}
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Creating credential %s...\n", args[0])
		// Never audit the secret (args[1]); only the credential id.
		if err := client.CreateCredential(args[0], args[1], args[2]); err != nil {
			return err
		}
		audit("credential.create", args[0])
		return nil
	},
}
