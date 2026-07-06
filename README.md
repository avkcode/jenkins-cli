# jc — Jenkins CLI

A high-performance, standalone Go CLI for Jenkins.

`jc` achieves complete coverage of Jenkins operations through 40+ idiomatic commands, a built-in MCP server for AI agents, Groovy script console access, and raw REST API call-through.

## Quick Start

```bash
# From source
go build -o jc . && sudo mv jc /usr/local/bin/

# From GitHub releases
curl -fsSL https://raw.githubusercontent.com/avkcode/jenkins-cli/main/scripts/install.sh | bash
```

### Authenticate (once)

```bash
jc login --url http://jenkins.example.com:8080 --user admin --token 11fea8a...
# Credentials saved to ~/.jenkins-cli.yaml
```

For self-signed certs: `jc login --url https://jenkins.internal --user admin --token xxx --insecure`

### Multiple Jenkins instances

```bash
jc context set lab --url http://lab:8080 --user admin --token xyz
jc context set prod --url https://prod.example.com --user bot --token abc
jc context use lab
jc context list
```

---

## Command Reference

### Jobs & Builds

```bash
jc job list
jc job build my-pipeline -p BRANCH=main --wait --logs
jc job build my-pipeline -p IMAGE=ubuntu --tui          # live progress TUI
jc job info my-pipeline                                 # latest build summary
jc job info my-pipeline 42
jc job get my-pipeline > config.xml                     # export config
jc job apply my-pipeline -f config.xml                  # import config
jc job disable my-pipeline
jc job enable my-pipeline
jc job rename my-pipeline new-name
jc job delete my-pipeline
jc job stages my-pipeline                               # stage-by-stage status
jc job tests my-pipeline                                # JUnit failure stack traces
jc job history my-pipeline                              # recent 20 builds
jc job workspace my-pipeline                            # list workspace files
jc job clean my-pipeline                                # wipe workspace + @libs cache
jc job restart my-pipeline 10 Deploy                    # restart from stage
jc job branches my-pipeline                             # list Multibranch branches
jc job scan my-pipeline                                 # trigger Multibranch indexing
jc job scan-logs my-pipeline                            # view indexing logs
jc job script my-pipeline 42                            # get executed Jenkinsfile
```

### Logs — Streaming and Snapshots

```bash
jc logs my-pipeline                                     # latest build, follow
jc logs my-pipeline 42 --follow                         # stream until complete
jc logs my-pipeline 42 --follow=false --raw             # one-shot snapshot
jc logs my-pipeline --grep ERROR                        # filter by pattern
jc logs my-pipeline --stage Deploy                      # filter by pipeline stage
jc logs my-pipeline --node agent-01                     # filter by node
jc logs my-pipeline --grep 'ERROR|FATAL' -o json        # structured output
jc job logs my-pipeline 42 --follow                     # alternate via job subcommand
```

### Build Comparison — `jc diff`

```bash
jc diff vagabond-jenkins-plugin                         # last two builds
jc diff vagabond-jenkins-plugin 5 6                     # explicit builds
jc diff vagabond-jenkins-plugin --tests                 # duration + test regressions
jc diff vagabond-jenkins-plugin -a                      # duration + tests + log diffs
```
Output: duration delta (+5.8% slower), result transition, timestamp shift, agent changes, new test failures, fixed tests, log-level diffs.

### Smart Watch — `jc watch`

```bash
jc watch vagabond-jenkins-plugin                        # poll every 15s
jc watch vagabond-jenkins-plugin --interval 5s          # faster polling
jc watch vagabond-jenkins-plugin --grep ERROR           # highlight errors
jc watch vagabond-jenkins-plugin --grep 'FAIL|PANIC'    # multi-pattern
jc watch vagabond-jenkins-plugin --on-failure \
  'curl -X POST -H "Content-Type: application/json" \
   -d '\''{"text":"Build failed"}'\'' \
   https://hooks.slack.com/services/xxx'

# Watch with timeout
jc watch vagabond-jenkins-plugin --timeout 30m
```
Polls for new builds, streams progress, greps console logs, alerts on failure with shell command execution.

### Cross-Instance Migration — `jc migrate`

```bash
# Dry-run: preview without touching target
jc migrate --source-url http://old:8080 \
  --source-user admin --source-token xyz \
  --jobs my-job,other-job --dry-run

# Migrate specific jobs
jc migrate --source-url http://old:8080 \
  --source-user admin --source-token xyz \
  --jobs my-job,other-job

# Migrate everything
jc migrate --source-url http://old:8080 \
  --source-user admin --source-token xyz \
  --all --credentials --plugins
```
Copies job config XML, credentials, and plugins from a source Jenkins to the current target. Use `--dry-run` first.

