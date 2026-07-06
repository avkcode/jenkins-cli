package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	clientpkg "github.com/avkcode/jenkins-cli/pkg/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	frozenInputId string
)

var frozenCmd = &cobra.Command{
	Use:     "frozen",
	Short:   "Manage frozen builds (paused on input step)",
	Long:    `List, inspect, fix, and thaw builds that are frozen on an input step, preserving their workspace for interactive debugging.`,
	GroupID: GroupCore,
}

var frozenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List builds frozen on an input step",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		builds, err := client.ListFrozenJobs()
		if err != nil {
			return fmt.Errorf("failed to list frozen builds: %w", err)
		}

		out := getOutput()

		switch format := viper.GetString("output"); format {
		case "json":
			return out.PrintJSON(builds)
		case "yaml":
			return out.PrintYAML(builds)
		default:
			if len(builds) == 0 {
				fmt.Fprintln(os.Stderr, "No frozen builds found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "JOB\tBUILD\tNODE\tPROMPT")
			for _, b := range builds {
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", b.Job, b.Build, b.Node, b.Prompt)
			}
			return w.Flush()
		}
	},
}

var frozenExecCmd = &cobra.Command{
	Use:   "exec <job> <build> <command...>",
	Short: "Execute a shell command in a frozen build's workspace",
	Long: `Execute a shell command on the agent where the frozen build is running,
using the preserved workspace as the working directory. Example:
  jc frozen exec my-pipeline 42 "cat test-output.log"
  jc frozen exec my-pipeline 42 "env | sort"`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobName := args[0]
		buildNumber, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid build number: %s", args[1])
		}
		command := strings.Join(args[2:], " ")

		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		fb, err := resolveFrozenBuild(client, jobName, buildNumber)
		if err != nil {
			return err
		}

		result, err := client.AgentExec(fb.Node, fb.Workspace, command)
		if err != nil {
			return fmt.Errorf("exec failed: %w", err)
		}

		if !outputIsStructured() {
			fmt.Fprintf(os.Stderr, "Exec on %s (job=%s build=%d): %s\n", fb.Node, jobName, buildNumber, command)
			fmt.Fprint(os.Stdout, result.Stdout)
			if result.Stderr != "" {
				fmt.Fprint(os.Stderr, result.Stderr)
			}
			if result.ExitCode != 0 {
				fmt.Fprintf(os.Stderr, "\nExit code: %d\n", result.ExitCode)
			}
			return nil
		}
		return renderStructured(result)
	},
}

var frozenReadCmd = &cobra.Command{
	Use:   "read <job> <build> <file-path>",
	Short: "Read a file from a frozen build's workspace",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobName := args[0]
		buildNumber, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid build number: %s", args[1])
		}
		filePath := args[2]

		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		fb, err := resolveFrozenBuild(client, jobName, buildNumber)
		if err != nil {
			return err
		}

		content, err := client.AgentReadFile(fb.Node, fb.Workspace, filePath)
		if err != nil {
			return fmt.Errorf("read failed: %w", err)
		}

		fmt.Print(content)
		return nil
	},
}

var frozenWriteCmd = &cobra.Command{
	Use:   "write <job> <build> <file-path>",
	Short: "Write content into a frozen build's workspace",
	Long: `Write content to a file in the frozen build's workspace. Content is read from
stdin or from the --content flag. Example:
  echo 'JAVA_OPTS=-Xmx2g' | jc frozen write my-pipeline 42 .env
  jc frozen write my-pipeline 42 test.sh --content '#!/bin/bash\n./fixed-test.sh'`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobName := args[0]
		buildNumber, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid build number: %s", args[1])
		}
		filePath := args[2]

		content := writeContent
		if content == "" {
			// Read from stdin if not piped, show a prompt
			fi, _ := os.Stdin.Stat()
			if (fi.Mode() & os.ModeCharDevice) != 0 {
				fmt.Fprintf(os.Stderr, "Enter content (Ctrl+D to finish):\n")
			}
			var lines []string
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}
			content = strings.Join(lines, "\n")
		}

		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		fb, err := resolveFrozenBuild(client, jobName, buildNumber)
		if err != nil {
			return err
		}

		if err := client.AgentWriteFile(fb.Node, fb.Workspace, filePath, content); err != nil {
			return fmt.Errorf("write failed: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Wrote %d bytes to %s on %s\n", len(content), filePath, fb.Node)
		audit("frozen.write", fmt.Sprintf("%s#%d:%s", jobName, buildNumber, filePath))
		return nil
	},
}

