package cmd

import (
	"strings"
	"testing"
)

func TestRedactArgsMasksTokenFlag(t *testing.T) {
	got := redactArgs([]string{"/usr/bin/jc", "--token", "supersecret", "plugin", "list"})
	if strings.Contains(got, "supersecret") {
		t.Fatalf("token leaked: %q", got)
	}
	if !strings.Contains(got, "--token ***") {
		t.Fatalf("expected masked token, got %q", got)
	}
	if !strings.HasPrefix(got, "jc ") {
		t.Fatalf("expected program basename, got %q", got)
	}
}

func TestRedactArgsMasksTokenEquals(t *testing.T) {
	got := redactArgs([]string{"jc", "--token=supersecret", "ls"})
	if strings.Contains(got, "supersecret") || !strings.Contains(got, "--token=***") {
		t.Fatalf("expected masked token=, got %q", got)
	}
}

func TestRedactArgsMasksCredentialSecret(t *testing.T) {
	got := redactArgs([]string{"jc", "credential", "create", "my-id", "the-secret", "desc"})
	if strings.Contains(got, "the-secret") {
		t.Fatalf("credential secret leaked: %q", got)
	}
	if !strings.Contains(got, "my-id") {
		t.Fatalf("expected credential id retained, got %q", got)
	}
}

func TestContainsSeq(t *testing.T) {
	args := []string{"--context", "prod", "credential", "create", "id"}
	if !containsSeq(args, "credential", "create") {
		t.Fatal("expected sequence credential->create")
	}
	if containsSeq(args, "create", "credential") {
		t.Fatal("did not expect reversed sequence")
	}
}

func TestContextFlagAndDeleteExist(t *testing.T) {
	if rootCmd.PersistentFlags().Lookup("context") == nil {
		t.Fatal("missing --context persistent flag")
	}
	if c, _, err := rootCmd.Find([]string{"context", "delete"}); err != nil || c.Name() != "delete" {
		t.Fatalf("context delete not found: cmd=%v err=%v", c, err)
	}
}
