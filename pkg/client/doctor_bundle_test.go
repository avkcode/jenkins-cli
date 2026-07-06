package client

import (
	"archive/zip"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestWriteDoctorBundleCreatesPortableEvidenceZip(t *testing.T) {
	report := DoctorReport{
		JobName:      "kernel/job",
		BuildNumber:  42,
		Result:       "FAILURE",
		Category:     "pipeline-script-error",
		Confidence:   "high",
		LikelyCause:  "SyntaxError in generated script",
		ExactStage:   "Build kernel-01",
		ExactBranch:  "kernel-01",
		RerunCommand: "jc job build kernel/job --logs --timeout 30m",
		RelevantLogLines: []DoctorLogLine{
			{Number: 10, Text: "+ python3 -"},
			{Number: 11, Text: "SyntaxError: unterminated string literal"},
		},
		PipelineStaticModel: DoctorPipelineStaticModel{
			Source: "executed Jenkinsfile",
			Kind:   "scripted",
			Stages: []string{"Build kernel-01"},
			Agents: []string{"firecracker"},
		},
		StageTimings: []DoctorStageTiming{
			{Name: "Build kernel-01", Status: "FAILURE", StartTimeMillis: 1000},
		},
		AgentLifecycle: []DoctorAgentEvent{
			{Line: 7, Event: "agent running", Agent: "firecracker-agent", Stage: "Build kernel-01", Branch: "kernel-01"},
		},
		Artifacts: DoctorArtifactSummary{
			Total:           2,
			EvidenceBundles: []string{".firecracker-evidence/agent/hostd-evidence.tgz"},
			Manifests:       []string{"kernel-01/manifest.json"},
		},
	}
	outputPath := filepath.Join(t.TempDir(), "doctor.zip")

	files, err := writeDoctorBundle(outputPath, doctorBundleContent{
		Report:      report,
		Jenkinsfile: "node { stage('Build kernel-01') { sh 'python3 -' } }\n",
		ArtifactEntries: []DoctorArtifactEntry{
			{FileName: "manifest.json", Path: "kernel-01/manifest.json", Kind: "manifest"},
			{FileName: "hostd-evidence.tgz", Path: ".firecracker-evidence/agent/hostd-evidence.tgz", Kind: "evidence"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("expected bundle file list")
	}

	reader, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	entries := map[string]*zip.File{}
	for _, file := range reader.File {
		entries[file.Name] = file
	}
	for _, name := range []string{
		"manifest.json",
		"diagnosis.json",
		"diagnosis.txt",
		"log-excerpt.txt",
		"Jenkinsfile",
		"stage-graph.json",
		"artifact-manifest.json",
		"agent-events.json",
		"README.txt",
	} {
		if entries[name] == nil {
			t.Fatalf("bundle missing %s; entries=%v", name, keys(entries))
		}
	}

	var diagnosis DoctorReport
	readBundleJSON(t, entries["diagnosis.json"], &diagnosis)
	if diagnosis.Category != "pipeline-script-error" || diagnosis.ExactBranch != "kernel-01" {
		t.Fatalf("unexpected diagnosis payload: %#v", diagnosis)
	}

	var artifactManifest doctorArtifactManifestBundle
	readBundleJSON(t, entries["artifact-manifest.json"], &artifactManifest)
	if len(artifactManifest.Artifacts) != 2 {
		t.Fatalf("expected full artifact manifest, got %#v", artifactManifest.Artifacts)
	}
}

func readBundleJSON(t *testing.T, file *zip.File, out interface{}) {
	t.Helper()
	rc, err := file.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if err := json.NewDecoder(rc).Decode(out); err != nil {
		t.Fatal(err)
	}
}

func keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}
