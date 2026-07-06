package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/avkcode/jenkins-cli/pkg/client"
	"github.com/avkcode/jenkins-cli/pkg/output"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Exit codes
const (
	ExitSuccess       = 0
	ExitAuthError     = 1
	ExitNetworkError  = 2
	ExitTimeout       = 3
	ExitConfigError   = 4
	ExitNotFound      = 5
	ExitConflict      = 6
	ExitUsageError    = 7
	ExitInternalError = 99
)

var (
	cfgFile         string
	jenkinsURL      string
	username        string
	token           string
	outputFmt       string
	insecure        bool
	globalTimeout   time.Duration
	dryRun          bool
	logLevel        string
	contextOverride string

	// Version is set at build time via -ldflags="..."
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

var rootCmd = &cobra.Command{
	Use:     "jc",
	Short:   "Jenkins CLI",
	Long:    `jc is a high-performance CLI for Jenkins.`,
	Version: Version,
	// Group commands in help output
	CompletionOptions: cobra.CompletionOptions{HiddenDefaultCmd: true},
}

// Command groups for help output grouping
const (
	GroupCore   = "Core Commands"
	GroupAdmin  = "Administration"
	GroupConfig = "Configuration & Profiles"
)

func Execute() {
	err := rootCmd.Execute()
	auditInvocation(os.Args, err)
	if err != nil {
		err = friendlyError(err)
		code := classifyError(err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(code)
	}
}

// friendlyError rewrites opaque low-level errors into actionable messages. The
// most common one is a JSON decoder choking on an HTML response ("invalid
// character '<'"), which almost always means an auth failure, a redirect, or a
// wrong URL rather than a real parse problem.
func friendlyError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if contains(msg, "invalid character '<'", "looking for beginning of value", "<!DOCTYPE", "<html") {
		return fmt.Errorf("Jenkins returned an HTML page where JSON was expected; this usually means an authentication failure, a redirect, or the wrong --url. Verify your URL, user, and token (jc login / jc context use). underlying error: %s", msg)
	}
	return err
}

// classifyError returns an appropriate exit code based on the error.
func classifyError(err error) int {
	if err == nil {
		return ExitSuccess
	}
	msg := err.Error()
	switch {
	case contains(msg, "URL is not set", "credentials", "auth", "login", "unauthorized", "401", "403"):
		return ExitAuthError
	case contains(msg, "connection refused", "no such host", "dial tcp", "i/o timeout", "TLS"):
		return ExitNetworkError
	case contains(msg, "deadline exceeded", "timeout", "context deadline"):
		return ExitTimeout
	case contains(msg, "config", "not found", "404"):
		return ExitNotFound
	case contains(msg, "already exists", "conflict", "409"):
		return ExitConflict
	case contains(msg, "invalid", "required", "must", "usage", "flag"):
		return ExitUsageError
	default:
		return ExitInternalError
	}
}

func contains(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.SetVersionTemplate(fmt.Sprintf("jc version %s (commit: %s, built: %s)\n", Version, Commit, BuildDate))

	// Aliases for common operations
	lsCmd := &cobra.Command{Use: "ls", Short: "Alias for 'job list'", RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SetContext(cmd.Context())
		return jobListCmd.RunE(cmd, args)
	}, Hidden: true, GroupID: GroupCore}
	rmCmd := &cobra.Command{Use: "rm [job-name]", Short: "Alias for 'job delete'", Args: cobra.ExactArgs(1), RunE: jobDeleteCmd.RunE, Hidden: true}
	getCmd := &cobra.Command{Use: "get [job-name]", Short: "Alias for 'job get'", Args: cobra.ExactArgs(1), RunE: jobGetCmd.RunE, Hidden: true}
	applyCmd := &cobra.Command{Use: "apply [job-name]", Short: "Alias for 'job apply'", Args: cobra.ExactArgs(1), RunE: jobApplyCmd.RunE, Hidden: true}

	rootCmd.AddCommand(lsCmd, rmCmd, getCmd, applyCmd)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.jenkins-cli.yaml)")
	rootCmd.PersistentFlags().StringVar(&jenkinsURL, "url", "", "Jenkins Server URL")
	rootCmd.PersistentFlags().StringVarP(&username, "user", "u", "", "Jenkins Username")
	rootCmd.PersistentFlags().StringVarP(&token, "token", "t", "", "Jenkins API Token or Password")
	rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "table", "Output format: table, json, or yaml")
	rootCmd.PersistentFlags().BoolVarP(&insecure, "insecure", "k", false, "Skip TLS certificate verification")
	rootCmd.PersistentFlags().DurationVar(&globalTimeout, "timeout", 30*time.Second, "Global timeout for commands (0 = no timeout)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Preview changes without executing them")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn, error")
	rootCmd.PersistentFlags().StringVar(&contextOverride, "context", "", "Context to use for this command (overrides current-context)")

	viper.BindPFlag("url", rootCmd.PersistentFlags().Lookup("url"))
	viper.BindPFlag("user", rootCmd.PersistentFlags().Lookup("user"))
	viper.BindPFlag("token", rootCmd.PersistentFlags().Lookup("token"))
	viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	viper.BindPFlag("insecure", rootCmd.PersistentFlags().Lookup("insecure"))
	viper.BindPFlag("timeout", rootCmd.PersistentFlags().Lookup("timeout"))

	// Command group assignments
	rootCmd.AddGroup(&cobra.Group{ID: GroupCore, Title: GroupCore})
	rootCmd.AddGroup(&cobra.Group{ID: GroupAdmin, Title: GroupAdmin})
	rootCmd.AddGroup(&cobra.Group{ID: GroupConfig, Title: GroupConfig})
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".jenkins-cli")
	}

	viper.SetEnvPrefix("JENKINS")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		validateConfig()
	} else if cfgFile != "" {
		fmt.Fprintf(os.Stderr, "Warning: could not read config file: %v\n", err)
	}
}

