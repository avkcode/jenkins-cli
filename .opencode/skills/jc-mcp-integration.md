# Adding MCP tools

Use this skill when adding a new tool to the `jc mcp` server.

## Steps

1. **Add tool in `cmd/mcp.go`** → `registerMCPTools()` function
2. **Classify the tool** → `ReadOnly`, `Destructive`, `Idempotent`, `Script`
3. **Define input schema** → JSON Schema for arguments
4. **Implement handler** → `func(ctx, args) (any, error)`
5. **Add context support** → use `mcpConnect(ctx, args)` for per-call controller switching
6. **Test** → `go test ./cmd -run TestMCP`

## Template

```go
s.AddTool(mcp.Tool{
    Name:        "my_tool_name",
    Description: "What this tool does, in agent-friendly language.",
    ReadOnly:    true,      // does not mutate controller state
    Idempotent:  true,      // repeating has same effect
    InputSchema: schemaWithContext(
        map[string]any{
            "type": "object",
            "properties": map[string]any{
                "myArg": map[string]any{
                    "type":        "string",
                    "description": "Description of the argument",
                },
            },
            "required": []string{"myArg"},
        },
        nil,
    ),
    Handler: func(ctx context.Context, args map[string]any) (any, error) {
        jc, ctx, cancel, err := mcpConnect(ctx, args)
        if err != nil {
            return nil, err
        }
        defer cancel()

        myArg, ok := args["myArg"].(string)
        if !ok {
            return nil, mcp.Errorf("invalid_args", "myArg is required")
        }

        if isDryRun() {
            return dryRunResult("my_tool: " + myArg)
        }

        result, err := jc.Client.GetJob(ctx, myArg)
        if err != nil {
            return nil, mcp.Errorf("jenkins_error", "%v", err)
        }
        return map[string]any{
            "name": result.GetName(),
            "url":  result.Raw.URL,
        }, nil
    },
})
```

## Safety classification

| Flag | Meaning | Examples |
|:---|:---|:---|
| `ReadOnly: true` | No state change | `list_jobs`, `get_build`, `health` |
| `Idempotent: true` | Safe to repeat | `apply_job`, `install_plugin`, `set_node_offline` |
| `Destructive: true` | Deletes/disrupts | `delete_job`, `uninstall_plugin`, `safe_restart` |
| `Script: true` | Runs arbitrary Groovy | `run_groovy` |

For `ReadOnly` + `Idempotent` + `Destructive` + `Script`, there is a precedence:
- `Script` tools are disabled unless `--allow-script`
- `Destructive` tools are disabled when `--read-only`
- `ReadOnly` + `Idempotent` have no restrictions

## Input schema helpers

```go
// No extra args besides --context
InputSchema: schemaWithContext(nil, nil)

// Required arg
InputSchema: schemaWithContext(
    map[string]any{
        "type": "object",
        "properties": map[string]any{
            "name": map[string]any{"type": "string", "description": "job name"},
        },
        "required": []string{"name"},
    },
    nil,
)

// Optional args
InputSchema: schemaWithContext(
    map[string]any{
        "type": "object",
        "properties": map[string]any{
            "name": map[string]any{"type": "string", "description": "job name"},
            "dryRun": map[string]any{"type": "boolean", "description": "preview without applying"},
        },
        "required": []string{"name"},
    },
    nil,
)
```

## Context support

Every tool that connects to Jenkins should use `mcpConnect`:

```go
jc, ctx, cancel, err := mcpConnect(ctx, args)
```

This reads `args["context"]` to override the current controller context, allowing agents to target specific controllers per-call.

## Returning errors

Use `mcp.Errorf` for structured errors the client can parse:

```go
return nil, mcp.Errorf("not_found", "job %q not found", name)
return nil, mcp.Errorf("invalid_args", "name is required")
return nil, mcp.Errorf("jenkins_error", "%v", err)
```

## Testing

```go
func TestMCPToolsRegistered(t *testing.T) {
    // Verify tool counts
}
```

Run: `go test ./cmd -run TestMCP -count=1`

## Tool naming convention
- Use `snake_case`: `list_jobs`, `get_build_log`, `set_node_offline`
- Prefer `verb_noun` pattern: `list`, `get`, `create`, `delete`, `apply`, `enable`, `disable`, `install`, `uninstall`, `check`
- Tool names should match CLI command semantics where possible
