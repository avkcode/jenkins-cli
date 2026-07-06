package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var (
	libURL           string
	libVersion       string
	libCredentialsId string
	libFolder        string
)

var libraryCmd = &cobra.Command{
	Use:   "library",
	Short: "Manage Global and Folder-level Shared Libraries",
}

var libraryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all shared libraries (Global and Folder levels)",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		output, err := client.ListSharedLibraries()
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

var libraryAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Add or update a Git-based shared library",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		name := args[0]
		if libFolder != "" {
			fmt.Printf("Adding library %s to folder %s...\n", name, libFolder)
			return client.AddFolderSharedLibrary(libFolder, name, libURL, libVersion, libCredentialsId)
		}
		fmt.Printf("Adding global library %s (%s)...\n", name, libURL)
		return client.AddSharedLibrary(name, libURL, libVersion, libCredentialsId)
	},
}

var libraryUsageCmd = &cobra.Command{
	Use:   "usage [library-name]",
	Short: "Find all jobs using a specific shared library",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		output, err := client.AuditLibraryUsage(args[0])
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

var librarySignaturesCmd = &cobra.Command{
	Use:   "signatures [library-name]",
	Short: "List technical method signatures (def call) for custom steps",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		output, err := client.GetLibrarySignatures(args[0])
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

var libraryDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Remove a shared library",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("Removing library %s...\n", args[0])
		return client.DeleteSharedLibrary(args[0])
	},
}

func init() {
	libraryAddCmd.Flags().StringVar(&libURL, "url", "", "Git URL of the library")
	libraryAddCmd.Flags().StringVar(&libVersion, "version", "master", "Default version (branch/tag)")
	libraryAddCmd.Flags().StringVar(&libCredentialsId, "credentials", "", "Credentials ID for Git access")
	libraryAddCmd.Flags().StringVar(&libFolder, "folder", "", "Target folder (for folder-level libraries)")
	libraryAddCmd.MarkFlagRequired("url")

	libraryCmd.AddCommand(libraryListCmd)
	libraryCmd.AddCommand(libraryAddCmd)
	libraryCmd.AddCommand(libraryDeleteCmd)
	libraryCmd.AddCommand(libraryVarsCmd)
	libraryCmd.AddCommand(libraryDocCmd)
	libraryCmd.AddCommand(libraryEditCmd)
	libraryCmd.AddCommand(libraryUsageCmd)
	libraryCmd.AddCommand(librarySignaturesCmd)
	rootCmd.AddCommand(libraryCmd)
}

var libraryEditCmd = &cobra.Command{
	Use:   "edit [variable-name]",
	Short: "Live-edit a global library script (legacy workflow-libs/vars)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		varName := args[0]
		content, err := client.ReadGlobalLibraryScript(varName)
		if err != nil {
			// If not found, start with template
			content = "def call() {\n    echo 'Hello from " + varName + "'\n}\n"
		}

		tmpFile, err := os.CreateTemp("", "jc-lib-*.groovy")
		if err != nil {
			return err
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(content); err != nil {
			return err
		}
		tmpFile.Close()

		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim"
		}

		editCmd := exec.Command(editor, tmpFile.Name())
		editCmd.Stdin = os.Stdin
		editCmd.Stdout = os.Stdout
		editCmd.Stderr = os.Stderr
		if err := editCmd.Run(); err != nil {
			return fmt.Errorf("editor failed: %w", err)
		}

		newContent, err := os.ReadFile(tmpFile.Name())
		if err != nil {
			return err
		}

		if string(newContent) == content {
			fmt.Println("No changes made.")
			return nil
		}

		fmt.Printf("Updating global script %s...\n", varName)
		return client.WriteGlobalLibraryScript(varName, string(newContent))
	},
}

var libraryDocCmd = &cobra.Command{
	Use:   "doc [library] [variable]",
	Short: "Show documentation (.txt help) for a custom pipeline step",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		output, err := client.GetLibraryDoc(args[0], args[1])
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

var libraryVarsCmd = &cobra.Command{
	Use:   "vars [name]",
	Short: "List custom steps (global variables) provided by a library",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		output, err := client.ListLibraryVars(args[0])
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}
