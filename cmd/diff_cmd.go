package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var diffFlags struct {
	logs  bool
	tests bool
	all   bool
}

var diffCmd = &cobra.Command{
	Use:   "diff [job-name] [build-a] [build-b]",
	Short: "Compare builds side-by-side",
	Long: `Compare two builds of a job: duration delta, test regressions, log diffs, and flaky tests.

With one argument, compares the last two completed builds.
With two arguments (job + build), compares that build with the previous one.
With three arguments (job + build-a + build-b), compares the two explicit builds.`,
	GroupID: GroupCore,
	Args:    cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getTimeoutContext(cmd.Context())
		defer cancel()

		jc, err := getClient(ctx)
		if err != nil {
			return err
		}

		jobName := args[0]
		var buildA, buildB int64

		if len(args) == 3 {
			buildA, err = strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid build-a number: %w", err)
			}
			buildB, err = strconv.ParseInt(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid build-b number: %w", err)
			}
		} else if len(args) == 2 {
			buildA, err = strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid build number: %w", err)
			}
			// Single build given: compare with previous
			buildB = buildA
			buildA = buildB - 1
			if buildA < 1 {
				return fmt.Errorf("need at least build #2 for comparison")
			}
		} else {
			// One arg: compare last two completed builds
			job, err := jc.Client.GetJob(ctx, jobName)
			if err != nil {
				return fmt.Errorf("job not found: %w", err)
			}
			last, err := job.GetLastBuild(ctx)
			if err != nil || last == nil {
				return fmt.Errorf("no builds found for %s", jobName)
			}
			buildB = last.GetBuildNumber()
			buildA = buildB - 1
			if buildA < 1 {
				return fmt.Errorf("need at least two builds for comparison")
			}
		}

		showTests := diffFlags.tests || diffFlags.all
		showLogs := diffFlags.logs || diffFlags.all
		if !diffFlags.tests && !diffFlags.logs && !diffFlags.all {
			showTests = true
		}

		b1, err := jc.Client.GetBuild(ctx, jobName, buildA)
		if err != nil {
			return fmt.Errorf("failed to get build #%d: %w", buildA, err)
		}
		b2, err := jc.Client.GetBuild(ctx, jobName, buildB)
		if err != nil {
			return fmt.Errorf("failed to get build #%d: %w", buildB, err)
		}

		fmt.Printf("Comparing %s  #%d → #%d\n\n", jobName, buildA, buildB)

		// Duration delta
		d1 := time.Duration(b1.Raw.Duration) * time.Millisecond
		d2 := time.Duration(b2.Raw.Duration) * time.Millisecond
		delta := d2 - d1
		pct := 0.0
		if d1 > 0 {
			pct = float64(delta) / float64(d1) * 100
		}
		fmt.Printf("  Duration:  %s → %s", fmtDur(d1), fmtDur(d2))
		if delta > 0 {
			fmt.Printf("  (+%s, +%.1f%% slower)\n", fmtDur(delta), pct)
		} else if delta < 0 {
			fmt.Printf("  (%s, %.1f%% faster)\n", fmtDur(delta), pct)
		} else {
			fmt.Println("  (no change)")
		}

		// Result
		r1, r2 := b1.GetResult(), b2.GetResult()
		if r1 != r2 {
			fmt.Printf("  Result:    %s → %s\n", labelResult(r1), labelResult(r2))
		} else {
			fmt.Printf("  Result:    %s (unchanged)\n", labelResult(r2))
		}

		// Timestamps
		fmt.Printf("  Built:     %s → %s\n",
			time.UnixMilli(b1.Raw.Timestamp).Format(time.RFC3339),
			time.UnixMilli(b2.Raw.Timestamp).Format(time.RFC3339),
		)

		// On which node
		if b1.Raw.BuiltOn != b2.Raw.BuiltOn && b1.Raw.BuiltOn != "" {
			fmt.Printf("  Agent:     %s → %s\n", b1.Raw.BuiltOn, b2.Raw.BuiltOn)
		} else if b1.Raw.BuiltOn != "" {
			fmt.Printf("  Agent:     %s (same)\n", b1.Raw.BuiltOn)
		}

		// Test results
		if showTests {
			fmt.Println("\n--- Tests ---")
			raw1, err1 := jc.GetTestResults(jobName, buildA)
			raw2, err2 := jc.GetTestResults(jobName, buildB)
			if err1 == nil && err2 == nil {
				diffTests(raw1, raw2)
			} else {
				if err1 != nil {
					fmt.Printf("  #%d: %v\n", buildA, err1)
				}
				if err2 != nil {
					fmt.Printf("  #%d: %v\n", buildB, err2)
				}
			}
		}

		// Log diff
		if showLogs {
			fmt.Println("\n--- Log Diff ---")
			log1 := b1.GetConsoleOutput(ctx)
			log2 := b2.GetConsoleOutput(ctx)
			changed, d := renderDiff(log1, log2)
			if changed {
				fmt.Print(d)
			} else {
				fmt.Println("  No significant log changes.")
			}
		}

		return nil
	},
}

