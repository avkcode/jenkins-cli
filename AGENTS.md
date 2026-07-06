# AI Agent Source of Truth: jc (Jenkins CLI)

This document is the primary reference for AI agents and developers working on the `jc` Jenkins CLI tool.

## Purpose

`jc` is a standalone, high-performance Go CLI for Jenkins. It provides 40+ idiomatic commands, a Groovy script console, raw REST API access, built-in frozen build debugging, and a built-in MCP server (42 tools) for AI agent integration.

## Architecture

```
main.go
 └─ cmd/          54 command files (all register via init() in rootCmd)
      ├─ root.go       global flags (--url, --user, --token, --context, --output, --timeout, --dry-run)
      │                getClient(ctx) → *client.JenkinsClient
      │                getOutput()   → *output.Writer
      │                renderStructured(v) → JSON or YAML
      ├─ job*.go       job list/get/build/apply/manage/logs/info/stages/tests
      ├─ node*.go      node list/get/create/delete/logs
      ├─ plugin*.go    plugin list/install/uninstall/upload/check
      ├─ logs.go       console log streaming with logstream.Renderer
      ├─ diff_cmd.go   build comparison (duration, tests, log diffs)
      ├─ watch.go      build watcher with pattern alerts
      ├─ migrate.go    cross-instance job/credential/plugin migration
      ├─ mcp.go        42-tool MCP server (stdio JSON-RPC 2.0)
      ├─ controller.go declarative GitOps controller management
      ├─ library.go    shared library CRUD + audit
      ├─ approval.go   script security approvals
      ├─ frozen.go     frozen build: interactive failure debugging + AI agent loop
      └─ ...           (see README for full list)
 └─ pkg/
      ├─ client/    Jenkins HTTP client (gojenkins wrapper, 1604 lines)
      ├─ logstream/ console log parser (types.go, renderer.go)
      ├─ mcp/       MCP protocol server framework (server.go)
      ├─ tui/       Bubbletea-based terminal UI components
      └─ output/    table/json/yaml output formatter
```

## Conventions

### Adding a new command
1. Create `cmd/feature_name.go` in the `cmd` package
2. Define a `*cobra.Command` var with `RunE` handler
3. Register in `init()` with `rootCmd.AddCommand(...)` or a parent subcommand
4. Use `getClient(ctx)` for Jenkins access, `getOutput()` for formatted output
5. Add MCP tool in `cmd/mcp.go` `registerMCPTools()` if applicable
6. Add test in `cmd/feature_name_test.go`

### Adding a client method
1. Add method to `*client.JenkinsClient` in `pkg/client/client.go`
2. Use `jc.ctx` for context propagation
3. Handle auth via `jc.username`/`jc.password` + `setCrumb()`
4. Return typed results, not raw strings (unless Groovy script output)

### Client pattern
```go
jc, err := getClient(ctx)
job, err := jc.Client.GetJob(ctx, name)
build, err := job.GetLastBuild(ctx)
config, err := jc.GetJobConfig(name)
```

### Output pattern
```go
w := getOutput()
w.PrintTable([]string{"NAME", "STATUS"}, rows)
// Or for JSON/YAML:
renderStructured(data)
```

### Flag naming
- Global persistent flags: `--url`, `--user`, `--token`, `--output`, `--timeout`, `--dry-run`, `--context`, `--insecure`, `--log-level`
- Command-specific flags: descriptive, no shorthand conflicts with global `-t` (token), `-u` (user), `-o` (output), `-k` (insecure)

### Command groups
```go
GroupCore   = "Core Commands"      // job, node, queue, diff, watch, logs, dashboard, mcp
GroupAdmin  = "Administration"     // controller, credential, edit, plugin, system, migrate, user
GroupConfig = "Configuration & Profiles"  // config, context, login, logout
```

## MCP Server Architecture

The MCP server lives in `pkg/mcp/server.go` and provides a stdio-based JSON-RPC 2.0 transport. Tools are registered in `cmd/mcp.go` `registerMCPTools()`.

Each tool has:
- `Name` — unique identifier (snake_case)
- `Description` — user-facing documentation
- `InputSchema` — JSON Schema for arguments
- `Handler` — `func(ctx, args) (any, error)`
- Safety flags: `ReadOnly`, `Destructive`, `Idempotent`, `Script`

## Testing

- Unit tests: `go test ./...`
- E2E tests against lab: `jc node list --url http://141.105.65.227:8080 --user admin --token ...`
- Test files follow `*_test.go` convention alongside implementation
- Prefer table-driven tests for flag validation

## Go Module

- Module: `github.com/avkcode/jenkins-cli`
- Go version: 1.26.0
- Key deps: cobra, viper, gojenkins, bubbletea, lipgloss, yaml.v3

## Skills

This repo includes opencode skills for common development tasks:

| Skill | Purpose |
|:---|:---|
| `jc-commands` | Adding new CLI commands |
| `jc-mcp-integration` | Adding MCP server tools |
| `jc-client` | Adding client methods and Groovy scripts |

## Frozen Build Architecture

The frozen build system uses pure Jenkins primitives — no plugin, no external dependencies:

1. **Pipeline freeze**: `catchError` + `input` step pauses the build on failure, preserving the agent and workspace
2. **Remote exec**: `hudson.remoting.Channel.call(Callable)` runs commands on the agent via the existing remoting connection — no SSH, no new ports
3. **Base64 encoding**: Commands and file contents are Base64-encoded through Groovy string boundaries for safety
4. **MCP tools**: 7 new tools (`list_frozen_builds`, `agent_exec`, `agent_read_file`, `agent_write_file`, `agent_snapshot`, `agent_reattempt`, `thaw_build`) enable closed-loop AI agent debugging

### Frozen build client types (`pkg/client/client.go`)
```go
type FrozenBuild struct {
    Job       string // job full name
    Build     int64  // build number
    Node      string // agent node
    Workspace string // absolute workspace path
    InputID   string // input step ID
    Prompt    string // input message
}

type AgentExecResult struct {
    ExitCode int    // command exit code
    Stdout   string // standard output
    Stderr   string // standard error
}
```
