package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	jclient "github.com/avkcode/jenkins-cli/pkg/client"
	"github.com/avkcode/jenkins-cli/pkg/mcp"
	"github.com/bndr/gojenkins"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// mcpToolTimeout bounds every tool call so a long operation (e.g. a plugin
// install that polls, or wait_for_build) cannot hang the server forever.
const mcpToolTimeout = 5 * time.Minute

var (
	mcpReadOnly    bool
	mcpAllowScript bool
)

// ---- schema helpers ----

func objectSchema(required []string, props map[string]any) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
func numberProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

// schemaWithContext adds the common optional "context" arg (target controller).
func schemaWithContext(required []string, props map[string]any) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	props["context"] = stringProp("controller context to target (default: current)")
	return objectSchema(required, props)
}

// mutatingSchema adds "context" and "dryRun".
func mutatingSchema(required []string, props map[string]any) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	props["dryRun"] = boolProp("preview the change without applying it")
	return schemaWithContext(required, props)
}

// ---- arg/connection helpers ----

func mcpConnect(parent context.Context, args map[string]any) (*jclient.JenkinsClient, context.Context, context.CancelFunc, error) {
	ctxName := mcp.StringArg(args, "context")
	if ctxName == "" {
		ctxName = contextOverride
	}
	ctx, cancel := context.WithTimeout(parent, mcpToolTimeout)
	jc, err := getClientWithContext(ctx, ctxName)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	return jc, ctx, cancel, nil
}

func intArg(args map[string]any, name string) (int64, bool) {
	switch v := args[name].(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case string:
		var n int64
		if _, err := fmt.Sscan(v, &n); err == nil {
			return n, true
		}
	}
	return 0, false
}

func stringMapArg(args map[string]any, name string) map[string]string {
	out := map[string]string{}
	if m, ok := args[name].(map[string]any); ok {
		for k, v := range m {
			out[k] = fmt.Sprintf("%v", v)
		}
	}
	return out
}

func dryRunResult(action string) (any, error) {
	return map[string]any{"dryRun": true, "wouldDo": action}, nil
}

func resolveBuild(ctx context.Context, jc *jclient.JenkinsClient, name string, number int64, hasNumber bool) (*gojenkins.Build, error) {
	if hasNumber {
		return jc.Client.GetBuild(ctx, name, number)
	}
	job, err := jc.Client.GetJob(ctx, name)
	if err != nil {
		return nil, err
	}
	return job.GetLastBuild(ctx)
}

func buildSummary(b *gojenkins.Build) map[string]any {
	return map[string]any{
		"number":      b.Raw.Number,
		"result":      b.Raw.Result,
		"building":    b.Raw.Building,
		"durationMs":  b.Raw.Duration,
		"timestampMs": b.Raw.Timestamp,
		"url":         b.Raw.URL,
	}
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run an MCP server exposing jc operations as agent tools (stdio)",
	Long: `Starts a Model Context Protocol server over stdio so agents (e.g. opencode)
can drive Jenkins through jc. Every tool returns JSON. The server connects using
the active context (honoring --context/--url/--user/--token); tools may also take
a per-call "context" argument to target a specific controller.

Safety: mutating tools can be disabled with --read-only; the run_groovy tool is
disabled unless --allow-script is passed.`,
	GroupID: GroupCore,
	RunE: func(cmd *cobra.Command, args []string) error {
		s := mcp.NewServer("jc", Version, os.Stdin, os.Stdout)
		s.SetAllowWrite(!mcpReadOnly)
		s.SetAllowScript(mcpAllowScript)
		registerMCPTools(s)
		registerMCPResources(s)
		registerMCPPrompts(s)
		return s.Serve(cmd.Context())
	},
}

