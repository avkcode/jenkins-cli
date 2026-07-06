package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	jclient "github.com/avkcode/jenkins-cli/pkg/client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// controllerManifest is the desired state of a Jenkins controller, applied
// declaratively (GitOps-style). Secrets are never stored inline: credential
// values are read from environment variables named by secretEnv.
type controllerManifest struct {
	Plugins     []manifestPlugin     `yaml:"plugins" json:"plugins"`
	Jobs        []manifestJob        `yaml:"jobs" json:"jobs"`
	Credentials []manifestCredential `yaml:"credentials" json:"credentials"`
}

type manifestPlugin struct {
	ID      string `yaml:"id" json:"id"`
	Version string `yaml:"version" json:"version"`
}

type manifestJob struct {
	Name       string `yaml:"name" json:"name"`
	ConfigFile string `yaml:"configFile" json:"configFile"`
	Config     string `yaml:"config" json:"config"`
}

type manifestCredential struct {
	ID          string `yaml:"id" json:"id"`
	SecretEnv   string `yaml:"secretEnv" json:"secretEnv"`
	Description string `yaml:"description" json:"description"`
}

// ctrlAction is a single reconciliation step computed by diffing desired vs live.
type ctrlAction struct {
	Kind string `json:"kind"` // plugin | job | credential
	Name string `json:"name"`
	Op   string `json:"op"` // install | create | update | unchanged
	Diff string `json:"diff,omitempty"`
}

const credentialListGroovy = `
def repo = com.cloudbees.plugins.credentials.CredentialsProvider.lookupCredentials(
    com.cloudbees.plugins.credentials.common.StandardCredentials.class,
    jenkins.model.Jenkins.instance, null, java.util.Collections.emptyList())
repo.each { c -> println "${c.id}" }
`

func loadControllerManifest(path string) (*controllerManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m controllerManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	return &m, nil
}

func jobDesiredXML(job manifestJob, baseDir string) (string, error) {
	if strings.TrimSpace(job.Config) != "" {
		return job.Config, nil
	}
	if job.ConfigFile == "" {
		return "", fmt.Errorf("job %q: one of config or configFile is required", job.Name)
	}
	p := job.ConfigFile
	if !filepath.IsAbs(p) {
		p = filepath.Join(baseDir, p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("job %q: %w", job.Name, err)
	}
	return string(data), nil
}

func installedPluginSet(ctx context.Context, jc *jclient.JenkinsClient) (map[string]bool, error) {
	plugins, err := jc.Client.GetPlugins(ctx, 1)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool)
	for _, p := range plugins.Raw.Plugins {
		set[p.ShortName] = true
	}
	return set, nil
}

func installedCredentialIDs(jc *jclient.JenkinsClient) (map[string]bool, error) {
	out, err := jc.ExecuteGroovy(credentialListGroovy)
	if err != nil {
		return nil, err
	}
	return parseCredentialIDs(out), nil
}

func parseCredentialIDs(out string) map[string]bool {
	ids := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		id := strings.TrimSpace(line)
		if id != "" {
			ids[id] = true
		}
	}
	return ids
}

// planController computes the reconciliation actions needed to make the live
// controller match the manifest.
func planController(ctx context.Context, jc *jclient.JenkinsClient, m *controllerManifest, baseDir string) ([]ctrlAction, error) {
	var actions []ctrlAction

	if len(m.Plugins) > 0 {
		installed, err := installedPluginSet(ctx, jc)
		if err != nil {
			return nil, fmt.Errorf("listing plugins: %w", err)
		}
		for _, p := range m.Plugins {
			op := "install"
			if installed[p.ID] {
				op = "unchanged"
			}
			actions = append(actions, ctrlAction{Kind: "plugin", Name: p.ID, Op: op})
		}
	}

	for _, j := range m.Jobs {
		desired, err := jobDesiredXML(j, baseDir)
		if err != nil {
			return nil, err
		}
		op := "create"
		var diff string
		if existing, getErr := jc.GetJobConfig(j.Name); getErr == nil {
			changed, d := renderDiff(existing, desired)
			if !changed {
				op = "unchanged"
			} else {
				op = "update"
				diff = d
			}
		}
		actions = append(actions, ctrlAction{Kind: "job", Name: j.Name, Op: op, Diff: diff})
	}

	if len(m.Credentials) > 0 {
		ids, err := installedCredentialIDs(jc)
		if err != nil {
			return nil, fmt.Errorf("listing credentials: %w", err)
		}
		for _, c := range m.Credentials {
			op := "create"
			if ids[c.ID] {
				op = "unchanged" // credential values are write-only; presence only
			}
			actions = append(actions, ctrlAction{Kind: "credential", Name: c.ID, Op: op})
		}
	}

	return actions, nil
}

