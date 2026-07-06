package client

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bndr/gojenkins"
)

type DoctorBundleResult struct {
	Path   string       `json:"path" yaml:"path"`
	Files  []string     `json:"files" yaml:"files"`
	Report DoctorReport `json:"report" yaml:"report"`
}

type doctorBundleContent struct {
	Report          DoctorReport
	Jenkinsfile     string
	JenkinsfileErr  string
	ArtifactEntries []DoctorArtifactEntry
}

type DoctorArtifactEntry struct {
	FileName string `json:"file_name" yaml:"file_name"`
	Path     string `json:"path" yaml:"path"`
	Kind     string `json:"kind" yaml:"kind"`
}

type doctorBundleManifest struct {
	SchemaVersion string    `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`
	JobName       string    `json:"job_name"`
	BuildNumber   int64     `json:"build_number"`
	Result        string    `json:"result"`
	Files         []string  `json:"files"`
	Warnings      []string  `json:"warnings,omitempty"`
}

type doctorStageGraphBundle struct {
	StaticModel DoctorPipelineStaticModel `json:"static_model"`
	Timings     []DoctorStageTiming       `json:"timings"`
	ExactStage  string                    `json:"exact_stage,omitempty"`
	ExactBranch string                    `json:"exact_branch,omitempty"`
}

type doctorArtifactManifestBundle struct {
	Summary   DoctorArtifactSummary `json:"summary"`
	Artifacts []DoctorArtifactEntry `json:"artifacts"`
}

// CreateDoctorBundle writes a portable doctor evidence zip and returns the report used to create it.
func (jc *JenkinsClient) CreateDoctorBundle(jobName string, buildNumber int64, outputPath string) (DoctorBundleResult, error) {
	report, err := jc.DoctorReport(jobName, buildNumber)
	if err != nil {
		return DoctorBundleResult{}, err
	}
	if outputPath == "" {
		outputPath = defaultDoctorBundlePath(jobName, buildNumber)
	}

	content := doctorBundleContent{Report: report}
	if script, err := jc.GetExecutedScript(jobName, buildNumber); err == nil {
		content.Jenkinsfile = script
	} else {
		content.JenkinsfileErr = err.Error()
	}
	if build, err := jc.Client.GetBuild(jc.ctx, jobName, buildNumber); err == nil {
		content.ArtifactEntries = doctorArtifactEntries(build.GetArtifacts())
	} else {
		report.Warnings = append(report.Warnings, fmt.Sprintf("Full artifact manifest unavailable: %v", err))
		content.Report = report
	}

	files, err := writeDoctorBundle(outputPath, content)
	if err != nil {
		return DoctorBundleResult{}, err
	}
	return DoctorBundleResult{Path: outputPath, Files: files, Report: content.Report}, nil
}