func registerMCPTools(s *mcp.Server) {
	// ---------------- contexts ----------------
	s.AddTool(mcp.Tool{
		Name: "list_contexts", ReadOnly: true, Idempotent: true,
		Description: "List configured Jenkins controllers (contexts) and which is current.",
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			contexts := viper.GetStringMap("contexts")
			current := viper.GetString("current-context")
			type row struct {
				Name    string `json:"name"`
				URL     string `json:"url"`
				Current bool   `json:"current"`
			}
			rows := make([]row, 0, len(contexts))
			for name, v := range contexts {
				url := ""
				if c, ok := v.(map[string]interface{}); ok {
					if u, ok := c["url"].(string); ok {
						url = u
					}
				}
				rows = append(rows, row{Name: name, URL: url, Current: name == current})
			}
			return rows, nil
		},
	})

	// ---------------- jobs (read) ----------------
	s.AddTool(mcp.Tool{
		Name: "list_jobs", ReadOnly: true, Idempotent: true,
		Description: "List all Jenkins jobs (name, url, color/status).",
		InputSchema: schemaWithContext(nil, nil),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			jobs, err := jc.Client.GetAllJobs(ctx)
			if err != nil {
				return nil, err
			}
			type row struct {
				Name  string `json:"name"`
				URL   string `json:"url"`
				Color string `json:"color"`
			}
			rows := make([]row, 0, len(jobs))
			for _, j := range jobs {
				rows = append(rows, row{Name: j.Raw.Name, URL: j.Raw.URL, Color: j.Raw.Color})
			}
			return rows, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name: "get_job", ReadOnly: true, Idempotent: true,
		Description: "Get the XML config of a job.",
		InputSchema: schemaWithContext([]string{"name"}, map[string]any{"name": stringProp("job name")}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			name := mcp.StringArg(args, "name")
			if name == "" {
				return nil, mcp.Errorf("invalid_args", "name is required")
			}
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			cfg, err := jc.GetJobConfig(name)
			if err != nil {
				return nil, err
			}
			return map[string]any{"name": name, "config": cfg}, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name: "job_info", ReadOnly: true, Idempotent: true,
		Description: "Get a job's status: color, buildable, in-queue, last build number, and health score.",
		InputSchema: schemaWithContext([]string{"name"}, map[string]any{"name": stringProp("job name")}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			name := mcp.StringArg(args, "name")
			if name == "" {
				return nil, mcp.Errorf("invalid_args", "name is required")
			}
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			job, err := jc.Client.GetJob(ctx, name)
			if err != nil {
				return nil, err
			}
			info := map[string]any{
				"name":            job.Raw.Name,
				"color":           job.Raw.Color,
				"buildable":       job.Raw.Buildable,
				"inQueue":         job.Raw.InQueue,
				"lastBuildNumber": job.Raw.LastBuild.Number,
				"url":             job.Raw.URL,
			}
			if len(job.Raw.HealthReport) > 0 {
				info["healthScore"] = job.Raw.HealthReport[0].Score
				info["healthDescription"] = job.Raw.HealthReport[0].Description
			}
			return info, nil
		},
	})

	// ---------------- jobs (write) ----------------
	s.AddTool(mcp.Tool{
		Name: "apply_job", Idempotent: true,
		Description: "Create or update a job from XML config (idempotent).",
		InputSchema: mutatingSchema([]string{"name", "config"}, map[string]any{
			"name":   stringProp("job name"),
			"config": stringProp("job config XML"),
		}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			name := mcp.StringArg(args, "name")
			config := mcp.StringArg(args, "config")
			if name == "" || config == "" {
				return nil, mcp.Errorf("invalid_args", "name and config are required")
			}
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			op := "create"
			var diff string
			if existing, e := jc.GetJobConfig(name); e == nil {
				if changed, d := renderDiff(existing, config); changed {
					op, diff = "update", d
				} else {
					op = "unchanged"
				}
			}
			if mcp.BoolArg(args, "dryRun") {
				return map[string]any{"dryRun": true, "op": op, "diff": diff}, nil
			}
			if op == "unchanged" {
				return map[string]any{"applied": name, "op": op}, nil
			}
			if err := jc.CreateOrUpdateJob(name, config); err != nil {
				return nil, err
			}
			audit("mcp.apply_job", name)
			return map[string]any{"applied": name, "op": op}, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name: "enable_job", Idempotent: true,
		Description: "Enable a job.",
		InputSchema: mutatingSchema([]string{"name"}, map[string]any{"name": stringProp("job name")}),
		Handler:     jobToggleHandler(true),
	})
	s.AddTool(mcp.Tool{
		Name: "disable_job", Idempotent: true,
		Description: "Disable a job.",
		InputSchema: mutatingSchema([]string{"name"}, map[string]any{"name": stringProp("job name")}),
		Handler:     jobToggleHandler(false),
	})
	s.AddTool(mcp.Tool{
		Name: "delete_job", Destructive: true,
		Description: "Delete a job.",
		InputSchema: mutatingSchema([]string{"name"}, map[string]any{"name": stringProp("job name")}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			name := mcp.StringArg(args, "name")
			if name == "" {
				return nil, mcp.Errorf("invalid_args", "name is required")
			}
			if mcp.BoolArg(args, "dryRun") {
				return dryRunResult("delete job " + name)
			}
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			if _, err := jc.Client.DeleteJob(ctx, name); err != nil {
				return nil, err
			}
			audit("mcp.delete_job", name)
			return map[string]any{"deleted": name}, nil
		},
	})

	// ---------------- builds ----------------
	s.AddTool(mcp.Tool{
		Name:        "build_job",
		Description: "Trigger a build of a job, optionally with parameters. Returns the queue id.",
		InputSchema: mutatingSchema([]string{"name"}, map[string]any{
			"name":   stringProp("job name"),
			"params": map[string]any{"type": "object", "description": "build parameters (string values)"},
		}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			name := mcp.StringArg(args, "name")
			if name == "" {
				return nil, mcp.Errorf("invalid_args", "name is required")
			}
			if mcp.BoolArg(args, "dryRun") {
				return dryRunResult("trigger build of " + name)
			}
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			queueID, err := jc.Client.BuildJob(ctx, name, stringMapArg(args, "params"))
			if err != nil {
				return nil, err
			}
			audit("mcp.build_job", name)
			return map[string]any{"queued": name, "queueId": queueID}, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name: "get_build", ReadOnly: true, Idempotent: true,
		Description: "Get a build's status (result, building, duration, timestamp). Omit number for the last build.",
		InputSchema: schemaWithContext([]string{"name"}, map[string]any{
			"name":   stringProp("job name"),
			"number": numberProp("build number (default: last build)"),
		}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			name := mcp.StringArg(args, "name")
			if name == "" {
				return nil, mcp.Errorf("invalid_args", "name is required")
			}
			num, has := intArg(args, "number")
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			b, err := resolveBuild(ctx, jc, name, num, has)
			if err != nil {
				return nil, err
			}
			return buildSummary(b), nil
		},
	})

	s.AddTool(mcp.Tool{
		Name: "get_build_log", ReadOnly: true,
		Description: "Get a build's console log from a byte offset (pollable: pass the returned nextOffset to read more).",
		InputSchema: schemaWithContext([]string{"name"}, map[string]any{
			"name":   stringProp("job name"),
			"number": numberProp("build number (default: last build)"),
			"start":  numberProp("byte offset to read from (default: 0)"),
		}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			name := mcp.StringArg(args, "name")
			if name == "" {
				return nil, mcp.Errorf("invalid_args", "name is required")
			}
			num, has := intArg(args, "number")
			start, _ := intArg(args, "start")
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			b, err := resolveBuild(ctx, jc, name, num, has)
			if err != nil {
				return nil, err
			}
			cr, err := b.GetConsoleOutputFromIndex(ctx, start)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"number":     b.Raw.Number,
				"content":    cr.Content,
				"nextOffset": cr.Offset,
				"hasMore":    cr.HasMoreText,
			}, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name: "wait_for_build", ReadOnly: true,
		Description: "Wait until a build finishes (polls). Omit number for the last build. Returns the final status.",
		InputSchema: schemaWithContext([]string{"name"}, map[string]any{
			"name":   stringProp("job name"),
			"number": numberProp("build number (default: last build)"),
		}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			name := mcp.StringArg(args, "name")
			if name == "" {
				return nil, mcp.Errorf("invalid_args", "name is required")
			}
			num, has := intArg(args, "number")
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			for {
				b, err := resolveBuild(ctx, jc, name, num, has)
				if err != nil {
					return nil, err
				}
				if !b.Raw.Building {
					return buildSummary(b), nil
				}
				select {
				case <-ctx.Done():
					return nil, mcp.Errorf("timeout", "build still running when wait timed out")
				case <-time.After(3 * time.Second):
				}
			}
		},
	})

	// ---------------- queue ----------------
	s.AddTool(mcp.Tool{
		Name: "list_queue", ReadOnly: true, Idempotent: true,
		Description: "List items in the build queue.",
		InputSchema: schemaWithContext(nil, nil),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			q, err := jc.Client.GetQueue(ctx)
			if err != nil {
				return nil, err
			}
			type row struct {
				ID    int64  `json:"id"`
				Task  string `json:"task"`
				Why   string `json:"why"`
				Stuck bool   `json:"stuck"`
			}
			rows := make([]row, 0, len(q.Raw.Items))
			for _, it := range q.Raw.Items {
				rows = append(rows, row{ID: it.ID, Task: it.Task.Name, Why: it.Why, Stuck: it.Stuck})
			}
			return rows, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name:        "cancel_queue_item",
		Description: "Cancel a queued build by queue id.",
		InputSchema: mutatingSchema([]string{"id"}, map[string]any{"id": numberProp("queue item id")}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			id, ok := intArg(args, "id")
			if !ok {
				return nil, mcp.Errorf("invalid_args", "id is required")
			}
			if mcp.BoolArg(args, "dryRun") {
				return dryRunResult(fmt.Sprintf("cancel queue item %d", id))
			}
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			if err := jc.CancelQueueItem(fmt.Sprintf("%d", id)); err != nil {
				return nil, err
			}
			audit("mcp.cancel_queue_item", fmt.Sprintf("%d", id))
			return map[string]any{"cancelled": id}, nil
		},
	})

	// ---------------- plugins ----------------
	s.AddTool(mcp.Tool{
		Name: "list_plugins", ReadOnly: true, Idempotent: true,
		Description: "List installed Jenkins plugins (short_name, version, active, has_update).",
		InputSchema: schemaWithContext(nil, nil),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			plugins, err := jc.Client.GetPlugins(ctx, 1)
			if err != nil {
				return nil, err
			}
			type row struct {
				ShortName string `json:"short_name"`
				Version   string `json:"version"`
				Active    bool   `json:"active"`
				HasUpdate bool   `json:"has_update"`
			}
			rows := make([]row, 0, len(plugins.Raw.Plugins))
			for _, p := range plugins.Raw.Plugins {
				rows = append(rows, row{ShortName: p.ShortName, Version: p.Version, Active: p.Active, HasUpdate: p.HasUpdate})
			}
			return rows, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name: "check_plugin_updates", ReadOnly: true, Idempotent: true,
		Description: "List plugins that have an available update.",
		InputSchema: schemaWithContext(nil, nil),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			plugins, err := jc.Client.GetPlugins(ctx, 1)
			if err != nil {
				return nil, err
			}
			type row struct {
				ShortName string `json:"short_name"`
				Version   string `json:"version"`
			}
			rows := []row{}
			for _, p := range plugins.Raw.Plugins {
				if p.HasUpdate {
					rows = append(rows, row{ShortName: p.ShortName, Version: p.Version})
				}
			}
			return rows, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name: "install_plugin", Idempotent: true,
		Description: "Install a plugin and its dependencies, waiting for completion.",
		InputSchema: mutatingSchema([]string{"id"}, map[string]any{
			"id":      stringProp("plugin id (short name)"),
			"version": stringProp("version (default: latest)"),
		}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			id := mcp.StringArg(args, "id")
			if id == "" {
				return nil, mcp.Errorf("invalid_args", "id is required")
			}
			version := mcp.StringArg(args, "version")
			if version == "" {
				version = "latest"
			}
			if mcp.BoolArg(args, "dryRun") {
				return dryRunResult("install plugin " + id + "@" + version)
			}
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			if err := jc.InstallPlugin(ctx, id, version); err != nil {
				return nil, err
			}
			audit("mcp.install_plugin", id)
			return map[string]any{"installed": id, "version": version}, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name: "enable_plugin", Idempotent: true,
		Description: "Enable an installed plugin.",
		InputSchema: mutatingSchema([]string{"id"}, map[string]any{"id": stringProp("plugin id")}),
		Handler:     pluginToggleHandler(true),
	})
	s.AddTool(mcp.Tool{
		Name: "disable_plugin", Idempotent: true,
		Description: "Disable an installed plugin.",
		InputSchema: mutatingSchema([]string{"id"}, map[string]any{"id": stringProp("plugin id")}),
		Handler:     pluginToggleHandler(false),
	})
	s.AddTool(mcp.Tool{
		Name: "uninstall_plugin", Destructive: true,
		Description: "Uninstall a plugin.",
		InputSchema: mutatingSchema([]string{"id"}, map[string]any{"id": stringProp("plugin id")}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			id := mcp.StringArg(args, "id")
			if id == "" {
				return nil, mcp.Errorf("invalid_args", "id is required")
			}
			if mcp.BoolArg(args, "dryRun") {
				return dryRunResult("uninstall plugin " + id)
			}
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			if err := jc.Client.UninstallPlugin(ctx, id); err != nil {
				return nil, err
			}
			audit("mcp.uninstall_plugin", id)
			return map[string]any{"uninstalled": id}, nil
		},
	})

	// ---------------- credentials ----------------
	s.AddTool(mcp.Tool{
		Name: "list_credentials", ReadOnly: true, Idempotent: true,
		Description: "List credential ids configured on the controller.",
		InputSchema: schemaWithContext(nil, nil),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			ids, err := installedCredentialIDs(jc)
			if err != nil {
				return nil, err
			}
			list := make([]string, 0, len(ids))
			for id := range ids {
				list = append(list, id)
			}
			return map[string]any{"ids": list}, nil
		},
	})
	s.AddTool(mcp.Tool{
		Name: "create_credential", Idempotent: true,
		Description: "Create or update a Secret Text credential.",
		InputSchema: mutatingSchema([]string{"id", "secret"}, map[string]any{
			"id":          stringProp("credential id"),
			"secret":      stringProp("secret text"),
			"description": stringProp("description"),
		}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			id := mcp.StringArg(args, "id")
			secret := mcp.StringArg(args, "secret")
			if id == "" || secret == "" {
				return nil, mcp.Errorf("invalid_args", "id and secret are required")
			}
			if mcp.BoolArg(args, "dryRun") {
				return dryRunResult("create credential " + id)
			}
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			if err := jc.CreateCredential(id, secret, mcp.StringArg(args, "description")); err != nil {
				return nil, err
			}
			audit("mcp.create_credential", id)
			return map[string]any{"created": id}, nil
		},
	})
	s.AddTool(mcp.Tool{
		Name: "delete_credential", Destructive: true,
		Description: "Delete a credential by id.",
		InputSchema: mutatingSchema([]string{"id"}, map[string]any{"id": stringProp("credential id")}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			id := mcp.StringArg(args, "id")
			if id == "" {
				return nil, mcp.Errorf("invalid_args", "id is required")
			}
			if mcp.BoolArg(args, "dryRun") {
				return dryRunResult("delete credential " + id)
			}
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			if err := jc.DeleteCredential(id); err != nil {
				return nil, err
			}
			audit("mcp.delete_credential", id)
			return map[string]any{"deleted": id}, nil
		},
	})

	// ---------------- nodes ----------------
	s.AddTool(mcp.Tool{
		Name: "list_nodes", ReadOnly: true, Idempotent: true,
		Description: "List build nodes/agents (name, offline, idle).",
		InputSchema: schemaWithContext(nil, nil),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			nodes, err := jc.Client.GetAllNodes(ctx)
			if err != nil {
				return nil, err
			}
			type row struct {
				Name    string `json:"name"`
				Offline bool   `json:"offline"`
				Idle    bool   `json:"idle"`
			}
			rows := make([]row, 0, len(nodes))
			for _, n := range nodes {
				rows = append(rows, row{Name: n.GetName(), Offline: n.Raw.Offline, Idle: n.Raw.Idle})
			}
			return rows, nil
		},
	})
	s.AddTool(mcp.Tool{
		Name: "node_info", ReadOnly: true, Idempotent: true,
		Description: "Get a node's status (offline, temporarily offline, executors).",
		InputSchema: schemaWithContext([]string{"name"}, map[string]any{"name": stringProp("node name")}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			name := mcp.StringArg(args, "name")
			if name == "" {
				return nil, mcp.Errorf("invalid_args", "name is required")
			}
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			n, err := jc.Client.GetNode(ctx, name)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"name":               n.GetName(),
				"offline":            n.Raw.Offline,
				"temporarilyOffline": n.Raw.TemporarilyOffline,
				"idle":               n.Raw.Idle,
				"numExecutors":       n.Raw.NumExecutors,
			}, nil
		},
	})
	s.AddTool(mcp.Tool{
		Name: "node_logs", ReadOnly: true,
		Description: "Get an agent's log text.",
		InputSchema: schemaWithContext([]string{"name"}, map[string]any{"name": stringProp("node name")}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			name := mcp.StringArg(args, "name")
			if name == "" {
				return nil, mcp.Errorf("invalid_args", "name is required")
			}
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			n, err := jc.Client.GetNode(ctx, name)
			if err != nil {
				return nil, err
			}
			text, err := n.GetLogText(ctx)
			if err != nil {
				return nil, err
			}
			return map[string]any{"name": name, "log": text}, nil
		},
	})
	s.AddTool(mcp.Tool{
		Name: "set_node_offline", Destructive: true, Idempotent: true,
		Description: "Take a node temporarily offline.",
		InputSchema: mutatingSchema([]string{"name"}, map[string]any{
			"name":    stringProp("node name"),
			"message": stringProp("offline reason"),
		}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			name := mcp.StringArg(args, "name")
			if name == "" {
				return nil, mcp.Errorf("invalid_args", "name is required")
			}
			if mcp.BoolArg(args, "dryRun") {
				return dryRunResult("take node " + name + " offline")
			}
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			n, err := jc.Client.GetNode(ctx, name)
			if err != nil {
				return nil, err
			}
			if _, err := n.SetOffline(ctx, mcp.StringArg(args, "message")); err != nil {
				return nil, err
			}
			audit("mcp.set_node_offline", name)
			return map[string]any{"offline": name}, nil
		},
	})
	s.AddTool(mcp.Tool{
		Name: "set_node_online", Idempotent: true,
		Description: "Bring a temporarily-offline node back online.",
		InputSchema: mutatingSchema([]string{"name"}, map[string]any{"name": stringProp("node name")}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			name := mcp.StringArg(args, "name")
			if name == "" {
				return nil, mcp.Errorf("invalid_args", "name is required")
			}
			if mcp.BoolArg(args, "dryRun") {
				return dryRunResult("bring node " + name + " online")
			}
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			n, err := jc.Client.GetNode(ctx, name)
			if err != nil {
				return nil, err
			}
			if _, err := n.SetOnline(ctx); err != nil {
				return nil, err
			}
			audit("mcp.set_node_online", name)
			return map[string]any{"online": name}, nil
		},
	})

	// ---------------- controller (declarative) ----------------
	s.AddTool(mcp.Tool{
		Name: "controller_diff", ReadOnly: true, Idempotent: true,
		Description: "Diff a controller.yaml manifest against the live controller (read-only).",
		InputSchema: schemaWithContext([]string{"file"}, map[string]any{"file": stringProp("path to controller.yaml")}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			file := mcp.StringArg(args, "file")
			if file == "" {
				return nil, mcp.Errorf("invalid_args", "file is required")
			}
			m, err := loadControllerManifest(file)
			if err != nil {
				return nil, err
			}
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			return planController(ctx, jc, m, filepath.Dir(file))
		},
	})
	s.AddTool(mcp.Tool{
		Name: "controller_apply", Idempotent: true,
		Description: "Reconcile the live controller to match a controller.yaml manifest.",
		InputSchema: mutatingSchema([]string{"file"}, map[string]any{"file": stringProp("path to controller.yaml")}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			file := mcp.StringArg(args, "file")
			if file == "" {
				return nil, mcp.Errorf("invalid_args", "file is required")
			}
			m, err := loadControllerManifest(file)
			if err != nil {
				return nil, err
			}
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			baseDir := filepath.Dir(file)
			actions, err := planController(ctx, jc, m, baseDir)
			if err != nil {
				return nil, err
			}
			if mcp.BoolArg(args, "dryRun") {
				return map[string]any{"dryRun": true, "plan": actions}, nil
			}
			if err := applyController(ctx, jc, m, baseDir, actions); err != nil {
				return nil, err
			}
			audit("mcp.controller_apply", file)
			return map[string]any{"applied": true, "plan": actions}, nil
		},
	})

	// ---------------- system ----------------
	s.AddTool(mcp.Tool{
		Name: "system_info", ReadOnly: true, Idempotent: true,
		Description: "Get controller system info (mode, executors, quieting-down, security).",
		InputSchema: schemaWithContext(nil, nil),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			info, err := jc.GetInfo()
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"nodeDescription": info.NodeDescription,
				"mode":            info.Mode,
				"numExecutors":    info.NumExecutors,
				"quietingDown":    info.QuietingDown,
				"useSecurity":     info.UseSecurity,
				"useCrumbs":       info.UseCrumbs,
			}, nil
		},
	})
	s.AddTool(mcp.Tool{
		Name: "health", ReadOnly: true, Idempotent: true,
		Description: "Check controller reachability (connects and fetches system info).",
		InputSchema: schemaWithContext(nil, nil),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return map[string]any{"healthy": false, "error": err.Error()}, nil
			}
			defer cancel()
			if _, err := jc.GetInfo(); err != nil {
				return map[string]any{"healthy": false, "error": err.Error()}, nil
			}
			return map[string]any{"healthy": true}, nil
		},
	})
	s.AddTool(mcp.Tool{
		Name: "safe_restart", Destructive: true,
		Description: "Safely restart the controller (waits for builds to finish).",
		InputSchema: mutatingSchema(nil, nil),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			if mcp.BoolArg(args, "dryRun") {
				return dryRunResult("safe-restart the controller")
			}
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			if err := jc.SafeRestart(); err != nil {
				return nil, err
			}
			audit("mcp.safe_restart", "")
			return map[string]any{"restarting": true}, nil
		},
	})
	s.AddTool(mcp.Tool{
		Name: "quiet_down", Idempotent: true,
		Description: "Put the controller in quiet-down mode (set quiet=false to cancel).",
		InputSchema: mutatingSchema(nil, map[string]any{"quiet": boolProp("true=quiet down, false=cancel (default true)")}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			path := "/quietDown"
			action := "quiet down"
			if v, ok := args["quiet"].(bool); ok && !v {
				path = "/cancelQuietDown"
				action = "cancel quiet down"
			}
			if mcp.BoolArg(args, "dryRun") {
				return dryRunResult(action)
			}
			jc, ctx, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			if _, err := jc.Client.Requester.Post(ctx, path, nil, nil, nil); err != nil {
				return nil, err
			}
			audit("mcp.quiet_down", action)
			return map[string]any{"ok": true, "action": action}, nil
		},
	})

	// ---------------- script (gated) ----------------
	s.AddTool(mcp.Tool{
		Name: "run_groovy", Script: true,
		Description: "Execute an arbitrary Groovy script on the controller (powerful; gated by --allow-script).",
		InputSchema: schemaWithContext([]string{"script"}, map[string]any{"script": stringProp("Groovy script")}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			script := mcp.StringArg(args, "script")
			if script == "" {
				return nil, mcp.Errorf("invalid_args", "script is required")
			}
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			out, err := jc.ExecuteGroovy(script)
			if err != nil {
				return nil, err
			}
			audit("mcp.run_groovy", "script")
			return map[string]any{"output": out}, nil
		},
	})

	// ---------------- frozen builds ----------------
	s.AddTool(mcp.Tool{
		Name: "list_frozen_builds", ReadOnly: true, Idempotent: true,
		Description: "List builds that are frozen on an input step, with their agent node and workspace preserved for interactive debugging.",
		InputSchema: schemaWithContext(nil, nil),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			builds, err := jc.ListFrozenJobs()
			if err != nil {
				return nil, err
			}
			return builds, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name: "agent_exec", ReadOnly: true,
		Description: "Execute a shell command on a frozen build's agent workspace and return exit code, stdout, and stderr.",
		InputSchema: schemaWithContext([]string{"node", "workspace", "command"}, map[string]any{
			"node":      stringProp("agent node name"),
			"workspace": stringProp("absolute workspace path"),
			"command":   stringProp("shell command to execute"),
		}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			node := mcp.StringArg(args, "node")
			ws := mcp.StringArg(args, "workspace")
			cmd := mcp.StringArg(args, "command")
			if node == "" || ws == "" || cmd == "" {
				return nil, mcp.Errorf("invalid_args", "node, workspace, and command are required")
			}
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			result, err := jc.AgentExec(node, ws, cmd)
			if err != nil {
				return nil, err
			}
			return result, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name: "agent_read_file", ReadOnly: true, Idempotent: true,
		Description: "Read a file from a frozen build's workspace on the agent.",
		InputSchema: schemaWithContext([]string{"node", "workspace", "filePath"}, map[string]any{
			"node":      stringProp("agent node name"),
			"workspace": stringProp("absolute workspace path"),
			"filePath":  stringProp("file path relative to workspace"),
		}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			node := mcp.StringArg(args, "node")
			ws := mcp.StringArg(args, "workspace")
			fp := mcp.StringArg(args, "filePath")
			if node == "" || ws == "" || fp == "" {
				return nil, mcp.Errorf("invalid_args", "node, workspace, and filePath are required")
			}
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			content, err := jc.AgentReadFile(node, ws, fp)
			if err != nil {
				return nil, err
			}
			return map[string]any{"node": node, "filePath": fp, "content": content}, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name: "agent_write_file", Idempotent: true,
		Description: "Write content to a file in a frozen build's workspace on the agent.",
		InputSchema: mutatingSchema([]string{"node", "workspace", "filePath", "content"}, map[string]any{
			"node":      stringProp("agent node name"),
			"workspace": stringProp("absolute workspace path"),
			"filePath":  stringProp("file path relative to workspace"),
			"content":   stringProp("content to write"),
		}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			node := mcp.StringArg(args, "node")
			ws := mcp.StringArg(args, "workspace")
			fp := mcp.StringArg(args, "filePath")
			content := mcp.StringArg(args, "content")
			if node == "" || ws == "" || fp == "" {
				return nil, mcp.Errorf("invalid_args", "node, workspace, filePath, and content are required")
			}
			if mcp.BoolArg(args, "dryRun") {
				return dryRunResult(fmt.Sprintf("write to %s:%s", node, fp))
			}
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			if err := jc.AgentWriteFile(node, ws, fp, content); err != nil {
				return nil, err
			}
			audit("mcp.agent_write_file", fmt.Sprintf("%s:%s", node, fp))
			return map[string]any{"written": true, "node": node, "filePath": fp, "bytes": len(content)}, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name:        "agent_snapshot",
		Description: "Create a tarball snapshot of a frozen build's workspace on the agent. Returns the remote path and file info.",
		InputSchema: mutatingSchema([]string{"node", "workspace"}, map[string]any{
			"node":      stringProp("agent node name"),
			"workspace": stringProp("absolute workspace path"),
		}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			node := mcp.StringArg(args, "node")
			ws := mcp.StringArg(args, "workspace")
			if node == "" || ws == "" {
				return nil, mcp.Errorf("invalid_args", "node and workspace are required")
			}
			if mcp.BoolArg(args, "dryRun") {
				return dryRunResult(fmt.Sprintf("snapshot workspace on %s", node))
			}
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			info, err := jc.AgentSnapshotWorkspace(node, ws)
			if err != nil {
				return nil, err
			}
			audit("mcp.agent_snapshot", fmt.Sprintf("%s:%s", node, ws))
			return info, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name:        "agent_reattempt",
		Description: "Re-run a shell command in a frozen build's workspace and return the exit code, stdout, and stderr. Use to verify a fix before thawing.",
		InputSchema: schemaWithContext([]string{"node", "workspace", "command"}, map[string]any{
			"node":      stringProp("agent node name"),
			"workspace": stringProp("absolute workspace path"),
			"command":   stringProp("shell command to execute"),
		}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			node := mcp.StringArg(args, "node")
			ws := mcp.StringArg(args, "workspace")
			cmd := mcp.StringArg(args, "command")
			if node == "" || ws == "" || cmd == "" {
				return nil, mcp.Errorf("invalid_args", "node, workspace, and command are required")
			}
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			result, err := jc.AgentReattempt(node, ws, cmd)
			if err != nil {
				return nil, err
			}
			return result, nil
		},
	})

	s.AddTool(mcp.Tool{
		Name: "thaw_build", Destructive: true, Idempotent: true,
		Description: "Thaw (resume) a frozen build by submitting its pending input dialog.",
		InputSchema: mutatingSchema([]string{"job", "build"}, map[string]any{
			"job":     stringProp("job name (full path)"),
			"build":   numberProp("build number"),
			"inputId": stringProp("specific input ID (default: submit all)"),
		}),
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			job := mcp.StringArg(args, "job")
			if job == "" {
				return nil, mcp.Errorf("invalid_args", "job is required")
			}
			bn, ok := intArg(args, "build")
			if !ok {
				return nil, mcp.Errorf("invalid_args", "build is required")
			}
			inputID := mcp.StringArg(args, "inputId")
			if mcp.BoolArg(args, "dryRun") {
				return dryRunResult(fmt.Sprintf("thaw build %s#%d", job, bn))
			}
			jc, _, cancel, err := mcpConnect(ctx, args)
			if err != nil {
				return nil, err
			}
			defer cancel()
			if err := jc.ThawFrozenBuild(job, bn, inputID); err != nil {
				return nil, err
			}
			audit("mcp.thaw_build", fmt.Sprintf("%s#%d", job, bn))
			return map[string]any{"thawed": fmt.Sprintf("%s#%d", job, bn)}, nil
		},
	})
}

func jobToggleHandler(enable bool) mcp.ToolHandler {
	return func(ctx context.Context, args map[string]any) (any, error) {
		name := mcp.StringArg(args, "name")
		if name == "" {
			return nil, mcp.Errorf("invalid_args", "name is required")
		}
		verb := "disable"
		if enable {
			verb = "enable"
		}
		if mcp.BoolArg(args, "dryRun") {
			return dryRunResult(verb + " job " + name)
		}
		jc, ctx, cancel, err := mcpConnect(ctx, args)
		if err != nil {
			return nil, err
		}
		defer cancel()
		job, err := jc.Client.GetJob(ctx, name)
		if err != nil {
			return nil, err
		}
		if enable {
			_, err = job.Enable(ctx)
		} else {
			_, err = job.Disable(ctx)
		}
		if err != nil {
			return nil, err
		}
		audit("mcp."+verb+"_job", name)
		return map[string]any{verb + "d": name}, nil
	}
}

func pluginToggleHandler(enable bool) mcp.ToolHandler {
	return func(ctx context.Context, args map[string]any) (any, error) {
		id := mcp.StringArg(args, "id")
		if id == "" {
			return nil, mcp.Errorf("invalid_args", "id is required")
		}
		verb := "disable"
		if enable {
			verb = "enable"
		}
		if mcp.BoolArg(args, "dryRun") {
			return dryRunResult(verb + " plugin " + id)
		}
		jc, _, cancel, err := mcpConnect(ctx, args)
		if err != nil {
			return nil, err
		}
		defer cancel()
		if enable {
			err = jc.EnablePlugin(id)
		} else {
			err = jc.DisablePlugin(id)
		}
		if err != nil {
			return nil, err
		}
		audit("mcp."+verb+"_plugin", id)
		return map[string]any{verb + "d": id}, nil
	}
}

// mcpConnectDefault connects using the server's default context (the --context
// flag or current-context), for resources/prompts which have no per-call args.
func mcpConnectDefault(parent context.Context) (*jclient.JenkinsClient, context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(parent, mcpToolTimeout)
	jc, err := getClientWithContext(ctx, contextOverride)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	return jc, ctx, cancel, nil
}

func tailString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "...(truncated)...\n" + s[len(s)-max:]
}

func registerMCPResources(s *mcp.Server) {
	s.AddResourceTemplate(mcp.ResourceTemplate{
		URITemplate: "jenkins://job/{name}/config.xml",
		Name:        "job config", MimeType: "application/xml",
		Description: "XML config of a job",
	})
	s.AddResourceTemplate(mcp.ResourceTemplate{
		URITemplate: "jenkins://job/{name}/builds/{number}/console",
		Name:        "build console log", MimeType: "text/plain",
		Description: "Console log of a build (number or 'last')",
	})

	s.SetResourceLister(func(ctx context.Context) ([]mcp.Resource, error) {
		res := []mcp.Resource{{
			URI: "jenkins://controller/info", Name: "controller info",
			MimeType: "application/json", Description: "Controller system info",
		}}
		jc, ctx, cancel, err := mcpConnectDefault(ctx)
		if err != nil {
			return res, nil // best-effort: not connected -> static resources only
		}
		defer cancel()
		jobs, err := jc.Client.GetAllJobs(ctx)
		if err != nil {
			return res, nil
		}
		for _, j := range jobs {
			res = append(res, mcp.Resource{
				URI:         "jenkins://job/" + j.Raw.Name + "/config.xml",
				Name:        j.Raw.Name + " config",
				MimeType:    "application/xml",
				Description: "XML config of job " + j.Raw.Name,
			})
		}
		return res, nil
	})

	s.SetResourceReader(func(ctx context.Context, uri string) (mcp.ResourceContent, error) {
		jc, ctx, cancel, err := mcpConnectDefault(ctx)
		if err != nil {
			return mcp.ResourceContent{}, err
		}
		defer cancel()
		return readJenkinsResource(ctx, jc, uri)
	})
}

func readJenkinsResource(ctx context.Context, jc *jclient.JenkinsClient, uri string) (mcp.ResourceContent, error) {
	const scheme = "jenkins://"
	if !strings.HasPrefix(uri, scheme) {
		return mcp.ResourceContent{}, fmt.Errorf("unsupported resource uri: %s", uri)
	}
	path := strings.TrimPrefix(uri, scheme)
	switch {
	case path == "controller/info":
		info, err := jc.GetInfo()
		if err != nil {
			return mcp.ResourceContent{}, err
		}
		b, _ := json.MarshalIndent(info, "", "  ")
		return mcp.ResourceContent{URI: uri, MimeType: "application/json", Text: string(b)}, nil
	case strings.HasPrefix(path, "job/") && strings.HasSuffix(path, "/config.xml"):
		name := strings.TrimSuffix(strings.TrimPrefix(path, "job/"), "/config.xml")
		cfg, err := jc.GetJobConfig(name)
		if err != nil {
			return mcp.ResourceContent{}, err
		}
		return mcp.ResourceContent{URI: uri, MimeType: "application/xml", Text: cfg}, nil
	case strings.HasPrefix(path, "job/") && strings.HasSuffix(path, "/console"):
		mid := strings.TrimSuffix(strings.TrimPrefix(path, "job/"), "/console") // <name>/builds/<number>
		idx := strings.LastIndex(mid, "/builds/")
		if idx < 0 {
			return mcp.ResourceContent{}, fmt.Errorf("malformed build console uri: %s", uri)
		}
		name := mid[:idx]
		numStr := mid[idx+len("/builds/"):]
		var build *gojenkins.Build
		var err error
		if numStr == "" || numStr == "last" {
			job, e := jc.Client.GetJob(ctx, name)
			if e != nil {
				return mcp.ResourceContent{}, e
			}
			build, err = job.GetLastBuild(ctx)
		} else {
			n, e := strconv.ParseInt(numStr, 10, 64)
			if e != nil {
				return mcp.ResourceContent{}, fmt.Errorf("invalid build number %q", numStr)
			}
			build, err = jc.Client.GetBuild(ctx, name, n)
		}
		if err != nil {
			return mcp.ResourceContent{}, err
		}
		return mcp.ResourceContent{URI: uri, MimeType: "text/plain", Text: build.GetConsoleOutput(ctx)}, nil
	}
	return mcp.ResourceContent{}, fmt.Errorf("unknown resource: %s", uri)
}

func registerMCPPrompts(s *mcp.Server) {
	s.AddPrompt(mcp.Prompt{
		Name:        "diagnose_failing_build",
		Description: "Gather a build's status and console log and ask for a root-cause diagnosis and fix.",
		Arguments: []mcp.PromptArg{
			{Name: "job", Description: "job name", Required: true},
			{Name: "number", Description: "build number (default: last build)"},
		},
		Handler: func(ctx context.Context, args map[string]string) (mcp.PromptResult, error) {
			job := args["job"]
			if job == "" {
				return mcp.PromptResult{}, fmt.Errorf("job is required")
			}
			guidance := "Diagnose the root cause of the failure and propose a concrete fix. Use the jc MCP tools (get_build, get_build_log, get_job, list_nodes, list_queue) to investigate further."
			fallback := func(extra string) mcp.PromptResult {
				return mcp.PromptResult{
					Description: "Diagnose a failing Jenkins build",
					Messages:    []mcp.PromptMessage{{Role: "user", Text: "Jenkins job " + job + " failed" + extra + ". " + guidance}},
				}
			}
			jc, ctx, cancel, err := mcpConnectDefault(ctx)
			if err != nil {
				return fallback(""), nil
			}
			defer cancel()
			num, has := int64(0), false
			if n := args["number"]; n != "" {
				if parsed, e := strconv.ParseInt(n, 10, 64); e == nil {
					num, has = parsed, true
				}
			}
			build, err := resolveBuild(ctx, jc, job, num, has)
			if err != nil {
				return fallback(" (could not fetch build: " + err.Error() + ")"), nil
			}
			logText := tailString(build.GetConsoleOutput(ctx), 4000)
			msg := fmt.Sprintf("Jenkins job %q build #%d finished with result=%s.\n\nConsole log (tail):\n```\n%s\n```\n\n%s",
				job, build.Raw.Number, build.Raw.Result, logText, guidance)
			return mcp.PromptResult{
				Description: "Diagnose a failing Jenkins build",
				Messages:    []mcp.PromptMessage{{Role: "user", Text: msg}},
			}, nil
		},
	})

	s.AddPrompt(mcp.Prompt{
		Name:        "review_job_config",
		Description: "Fetch a job's XML config and ask for a review.",
		Arguments:   []mcp.PromptArg{{Name: "job", Description: "job name", Required: true}},
		Handler: func(ctx context.Context, args map[string]string) (mcp.PromptResult, error) {
			job := args["job"]
			if job == "" {
				return mcp.PromptResult{}, fmt.Errorf("job is required")
			}
			jc, _, cancel, err := mcpConnectDefault(ctx)
			if err != nil {
				return mcp.PromptResult{}, err
			}
			defer cancel()
			cfg, err := jc.GetJobConfig(job)
			if err != nil {
				return mcp.PromptResult{}, err
			}
			msg := fmt.Sprintf("Review this Jenkins job config for %q and suggest improvements (security, reliability, best practices):\n```xml\n%s\n```", job, cfg)
			return mcp.PromptResult{
				Description: "Review a Jenkins job config",
				Messages:    []mcp.PromptMessage{{Role: "user", Text: msg}},
			}, nil
		},
	})
}

func init() {
	mcpCmd.Flags().BoolVar(&mcpReadOnly, "read-only", false, "Disable all mutating tools (safe for production)")
	mcpCmd.Flags().BoolVar(&mcpAllowScript, "allow-script", false, "Enable the run_groovy tool")
	rootCmd.AddCommand(mcpCmd)
}