func validateConfig() {
	contexts := viper.GetStringMap("contexts")
	for name, v := range contexts {
		c, ok := v.(map[string]interface{})
		if !ok {
			fmt.Fprintf(os.Stderr, "Warning: context %q is not a valid map, ignoring\n", name)
			continue
		}
		if _, ok := c["url"]; !ok {
			fmt.Fprintf(os.Stderr, "Warning: context %q is missing required 'url' field\n", name)
		}
	}
	current := viper.GetString("current-context")
	if current != "" {
		if _, ok := contexts[current]; !ok {
			fmt.Fprintf(os.Stderr, "Warning: current-context %q not found in contexts\n", current)
		}
	}
}

// debugf prints debug-level messages to stderr when log-level is debug.
func debugf(format string, args ...interface{}) {
	if logLevel == "debug" {
		fmt.Fprintf(os.Stderr, "[debug] "+format+"\n", args...)
	}
}

// isDryRun returns true if --dry-run is active.
func isDryRun() bool { return dryRun }

// dryRunMsg prints a dry-run notice to stderr.
func dryRunMsg(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[dry-run] "+format+"\n", args...)
}

// getTimeoutContext returns a context with the global timeout applied.
func getTimeoutContext(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := viper.GetDuration("timeout")
	if timeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, timeout)
}

func getClient(ctx context.Context) (*client.JenkinsClient, error) {
	return getClientWithContext(ctx, contextOverride)
}

// getClientWithContext connects using an explicit context name (empty = the
// configured current-context), then applies any --url/--user/--token overrides.
func getClientWithContext(ctx context.Context, ctxName string) (*client.JenkinsClient, error) {
	audit("getClient", "connect")

	explicit := ctxName
	currentContext := ctxName
	if currentContext == "" {
		currentContext = viper.GetString("current-context")
	}
	var url, user, tok string

	if currentContext != "" {
		contexts := viper.GetStringMap("contexts")
		rawCtx, ok := contexts[currentContext]
		if !ok {
			if explicit != "" {
				return nil, fmt.Errorf("context %q not found; run 'jc context list'", currentContext)
			}
		} else if c, ok := rawCtx.(map[string]interface{}); ok {
			if v, ok := c["url"].(string); ok {
				url = v
			}
			if v, ok := c["user"].(string); ok {
				user = v
			}
			if v, ok := c["token"].(string); ok {
				tok = v
			}
		}
	}

	if u := viper.GetString("url"); u != "" {
		url = u
	}
	if u := viper.GetString("user"); u != "" {
		user = u
	}
	if t := viper.GetString("token"); t != "" {
		tok = t
	}

	if url == "" {
		return nil, fmt.Errorf("no Jenkins URL set. Run 'jc login --url <url> --user <user> --token <token>' or 'jc context set <name> --url <url>'")
	}
	if user == "" || tok == "" {
		return nil, fmt.Errorf("no Jenkins credentials set. Run 'jc login --url %s --user <user> --token <token>'", url)
	}

	jc, err := client.NewClient(ctx, url, user, tok)
	if err != nil {
		return nil, err
	}
	return jc, nil
}