func applyController(ctx context.Context, jc *jclient.JenkinsClient, m *controllerManifest, baseDir string, actions []ctrlAction) error {
	jobByName := make(map[string]manifestJob, len(m.Jobs))
	for _, j := range m.Jobs {
		jobByName[j.Name] = j
	}
	pluginByID := make(map[string]manifestPlugin, len(m.Plugins))
	for _, p := range m.Plugins {
		pluginByID[p.ID] = p
	}
	credByID := make(map[string]manifestCredential, len(m.Credentials))
	for _, c := range m.Credentials {
		credByID[c.ID] = c
	}

	for _, a := range actions {
		if a.Op == "unchanged" {
			continue
		}
		switch a.Kind {
		case "plugin":
			ver := pluginByID[a.Name].Version
			if ver == "" {
				ver = "latest"
			}
			fmt.Fprintf(os.Stderr, "installing plugin %s...\n", a.Name)
			if err := jc.InstallPlugin(ctx, a.Name, ver); err != nil {
				return fmt.Errorf("plugin %s: %w", a.Name, err)
			}
		case "job":
			desired, err := jobDesiredXML(jobByName[a.Name], baseDir)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "%s job %s...\n", a.Op, a.Name)
			if err := jc.CreateOrUpdateJob(a.Name, desired); err != nil {
				return fmt.Errorf("job %s: %w", a.Name, err)
			}
		case "credential":
			c := credByID[a.Name]
			if c.SecretEnv == "" {
				return fmt.Errorf("credential %s: secretEnv is required to supply the secret", a.Name)
			}
			secret := os.Getenv(c.SecretEnv)
			if secret == "" {
				return fmt.Errorf("credential %s: environment variable %s is empty", a.Name, c.SecretEnv)
			}
			fmt.Fprintf(os.Stderr, "creating credential %s...\n", a.Name)
			if err := jc.CreateCredential(a.Name, secret, c.Description); err != nil {
				return fmt.Errorf("credential %s: %w", a.Name, err)
			}
		}
		audit("controller.apply", a.Kind+"/"+a.Name+" "+a.Op)
	}
	return nil
}

func printControllerPlan(actions []ctrlAction, showDiff bool) {
	var changes int
	for _, a := range actions {
		marker := "  "
		switch a.Op {
		case "unchanged":
			marker = "= "
		default:
			marker = "~ "
			changes++
		}
		fmt.Fprintf(os.Stderr, "%s%s/%s: %s\n", marker, a.Kind, a.Name, a.Op)
		if showDiff && a.Diff != "" {
			for _, line := range strings.Split(strings.TrimRight(a.Diff, "\n"), "\n") {
				fmt.Fprintf(os.Stderr, "    %s\n", line)
			}
		}
	}
	fmt.Fprintf(os.Stderr, "plan: %d change(s), %d resource(s) total\n", changes, len(actions))
}

var (
	controllerFile     string
	controllerShowDiff bool
)

var controllerCmd = &cobra.Command{
	Use:     "controller",
	Short:   "Declaratively manage a whole Jenkins controller (GitOps)",
	Long:    "Apply or diff a controller.yaml manifest describing desired plugins, jobs, and credentials.",
	GroupID: GroupAdmin,
}

var controllerDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show drift between the manifest and the live controller",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		m, err := loadControllerManifest(controllerFile)
		if err != nil {
			return err
		}
		jc, err := getClient(ctx)
		if err != nil {
			return err
		}
		actions, err := planController(ctx, jc, m, filepath.Dir(controllerFile))
		if err != nil {
			return err
		}
		if outputIsStructured() {
			return renderStructured(actions)
		}
		printControllerPlan(actions, true)
		return nil
	},
}

var controllerApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Reconcile the live controller to match the manifest",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		m, err := loadControllerManifest(controllerFile)
		if err != nil {
			return err
		}
		jc, err := getClient(ctx)
		if err != nil {
			return err
		}
		baseDir := filepath.Dir(controllerFile)
		actions, err := planController(ctx, jc, m, baseDir)
		if err != nil {
			return err
		}
		printControllerPlan(actions, controllerShowDiff)
		if isDryRun() {
			dryRunMsg("no changes applied")
			return nil
		}
		if err := applyController(ctx, jc, m, baseDir, actions); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "controller reconciled")
		return nil
	},
}

func init() {
	controllerDiffCmd.Flags().StringVarP(&controllerFile, "file", "f", "", "controller manifest (controller.yaml)")
	controllerDiffCmd.MarkFlagRequired("file")
	controllerApplyCmd.Flags().StringVarP(&controllerFile, "file", "f", "", "controller manifest (controller.yaml)")
	controllerApplyCmd.Flags().BoolVar(&controllerShowDiff, "diff", false, "Show per-job diffs in the plan")
	controllerApplyCmd.MarkFlagRequired("file")

	controllerCmd.AddCommand(controllerDiffCmd)
	controllerCmd.AddCommand(controllerApplyCmd)
	rootCmd.AddCommand(controllerCmd)
}