### Nodes & Agents

```bash
jc node list
jc node get agent-01
jc node create agent-02 --label linux --executors 4
jc node delete agent-02
jc node logs agent-01                                   # connection log stream
jc set-node-offline agent-01 --message "maintenance"
jc set-node-online agent-01
```

### Plugins

```bash
jc plugin list
jc plugin check                                         # updates available
jc plugin install docker-workflow
jc plugin install blueocean --version 1.27.14
jc plugin uninstall old-plugin
jc plugin upload ./custom.hpi                            # local .hpi install
jc plugin disable unused-plugin
jc plugin enable unused-plugin
```

### Credentials

```bash
jc credential list
jc credential create my-token "secret-value" "GitHub PAT"
jc credential delete my-token
```

### Users

```bash
jc user list
jc user create bot-user "s3cret" "Bot User" bot@example.com
```

### Queue

```bash
jc queue list
jc queue cancel 123
```

### System Administration

```bash
jc system info                                           # controller overview
jc system get-config > system.xml                        # export config.xml
jc system apply-config -f system.xml                     # import config.xml
jc system restart                                        # safe restart
jc system logs                                           # system log
jc maint quiet-down                                      # prepare for restart
jc maint cancel                                          # cancel quiet-down
jc maint safe-restart                                    # safe restart alias
jc health                                                # connectivity check
jc doctor my-pipeline 42                                 # build failure analysis
jc doctor my-pipeline 42 --bundle                        # export evidence bundle
```

### Shared Libraries

```bash
jc library list                                          # all global + folder libs
jc library vars common-lib                               # list custom steps
jc library signatures common-lib                         # extract def call params
jc library doc common-lib myStep                         # read .txt help
jc library usage pipeline-helpers                        # who uses this lib
jc library add my-lib --url https://git.example.com/lib.git --version main

# Folder-level
jc library add my-lib --folder MyFolder --url https://git.example.com/lib.git
jc library delete my-lib                                 # remove global lib
```

### Script Approvals

```bash
jc approval list                                         # pending scripts
jc approval approve method hudson.model.Item getFullName
jc approval approve script a1b2c3d4...
```

### Pipeline Inputs

```bash
jc input list                                            # waiting inputs
jc input proceed my-pipeline 42                          # approve
jc input abort my-pipeline 42                            # reject
```

### Frozen Builds — Interactive Failure Debugging

When a build fails at a stage wrapped with `jcFreeze { ... }`, the Pipeline pauses on an input step, preserving the agent and workspace. `jc frozen` commands let you shell in, inspect state, apply fixes, and reattempt the failing step before thawing the build.

```bash
jc frozen list                                           # discover frozen builds
jc frozen exec my-pipeline 42 "cat test-output.log"      # shell into workspace
jc frozen read my-pipeline 42 .env                       # read a file
jc frozen write my-pipeline 42 .env --content 'JAVA_OPTS=-Xmx2g'  # apply fix
jc frozen snapshot my-pipeline 42                        # checkpoint workspace
jc frozen reattempt my-pipeline 42 "./test.sh"           # verify fix
jc frozen thaw my-pipeline 42                            # resume pipeline
```

**AI agent loop:** An agent calls `list_frozen_builds` to find failures, `agent_exec` to diagnose, `agent_write_file` to apply fixes, `agent_reattempt` to verify, and `thaw_build` to resume — all without human intervention.

**Shared library step** (`scripts/jcFreeze.groovy`):
```groovy
stage('integration') {
    steps {
        jcFreeze(ttl: 7200, message: 'Integration tests failed') {
            sh './integration-test.sh'
        }
    }
}
```

### Build Artifacts

```bash
jc artifact download my-pipeline 42 result.tar.gz ./result.tar.gz
```

### Environment Variables

```bash
jc env list                                              # global env vars
jc env set MY_VAR value                                  # set
jc env delete MY_VAR                                     # delete
```

### Configuration & Contexts

```bash
jc config view                                           # show current config
jc context list
jc context use lab
jc context set prod --url https://prod --user bot --token xyz
jc context delete old-context
```

### Escape Hatches: 100% Coverage

```bash
# Groovy Script Console — root access to Jenkins JVM
jc script 'println Jenkins.instance.items.size()'
jc script "$(cat cleanup.groovy)"
jc script 'Jenkins.instance.items.findAll{it.name.startsWith("test-")}.each{it.delete()}; println "Done"'

# Raw REST API — any endpoint, CSRF handled automatically
jc api GET '/computer/my-agent/api/json?pretty=true'
jc api POST '/quietDown'
jc api POST '/job/my-pipeline/42/stop'
```

### TUI Dashboard & Shell

```bash
jc dashboard                                            # interactive terminal dashboard
jc shell                                                # interactive Groovy shell (REPL)
```

