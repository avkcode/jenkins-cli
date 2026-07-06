package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	clientpkg "github.com/avkcode/jenkins-cli/pkg/client"
	"github.com/avkcode/jenkins-cli/pkg/logstream"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	logsRaw          bool
	logsFollow       bool
	logsBuildNumber  int64
	logsPollInterval time.Duration
	logsGrep         string
	logsStage        string
	logsNode         string
)

var logsCmd = &cobra.Command{
	Use:   "logs [job-name] [build-number]",
	Short: "Stream live logs from a Jenkins build",
	Long: `Stream live logs without the Jenkins GUI.

Streams the Jenkins progressive console log for the specified job. When no
build number is supplied, the latest build is used.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobName := args[0]
		buildNumber := logsBuildNumber
		if len(args) > 1 {
			if n, err := fmt.Sscanf(args[1], "%d", &buildNumber); n != 1 || err != nil || buildNumber <= 0 {
				return fmt.Errorf("invalid build number: %s", args[1])
			}
		}
		return streamLogsForJenkinsJob(cmd.Context(), jobName, buildNumber, os.Stdout)
	},
}

func streamLogsForJenkinsJob(ctx context.Context, jobName string, buildNumber int64, out io.Writer) error {
	jc, err := getClient(ctx)
	if err != nil {
		return err
	}

	if buildNumber <= 0 {
		job, err := jc.Client.GetJob(ctx, jobName)
		if err != nil {
			return fmt.Errorf("failed to get job %s: %w", jobName, err)
		}
		lastBuild, err := job.GetLastBuild(ctx)
		if err != nil {
			return fmt.Errorf("failed to get last build for %s: %w", jobName, err)
		}
		buildNumber = lastBuild.GetBuildNumber()
		fmt.Fprintf(os.Stderr, "Streaming Jenkins logs for latest build (#%d)\n", buildNumber)
	}

	build, err := jc.Client.GetBuild(ctx, jobName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to get build #%d: %w", buildNumber, err)
	}
	if build.Raw.BuiltOn != "" {
		jc.NodeName = build.Raw.BuiltOn
	}

	renderOpts, err := prepareLogRenderOptions(logstream.RenderOptions{
		Job:     jobName,
		Build:   buildNumber,
		Source:  "jenkins",
		Subject: "jenkins.http",
		Prefix:  false,
		Node:    build.Raw.BuiltOn,
	})
	if err != nil {
		return err
	}
	renderer, err := logstream.NewRenderer(out, renderOpts)
	if err != nil {
		return err
	}
	defer renderer.Flush()

	return jc.StreamLogsWithOptions(jobName, buildNumber, renderer, clientpkg.LogStreamOptions{
		PollInterval: logsPollInterval,
		Raw:          logsRaw,
		Follow:       logsFollow,
	})
}

func prepareLogRenderOptions(opts logstream.RenderOptions) (logstream.RenderOptions, error) {
	opts.Grep = logsGrep
	opts.Stage = logsStage
	opts.Node = logsNode
	opts.Format = viper.GetString("output")
	opts.Err = os.Stderr
	return opts, nil
}

func init() {
	logsCmd.Flags().BoolVar(&logsRaw, "raw", false, "Stream raw Jenkins console output including hidden annotations")
	logsCmd.Flags().BoolVar(&logsFollow, "follow", true, "Continue following Jenkins progressive logs until the build completes")
	logsCmd.Flags().Int64Var(&logsBuildNumber, "build", 0, "Jenkins build number for job log streaming")
	logsCmd.Flags().DurationVar(&logsPollInterval, "poll-interval", 2*time.Second, "Polling interval for Jenkins progressive logs")
	logsCmd.Flags().StringVar(&logsGrep, "grep", "", "Only print log lines matching this regular expression")
	logsCmd.Flags().StringVar(&logsStage, "stage", "", "Only print log lines associated with this Jenkins Pipeline stage")
	logsCmd.Flags().StringVar(&logsNode, "node", "", "Only print log lines for this Jenkins node")
	rootCmd.AddCommand(logsCmd)
}
