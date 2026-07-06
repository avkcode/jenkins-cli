package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadControllerManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "controller.yaml")
	yaml := `plugins:
  - id: timestamper
  - id: configuration-as-code
    version: latest
jobs:
  - name: my-job
    configFile: jobs/my-job.xml
credentials:
  - id: my-secret
    secretEnv: MY_SECRET
    description: test cred
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := loadControllerManifest(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(m.Plugins) != 2 || m.Plugins[1].ID != "configuration-as-code" || m.Plugins[1].Version != "latest" {
		t.Fatalf("plugins parsed wrong: %+v", m.Plugins)
	}
	if len(m.Jobs) != 1 || m.Jobs[0].ConfigFile != "jobs/my-job.xml" {
		t.Fatalf("jobs parsed wrong: %+v", m.Jobs)
	}
	if len(m.Credentials) != 1 || m.Credentials[0].SecretEnv != "MY_SECRET" {
		t.Fatalf("credentials parsed wrong: %+v", m.Credentials)
	}
}

func TestParseCredentialIDs(t *testing.T) {
	ids := parseCredentialIDs("id1\nid2\n   \nid3\n")
	for _, want := range []string{"id1", "id2", "id3"} {
		if !ids[want] {
			t.Fatalf("missing %q in %v", want, ids)
		}
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 ids, got %v", ids)
	}
}

func TestJobDesiredXMLInlineAndFile(t *testing.T) {
	if got, err := jobDesiredXML(manifestJob{Name: "j", Config: "<inline/>"}, ""); err != nil || got != "<inline/>" {
		t.Fatalf("inline: got %q err %v", got, err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "jobs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "jobs", "my-job.xml"), []byte("<x/>"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := jobDesiredXML(manifestJob{Name: "my-job", ConfigFile: "jobs/my-job.xml"}, dir)
	if err != nil || got != "<x/>" {
		t.Fatalf("file: got %q err %v", got, err)
	}
}

func TestControllerCommandsExist(t *testing.T) {
	for _, sub := range []string{"apply", "diff"} {
		c, _, err := rootCmd.Find([]string{"controller", sub})
		if err != nil || c.Name() != sub {
			t.Fatalf("controller %s not found: %v", sub, err)
		}
		if c.Flags().Lookup("file") == nil {
			t.Fatalf("controller %s missing --file flag", sub)
		}
	}
}