func fmtDur(d time.Duration) string {
	d = d.Round(time.Millisecond)
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := d - time.Duration(m)*time.Minute
	return fmt.Sprintf("%dm %ds", m, int(s.Seconds()))
}

func labelResult(r string) string {
	switch strings.ToUpper(r) {
	case "SUCCESS":
		return r
	case "FAILURE":
		return r + " (FAIL)"
	case "UNSTABLE":
		return r + " (UNSTABLE)"
	case "ABORTED":
		return r + " (ABORTED)"
	default:
		return r
	}
}

func diffTests(raw1, raw2 string) {
	parse := func(raw string) map[string]string {
		m := map[string]string{}
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "TOTAL") || strings.HasPrefix(line, "Result") {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) == 2 {
				m[parts[0]] = parts[1]
			}
		}
		return m
	}
	t1, t2 := parse(raw1), parse(raw2)

	if s := extractLine(raw1, "TOTAL"); s != "" {
		fmt.Printf("  #%s\n", s)
	}
	if s := extractLine(raw2, "TOTAL"); s != "" {
		fmt.Printf("  #%s\n", s)
	}

	newFails := map[string]bool{}
	for name, status := range t2 {
		lo := strings.ToLower(status)
		if strings.Contains(lo, "fail") || strings.Contains(lo, "error") {
			if _, ok := t1[name]; !ok {
				newFails[name] = true
			}
		}
	}
	if len(newFails) > 0 {
		fmt.Printf("\n  New failures (%d):\n", len(newFails))
		for name := range newFails {
			fmt.Printf("    - %s\n", name)
		}
	}

	fixed := map[string]bool{}
	for name, status := range t1 {
		lo := strings.ToLower(status)
		if strings.Contains(lo, "fail") || strings.Contains(lo, "error") {
			if s2, ok := t2[name]; ok {
				lo2 := strings.ToLower(s2)
				if !strings.Contains(lo2, "fail") && !strings.Contains(lo2, "error") {
					fixed[name] = true
				}
			}
		}
	}
	if len(fixed) > 0 {
		fmt.Printf("\n  Fixed (%d):\n", len(fixed))
		for name := range fixed {
			fmt.Printf("    + %s\n", name)
		}
	}

	if len(newFails) == 0 && len(fixed) == 0 {
		fmt.Println("  No test result changes.")
	}
}

func extractLine(raw, prefix string) string {
	idx := strings.Index(raw, prefix)
	if idx < 0 {
		return ""
	}
	end := strings.Index(raw[idx:], "\n")
	if end < 0 {
		return raw[idx:]
	}
	return raw[idx : idx+end]
}

func init() {
	diffCmd.Flags().BoolVarP(&diffFlags.logs, "logs", "l", false, "Compare build logs (can be slow)")
	diffCmd.Flags().BoolVar(&diffFlags.tests, "tests", false, "Compare test results")
	diffCmd.Flags().BoolVarP(&diffFlags.all, "all", "a", false, "Compare everything (tests + logs)")
	rootCmd.AddCommand(diffCmd)
}