var frozenSnapshotCmd = &cobra.Command{
	Use:   "snapshot <job> <build>",
	Short: "Snapshot the workspace of a frozen build as a tarball",
	Long:  `Create a tarball of the frozen build's workspace on the agent for later retrieval.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobName := args[0]
		buildNumber, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid build number: %s", args[1])
		}

		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		fb, err := resolveFrozenBuild(client, jobName, buildNumber)
		if err != nil {
			return err
		}

		if isDryRun() {
			dryRunMsg("would snapshot workspace for %s#%d on %s", jobName, buildNumber, fb.Node)
			return nil
		}

		info, err := client.AgentSnapshotWorkspace(fb.Node, fb.Workspace)
		if err != nil {
			return fmt.Errorf("snapshot failed: %w", err)
		}

		if outputIsStructured() {
			return renderStructured(info)
		}

		fmt.Fprintf(os.Stderr, "Snapshot created on agent %s:\n", fb.Node)
		fmt.Printf("  path: %v\n  size: %v bytes\n  name: %v\n",
			info["path"], info["size"], info["name"])
		audit("frozen.snapshot", fmt.Sprintf("%s#%d", jobName, buildNumber))
		return nil
	},
}

var frozenReattemptCmd = &cobra.Command{
	Use:   "reattempt <job> <build> <command...>",
	Short: "Re-run a command in the frozen workspace and report the result",
	Long: `Execute a shell command in the frozen build's workspace and report the
exit code. Use this to verify a fix before thawing the build. Example:
  jc frozen reattempt my-pipeline 42 "mvn test -Dtest=FailedTest"`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobName := args[0]
		buildNumber, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid build number: %s", args[1])
		}
		command := strings.Join(args[2:], " ")

		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		fb, err := resolveFrozenBuild(client, jobName, buildNumber)
		if err != nil {
			return err
		}

		result, err := client.AgentReattempt(fb.Node, fb.Workspace, command)
		if err != nil {
			return fmt.Errorf("reattempt failed: %w", err)
		}

		if !outputIsStructured() {
			fmt.Fprintf(os.Stderr, "Reattempt on %s (job=%s build=%d): %s\n", fb.Node, jobName, buildNumber, command)
			fmt.Fprint(os.Stdout, result.Stdout)
			if result.Stderr != "" {
				fmt.Fprint(os.Stderr, result.Stderr)
			}
			if result.ExitCode == 0 {
				fmt.Fprintln(os.Stderr, "\n✓ Command succeeded (exit 0)")
			} else {
				fmt.Fprintf(os.Stderr, "\n✗ Command failed (exit %d)\n", result.ExitCode)
			}
			os.Exit(result.ExitCode)
			return nil
		}
		return renderStructured(result)
	},
}

var frozenThawCmd = &cobra.Command{
	Use:   "thaw <job> <build>",
	Short: "Thaw (resume) a frozen build by submitting its input dialog",
	Long: `Submit the pending input on a frozen build, allowing the pipeline to continue
or finish. If the build has multiple pending inputs, use --input-id to specify
which one to submit. Example:
  jc frozen thaw my-pipeline 42`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobName := args[0]
		buildNumber, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid build number: %s", args[1])
		}

		if isDryRun() {
			dryRunMsg("would thaw (submit input) for %s#%d", jobName, buildNumber)
			return nil
		}

		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		if err := client.ThawFrozenBuild(jobName, buildNumber, frozenInputId); err != nil {
			return fmt.Errorf("thaw failed: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Thawed %s#%d - input submitted, build will resume.\n", jobName, buildNumber)
		audit("frozen.thaw", fmt.Sprintf("%s#%d", jobName, buildNumber))
		return nil
	},
}

// resolveFrozenBuild looks up a specific frozen build by job name and build
// number, returning its FrozenBuild details or an error if not found.
func resolveFrozenBuild(cl *clientpkg.JenkinsClient, jobName string, buildNumber int64) (*clientpkg.FrozenBuild, error) {
	builds, err := cl.ListFrozenJobs()
	if err != nil {
		return nil, err
	}

	// Try exact match first
	for i := range builds {
		if builds[i].Job == jobName && builds[i].Build == buildNumber {
			return &builds[i], nil
		}
	}

	// Try partial match on job name (e.g. "my-pipeline" matches "folder/my-pipeline")
	for i := range builds {
		if strings.HasSuffix(builds[i].Job, "/"+jobName) && builds[i].Build == buildNumber {
			return &builds[i], nil
		}
	}

	b, _ := json.MarshalIndent(builds, "", "  ")
	return nil, fmt.Errorf("frozen build %s#%d not found.\nFrozen builds:\n%s", jobName, buildNumber, string(b))
}

var writeContent string

func init() {
	frozenWriteCmd.Flags().StringVar(&writeContent, "content", "", "Content to write (alternative to stdin)")

	frozenThawCmd.Flags().StringVar(&frozenInputId, "input-id", "", "Specific input ID to submit (if multiple inputs are pending)")

	frozenCmd.AddCommand(frozenListCmd)
	frozenCmd.AddCommand(frozenExecCmd)
	frozenCmd.AddCommand(frozenReadCmd)
	frozenCmd.AddCommand(frozenWriteCmd)
	frozenCmd.AddCommand(frozenSnapshotCmd)
	frozenCmd.AddCommand(frozenReattemptCmd)
	frozenCmd.AddCommand(frozenThawCmd)

	rootCmd.AddCommand(frozenCmd)
}
