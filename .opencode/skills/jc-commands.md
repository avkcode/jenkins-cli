# Adding a new CLI command

Use this skill when adding a new `jc` subcommand.

## Steps

1. **Create the command file** — `cmd/feature_name.go`
2. **Define the cobra command** — follow existing patterns
3. **Register in init()** — attach to `rootCmd`, `jobCmd`, or other parent
4. **Add flags** — no shorthand conflicts with globals (`-t`, `-u`, `-o`, `-k`)
5. **Add MCP tool** — in `cmd/mcp.go` `registerMCPTools()`
6. **Add tests** — `cmd/feature_name_test.go`

## Template

```go
package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
)

var featureFlags struct {
    flagA bool
    flagB string
}

var featureCmd = &cobra.Command{
    Use:     "feature-name [args]",
    Short:   "Brief description",
    Long:    `Longer description for --help.`,
    GroupID: GroupCore,  // or GroupAdmin, GroupConfig
    Args:    cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        ctx, cancel := getTimeoutContext(cmd.Context())
        defer cancel()

        if isDryRun() {
            dryRunMsg("Would do XYZ with %v", args)
            return nil
        }

        jc, err := getClient(ctx)
        if err != nil {
            return err
        }

        // Command logic here
        job, err := jc.Client.GetJob(ctx, args[0])
        if err != nil {
            return fmt.Errorf("job not found: %w", err)
        }

        // Output
        switch viper.GetString("output") {
        case "json":
            return renderStructured(result)
        case "yaml":
            return renderStructured(result)
        default:
            fmt.Printf("Result: %v\n", job.GetName())
            return nil
        }
    },
}

func init() {
    featureCmd.Flags().BoolVar(&featureFlags.flagA, "flag-a", false, "Description")
    featureCmd.Flags().StringVar(&featureFlags.flagB, "flag-b", "", "Description")
    rootCmd.AddCommand(featureCmd)  // or jobCmd, systemCmd, etc.
}
```

## Key patterns

### Getting the Jenkins client
```go
jc, err := getClient(ctx)
// jc.Client.GetJob(), jc.GetJobConfig(), jc.ExecuteGroovy(), etc.
```

### Getting a build by number or latest
```go
buildNumber := resolveArg(args, 1)
if buildNumber <= 0 {
    job, _ := jc.Client.GetJob(ctx, jobName)
    lb, _ := job.GetLastBuild(ctx)
    buildNumber = lb.GetBuildNumber()
}
build, _ := jc.Client.GetBuild(ctx, jobName, buildNumber)
```

### Structured output
```go
switch viper.GetString("output") {
case "json":
    return renderStructured(data)
case "yaml":
    return renderStructured(data)
default:
    fmt.Println(data)
    return nil
}
```

### Dry-run support
```go
if isDryRun() {
    dryRunMsg("Would %s %d jobs", action, len(items))
    return nil
}
```

### Report to stderr, results to stdout
```go
fmt.Fprintf(os.Stderr, "Processing %s...\n", name)   // progress
fmt.Printf("%s\t%s\n", name, result)                  // output
```

## Flag shorthand rules
- Never use: `-t` (global --token), `-u` (global --user), `-o` (global --output), `-k` (global --insecure), `-h` (help)
- Safe shorthands: `-w`, `-l`, `-p`, `-n`, `-f`, `-i`, `-j`, `-a`, `-g`, `-v`, `-s`
- Check with: `grep -r 'BoolVarP\|StringVarP' cmd/ | grep '"[a-z]"'`
