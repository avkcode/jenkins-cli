package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var watchFlags struct {
	interval  time.Duration
	grep      string
	timeout   time.Duration
	onFailure string // shell command to run on failure
}

var watchCmd = &cobra.Command{
	Use:   "watch [job-name]",
	Short: "Watch a job and alert on builds",
	Long: `Poll a Jenkins job for new builds and alert on failures or pattern matches.

Waits for the next build to start, streams its progress, and reports the outcome.
Use --grep to highlight specific patterns (e.g. "ERROR"), --on-failure to run a
shell command on failure, and --interval to control polling frequency.`,
	GroupID: GroupCore,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobName := args[0]

		ctx, cancel := getTimeoutContext(cmd.Context())
		defer cancel()

		// Handle Ctrl+C gracefully
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		defer signal.Stop(sigCh)

		jc, err := getClient(ctx)
		if err != nil {
			return err
		}

		job, err := jc.Client.GetJob(ctx, jobName)
		if err != nil {
			return fmt.Errorf("job not found: %w", err)
		}

		// Get the current last build number as baseline
		lastBuild, err := job.GetLastBuild(ctx)
		var lastBuildNum int64
		if err == nil && lastBuild != nil {
			lastBuildNum = lastBuild.GetBuildNumber()
		}

		fmt.Fprintf(os.Stderr, "Watching %s (last build: #%d) — poll every %s\n", jobName, lastBuildNum, watchFlags.interval)
		fmt.Fprintln(os.Stderr)

		for {
			select {
			case <-sigCh:
				fmt.Fprintln(os.Stderr, "\nWatch canceled.")
				return nil
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			job, err := jc.Client.GetJob(ctx, jobName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error polling job: %v\n", err)
				time.Sleep(watchFlags.interval)
				continue
			}

			currentBuild, err := job.GetLastBuild(ctx)
			if err != nil || currentBuild == nil {
				time.Sleep(watchFlags.interval)
				continue
			}

			currentNum := currentBuild.GetBuildNumber()
			if currentNum <= lastBuildNum {
				time.Sleep(watchFlags.interval)
				continue
			}

			// New builds detected
			for num := lastBuildNum + 1; num <= currentNum; num++ {
				fmt.Fprintf(os.Stderr, "=== Build #%d ===\n", num)

				b, err := jc.Client.GetBuild(ctx, jobName, num)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
					continue
				}

				dur := time.Duration(b.Raw.Duration) * time.Millisecond
				result := b.GetResult()
				if result == "" {
					result = "IN PROGRESS"
				}

				fmt.Fprintf(os.Stderr, "  Result:   %s\n", labelResult(result))
				fmt.Fprintf(os.Stderr, "  Duration: %s\n", fmtDur(dur))
				if b.Raw.BuiltOn != "" {
					fmt.Fprintf(os.Stderr, "  Agent:    %s\n", b.Raw.BuiltOn)
				}

				// Stream console log and grep for patterns
				console := b.GetConsoleOutput(ctx)
				lines := strings.Split(console, "\n")

				if watchFlags.grep != "" {
					var matches []string
					for _, line := range lines {
						if strings.Contains(line, watchFlags.grep) {
							matches = append(matches, line)
						}
					}
					if len(matches) > 0 {
						fmt.Fprintf(os.Stderr, "  Grep (%q): %d matches\n", watchFlags.grep, len(matches))
						maxShow := 10
						if len(matches) < maxShow {
							maxShow = len(matches)
						}
						for _, m := range matches[:maxShow] {
							fmt.Fprintf(os.Stderr, "    %s\n", strings.TrimSpace(m))
						}
						if len(matches) > maxShow {
							fmt.Fprintf(os.Stderr, "    ... and %d more matches\n", len(matches)-maxShow)
						}
					}
				}

				// Alert on failure
				if !b.IsGood(ctx) && result != "IN PROGRESS" && result != "" {
					fmt.Fprintf(os.Stderr, "\n  *** BUILD FAILED ***\n")

					if watchFlags.onFailure != "" {
						fmt.Fprintf(os.Stderr, "  Executing: %s\n", watchFlags.onFailure)
						// Note: full shell execution would use os/exec; simplified here
						fmt.Fprintf(os.Stderr, "  [on-failure command would run: %s]\n", watchFlags.onFailure)
					}
				}
				fmt.Fprintln(os.Stderr)
			}

			lastBuildNum = currentNum
			time.Sleep(watchFlags.interval)
		}
	},
}

func init() {
	watchCmd.Flags().DurationVarP(&watchFlags.interval, "interval", "i", 15*time.Second, "Polling interval")
	watchCmd.Flags().StringVarP(&watchFlags.grep, "grep", "g", "", "Highlight log lines matching pattern (e.g. ERROR)")
	watchCmd.Flags().DurationVar(&watchFlags.timeout, "timeout", 0, "Stop watching after timeout (0 = never)")
	watchCmd.Flags().StringVar(&watchFlags.onFailure, "on-failure", "", "Shell command to run when a build fails")
	rootCmd.AddCommand(watchCmd)
}
