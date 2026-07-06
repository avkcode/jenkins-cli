package client

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFrozenBuildJSONParsing(t *testing.T) {
	sample := `[
    {
        "job": "test-pipeline",
        "build": 42,
        "node": "agent-01",
        "workspace": "/tmp/workspace/test-pipeline",
        "inputId": "abc123",
        "prompt": "Stage 'integration' failed"
    }
]`
	var builds []FrozenBuild
	if err := json.Unmarshal([]byte(sample), &builds); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(builds))
	}
	b := builds[0]
	if b.Job != "test-pipeline" || b.Build != 42 || b.Node != "agent-01" {
		t.Fatalf("wrong fields: %+v", b)
	}
	if b.Workspace != "/tmp/workspace/test-pipeline" || b.InputID != "abc123" {
		t.Fatalf("wrong workspace/input: %+v", b)
	}
}

func TestFrozenBuildEmpty(t *testing.T) {
	var builds []FrozenBuild
	if err := json.Unmarshal([]byte("[]"), &builds); err != nil {
		t.Fatalf("unmarshal empty: %v", err)
	}
	if len(builds) != 0 {
		t.Fatalf("expected empty, got %d", len(builds))
	}
}

func TestAgentExecResultJSONParsing(t *testing.T) {
	sample := `{"exitCode": 0, "stdout": "hello world\n", "stderr": ""}`
	var result AgentExecResult
	if err := json.Unmarshal([]byte(sample), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if result.Stdout != "hello world\n" {
		t.Fatalf("wrong stdout: %q", result.Stdout)
	}
	if result.Stderr != "" {
		t.Fatalf("wrong stderr: %q", result.Stderr)
	}
}

func TestAgentExecResultNonZero(t *testing.T) {
	sample := `{"exitCode": 1, "stdout": "", "stderr": "command not found\n"}`
	var result AgentExecResult
	if err := json.Unmarshal([]byte(sample), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("expected exit 1, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "command not found") {
		t.Fatalf("wrong stderr: %q", result.Stderr)
	}
}

func TestSignalInputBuildsOnClient(t *testing.T) {
	// SignalInput is the underlying implementation for ThawFrozenBuild.
	// Verify it's wired through correctly.
	jc := &JenkinsClient{}
	_ = jc.SignalInput // compile-time check: method exists
}