func getOutput() *output.Writer {
	return output.NewWriter(os.Stdout, viper.GetString("output"))
}

// outputIsStructured reports whether the requested output format is machine
// readable (json or yaml).
func outputIsStructured() bool {
	f := viper.GetString("output")
	return f == "json" || f == "yaml"
}

// renderStructured prints v as JSON or YAML according to the output format.
func renderStructured(v interface{}) error {
	if viper.GetString("output") == "yaml" {
		return getOutput().PrintYAML(v)
	}
	return getOutput().PrintJSON(v)
}

// CI convenience functions for GitHub Actions output formatting.

// ciGroup starts a collapsible group in GitHub Actions.
func ciGroup(name string) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		fmt.Fprintf(os.Stdout, "::group::%s\n", name)
	}
}

// ciEndGroup ends a collapsible group.
func ciEndGroup() {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		fmt.Fprintln(os.Stdout, "::endgroup::")
	}
}

// ciNotice prints a GitHub Actions notice annotation.
func ciNotice(title, message string) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		fmt.Fprintf(os.Stdout, "::notice title=%s::%s\n", title, message)
	}
}

// audit writes a timestamped audit entry to ~/.jenkins-cli-audit.log.
func audit(action, detail string) {
	if os.Getenv("JC_NO_AUDIT") != "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".jenkins-cli-audit.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(f, "%s [%s] %s - %s\n", ts, os.Getenv("USER"), action, detail)
}

// auditInvocation records every jc invocation (command, redacted args, context,
// result) so there is a who/what/when trail of operations across controllers.
func auditInvocation(args []string, err error) {
	result := "ok"
	if err != nil {
		result = "error: " + err.Error()
	}
	ctxName := contextOverride
	if ctxName == "" {
		ctxName = viper.GetString("current-context")
	}
	if ctxName == "" {
		ctxName = "-"
	}
	audit("invocation", fmt.Sprintf("context=%s cmd=[%s] result=%s", ctxName, redactArgs(args), result))
}

// redactArgs renders a command line for the audit log with secret values masked
// (token/password flags and the credential-create secret positionals).
func redactArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	parts = append(parts, filepath.Base(args[0]))
	rest := args[1:]
	credCreate := containsSeq(rest, "credential", "create")
	redactNext := false
	seenCreate := false
	credPos := 0
	for _, a := range rest {
		switch {
		case redactNext:
			parts = append(parts, "***")
			redactNext = false
		case a == "--token" || a == "-t" || a == "--password":
			parts = append(parts, a)
			redactNext = true
		case strings.HasPrefix(a, "--token=") || strings.HasPrefix(a, "--password=") || strings.HasPrefix(a, "-t="):
			parts = append(parts, a[:strings.IndexByte(a, '=')]+"=***")
		default:
			if credCreate && seenCreate && !strings.HasPrefix(a, "-") {
				credPos++
				if credPos >= 2 { // 1st positional is the id; mask the secret/description
					parts = append(parts, "***")
					continue
				}
			}
			if a == "create" {
				seenCreate = true
			}
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}

// containsSeq reports whether a appears in args followed (later) by b.
func containsSeq(args []string, a, b string) bool {
	ai := -1
	for i, s := range args {
		if s == a {
			ai = i
			break
		}
	}
	if ai < 0 {
		return false
	}
	for _, s := range args[ai+1:] {
		if s == b {
			return true
		}
	}
	return false
}
