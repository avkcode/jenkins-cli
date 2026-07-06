package client

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/bndr/gojenkins"
)

type DoctorReport struct {
	JobName             string                    `json:"job_name" yaml:"job_name"`
	BuildNumber         int64                     `json:"build_number" yaml:"build_number"`
	Result              string                    `json:"result" yaml:"result"`
	BuildURL            string                    `json:"build_url,omitempty" yaml:"build_url,omitempty"`
	DurationMs          int64                     `json:"duration_ms,omitempty" yaml:"duration_ms,omitempty"`
	Category            string                    `json:"category" yaml:"category"`
	Confidence          string                    `json:"confidence" yaml:"confidence"`
	LikelyCause         string                    `json:"likely_cause" yaml:"likely_cause"`
	ExactStage          string                    `json:"exact_stage,omitempty" yaml:"exact_stage,omitempty"`
	ExactBranch         string                    `json:"exact_branch,omitempty" yaml:"exact_branch,omitempty"`
	Evidence            []string                  `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	RelevantLogLines    []DoctorLogLine           `json:"relevant_log_lines,omitempty" yaml:"relevant_log_lines,omitempty"`
	SuggestedFixes      []string                  `json:"suggested_fixes,omitempty" yaml:"suggested_fixes,omitempty"`
	RerunCommand        string                    `json:"rerun_command" yaml:"rerun_command"`
	PipelineStaticModel DoctorPipelineStaticModel `json:"pipeline_static_model" yaml:"pipeline_static_model"`
	StageTimings        []DoctorStageTiming       `json:"stage_timings,omitempty" yaml:"stage_timings,omitempty"`
	AgentLifecycle      []DoctorAgentEvent        `json:"agent_lifecycle,omitempty" yaml:"agent_lifecycle,omitempty"`
	Artifacts           DoctorArtifactSummary     `json:"artifacts" yaml:"artifacts"`
	Warnings            []string                  `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

type DoctorLogLine struct {
	Number int    `json:"number" yaml:"number"`
	Text   string `json:"text" yaml:"text"`
}

type DoctorPipelineStaticModel struct {
	Source      string   `json:"source" yaml:"source"`
	Kind        string   `json:"kind" yaml:"kind"`
	Stages      []string `json:"stages,omitempty" yaml:"stages,omitempty"`
	Agents      []string `json:"agents,omitempty" yaml:"agents,omitempty"`
	EnvVars     []string `json:"env_vars,omitempty" yaml:"env_vars,omitempty"`
	Credentials []string `json:"credentials,omitempty" yaml:"credentials,omitempty"`
	Artifacts   []string `json:"artifacts,omitempty" yaml:"artifacts,omitempty"`
	Warnings    []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

type DoctorStageTiming struct {
	Name            string `json:"name" yaml:"name"`
	Status          string `json:"status,omitempty" yaml:"status,omitempty"`
	StartTimeMillis int64  `json:"start_time_millis,omitempty" yaml:"start_time_millis,omitempty"`
	DurationMillis  int64  `json:"duration_millis,omitempty" yaml:"duration_millis,omitempty"`
	PauseMillis     int64  `json:"pause_millis,omitempty" yaml:"pause_millis,omitempty"`
}

type DoctorAgentEvent struct {
	Line   int    `json:"line,omitempty" yaml:"line,omitempty"`
	Event  string `json:"event" yaml:"event"`
	Agent  string `json:"agent,omitempty" yaml:"agent,omitempty"`
	Stage  string `json:"stage,omitempty" yaml:"stage,omitempty"`
	Branch string `json:"branch,omitempty" yaml:"branch,omitempty"`
	Text   string `json:"text,omitempty" yaml:"text,omitempty"`
}

type DoctorArtifactSummary struct {
	Total           int      `json:"total" yaml:"total"`
	EvidenceBundles []string `json:"evidence_bundles,omitempty" yaml:"evidence_bundles,omitempty"`
	Manifests       []string `json:"manifests,omitempty" yaml:"manifests,omitempty"`
	Checksums       []string `json:"checksums,omitempty" yaml:"checksums,omitempty"`
	OtherExamples   []string `json:"other_examples,omitempty" yaml:"other_examples,omitempty"`
}

type doctorCandidate struct {
	category     string
	likelyCause  string
	confidence   string
	priority     int
	lineNumber   int
	lineText     string
	stage        string
	branch       string
	evidence     string
	suggestedFix []string
}

type doctorLineContext struct {
	stage  string
	branch string
	agent  string
}

var (
	pipelineContextPattern    = regexp.MustCompile(`^\[Pipeline\]\s+\{\s+\(([^)]+)\)`)
	runningOnPattern          = regexp.MustCompile(`Running on ([^ ]+)(?: in |$)`)
	nodeAllocatedPattern      = regexp.MustCompile(`(?:Agent|Node) ([^ ]+) (?:is online|connected|ready)`)
	localVariantPattern       = regexp.MustCompile(`\b(kernel-[0-9]{2})\b`)
	stageCallDoctorPattern    = regexp.MustCompile(`(?s)\bstage\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	labelDoctorPattern        = regexp.MustCompile(`(?s)\blabel\s+['"]([^'"]+)['"]`)
	nodeDoctorPattern         = regexp.MustCompile(`(?s)\bnode\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	envAssignmentPattern      = regexp.MustCompile(`(?m)^\s*([A-Z_][A-Z0-9_]*)\s*=`)
	withEnvPattern            = regexp.MustCompile(`(?s)\bwithEnv\s*\(\s*\[([^\]]*)\]`)
	credentialsDoctorPattern  = regexp.MustCompile(`(?s)\bcredentials\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	withCredentialsPattern    = regexp.MustCompile(`(?s)\bwithCredentials\s*\(\s*\[([^\]]*)\]`)
	archiveArtifactsPattern   = regexp.MustCompile(`(?s)\barchiveArtifacts\s*(?:\(|\s)(?:artifacts\s*:\s*)?['"]([^'"]+)['"]`)
	imageDoctorPattern        = regexp.MustCompile(`(?s)\bimage\s+['"]([^'"]+)['"]`)
	firecrackerStepPatternDoc = regexp.MustCompile(`\b(?:firecrackerAgent|aeroAgent|firecracker)\b`)
)

// DoctorReport collects build facts and produces an explainable CI diagnosis.
func (jc *JenkinsClient) DoctorReport(jobName string, buildNumber int64) (DoctorReport, error) {
	build, err := jc.Client.GetBuild(jc.ctx, jobName, buildNumber)
	if err != nil {
		return DoctorReport{}, err
	}

	logs := sanitizeDoctorConsoleLog(build.GetConsoleOutput(jc.ctx))
	report := diagnoseDoctorLog(jobName, buildNumber, build.GetResult(), build.GetUrl(), int64(build.GetDuration()), logs)

	staticModel, warning := jc.collectDoctorStaticModel(jobName, buildNumber)
	report.PipelineStaticModel = staticModel
	if warning != "" {
		report.Warnings = append(report.Warnings, warning)
	}

	stageTimings, warning := jc.collectDoctorStageTimings(jobName, buildNumber)
	if warning != "" {
		report.Warnings = append(report.Warnings, warning)
	}
	report.StageTimings = stageTimings
	mergeStageFacts(&report)

	report.Artifacts = summarizeDoctorArtifacts(build.GetArtifacts())
	if report.Artifacts.Total > 0 {
		report.Evidence = append(report.Evidence, fmt.Sprintf("Build published %d artifact(s), including %d evidence-related file(s).", report.Artifacts.Total, len(report.Artifacts.EvidenceBundles)+len(report.Artifacts.Manifests)+len(report.Artifacts.Checksums)))
	}
	if build.Raw != nil && build.Raw.BuiltOn != "" {
		report.AgentLifecycle = append([]DoctorAgentEvent{{
			Event: "build assigned",
			Agent: build.Raw.BuiltOn,
			Text:  "Jenkins build metadata reports builtOn=" + build.Raw.BuiltOn,
		}}, report.AgentLifecycle...)
	}

	if len(report.PipelineStaticModel.Stages) > 0 {
		report.Evidence = append(report.Evidence, fmt.Sprintf("Static pipeline model found %d stage(s).", len(report.PipelineStaticModel.Stages)))
	}
	if len(report.StageTimings) > 0 {
		report.Evidence = append(report.Evidence, fmt.Sprintf("Stage timing API returned %d stage node(s).", len(report.StageTimings)))
	}
	if len(report.AgentLifecycle) > 0 {
		report.Evidence = append(report.Evidence, fmt.Sprintf("Console/build metadata exposed %d agent lifecycle event(s).", len(report.AgentLifecycle)))
	}
	report.Evidence = uniqueStrings(report.Evidence, 12)
	if report.RerunCommand == "" {
		report.RerunCommand = buildDoctorRerunCommand(jobName)
	}
	return report, nil
}

func diagnoseDoctorLog(jobName string, buildNumber int64, result string, buildURL string, durationMs int64, logs string) DoctorReport {
	lines := splitDoctorLines(logs)
	contexts := scanDoctorContexts(lines)
	candidate := chooseDoctorCandidate(lines, contexts)

	report := DoctorReport{
		JobName:      jobName,
		BuildNumber:  buildNumber,
		Result:       result,
		BuildURL:     buildURL,
		DurationMs:   durationMs,
		Category:     candidate.category,
		Confidence:   candidate.confidence,
		LikelyCause:  candidate.likelyCause,
		ExactStage:   candidate.stage,
		ExactBranch:  candidate.branch,
		RerunCommand: buildDoctorRerunCommand(jobName),
	}
	if report.Category == "" {
		report.Category = "unknown"
		report.Confidence = "low"
		report.LikelyCause = "No high-confidence failure signature was found in the console log. Review the relevant log window and stage timing for the first non-successful step."
	}
	if candidate.evidence != "" {
		report.Evidence = append(report.Evidence, candidate.evidence)
	}
	report.Evidence = append(report.Evidence, fmt.Sprintf("Build result is %s after %s.", emptyAs(result, "UNKNOWN"), formatDoctorDuration(durationMs)))
	if candidate.lineNumber > 0 {
		report.RelevantLogLines = relevantDoctorLogLines(lines, candidate.lineNumber, 6, 12)
	} else {
		report.RelevantLogLines = tailDoctorLogLines(lines, 24)
	}
	report.AgentLifecycle = parseDoctorAgentLifecycle(lines, contexts)
	report.SuggestedFixes = candidate.suggestedFix
	if len(report.SuggestedFixes) == 0 {
		report.SuggestedFixes = []string{
			"Open the first failing stage in the relevant log window and fix the command or infrastructure error reported there.",
			"Rerun with `jc job build --logs` so the streamed console confirms the same failure signature is gone.",
		}
	}
	refineDoctorLocationFromLogs(&report)
	return report
}

func chooseDoctorCandidate(lines []string, contexts []doctorLineContext) doctorCandidate {
	best := doctorCandidate{}
	for i, line := range lines {
		candidate, ok := classifyDoctorLine(line)
		if !ok {
			continue
		}
		candidate.lineNumber = i + 1
		candidate.lineText = line
		if i < len(contexts) {
			candidate.stage = contexts[i].stage
			candidate.branch = contexts[i].branch
		}
		candidate.evidence = fmt.Sprintf("Line %d matched %s: %s", candidate.lineNumber, candidate.category, strings.TrimSpace(line))
		if betterDoctorCandidate(candidate, best) {
			best = candidate
		}
	}
	return best
}

func betterDoctorCandidate(candidate, current doctorCandidate) bool {
	if current.priority == 0 {
		return true
	}
	if candidate.priority != current.priority {
		return candidate.priority > current.priority
	}
	return candidate.lineNumber < current.lineNumber
}

func classifyDoctorLine(line string) (doctorCandidate, bool) {
	lower := strings.ToLower(line)
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.Contains(trimmed, "SyntaxError:"):
		return doctorCandidate{
			category:    "pipeline-script-error",
			likelyCause: "A pipeline shell/Python script failed with " + trimmed,
			confidence:  "high",
			priority:    100,
			suggestedFix: []string{
				"Fix the generated script or heredoc/quote escaping in the failing stage.",
				"Run the embedded script locally or with a lightweight syntax check before rerunning the Jenkins job.",
			},
		}, true
	case strings.Contains(lower, "circuit breaker open") || strings.Contains(lower, "fleet host communication suspended"):
		return doctorCandidate{
			category:    "firecracker-fleet-connectivity",
			likelyCause: "Firecracker agent provisioning failed because Jenkins could not communicate with the fleet host or hostd endpoint.",
			confidence:  "high",
			priority:    98,
			suggestedFix: []string{
				"Check the Firecracker hostd service, fleet endpoint URL, port reachability, and circuit breaker state.",
				"After hostd health is green, rerun with log streaming and confirm agent lifecycle events reach the running state.",
			},
		}, true
	case strings.Contains(lower, "java.net.connectexception") || strings.Contains(lower, "connection refused"):
		return doctorCandidate{
			category:    "network-connectivity",
			likelyCause: "Jenkins hit a network connection failure while the build was running or provisioning an agent.",
			confidence:  "medium",
			priority:    92,
			suggestedFix: []string{
				"Verify the target service is listening and reachable from the Jenkins controller or agent.",
				"If this happened during Firecracker provisioning, check hostd health and the configured fleet endpoint.",
			},
		}, true
	case strings.Contains(lower, "pausing (shutting down)") || strings.Contains(lower, "resuming build"):
		return doctorCandidate{
			category:    "jenkins-restart",
			likelyCause: "Jenkins restarted while the pipeline was running, so the build resumed from persisted state.",
			confidence:  "medium",
			priority:    88,
			suggestedFix: []string{
				"Check Jenkins restart history and controller logs for the restart reason.",
				"Rerun after the controller is stable; ephemeral agents may need to be reprovisioned after resume.",
			},
		}, true
	case strings.Contains(lower, "removed or offline") || strings.Contains(lower, "channelclosedexception") || strings.Contains(lower, "closedchannelexception"):
		return doctorCandidate{
			category:    "agent-lost",
			likelyCause: "The Jenkins agent disappeared or its remoting channel closed before the pipeline finished.",
			confidence:  "high",
			priority:    90,
			suggestedFix: []string{
				"Inspect the agent lifecycle events and Firecracker/hostd evidence bundle for why the VM exited.",
				"Rerun after fixing the agent lease, network, or controller restart condition.",
			},
		}, true
	case strings.Contains(trimmed, "OutOfMemoryError") || strings.Contains(lower, "oomkilled") || strings.Contains(lower, "cannot allocate memory"):
		return doctorCandidate{
			category:    "out-of-memory",
			likelyCause: "The build or Jenkins process ran out of memory.",
			confidence:  "high",
			priority:    94,
			suggestedFix: []string{
				"Increase the agent memory size or reduce parallelism for the failing stage.",
				"Check JVM/container memory limits and capture heap or cgroup evidence before rerunning.",
			},
		}, true
	case strings.Contains(lower, "no space left on device"):
		return doctorCandidate{
			category:    "disk-full",
			likelyCause: "The agent or controller ran out of disk space.",
			confidence:  "high",
			priority:    94,
			suggestedFix: []string{
				"Clean workspace/cache directories or increase disk size for the affected agent.",
				"Archive required evidence first, then rerun with enough free space for artifacts and caches.",
			},
		}, true
	case strings.Contains(lower, "script returned exit code"):
		exitCode := extractDoctorExitCode(lower)
		cause := "A shell step failed"
		if exitCode != "" {
			cause += " with exit code " + exitCode
		}
		return doctorCandidate{
			category:    "command-exit",
			likelyCause: cause + ". The more specific command error should appear directly above this line.",
			confidence:  "medium",
			priority:    70,
			suggestedFix: []string{
				"Fix the command failure shown immediately before the exit-code line.",
				"Rerun with `jc job build --logs` and confirm the command exits zero.",
			},
		}, true
	case strings.Contains(lower, "timed out") || strings.Contains(lower, "timeout"):
		return doctorCandidate{
			category:    "timeout",
			likelyCause: "A Jenkins step, queue wait, or external dependency timed out.",
			confidence:  "medium",
			priority:    76,
			suggestedFix: []string{
				"Identify whether the timeout happened in queueing, provisioning, or the step itself from the surrounding log lines.",
				"Increase the correct timeout only after checking the blocked dependency or agent capacity.",
			},
		}, true
	default:
		return doctorCandidate{}, false
	}
}

func scanDoctorContexts(lines []string) []doctorLineContext {
	contexts := make([]doctorLineContext, len(lines))
	var current doctorLineContext
	for i, line := range lines {
		if match := pipelineContextPattern.FindStringSubmatch(line); len(match) == 2 {
			name := strings.TrimSpace(match[1])
			if strings.HasPrefix(name, "Branch:") {
				current.branch = strings.TrimSpace(strings.TrimPrefix(name, "Branch:"))
			} else {
				current.stage = name
				if current.branch == "" {
					current.branch = inferDoctorBranchFromText(name)
				}
			}
		}
		if match := runningOnPattern.FindStringSubmatch(line); len(match) == 2 {
			current.agent = strings.TrimSpace(match[1])
		}
		contexts[i] = current
	}
	return contexts
}

func relevantDoctorLogLines(lines []string, centerLine int, before int, after int) []DoctorLogLine {
	if len(lines) == 0 {
		return nil
	}
	start := centerLine - before
	if start < 1 {
		start = 1
	}
	end := centerLine + after
	if end > len(lines) {
		end = len(lines)
	}
	out := make([]DoctorLogLine, 0, end-start+1)
	for i := start; i <= end; i++ {
		text := strings.TrimRight(lines[i-1], "\r")
		if strings.TrimSpace(text) == "" {
			continue
		}
		out = append(out, DoctorLogLine{Number: i, Text: text})
	}
	return out
}

func tailDoctorLogLines(lines []string, count int) []DoctorLogLine {
	if len(lines) == 0 {
		return nil
	}
	start := len(lines) - count + 1
	if start < 1 {
		start = 1
	}
	return relevantDoctorLogLines(lines, start, 0, count)
}

func parseDoctorAgentLifecycle(lines []string, contexts []doctorLineContext) []DoctorAgentEvent {
	events := make([]DoctorAgentEvent, 0)
	for i, line := range lines {
		lower := strings.ToLower(line)
		event := ""
		switch {
		case strings.Contains(lower, "waiting for next available executor") || strings.Contains(lower, "still waiting to schedule task"):
			event = "queue wait"
		case strings.Contains(lower, "firecrackeragent") || strings.Contains(lower, "aeroagent"):
			event = "firecracker agent request"
		case strings.Contains(lower, "running on "):
			event = "agent running"
		case strings.Contains(lower, "removed or offline") || strings.Contains(lower, "channelclosedexception") || strings.Contains(lower, "closedchannelexception"):
			event = "agent lost"
		case strings.Contains(lower, "pausing (shutting down)") || strings.Contains(lower, "resuming build"):
			event = "controller restart/resume"
		case strings.Contains(lower, "circuit breaker open") || strings.Contains(lower, "fleet host communication suspended"):
			event = "fleet circuit breaker"
		default:
			continue
		}
		ctx := doctorLineContext{}
		if i < len(contexts) {
			ctx = contexts[i]
		}
		agent := ctx.agent
		if match := runningOnPattern.FindStringSubmatch(line); len(match) == 2 {
			agent = strings.TrimSpace(match[1])
		} else if match := nodeAllocatedPattern.FindStringSubmatch(line); len(match) == 2 {
			agent = strings.TrimSpace(match[1])
		}
		branch := inferDoctorBranchFromText(line)
		events = append(events, DoctorAgentEvent{
			Line:   i + 1,
			Event:  event,
			Agent:  agent,
			Stage:  ctx.stage,
			Branch: branch,
			Text:   strings.TrimSpace(line),
		})
	}
	return events
}

func limitDoctorAgentEvents(events []DoctorAgentEvent, limit int) []DoctorAgentEvent {
	if len(events) <= limit {
		return events
	}
	head := append([]DoctorAgentEvent(nil), events[:limit-1]...)
	head = append(head, DoctorAgentEvent{Event: "truncated", Text: fmt.Sprintf("%d additional agent event(s) omitted", len(events)-limit+1)})
	return head
}

func (jc *JenkinsClient) collectDoctorStaticModel(jobName string, buildNumber int64) (DoctorPipelineStaticModel, string) {
	if script, err := jc.GetExecutedScript(jobName, buildNumber); err == nil && strings.TrimSpace(script) != "" {
		model := analyzeDoctorPipelineScript("executed Jenkinsfile", script)
		return model, ""
	}

	configXML, err := jc.GetJobConfig(jobName)
	if err != nil {
		return DoctorPipelineStaticModel{
			Source:   "unavailable",
			Kind:     "unknown",
			Warnings: []string{"could not read executed Jenkinsfile or job config"},
		}, fmt.Sprintf("Pipeline static model unavailable: %v", err)
	}
	script, scriptPath := extractDoctorPipelineScriptFromConfig(configXML)
	if strings.TrimSpace(script) != "" {
		return analyzeDoctorPipelineScript("job config Jenkinsfile", script), ""
	}
	if scriptPath != "" {
		return DoctorPipelineStaticModel{
			Source:   "SCM Jenkinsfile path: " + scriptPath,
			Kind:     "scm",
			Warnings: []string{"SCM-backed Jenkinsfile content is not embedded in job config"},
		}, "Pipeline static model is limited because the job uses an SCM Jenkinsfile."
	}
	return DoctorPipelineStaticModel{
		Source:   "job config",
		Kind:     "unknown",
		Warnings: []string{"no Pipeline script block found in job config"},
	}, "Pipeline static model is limited because no Pipeline script block was found."
}

func extractDoctorPipelineScriptFromConfig(configXML string) (string, string) {
	var cfg struct {
		Definition struct {
			Class      string `xml:"class,attr"`
			Script     string `xml:"script"`
			ScriptPath string `xml:"scriptPath"`
		} `xml:"definition"`
	}
	if err := xml.Unmarshal([]byte(configXML), &cfg); err != nil {
		return "", ""
	}
	return cfg.Definition.Script, cfg.Definition.ScriptPath
}

func analyzeDoctorPipelineScript(source string, script string) DoctorPipelineStaticModel {
	normalized := strings.ReplaceAll(script, "\r\n", "\n")
	model := DoctorPipelineStaticModel{
		Source: source,
		Kind:   "scripted",
	}
	if regexp.MustCompile(`(?m)^\s*pipeline\s*\{`).MatchString(normalized) {
		model.Kind = "declarative"
	}
	model.Stages = uniqueStrings(matchesBySubmatch(stageCallDoctorPattern, normalized, 1), 80)
	model.Agents = collectDoctorAgents(normalized)
	model.EnvVars = collectDoctorEnvVars(normalized)
	model.Credentials = collectDoctorCredentials(normalized)
	model.Artifacts = uniqueStrings(matchesBySubmatch(archiveArtifactsPattern, normalized, 1), 40)
	if len(model.Stages) == 0 {
		model.Warnings = append(model.Warnings, "no static stage('name') calls found")
	}
	return model
}

func collectDoctorAgents(script string) []string {
	var agents []string
	for _, label := range matchesBySubmatch(labelDoctorPattern, script, 1) {
		agents = append(agents, "label:"+label)
	}
	for _, label := range matchesBySubmatch(nodeDoctorPattern, script, 1) {
		agents = append(agents, "node:"+label)
	}
	for _, image := range matchesBySubmatch(imageDoctorPattern, script, 1) {
		agents = append(agents, "image:"+image)
	}
	if firecrackerStepPatternDoc.MatchString(script) {
		agents = append(agents, "firecracker")
	}
	return uniqueStrings(agents, 40)
}

func collectDoctorEnvVars(script string) []string {
	var envs []string
	envs = append(envs, matchesBySubmatch(envAssignmentPattern, script, 1)...)
	for _, block := range matchesBySubmatch(withEnvPattern, script, 1) {
		for _, entry := range strings.Split(block, ",") {
			entry = strings.Trim(entry, " \n\r\t'\"")
			if eq := strings.Index(entry, "="); eq > 0 {
				envs = append(envs, entry[:eq])
			}
		}
	}
	return uniqueStrings(envs, 60)
}

func collectDoctorCredentials(script string) []string {
	var credentials []string
	credentials = append(credentials, matchesBySubmatch(credentialsDoctorPattern, script, 1)...)
	if withCredentialsPattern.MatchString(script) {
		credentials = append(credentials, "withCredentials")
	}
	return uniqueStrings(credentials, 40)
}

func matchesBySubmatch(pattern *regexp.Regexp, src string, index int) []string {
	matches := pattern.FindAllStringSubmatch(src, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if index < len(match) {
			value := strings.TrimSpace(match[index])
			if value != "" {
				out = append(out, value)
			}
		}
	}
	return out
}

func (jc *JenkinsClient) collectDoctorStageTimings(jobName string, buildNumber int64) ([]DoctorStageTiming, string) {
	stages, err := jc.collectDoctorStageTimingsWFAPI(jobName, buildNumber)
	if err == nil && len(stages) > 0 {
		return stages, ""
	}
	if err != nil {
		if fallback, fallbackErr := jc.collectDoctorStageNamesFromGroovy(jobName, buildNumber); fallbackErr == nil && len(fallback) > 0 {
			return fallback, fmt.Sprintf("Stage timing duration unavailable from wfapi: %v", err)
		}
		return nil, fmt.Sprintf("Stage timing unavailable: %v", err)
	}
	return nil, "Stage timing unavailable: Jenkins returned no stage nodes."
}

func (jc *JenkinsClient) collectDoctorStageTimingsWFAPI(jobName string, buildNumber int64) ([]DoctorStageTiming, error) {
	endpoint := fmt.Sprintf("%s%s/%d/wfapi/describe", strings.TrimRight(jc.Client.Server, "/"), jenkinsJobURLPath(jobName), buildNumber)
	req, err := http.NewRequestWithContext(jc.ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if jc.username != "" {
		req.SetBasicAuth(jc.username, jc.password)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("wfapi is not available for this build")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("wfapi returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		Stages []struct {
			Name                string `json:"name"`
			Status              string `json:"status"`
			StartTimeMillis     int64  `json:"startTimeMillis"`
			DurationMillis      int64  `json:"durationMillis"`
			PauseDurationMillis int64  `json:"pauseDurationMillis"`
		} `json:"stages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]DoctorStageTiming, 0, len(body.Stages))
	for _, stage := range body.Stages {
		if strings.TrimSpace(stage.Name) == "" {
			continue
		}
		out = append(out, DoctorStageTiming{
			Name:            stage.Name,
			Status:          stage.Status,
			StartTimeMillis: stage.StartTimeMillis,
			DurationMillis:  stage.DurationMillis,
			PauseMillis:     stage.PauseDurationMillis,
		})
	}
	return out, nil
}

func (jc *JenkinsClient) collectDoctorStageNamesFromGroovy(jobName string, buildNumber int64) ([]DoctorStageTiming, error) {
	script := fmt.Sprintf(`
import org.jenkinsci.plugins.workflow.graph.FlowGraphWalker
import org.jenkinsci.plugins.workflow.actions.LabelAction
import org.jenkinsci.plugins.workflow.actions.TimingAction

def job = Jenkins.instance.getItemByFullName(%s)
def build = job?.getBuildByNumber(%d)
if (build == null || build.execution == null) { return }

def rows = []
for (def n : new FlowGraphWalker(build.execution)) {
    def label = n.getAction(LabelAction.class)
    if (label == null || label.displayName == null) { continue }
    def name = label.displayName.toString()
    if (name.startsWith('Branch:') || name.startsWith('Firecracker agent ')) { continue }
    def start = 0L
    try { start = TimingAction.getStartTime(n) as long } catch (Throwable ignored) { start = 0L }
    rows << [id: n.id as int, name: name, start: start]
}
rows.sort { it.id }.each { println "${it.name}\tUNKNOWN\t${it.start}" }
`, doctorGroovyString(jobName), buildNumber)
	out, err := jc.ExecuteGroovy(script)
	if err != nil {
		return nil, err
	}
	var stages []DoctorStageTiming
	for _, line := range splitDoctorLines(out) {
		if strings.HasPrefix(line, "Result:") || strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}
		status := ""
		if len(parts) > 1 {
			status = strings.TrimSpace(parts[1])
		}
		var start int64
		if len(parts) > 2 {
			start, _ = strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
		}
		stages = append(stages, DoctorStageTiming{Name: name, Status: status, StartTimeMillis: start})
	}
	return stages, nil
}

func mergeStageFacts(report *DoctorReport) {
	if report.ExactStage == "" {
		for _, stage := range report.StageTimings {
			status := strings.ToLower(stage.Status)
			if strings.Contains(status, "fail") || strings.Contains(status, "error") || strings.Contains(status, "abort") {
				report.ExactStage = stage.Name
				break
			}
		}
	}
	if report.ExactStage == "" && len(report.RelevantLogLines) > 0 {
		for _, line := range report.RelevantLogLines {
			if match := pipelineContextPattern.FindStringSubmatch(line.Text); len(match) == 2 && !strings.HasPrefix(match[1], "Branch:") {
				report.ExactStage = strings.TrimSpace(match[1])
			}
		}
	}
	if report.ExactBranch == "" {
		report.ExactBranch = inferDoctorBranchFromText(report.ExactStage)
	}
	if report.Result != "" && report.Result != "SUCCESS" && report.ExactStage != "" {
		for i := range report.StageTimings {
			if report.StageTimings[i].Name == report.ExactStage && strings.EqualFold(report.StageTimings[i].Status, "UNKNOWN") {
				report.StageTimings[i].Status = report.Result
			}
		}
	}
}

func summarizeDoctorArtifacts(artifacts []gojenkins.Artifact) DoctorArtifactSummary {
	summary := DoctorArtifactSummary{Total: len(artifacts)}
	for _, artifact := range artifacts {
		path := doctorArtifactPath(artifact)
		lower := strings.ToLower(path)
		switch {
		case strings.Contains(lower, "sha256") || strings.Contains(lower, "checksum"):
			summary.Checksums = append(summary.Checksums, path)
		case strings.Contains(lower, "manifest") || strings.HasSuffix(lower, ".json"):
			summary.Manifests = append(summary.Manifests, path)
		case strings.Contains(lower, "evidence") || strings.Contains(lower, "hostd") || strings.Contains(lower, "junit") || strings.Contains(lower, "build.log") || strings.HasSuffix(lower, ".tgz"):
			summary.EvidenceBundles = append(summary.EvidenceBundles, path)
		default:
			if len(summary.OtherExamples) < 8 {
				summary.OtherExamples = append(summary.OtherExamples, path)
			}
		}
	}
	summary.EvidenceBundles = uniqueStrings(summary.EvidenceBundles, 20)
	summary.Manifests = uniqueStrings(summary.Manifests, 20)
	summary.Checksums = uniqueStrings(summary.Checksums, 20)
	return summary
}

func doctorArtifactPath(artifact gojenkins.Artifact) string {
	if idx := strings.Index(artifact.Path, "/artifact/"); idx >= 0 {
		path := artifact.Path[idx+len("/artifact/"):]
		if decoded, err := url.PathUnescape(path); err == nil {
			return decoded
		}
		return path
	}
	if artifact.FileName != "" {
		return artifact.FileName
	}
	return artifact.Path
}

func sanitizeDoctorConsoleLog(logs string) string {
	var out bytes.Buffer
	stream := newConsoleLogStream(&out)
	_, _ = stream.Write([]byte(logs))
	_ = stream.Flush()
	return out.String()
}

func splitDoctorLines(logs string) []string {
	normalized := strings.ReplaceAll(logs, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if normalized == "" {
		return nil
	}
	lines := strings.Split(normalized, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func extractDoctorExitCode(line string) string {
	idx := strings.LastIndex(line, "exit code")
	if idx < 0 {
		return ""
	}
	fields := strings.Fields(line[idx:])
	for _, field := range fields {
		if _, err := strconv.Atoi(field); err == nil {
			return field
		}
	}
	return ""
}

func inferDoctorBranch(lines []DoctorLogLine) string {
	for _, line := range lines {
		if branch := inferDoctorBranchFromText(line.Text); branch != "" {
			return branch
		}
	}
	return ""
}

func refineDoctorLocationFromLogs(report *DoctorReport) {
	branch := inferDoctorBranch(report.RelevantLogLines)
	if branch == "" {
		if report.Category == "firecracker-fleet-connectivity" || strings.Contains(strings.ToLower(report.ExactStage), "parallel") {
			report.ExactBranch = ""
		}
		if report.ExactBranch == "" {
			report.ExactBranch = inferDoctorBranchFromText(report.ExactStage)
		}
		return
	}
	report.ExactBranch = branch
	expectedStage := "Build " + branch
	if report.ExactStage == "" || (strings.Contains(report.ExactStage, "kernel-") && !strings.Contains(report.ExactStage, branch)) {
		report.ExactStage = expectedStage
	}
}

func inferDoctorBranchFromText(text string) string {
	if match := localVariantPattern.FindStringSubmatch(text); len(match) == 2 {
		return match[1]
	}
	return ""
}

func jenkinsJobURLPath(jobName string) string {
	var b strings.Builder
	for _, part := range strings.Split(jobName, "/") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		b.WriteString("/job/")
		b.WriteString(url.PathEscape(part))
	}
	return b.String()
}

func buildDoctorRerunCommand(jobName string) string {
	return fmt.Sprintf("jc job build %s --logs --timeout 30m", doctorShellQuote(jobName))
}

func doctorShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if regexp.MustCompile(`^[A-Za-z0-9_./:-]+$`).MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func doctorGroovyString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	return "'" + value + "'"
}

func formatDoctorDuration(ms int64) string {
	if ms <= 0 {
		return "unknown duration"
	}
	return formatDoctorElapsed(ms)
}

func formatDoctorElapsed(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	seconds := ms / 1000
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	seconds = seconds % 60
	return fmt.Sprintf("%dm%02ds", minutes, seconds)
}

func firstDoctorStageStart(stages []DoctorStageTiming) int64 {
	var first int64
	for _, stage := range stages {
		if stage.StartTimeMillis <= 0 {
			continue
		}
		if first == 0 || stage.StartTimeMillis < first {
			first = stage.StartTimeMillis
		}
	}
	return first
}

func emptyAs(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func uniqueStrings(values []string, limit int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func sortedDoctorStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func (r DoctorReport) FormatText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Job: %s\n", r.JobName)
	fmt.Fprintf(&b, "Build: #%d\n", r.BuildNumber)
	fmt.Fprintf(&b, "Result: %s\n", emptyAs(r.Result, "UNKNOWN"))
	if r.BuildURL != "" {
		fmt.Fprintf(&b, "URL: %s\n", r.BuildURL)
	}
	if r.DurationMs > 0 {
		fmt.Fprintf(&b, "Duration: %s\n", formatDoctorDuration(r.DurationMs))
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Likely cause:")
	fmt.Fprintf(&b, "  [%s/%s] %s\n", emptyAs(r.Category, "unknown"), emptyAs(r.Confidence, "low"), r.LikelyCause)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Exact stage/branch:")
	fmt.Fprintf(&b, "  Stage:  %s\n", emptyAs(r.ExactStage, "unknown"))
	fmt.Fprintf(&b, "  Branch: %s\n", emptyAs(r.ExactBranch, "unknown"))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Evidence:")
	if len(r.Evidence) == 0 {
		fmt.Fprintln(&b, "  - No structured evidence collected.")
	} else {
		for _, item := range r.Evidence {
			fmt.Fprintf(&b, "  - %s\n", item)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Relevant log lines:")
	if len(r.RelevantLogLines) == 0 {
		fmt.Fprintln(&b, "  (no console lines available)")
	} else {
		for _, line := range r.RelevantLogLines {
			fmt.Fprintf(&b, "  %5d | %s\n", line.Number, line.Text)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Suggested fix:")
	for i, fix := range r.SuggestedFixes {
		fmt.Fprintf(&b, "  %d. %s\n", i+1, fix)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Rerun command:")
	fmt.Fprintf(&b, "  %s\n", r.RerunCommand)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Pipeline static model:")
	fmt.Fprintf(&b, "  Source:      %s\n", emptyAs(r.PipelineStaticModel.Source, "unavailable"))
	fmt.Fprintf(&b, "  Kind:        %s\n", emptyAs(r.PipelineStaticModel.Kind, "unknown"))
	fmt.Fprintf(&b, "  Stages:      %s\n", formatDoctorList(r.PipelineStaticModel.Stages, 12))
	fmt.Fprintf(&b, "  Agents:      %s\n", formatDoctorList(r.PipelineStaticModel.Agents, 8))
	fmt.Fprintf(&b, "  Env vars:    %s\n", formatDoctorList(sortedDoctorStrings(r.PipelineStaticModel.EnvVars), 8))
	fmt.Fprintf(&b, "  Credentials: %s\n", formatDoctorList(r.PipelineStaticModel.Credentials, 8))
	fmt.Fprintf(&b, "  Artifacts:   %s\n", formatDoctorList(r.PipelineStaticModel.Artifacts, 8))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Stage timing:")
	if len(r.StageTimings) == 0 {
		fmt.Fprintln(&b, "  (stage timing unavailable)")
	} else {
		firstStart := firstDoctorStageStart(r.StageTimings)
		for _, stage := range r.StageTimings {
			start := ""
			if stage.StartTimeMillis > 0 && firstStart > 0 {
				start = ", start=+" + formatDoctorElapsed(stage.StartTimeMillis-firstStart)
			}
			duration := ""
			if stage.DurationMillis > 0 {
				duration = ", duration=" + formatDoctorDuration(stage.DurationMillis)
			}
			pause := ""
			if stage.PauseMillis > 0 {
				pause = ", paused=" + formatDoctorDuration(stage.PauseMillis)
			}
			fmt.Fprintf(&b, "  - %s: %s%s%s%s\n", stage.Name, emptyAs(stage.Status, "UNKNOWN"), start, duration, pause)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Agent lifecycle:")
	if len(r.AgentLifecycle) == 0 {
		fmt.Fprintln(&b, "  (no agent lifecycle events found)")
	} else {
		for _, event := range limitDoctorAgentEvents(r.AgentLifecycle, 20) {
			location := ""
			switch {
			case event.Stage != "" && event.Branch != "":
				location = fmt.Sprintf(" [%s/%s]", event.Stage, event.Branch)
			case event.Stage != "":
				location = fmt.Sprintf(" [%s]", event.Stage)
			case event.Branch != "":
				location = fmt.Sprintf(" [%s]", event.Branch)
			}
			line := ""
			if event.Line > 0 {
				line = fmt.Sprintf(" line %d", event.Line)
			}
			agent := ""
			if event.Agent != "" {
				agent = " agent=" + event.Agent
			}
			fmt.Fprintf(&b, "  - %s%s%s%s", event.Event, line, agent, location)
			if event.Text != "" {
				fmt.Fprintf(&b, ": %s", event.Text)
			}
			fmt.Fprintln(&b)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Artifacts/evidence bundles:")
	fmt.Fprintf(&b, "  Total artifacts: %d\n", r.Artifacts.Total)
	fmt.Fprintf(&b, "  Evidence:        %s\n", formatDoctorList(r.Artifacts.EvidenceBundles, 8))
	fmt.Fprintf(&b, "  Manifests:       %s\n", formatDoctorList(r.Artifacts.Manifests, 8))
	fmt.Fprintf(&b, "  Checksums:       %s\n", formatDoctorList(r.Artifacts.Checksums, 8))
	if len(r.Warnings) > 0 || len(r.PipelineStaticModel.Warnings) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "Warnings:")
		for _, warning := range r.Warnings {
			fmt.Fprintf(&b, "  - %s\n", warning)
		}
		for _, warning := range r.PipelineStaticModel.Warnings {
			fmt.Fprintf(&b, "  - %s\n", warning)
		}
	}
	return b.String()
}

func formatDoctorList(values []string, limit int) string {
	values = uniqueStrings(values, 0)
	if len(values) == 0 {
		return "(none)"
	}
	if limit > 0 && len(values) > limit {
		return strings.Join(values[:limit], ", ") + fmt.Sprintf(" (+%d more)", len(values)-limit)
	}
	return strings.Join(values, ", ")
}
