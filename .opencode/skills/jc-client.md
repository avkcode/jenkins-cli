# Adding client methods and Groovy scripts

Use this skill when adding a new method to `pkg/client/client.go` or writing a Groovy script-based Jenkins operation.

## Steps

1. **Add method to `*client.JenkinsClient`** in `pkg/client/client.go`
2. **Use HTTP or Groovy** — decide which approach
3. **Handle errors** — return structured errors, not raw strings
4. **Add tests** — `pkg/client/*_test.go`

## HTTP method template

```go
// GetFoo retrieves foo from Jenkins.
func (jc *JenkinsClient) GetFoo(ctx context.Context, name string) (string, error) {
    endpoint := strings.TrimRight(jc.Client.Server, "/") + "/foo/" + url.PathEscape(name)
    req, err := http.NewRequestWithContext(jc.ctx, "GET", endpoint, nil)
    if err != nil {
        return "", err
    }
    if jc.username != "" {
        req.SetBasicAuth(jc.username, jc.password)
    }
    jc.setCrumb(req)
    resp, err := jc.httpClient().Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", err
    }
    if resp.StatusCode >= 400 {
        return "", fmt.Errorf("jenkins returned %d: %s", resp.StatusCode, trimHTTPErrorBody(body))
    }
    return string(body), nil
}
```

## Groovy script template

For operations that can't be done via REST API (admin tasks, internals):

```go
// MyAdminOperation does something internal via Groovy.
func (jc *JenkinsClient) MyAdminOperation(param string) error {
    script := fmt.Sprintf(`
def result = Jenkins.instance.doSomething('%s')
if (result) {
    println "SUCCESS"
} else {
    println "ERROR: operation failed"
}
`, param)
    out, err := jc.ExecuteGroovy(script)
    if err != nil {
        return err
    }
    if strings.HasPrefix(out, "ERROR:") {
        return errors.New(strings.TrimSpace(out))
    }
    return nil
}
```

## ExecuteGroovy

The `ExecuteGroovy` method posts to `/scriptText` and returns the raw output:

```go
func (jc *JenkinsClient) ExecuteGroovy(script string) (string, error) {
    data := url.Values{}
    data.Set("script", script)
    endpoint := strings.TrimRight(jc.Client.Server, "/") + "/scriptText"
    resp, err := jc.postForm(endpoint, data)
    // ... handle response ...
    return string(body), nil
}
```

## Groovy scripting patterns

### Always print a marker for success/failure
```groovy
println "SUCCESS"
// or
println "ERROR: reason"
```

### Escape single quotes in Go
```go
escaped := strings.ReplaceAll(content, "'", "\\'")
script := fmt.Sprintf("def x = '''%s'''", escaped)
```

### Access internal APIs
```groovy
Jenkins.instance.items                          // all jobs
Jenkins.instance.clouds                         // all clouds
Jenkins.instance.pluginManager.getPlugin(id)     // specific plugin
Jenkins.instance.getItem(name)                  // job by name
Jenkins.instance.getExtensionList(Descriptor)   // extensions
SystemCredentialsProvider.instance.store         // credentials
GlobalLibraries.get().libraries                 // shared libraries
ScriptApproval.get()                            // script approvals
```

### Avoid common pitfalls
- Don't use `println` for result data if the caller parses output — print exactly the format expected
- Groovy scripts run on the master — heavy operations block the Jenkins event loop
- The script runs as SYSTEM user — no permission checks
- Always handle null: `if (build == null) { println "Build not found"; return }`

## Using gojenkins directly

For standard Jenkins REST API operations, prefer gojenkins over raw HTTP:

```go
job, err := jc.Client.GetJob(ctx, jobName)
build, err := job.GetBuild(ctx, 42)
builds, err := job.GetAllBuildIds(ctx)
result := build.GetResult()
duration := build.GetDuration()
console := build.GetConsoleOutput(ctx)
downstream, err := build.GetDownstreamBuilds(ctx)
config, err := job.GetConfig(ctx)
```

## Testing client methods

```go
func TestMyAdminOperation(t *testing.T) {
    // For unit tests, mock the HTTP server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Verify request
        if r.URL.Path != "/scriptText" {
            t.Fatalf("expected /scriptText, got %s", r.URL.Path)
        }
        w.Write([]byte("SUCCESS"))
    }))
    defer server.Close()

    jc, err := NewClient(context.Background(), server.URL, "", "")
    if err != nil {
        t.Fatal(err)
    }
    // ... test ...
}
```
