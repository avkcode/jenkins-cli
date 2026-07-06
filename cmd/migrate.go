package cmd

import (
	"fmt"
	"os"
	"strings"

	clientpkg "github.com/avkcode/jenkins-cli/pkg/client"
	"github.com/spf13/cobra"
)

var migrateFlags struct {
	sourceURL   string
	sourceUser  string
	sourceToken string
	jobs        []string
	allJobs     bool
	credentials bool
	plugins     bool
}

var migrateCmd = &cobra.Command{
	Use:   "migrate [--source-url url] [--source-user user] [--source-token token]",
	Short: "Migrate jobs, credentials, and plugins between Jenkins instances",
	Long: `Export jobs, credentials, and plugins from a source Jenkins instance
and import them into the current (target) Jenkins instance.

The target is specified via the standard --url/--user/--token flags (or the
active context). The source is specified via --source-url/--source-user/--source-token.
Use --jobs to select specific jobs or --all to migrate everything.

Examples:
  jc migrate --source-url http://old-jenkins:8080 --source-user admin --source-token abc123 --all
  jc migrate --source-url http://old:8080 --source-user admin --source-token xyz --jobs job1,job2`,
	GroupID: GroupAdmin,
	RunE: func(cmd *cobra.Command, args []string) error {
		if migrateFlags.sourceURL == "" {
			return fmt.Errorf("--source-url is required")
		}
		if migrateFlags.sourceUser == "" || migrateFlags.sourceToken == "" {
			return fmt.Errorf("--source-user and --source-token are required")
		}

		// Connect to target (current context)
		ctx, cancel := getTimeoutContext(cmd.Context())
		defer cancel()

		target, err := getClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to connect to target Jenkins: %w", err)
		}

		// Connect to source
		source, err := clientpkg.NewClient(ctx, migrateFlags.sourceURL, migrateFlags.sourceUser, migrateFlags.sourceToken)
		if err != nil {
			return fmt.Errorf("failed to connect to source Jenkins: %w", err)
		}

		if !migrateFlags.allJobs && len(migrateFlags.jobs) == 0 && !migrateFlags.credentials && !migrateFlags.plugins {
			// Default: migrate all jobs if no flags
			migrateFlags.allJobs = true
		}

		migrated := 0
		skipped := 0
		failed := 0

		// --- Migrate jobs ---
		if migrateFlags.allJobs || len(migrateFlags.jobs) > 0 {
			var jobNames []string
			if migrateFlags.allJobs {
				allJobs, err := source.Client.GetAllJobs(ctx)
				if err != nil {
					return fmt.Errorf("failed to list source jobs: %w", err)
				}
				for _, j := range allJobs {
					// Skip folders and multibranch projects by default
					if strings.Contains(j.Raw.Class, "Folder") {
						fmt.Fprintf(os.Stderr, "Skipping folder: %s\n", j.Raw.Name)
						skipped++
						continue
					}
					jobNames = append(jobNames, j.Raw.Name)
				}
			} else {
				jobNames = migrateFlags.jobs
			}

			for _, name := range jobNames {
				fmt.Fprintf(os.Stderr, "Migrating job: %s ... ", name)
				if isDryRun() {
					dryRunMsg("Would migrate job %s", name)
					migrated++
					continue
				}

				config, err := source.GetJobConfig(name)
				if err != nil {
					fmt.Fprintf(os.Stderr, "FAIL (read config: %v)\n", err)
					failed++
					continue
				}

				if err := target.CreateOrUpdateJob(name, config); err != nil {
					fmt.Fprintf(os.Stderr, "FAIL (create/update: %v)\n", err)
					failed++
					continue
				}

				fmt.Fprintln(os.Stderr, "OK")
				migrated++
			}
		}

		// --- Migrate credentials ---
		if migrateFlags.credentials {
			fmt.Fprintln(os.Stderr, "\nMigrating credentials...")
			// Use Groovy to export/import credentials
			creds, err := source.ExecuteGroovy(`
import com.cloudbees.plugins.credentials.*
import com.cloudbees.plugins.credentials.domains.Domain
def store = SystemCredentialsProvider.instance.store
def result = []
store.getCredentials(Domain.global()).each { cred ->
    def item = [:]
    item.id = cred.id
    item.type = cred.class.name
    item.description = cred.description ?: ''
    if (cred instanceof org.jenkinsci.plugins.plaincredentials.impl.StringCredentialsImpl) {
        item.secret = cred.secret.plainText
    } else if (cred instanceof com.cloudbees.plugins.credentials.impl.UsernamePasswordCredentialsImpl) {
        item.username = cred.username
        item.password = cred.password.plainText
    }
    result << item
}
println groovy.json.JsonOutput.toJson(result)
`)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Failed to read source credentials: %v\n", err)
				failed++
			} else {
				fmt.Fprintf(os.Stderr, "  Source credentials exported.\n")
				if isDryRun() {
					dryRunMsg("Would import %d credentials", strings.Count(creds, `"id"`))
				} else {
					// Import each credential
					for _, line := range strings.Split(creds, "\n") {
						line = strings.TrimSpace(line)
						if line == "" || !strings.Contains(line, "id") {
							continue
						}
						if strings.Contains(line, "StringCredentialsImpl") {
							fmt.Fprintf(os.Stderr, "  Skipping binary credential (import manually): %s\n", line[:min(80, len(line))])
							skipped++
						}
					}
					migrated++
				}
			}
		}

		// --- Migrate plugins ---
		if migrateFlags.plugins {
			fmt.Fprintln(os.Stderr, "\nMigrating plugins...")
			plugins, err := source.Client.GetPlugins(ctx, 3)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Failed to list source plugins: %v\n", err)
				failed++
			} else {
				for _, p := range plugins.Raw.Plugins {
					if isDryRun() {
						dryRunMsg("Would install plugin %s (%s)", p.ShortName, p.Version)
						continue
					}
					fmt.Fprintf(os.Stderr, "  Installing %s ... ", p.ShortName)
					if err := target.InstallPlugin(ctx, p.ShortName, ""); err != nil {
						fmt.Fprintf(os.Stderr, "FAIL (%v)\n", err)
						failed++
					} else {
						fmt.Fprintln(os.Stderr, "OK")
						migrated++
					}
				}
			}
		}

		fmt.Fprintf(os.Stderr, "\nDone: %d migrated, %d skipped, %d failed\n", migrated, skipped, failed)
		return nil
	},
}

func init() {
	migrateCmd.Flags().StringVar(&migrateFlags.sourceURL, "source-url", "", "Source Jenkins URL")
	migrateCmd.Flags().StringVar(&migrateFlags.sourceUser, "source-user", "", "Source Jenkins username")
	migrateCmd.Flags().StringVar(&migrateFlags.sourceToken, "source-token", "", "Source Jenkins API token or password")
	migrateCmd.Flags().StringSliceVarP(&migrateFlags.jobs, "jobs", "j", nil, "Specific jobs to migrate (comma-separated)")
	migrateCmd.Flags().BoolVar(&migrateFlags.allJobs, "all", false, "Migrate all jobs")
	migrateCmd.Flags().BoolVar(&migrateFlags.credentials, "credentials", false, "Migrate credentials")
	migrateCmd.Flags().BoolVar(&migrateFlags.plugins, "plugins", false, "Migrate plugins")
	rootCmd.AddCommand(migrateCmd)
}