### Completion

```bash
source <(jc completion bash)
source <(jc completion zsh)
jc completion fish > ~/.config/fish/completions/jc.fish
```

---

## AI Agent Integration (MCP Server)

`jc` includes a built-in **Model Context Protocol server** with 42 tools. AI agents like opencode, Claude, and Cursor can drive Jenkins directly through `jc mcp`.

```bash
jc mcp                                                  # stdio MCP server (42 tools)
jc mcp --read-only                                      # read-only mode
jc mcp --allow-script                                   # enable run_groovy tool
```

### Available MCP Tools

**Read-only:**
`list_contexts`, `list_jobs`, `get_job`, `job_info`, `get_build`, `get_build_log`, `wait_for_build`, `list_queue`, `list_plugins`, `check_plugin_updates`, `list_credentials`, `list_nodes`, `node_info`, `node_logs`, `controller_diff`, `system_info`, `health`, `list_frozen_builds`, `agent_exec`, `agent_read_file`

**Mutating:**
`apply_job`, `enable_job`, `disable_job`, `delete_job`, `build_job`, `cancel_queue_item`, `install_plugin`, `enable_plugin`, `disable_plugin`, `uninstall_plugin`, `create_credential`, `delete_credential`, `set_node_offline`, `set_node_online`, `controller_apply`, `safe_restart`, `quiet_down`, `agent_write_file`, `agent_snapshot`, `agent_reattempt`, `thaw_build`, `run_groovy`

### opencode Integration

```json
// ~/.config/opencode/opencode.jsonc or .opencode/opencode.jsonc
{
  "mcp_servers": {
    "jenkins": {
      "command": "jc",
      "args": ["mcp", "--allow-script"],
      "description": "Jenkins CI operations"
    }
  }
}
```

### Safety Model

| Flag | Effect |
|:---|:---|
| (default) | All tools except `run_groovy` and destructive operations |
| `--read-only` | Read-only tools only (no mutations) |
| `--allow-script` | Enables `run_groovy` (arbitrary Groovy execution) |

---

## Architecture

```
jc main.go
 └─ cmd/         54 command files (all register via init() in rootCmd)
      ├─ root.go       global flags, context, client injection
      ├─ job*.go       job list/get/build/apply/manage/logs
      ├─ node*.go      node list/get/create/delete/logs
      ├─ plugin*.go    plugin list/install/uninstall/upload/check
      ├─ logs.go       console log streaming with renderer
      ├─ diff_cmd.go   build comparison engine
      ├─ watch.go      build watcher with pattern alerts
      ├─ migrate.go    cross-instance job/credential/plugin migration
      ├─ mcp.go        42-tool Model Context Protocol server
      ├─ doctor.go     build failure analysis + evidence bundles
      ├─ library.go    shared library CRUD + audit
      ├─ approval.go   script security approvals
      ├─ frozen.go     frozen build: interactive failure debugging
      ├─ input.go      pipeline input management
      ├─ artifact.go   build artifact download
      ├─ maint.go      maintenance (quiet-down, safe-restart)
      ├─ script.go     Groovy console execution
      ├─ api.go        raw REST API call-through
      ├─ controller.go declarative GitOps controller management
      ├─ dashboard.go  interactive TUI
      ├─ shell.go      interactive Groovy REPL
      ├─ edit.go/view.go/diff.go  config editing and rendering
      ├─ login.go/logout.go       authentication
      ├─ context.go               multi-controller switching
      ├─ credential.go/user.go    identity management
      ├─ completion.go            shell completions
      └─ config_view.go           configuration inspection
 └─ pkg/
      ├─ client/    Jenkins HTTP client (gojenkins wrapper)
      ├─ logstream/ console log parser + renderer
      ├─ mcp/       MCP protocol server framework
      ├─ tui/       Bubbletea-based terminal UI
      └─ output/    table/json/yaml formatting
```

### Dependency Map (minimal)

```
gojenkins (HTTP API) ← client ← cmd/* ← main
bubbletea + lipgloss ← tui, dashboard
cobra + viper        ← root, context, config
yaml.v3              ← logstream output
encoding/base64       ← client (AgentExec command encoding)
```

---

## Build

```bash
make build          # go build
make test           # go test ./...
make lint           # golangci-lint
make tidy           # go mod tidy
make docker         # distroless container image
```

## Contributing

1. Trunk-based development on `main`
2. Follow Go conventions (gofmt, goimports, golangci-lint)
3. New commands go in `cmd/command_name.go`, register in `init()`
4. New client methods go in `pkg/client/`
5. New MCP tools go in `cmd/mcp.go` `registerMCPTools()`
6. Tests in `*_test.go` alongside implementation
