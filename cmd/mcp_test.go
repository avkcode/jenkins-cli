package cmd

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/avkcode/jenkins-cli/pkg/mcp"
)

// TestMCPToolParity asserts that every operation we expose through the CLI has a
// corresponding MCP tool. Adding a new control-plane operation should add a tool
// (and an entry here), keeping CLI<->MCP coverage in lockstep.
func TestMCPToolParity(t *testing.T) {
	s := mcp.NewServer("jc", "test", strings.NewReader(""), io.Discard)
	registerMCPTools(s)
	got := make(map[string]bool)
	for _, n := range s.ToolNames() {
		got[n] = true
	}
	want := []string{
		// contexts
		"list_contexts",
		// jobs
		"list_jobs", "get_job", "job_info", "apply_job", "enable_job", "disable_job", "delete_job",
		// builds
		"build_job", "get_build", "get_build_log", "wait_for_build", "list_queue", "cancel_queue_item",
		// plugins
		"list_plugins", "check_plugin_updates", "install_plugin", "enable_plugin", "disable_plugin", "uninstall_plugin",
		// credentials
		"list_credentials", "create_credential", "delete_credential",
		// nodes
		"list_nodes", "node_info", "node_logs", "set_node_offline", "set_node_online",
		// controller (declarative)
		"controller_diff", "controller_apply",
		// system
		"system_info", "health", "safe_restart", "quiet_down", "run_groovy",
		// frozen builds
		"list_frozen_builds", "agent_exec", "agent_read_file", "agent_write_file", "agent_snapshot", "agent_reattempt", "thaw_build",
	}
	for _, w := range want {
		if !got[w] {
			t.Fatalf("MCP tool %q is missing (CLI<->MCP parity regression)", w)
		}
	}
}

func TestMCPCommandFlags(t *testing.T) {
	c, _, err := rootCmd.Find([]string{"mcp"})
	if err != nil {
		t.Fatalf("find mcp: %v", err)
	}
	for _, name := range []string{"read-only", "allow-script"} {
		if c.Flags().Lookup(name) == nil {
			t.Fatalf("mcp missing --%s flag", name)
		}
	}
}

func TestMCPResourcesAndPromptsRegistered(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"resources/templates/list"}`,
		`{"jsonrpc":"2.0","id":2,"method":"prompts/list"}`,
	}, "\n") + "\n")
	var out bytes.Buffer
	s := mcp.NewServer("jc", "test", in, &out)
	registerMCPResources(s)
	registerMCPPrompts(s)
	if err := s.Serve(context.Background()); err != nil {
		t.Fatalf("serve: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"jenkins://job/{name}/config.xml",
		"jenkins://job/{name}/builds/{number}/console",
		"diagnose_failing_build",
		"review_job_config",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}
