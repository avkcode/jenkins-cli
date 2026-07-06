package client

import (
	"strings"
	"testing"

	"github.com/bndr/gojenkins"
)

func TestDiagnoseDoctorLogFindsPipelineScriptSyntaxError(t *testing.T) {
	logs := strings.Join([]string{
		"[Pipeline] { (Build kernel-04)",
		"+ VARIANT=kernel-04",
		"+ python3 -",
		"  File \"<stdin>\", line 9",
		"    manifest.write_text(\"bad",
		"                        ^",
		"SyntaxError: unterminated string literal (detected at line 9)",
		"[Pipeline] }",
		"script returned exit code 1",
		"Finished: FAILURE",
	}, "\n")

	report := diagnoseDoctorLog("firecracker-linux-kernel-10way-latest", 4, "FAILURE", "http://jenkins/job/x/4/", 37000, logs)

	if report.Category != "pipeline-script-error" {
		t.Fatalf("category = %q, want pipeline-script-error", report.Category)
	}
	if report.ExactStage != "Build kernel-04" {
		t.Fatalf("stage = %q, want Build kernel-04", report.ExactStage)
	}
	if report.ExactBranch != "kernel-04" {
		t.Fatalf("branch = %q, want kernel-04", report.ExactBranch)
	}
	if !strings.Contains(report.LikelyCause, "SyntaxError") {
		t.Fatalf("likely cause did not include SyntaxError: %q", report.LikelyCause)
	}
	if len(report.RelevantLogLines) == 0 || !containsDoctorLine(report.RelevantLogLines, "unterminated string literal") {
		t.Fatalf("relevant log lines did not include syntax error: %#v", report.RelevantLogLines)
	}
	if !strings.Contains(report.RerunCommand, "jc job build firecracker-linux-kernel-10way-latest --logs") {
		t.Fatalf("unexpected rerun command: %q", report.RerunCommand)
	}
}

func TestDiagnoseDoctorLogFindsFirecrackerFleetConnectivity(t *testing.T) {
	logs := strings.Join([]string{
		"[Pipeline] { (Provision agent)",
		"[Pipeline] firecrackerAgent",
		"java.net.ConnectException: Connection refused",
		"Circuit breaker open - fleet host communication suspended",
		"Finished: FAILURE",
	}, "\n")

	report := diagnoseDoctorLog("kernel", 3, "FAILURE", "", 12000, logs)

	if report.Category != "firecracker-fleet-connectivity" {
		t.Fatalf("category = %q, want firecracker-fleet-connectivity", report.Category)
	}
	if report.ExactStage != "Provision agent" {
		t.Fatalf("stage = %q, want Provision agent", report.ExactStage)
	}
	if !containsDoctorEvidence(report.Evidence, "fleet") {
		t.Fatalf("evidence did not mention fleet: %#v", report.Evidence)
	}
	if !containsDoctorFix(report.SuggestedFixes, "hostd") {
		t.Fatalf("suggested fixes did not mention hostd: %#v", report.SuggestedFixes)
	}
	if len(report.AgentLifecycle) == 0 {
		t.Fatal("expected agent lifecycle events")
	}
}

func TestDiagnoseDoctorLogFindsAgentLossAfterRestart(t *testing.T) {
	logs := strings.Join([]string{
		"[Pipeline] { (Build kernel-01)",
		"Running on firecracker-agent-01 in /tmp/ws",
		"Pausing (shutting down)",
		"Resuming build at Fri Jun 12 12:00:00 UTC 2026 after Jenkins restart",
		"Agent firecracker-agent-01 seems to be removed or offline",
		"hudson.remoting.ChannelClosedException: Channel closed",
		"Finished: ABORTED",
	}, "\n")

	report := diagnoseDoctorLog("kernel", 1, "ABORTED", "", 90000, logs)

	if report.Category != "agent-lost" {
		t.Fatalf("category = %q, want agent-lost", report.Category)
	}
	if report.ExactStage != "Build kernel-01" {
		t.Fatalf("stage = %q, want Build kernel-01", report.ExactStage)
	}
	if report.ExactBranch != "kernel-01" {
		t.Fatalf("branch = %q, want kernel-01", report.ExactBranch)
	}
	if !containsAgentEvent(report.AgentLifecycle, "controller restart/resume") {
		t.Fatalf("missing restart lifecycle event: %#v", report.AgentLifecycle)
	}
	if !containsAgentEvent(report.AgentLifecycle, "agent lost") {
		t.Fatalf("missing agent-lost lifecycle event: %#v", report.AgentLifecycle)
	}
}

func TestAnalyzeDoctorPipelineScriptAndArtifacts(t *testing.T) {
	script := `
pipeline {
  agent { label 'linux && firecracker' }
  environment {
    CACHE_ROOT = '/tmp/cache'
    TOKEN = credentials('registry-token')
  }
  stages {
    stage('Checkout') { steps { checkout scm } }
    stage('Build') {
      steps {
        withEnv(['JOBS=2']) {
          archiveArtifacts artifacts: 'kernel-*/manifest.json'
        }
      }
    }
  }
}
`
	model := analyzeDoctorPipelineScript("test", script)
	if model.Kind != "declarative" {
		t.Fatalf("kind = %q, want declarative", model.Kind)
	}
	if !containsString(model.Stages, "Checkout") || !containsString(model.Stages, "Build") {
		t.Fatalf("stages missing expected values: %#v", model.Stages)
	}
	if !containsString(model.Agents, "label:linux && firecracker") {
		t.Fatalf("agents missing label: %#v", model.Agents)
	}
	if !containsString(model.EnvVars, "CACHE_ROOT") || !containsString(model.EnvVars, "JOBS") {
		t.Fatalf("env vars missing expected values: %#v", model.EnvVars)
	}
	if !containsString(model.Credentials, "registry-token") {
		t.Fatalf("credentials missing expected value: %#v", model.Credentials)
	}
	if !containsString(model.Artifacts, "kernel-*/manifest.json") {
		t.Fatalf("artifacts missing expected value: %#v", model.Artifacts)
	}

	summary := summarizeDoctorArtifacts([]gojenkins.Artifact{
		{FileName: "manifest.json", Path: "job/kernel/5/artifact/kernel-01/manifest.json"},
		{FileName: "sha256.txt", Path: "job/kernel/5/artifact/kernel-01/sha256.txt"},
		{FileName: "hostd-evidence.tgz", Path: "job/kernel/5/artifact/.firecracker-evidence/hostd-evidence.tgz"},
	})
	if summary.Total != 3 {
		t.Fatalf("total = %d, want 3", summary.Total)
	}
	if len(summary.Manifests) != 1 || len(summary.Checksums) != 1 || len(summary.EvidenceBundles) != 1 {
		t.Fatalf("unexpected artifact grouping: %#v", summary)
	}
}

func containsDoctorLine(lines []DoctorLogLine, needle string) bool {
	for _, line := range lines {
		if strings.Contains(line.Text, needle) {
			return true
		}
	}
	return false
}

func containsDoctorEvidence(items []string, needle string) bool {
	for _, item := range items {
		if strings.Contains(strings.ToLower(item), strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func containsDoctorFix(items []string, needle string) bool {
	return containsDoctorEvidence(items, needle)
}

func containsAgentEvent(events []DoctorAgentEvent, event string) bool {
	for _, item := range events {
		if item.Event == event {
			return true
		}
	}
	return false
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
