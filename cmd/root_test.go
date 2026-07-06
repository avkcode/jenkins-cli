package cmd

import (
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func TestInitConfig_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	oldCfg := cfgFile
	cfgFile = filepath.Join(tmpDir, ".jenkins-cli.yaml")
	viper.Reset()
	initConfig()
	if v := viper.GetString("url"); v != "" {
		t.Fatalf("expected empty default url, got %q", v)
	}
	cfgFile = oldCfg
}

func TestRootCmd_HasVersion(t *testing.T) {
	if rootCmd.Version == "" {
		t.Fatal("expected version to be set")
	}
}

func TestRootCmd_HasSubcommands(t *testing.T) {
	names := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		names[c.Name()] = true
	}
	for _, name := range []string{"job", "node", "plugin", "queue", "system", "login", "dashboard", "completion", "context", "logout", "edit", "shell"} {
		if !names[name] {
			t.Fatalf("expected subcommand %q to exist", name)
		}
	}
}

func TestRootCmd_PersistentFlags(t *testing.T) {
	flags := []string{"url", "user", "token", "output", "config"}
	for _, f := range flags {
		if rootCmd.PersistentFlags().Lookup(f) == nil {
			t.Fatalf("expected persistent flag %q to exist", f)
		}
	}
}

func TestCompletionsCmd_ValidArgs(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		cmd, _, _ := rootCmd.Find([]string{"completion", shell})
		if cmd == nil {
			t.Fatalf("expected completion %q to be found", shell)
		}
	}
}

func TestCompletionsCmd_InvalidArgRejected(t *testing.T) {
	err := completionCmd.Args(completionCmd, []string{"invalid"})
	if err == nil {
		t.Fatal("expected error for invalid shell argument")
	}
}

func TestOutputFlag_Defaults(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("output")
	if flag == nil {
		t.Fatal("expected --output flag to exist")
	}
	if flag.DefValue != "table" {
		t.Fatalf("expected default 'table', got %q", flag.DefValue)
	}
}

func TestBuildFlags_Configurable(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"job", "build"})
	if cmd == nil {
		t.Fatal("expected job build subcommand to exist")
	}
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		switch f.Name {
		case "timeout":
			if f.DefValue != "5m0s" {
				t.Fatalf("expected --timeout default 5m0s, got %q", f.DefValue)
			}
		case "param":
			// just verify it exists
		}
	})
}
