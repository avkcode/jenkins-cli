package cmd

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func TestJobListCmd_Valid(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"job", "list"})
	if cmd == nil {
		t.Fatal("expected 'job list' to exist")
	}
	if cmd.Short == "" {
		t.Fatal("expected non-empty Short description")
	}
}

func TestJobBuildCmd_Flags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"job", "build"})
	if cmd == nil {
		t.Fatal("expected 'job build' to exist")
	}
	m := map[string]bool{}
	cmd.Flags().VisitAll(func(f *pflag.Flag) { m[f.Name] = true })
	for _, name := range []string{"wait", "logs", "raw", "param", "timeout"} {
		if !m[name] {
			t.Fatalf("expected flag --%s to exist", name)
		}
	}
}

func TestJobLogsCmd_Flags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"job", "logs"})
	if cmd == nil {
		t.Fatal("expected 'job logs' to exist")
	}
	if cmd.Flags().Lookup("raw") == nil {
		t.Fatal("expected 'job logs --raw' flag to exist")
	}
	if cmd.Flags().Lookup("follow") == nil {
		t.Fatal("expected 'job logs --follow' flag to exist")
	}
}

func TestJobManageCmds_Exist(t *testing.T) {
	for _, name := range []string{"disable", "enable", "rename", "delete", "stages", "tests", "history", "workspace"} {
		cmd, _, _ := rootCmd.Find([]string{"job", name})
		if cmd == nil {
			t.Fatalf("expected 'job %s' to exist", name)
		}
	}
}

func TestNodeCmds_Exist(t *testing.T) {
	for _, name := range []string{"list", "get", "create", "delete", "logs"} {
		cmd, _, _ := rootCmd.Find([]string{"node", name})
		if cmd == nil {
			t.Fatalf("expected 'node %s' to exist", name)
		}
	}
}

func TestPluginCmds_Exist(t *testing.T) {
	for _, name := range []string{"list", "install", "uninstall", "upload"} {
		cmd, _, _ := rootCmd.Find([]string{"plugin", name})
		if cmd == nil {
			t.Fatalf("expected 'plugin %s' to exist", name)
		}
	}
}

func TestQueueCmds_Exist(t *testing.T) {
	for _, name := range []string{"list", "cancel"} {
		cmd, _, _ := rootCmd.Find([]string{"queue", name})
		if cmd == nil {
			t.Fatalf("expected 'queue %s' to exist", name)
		}
	}
}

func TestSystemCmds_Exist(t *testing.T) {
	for _, name := range []string{"info", "restart", "get-config", "apply-config", "logs"} {
		cmd, _, _ := rootCmd.Find([]string{"system", name})
		if cmd == nil {
			t.Fatalf("expected 'system %s' to exist", name)
		}
	}
}

func TestUserCmds_Exist(t *testing.T) {
	for _, name := range []string{"list", "create"} {
		cmd, _, _ := rootCmd.Find([]string{"user", name})
		if cmd == nil {
			t.Fatalf("expected 'user %s' to exist", name)
		}
	}
}

func TestFrozenCmds_Exist(t *testing.T) {
	for _, name := range []string{"list", "exec", "read", "write", "snapshot", "reattempt", "thaw"} {
		cmd, _, _ := rootCmd.Find([]string{"frozen", name})
		if cmd == nil {
			t.Fatalf("expected 'frozen %s' to exist", name)
		}
	}
}

func TestFrozenListFlags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"frozen", "list"})
	if cmd == nil {
		t.Fatal("'frozen list' not found")
	}
}

func TestFrozenThawFlags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"frozen", "thaw"})
	if cmd == nil {
		t.Fatal("'frozen thaw' not found")
	}
	if cmd.Flags().Lookup("input-id") == nil {
		t.Fatal("'frozen thaw' missing --input-id flag")
	}
}

func TestFrozenWriteFlags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"frozen", "write"})
	if cmd == nil {
		t.Fatal("'frozen write' not found")
	}
	if cmd.Flags().Lookup("content") == nil {
		t.Fatal("'frozen write' missing --content flag")
	}
}

func TestContextCmds_Exist(t *testing.T) {
	for _, name := range []string{"list", "use", "set"} {
		cmd, _, _ := rootCmd.Find([]string{"context", name})
		if cmd == nil {
			t.Fatalf("expected 'context %s' to exist", name)
		}
	}
}

func TestAliases_Exist(t *testing.T) {
	for _, name := range []string{"ls", "rm", "get", "apply"} {
		cmd, _, _ := rootCmd.Find([]string{name})
		if cmd == nil {
			t.Fatalf("expected alias %q to exist", name)
		}
	}
}

func TestDashboardCmd_RefreshFlagDefault(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"dashboard"})
	if cmd == nil {
		t.Fatal("expected 'dashboard' to exist")
	}
	f := cmd.Flags().Lookup("refresh")
	if f == nil {
		t.Fatal("expected --refresh flag to exist")
	}
	if f.DefValue != "3s" {
		t.Fatalf("expected --refresh default 3s, got %q", f.DefValue)
	}
}

func TestOutputFlag_OnListCmds(t *testing.T) {
	for _, args := range [][]string{
		{"job", "list", "-o", "json"},
		{"node", "list", "-o", "json"},
		{"plugin", "list", "-o", "yaml"},
		{"queue", "list", "-o", "json"},
		{"system", "info", "-o", "json"},
	} {
		cmd, _, err := rootCmd.Find(args)
		if err != nil {
			t.Fatalf("expected %v to be found, got: %v", args, err)
		}
		if cmd == nil {
			t.Fatalf("expected %v to exist", args)
		}
	}
}

func TestConfigValidation_DoesNotPanic(t *testing.T) {
	validateConfig()
}

func TestGetClientErrorHints(t *testing.T) {
	viper.Reset()
	viper.Set("url", "http://localhost:8080")
	viper.Set("user", "")
	viper.Set("token", "")

	rootCmd.SetArgs([]string{"job", "list"})
	_, err := getClient(context.Background())
	if err == nil {
		t.Fatal("expected error for empty credentials")
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
	viper.Reset()
}
