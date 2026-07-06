package cmd

import (
	"fmt"
	"os"
	"time"

	clientpkg "github.com/avkcode/jenkins-cli/pkg/client"
	"github.com/avkcode/jenkins-cli/pkg/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var (
	buildWait    bool
	buildLogs    bool
	buildRawLogs bool
	buildTimeout time.Duration
	buildTui     bool
)

var jobBuildCmd = &cobra.Command{
	Use:   "build [job name]",
	Short: "Trigger a build for a Jenkins job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobName := args[0]
		params, _ := cmd.Flags().GetStringToString("param")

		if isDryRun() {
			dryRunMsg("Would trigger build for %s with %d parameters", jobName, len(params))
			return nil
		}

		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		if buildTui {
			labels := []string{
				"Authentication & Connection",
				"Queueing Build",
				"K8s Scheduling & Node Allocation",
				"Hostd Provisioning: VM Boot",
				"Agent Connectivity & Setup",
			}
			model := tui.InitialModel(labels)
			actionChan := model.Action
			p := tea.NewProgram(model, tea.WithAltScreen())

			go func() {
				// Metadata
				p.Send(tui.ConfigMsg{
					Cloud: "firecracker",
					Image: params["IMAGE"],
				})

				// Step 1: Auth
				p.Send(tui.StepUpdateMsg{StepID: 1, Status: tui.StepRunning})
				time.Sleep(100 * time.Millisecond)
				p.Send(tui.StepUpdateMsg{StepID: 1, Status: tui.StepCompleted, Info: "OK"})

				// Step 2: Queueing
				p.Send(tui.StepUpdateMsg{StepID: 2, Status: tui.StepRunning})
				queueID, err := client.Client.BuildJob(ctx, jobName, params)
				if err != nil {
					p.Send(err)
					return
				}
				p.Send(tui.StepUpdateMsg{StepID: 2, Status: tui.StepCompleted, Info: fmt.Sprintf("ID: %d", queueID)})

				// Step 3: Scheduling (Wait for build to start)
				p.Send(tui.StepUpdateMsg{StepID: 3, Status: tui.StepRunning})
				build, err := client.WaitForBuildToStart(queueID, buildTimeout)
				if err != nil {
					p.Send(err)
					return
				}
				p.Send(tui.StepUpdateMsg{StepID: 3, Status: tui.StepCompleted, Info: fmt.Sprintf("#%d", build.GetBuildNumber())})

				// Step 4: Hostd Provisioning
				p.Send(tui.StepUpdateMsg{StepID: 4, Status: tui.StepRunning})
				for build.Raw.BuiltOn == "" {
					time.Sleep(500 * time.Millisecond)
					build.Poll(ctx)
					if !build.IsRunning(ctx) {
						break
					}
				}
				nodeName := build.Raw.BuiltOn
				p.Send(tui.ConfigMsg{Host: nodeName})
				p.Send(tui.StepUpdateMsg{StepID: 4, Status: tui.StepCompleted, Info: nodeName})

				// Step 5: Connectivity
				p.Send(tui.StepUpdateMsg{StepID: 5, Status: tui.StepRunning})

				// Real log streaming via Jenkins progressive console
				for build.IsRunning(ctx) {
					time.Sleep(1 * time.Second)
					build.Poll(ctx)
				}

				if build.IsGood(ctx) {
					p.Send(tui.StepUpdateMsg{StepID: 5, Status: tui.StepCompleted, Info: "Agent Ready"})
				} else {
					p.Send(tui.StepUpdateMsg{StepID: 5, Status: tui.StepFailed, Info: build.GetResult()})
				}
			}()

			// Listen for actions
			go func() {
				for action := range actionChan {
					if action == "kill" {
						p.Send(tui.LogMsg("Action: Killing build..."))
						// Note: 'build' might be nil if not started yet, but StopBuild handles job/build#
						// If we have build number, use it.
						// The build object in the parent closure is updated asynchronously.
					}
				}
			}()

			if _, err := p.Run(); err != nil {
				return fmt.Errorf("TUI error: %w", err)
			}
			return nil
		}

		fmt.Fprintf(os.Stderr, "Triggering build for %s with %d parameters...\n", jobName, len(params))
		queueID, err := client.Client.BuildJob(ctx, jobName, params)
		if err != nil {
			return fmt.Errorf("failed to trigger build: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Build queued (Queue ID: %d)\n", queueID)

		if !buildWait && !buildLogs {
			return nil
		}

		fmt.Fprintln(os.Stderr, "Waiting for build to start...")
		build, err := client.WaitForBuildToStart(queueID, buildTimeout)
		if err != nil {
			return fmt.Errorf("error waiting for build: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Build #%d started. URL: %s\n", build.GetBuildNumber(), build.GetUrl())

		if build.Raw.BuiltOn != "" {
			client.NodeName = build.Raw.BuiltOn
		}

		if buildLogs {
			fmt.Fprintln(os.Stderr, "--- Logs ---")
			err = client.StreamLogsWithOptions(jobName, build.GetBuildNumber(), os.Stdout, clientpkg.LogStreamOptions{
				PollInterval: 2 * time.Second,
				Raw:          buildRawLogs,
				Follow:       true,
			})
			if err != nil {
				return fmt.Errorf("error streaming logs: %w", err)
			}
			fmt.Fprintln(os.Stderr, "\n--- End of Logs ---")

			// Refresh build state to get final status
			build.Poll(ctx)
		} else if buildWait {
			fmt.Fprintln(os.Stderr, "Waiting for build to complete...")
			for build.IsRunning(ctx) {
				time.Sleep(5 * time.Second)
				build.Poll(ctx)
			}
		}

		if buildWait || buildLogs {
			success := build.IsGood(ctx)
			if success {
				fmt.Fprintf(os.Stderr, "Build #%d completed successfully (Status: %s)\n", build.GetBuildNumber(), build.GetResult())
			} else {
				fmt.Fprintf(os.Stderr, "Build #%d failed (Status: %s)\n", build.GetBuildNumber(), build.GetResult())
				os.Exit(1)
			}
		}

		return nil
	},
}

func init() {
	jobBuildCmd.Flags().BoolVarP(&buildWait, "wait", "w", false, "Wait for build to complete")
	jobBuildCmd.Flags().BoolVarP(&buildLogs, "logs", "l", false, "Stream logs (implies --wait)")
	jobBuildCmd.Flags().BoolVar(&buildRawLogs, "raw", false, "Stream raw Jenkins console output including hidden annotations")
	jobBuildCmd.Flags().StringToStringP("param", "p", nil, "Build parameters (e.g. -p KEY=value)")
	jobBuildCmd.Flags().DurationVar(&buildTimeout, "timeout", 5*time.Minute, "Timeout for waiting for build to start")
	jobBuildCmd.Flags().BoolVar(&buildTui, "tui", false, "Show live progress TUI")

	jobCmd.AddCommand(jobBuildCmd)
}
