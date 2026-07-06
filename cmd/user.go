package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var userCmd = &cobra.Command{
	Use:     "user",
	Short:   "Manage Jenkins users",
	GroupID: GroupAdmin,
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users",
	RunE: func(cmd *cobra.Command, args []string) error {
		// gojenkins doesn't have a direct GetAllUsers, so we use a Groovy script
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		script := `println "ID\tFull Name"
hudson.model.User.getAll().each { u ->
    println "${u.id}\t${u.fullName}"
}`
		output, err := client.ExecuteGroovy(script)
		if err != nil {
			return err
		}

		fmt.Print(output)
		return nil
	},
}

var userCreateCmd = &cobra.Command{
	Use:   "create [username] [password] [full-name] [email]",
	Short: "Create a new user",
	Args:  cobra.ExactArgs(4),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		_, err = client.Client.CreateUser(ctx, args[0], args[1], args[2], args[3])
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "User %s created successfully.\n", args[0])
		return nil
	},
}

func init() {
	userCmd.AddCommand(userListCmd)
	userCmd.AddCommand(userCreateCmd)
	rootCmd.AddCommand(userCmd)
}