func writeDoctorBundle(outputPath string, content doctorBundleContent) ([]string, error) {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil && filepath.Dir(outputPath) != "." {
		return nil, err
	}
	file, err := os.Create(outputPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	var files []string
	add := func(name string, data []byte) error {
		w, err := zipWriter.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
		files = append(files, name)
		return nil
	}
	addJSON := func(name string, value interface{}) error {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		return add(name, data)
	}

	report := content.Report
	if err := addJSON("diagnosis.json", report); err != nil {
		return nil, err
	}
	if err := add("diagnosis.txt", []byte(report.FormatText())); err != nil {
		return nil, err
	}
	if err := add("log-excerpt.txt", []byte(formatDoctorLogExcerpt(report.RelevantLogLines))); err != nil {
		return nil, err
	}
	if strings.TrimSpace(content.Jenkinsfile) != "" {
		if err := add("Jenkinsfile", []byte(content.Jenkinsfile)); err != nil {
			return nil, err
		}
	} else {
		msg := "Executed Jenkinsfile was not available.\n"
		if content.JenkinsfileErr != "" {
			msg += "Reason: " + content.JenkinsfileErr + "\n"
		}
		if err := add("Jenkinsfile.unavailable.txt", []byte(msg)); err != nil {
			return nil, err
		}
	}
	if err := addJSON("stage-graph.json", doctorStageGraphBundle{
		StaticModel: report.PipelineStaticModel,
		Timings:     report.StageTimings,
		ExactStage:  report.ExactStage,
		ExactBranch: report.ExactBranch,
	}); err != nil {
		return nil, err
	}
	if err := addJSON("artifact-manifest.json", doctorArtifactManifestBundle{
		Summary:   report.Artifacts,
		Artifacts: content.ArtifactEntries,
	}); err != nil {
		return nil, err
	}
	if err := addJSON("agent-events.json", report.AgentLifecycle); err != nil {
		return nil, err
	}
	if err := add("README.txt", []byte(formatDoctorBundleReadme(report))); err != nil {
		return nil, err
	}
	manifest := doctorBundleManifest{
		SchemaVersion: "jc.doctor.bundle/v1",
		GeneratedAt:   time.Now().UTC(),
		JobName:       report.JobName,
		BuildNumber:   report.BuildNumber,
		Result:        report.Result,
		Files:         append([]string(nil), files...),
		Warnings:      append(append([]string(nil), report.Warnings...), report.PipelineStaticModel.Warnings...),
	}
	manifest.Files = append(manifest.Files, "manifest.json")
	if err := addJSON("manifest.json", manifest); err != nil {
		return nil, err
	}

	if err := zipWriter.Close(); err != nil {
		return nil, err
	}
	return files, nil
}

func doctorArtifactEntries(artifacts []gojenkins.Artifact) []DoctorArtifactEntry {
	entries := make([]DoctorArtifactEntry, 0, len(artifacts))
	for _, artifact := range artifacts {
		path := doctorArtifactPath(artifact)
		entries = append(entries, DoctorArtifactEntry{
			FileName: artifact.FileName,
			Path:     path,
			Kind:     classifyDoctorArtifact(path),
		})
	}
	return entries
}

func classifyDoctorArtifact(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "sha256") || strings.Contains(lower, "checksum"):
		return "checksum"
	case strings.Contains(lower, "manifest") || strings.HasSuffix(lower, ".json"):
		return "manifest"
	case strings.Contains(lower, "evidence") || strings.Contains(lower, "hostd") || strings.Contains(lower, "junit") || strings.Contains(lower, "build.log") || strings.HasSuffix(lower, ".tgz"):
		return "evidence"
	default:
		return "artifact"
	}
}

func formatDoctorLogExcerpt(lines []DoctorLogLine) string {
	if len(lines) == 0 {
		return "No relevant console lines were available.\n"
	}
	var b strings.Builder
	for _, line := range lines {
		fmt.Fprintf(&b, "%5d | %s\n", line.Number, line.Text)
	}
	return b.String()
}

func formatDoctorBundleReadme(report DoctorReport) string {
	return fmt.Sprintf(`Jenkins Doctor Evidence Bundle

Job: %s
Build: #%d
Result: %s

Start here:
- diagnosis.txt: human-readable diagnosis.
- diagnosis.json: machine-readable diagnosis.
- log-excerpt.txt: relevant console lines around the detected failure.
- Jenkinsfile: executed Pipeline script when Jenkins exposes it.
- stage-graph.json: static pipeline model plus stage timing.
- artifact-manifest.json: archived artifact and evidence bundle inventory.
- agent-events.json: agent lifecycle events extracted from the build.
- manifest.json: bundle metadata and file list.
`, report.JobName, report.BuildNumber, emptyAs(report.Result, "UNKNOWN"))
}

func defaultDoctorBundlePath(jobName string, buildNumber int64) string {
	safeJob := regexp.MustCompile(`[^A-Za-z0-9_.-]+`).ReplaceAllString(jobName, "-")
	safeJob = strings.Trim(safeJob, "-")
	if safeJob == "" {
		safeJob = "jenkins-job"
	}
	return fmt.Sprintf("jc-doctor-%s-%d.zip", safeJob, buildNumber)
}
