package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bndr/gojenkins"
)

// JenkinsClient wraps the gojenkins client to provide context-aware and extended operations.
type JenkinsClient struct {
	Client   *gojenkins.Jenkins
	ctx      context.Context
	username string
	password string
	NodeName string
}

// NewClient creates a new authenticated Jenkins client.
func NewClient(ctx context.Context, serverUrl, username, password string) (*JenkinsClient, error) {
	if serverUrl == "" {
		return nil, errors.New("jenkins URL is required")
	}

	jenkins := gojenkins.CreateJenkins(nil, serverUrl, username, password)
	_, err := jenkins.Init(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Jenkins at %s: %w", serverUrl, err)
	}

	return &JenkinsClient{
		Client:   jenkins,
		ctx:      ctx,
		username: username,
		password: password,
	}, nil
}

// GetJobConfig retrieves the raw XML configuration of a job.
func (jc *JenkinsClient) GetJobConfig(jobName string) (string, error) {
	job, err := jc.Client.GetJob(jc.ctx, jobName)
	if err != nil {
		return "", err
	}
	return job.GetConfig(jc.ctx)
}

// UpdateJobConfig updates an existing job with the provided XML config.
func (jc *JenkinsClient) UpdateJobConfig(jobName string, configXML string) error {
	job, err := jc.Client.GetJob(jc.ctx, jobName)
	if err != nil {
		return err
	}
	return job.UpdateConfig(jc.ctx, configXML)
}

// CreateJob creates a new job with the provided XML config.
func (jc *JenkinsClient) CreateJob(jobName string, configXML string) (*gojenkins.Job, error) {
	return jc.Client.CreateJob(jc.ctx, configXML, jobName)
}

// CreateOrUpdateJob creates a new job or updates an existing one with the provided XML config.
func (jc *JenkinsClient) CreateOrUpdateJob(jobName string, configXML string) error {
	job, err := jc.Client.GetJob(jc.ctx, jobName)
	if err != nil {
		// Assume job doesn't exist, try creating it
		_, err = jc.Client.CreateJob(jc.ctx, configXML, jobName)
		return err
	}
	// Job exists, update it
	return job.UpdateConfig(jc.ctx, configXML)
}

// ListArtifacts returns a list of artifacts for a specific build.
func (jc *JenkinsClient) ListArtifacts(jobName string, buildNumber int64) ([]gojenkins.Artifact, error) {
	build, err := jc.Client.GetBuild(jc.ctx, jobName, buildNumber)
	if err != nil {
		return nil, err
	}
	return build.GetArtifacts(), nil
}

// DownloadAllArtifacts downloads all artifacts as a zip file.
func (jc *JenkinsClient) DownloadAllArtifacts(jobName string, buildNumber int64, out io.Writer) error {
	reqUrl := fmt.Sprintf("%s/job/%s/%d/artifact/*zip*/archive.zip", jc.Client.Server, url.PathEscape(jobName), buildNumber)

	req, err := http.NewRequestWithContext(jc.ctx, "GET", reqUrl, nil)
	if err != nil {
		return err
	}

	if jc.username != "" {
		req.SetBasicAuth(jc.username, jc.password)
	}

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download archive: status %d", resp.StatusCode)
	}

	_, err = io.Copy(out, resp.Body)
	return err
}

// DownloadArtifact downloads a build artifact.
func (jc *JenkinsClient) DownloadArtifact(jobName string, buildNumber int64, artifactName string, out io.Writer) error {
	build, err := jc.Client.GetBuild(jc.ctx, jobName, buildNumber)
	if err != nil {
		return err
	}

	for _, a := range build.GetArtifacts() {
		if a.FileName == artifactName {
			data, err := a.GetData(jc.ctx)
			if err != nil {
				return err
			}
			_, err = out.Write(data)
			return err
		}
	}
	return fmt.Errorf("artifact %s not found", artifactName)
}

// WaitForBuildToStart waits for a queued item to start building and returns the Build.
func (jc *JenkinsClient) WaitForBuildToStart(queueID int64, timeout time.Duration) (*gojenkins.Build, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		task, err := jc.Client.GetQueueItem(jc.ctx, queueID)
		if err != nil {
			return nil, err
		}

		if task.Raw.Executable.URL != "" {
			// Build has started, get the build
			jobName := task.Raw.Task.Name
			buildNumber := task.Raw.Executable.Number

			build, err := jc.Client.GetBuild(jc.ctx, jobName, buildNumber)
			if err == nil {
				return build, nil
			}
		}

		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("timeout waiting for build to start")
}

// StreamLogs streams the logs of a build to the provided io.Writer.
func (jc *JenkinsClient) StreamLogs(jobName string, buildNumber int64, out io.Writer, pollInterval time.Duration) error {
	return jc.StreamLogsWithOptions(jobName, buildNumber, out, LogStreamOptions{PollInterval: pollInterval, Follow: true})
}

type LogStreamOptions struct {
	PollInterval time.Duration
	Raw          bool
	Follow       bool
}

// StreamLogsWithOptions streams a build log with optional raw Jenkins console annotations.
func (jc *JenkinsClient) StreamLogsWithOptions(jobName string, buildNumber int64, out io.Writer, opts LogStreamOptions) error {
	if opts.PollInterval <= 0 {
		opts.PollInterval = 2 * time.Second
	}
	return jc.streamLogsHTTP(jobName, buildNumber, out, opts.PollInterval, opts.Raw, opts.Follow)
}

func (jc *JenkinsClient) StopBuild(jobName string, buildNumber int64) (bool, error) {
	endpoint := fmt.Sprintf("%s/job/%s/%d/stop", strings.TrimRight(jc.Client.Server, "/"), url.PathEscape(jobName), buildNumber)
	resp, err := jc.postForm(endpoint, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("jenkins returned error %d: %s", resp.StatusCode, string(body))
	}
	return true, nil
}

func (jc *JenkinsClient) CancelQueueItem(id string) error {
	endpoint := fmt.Sprintf("%s/queue/cancelItem?id=%s", strings.TrimRight(jc.Client.Server, "/"), url.QueryEscape(id))
	resp, err := jc.postForm(endpoint, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jenkins returned error %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (jc *JenkinsClient) streamLogsHTTP(jobName string, buildNumber int64, out io.Writer, pollInterval time.Duration, raw bool, follow bool) error {
	var offset int64 = 0
	httpClient := &http.Client{}
	console := newBuildLogStream(out, raw)
	defer console.Flush()
	transientErrors := 0

	for {
		reqUrl := fmt.Sprintf("%s/job/%s/%d/logText/progressiveText?start=%d", jc.Client.Server, url.PathEscape(jobName), buildNumber, offset)

		req, err := http.NewRequestWithContext(jc.ctx, "GET", reqUrl, nil)
		if err != nil {
			return err
		}

		if jc.username != "" {
			req.SetBasicAuth(jc.username, jc.password)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			transientErrors++
			if err := waitForLogStreamRetry(jc.ctx, pollInterval, transientErrors, fmt.Errorf("failed to get logs: %w", err)); err != nil {
				return err
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			transientErrors++
			if err := waitForLogStreamRetry(jc.ctx, pollInterval, transientErrors, err); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode >= 400 {
			if isTransientLogStreamStatus(resp.StatusCode) {
				transientErrors++
				if err := waitForLogStreamRetry(
					jc.ctx,
					pollInterval,
					transientErrors,
					fmt.Errorf("jenkins returned error %d while streaming logs: %s", resp.StatusCode, trimHTTPErrorBody(body)),
				); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("jenkins returned error %d while streaming logs: %s", resp.StatusCode, trimHTTPErrorBody(body))
		}
		transientErrors = 0

		if len(body) > 0 {
			if _, err := console.Write(body); err != nil {
				return err
			}
		}

		newOffsetStr := resp.Header.Get("X-Text-Size")
		if newOffsetStr != "" {
			newOffset, err := strconv.ParseInt(newOffsetStr, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid Jenkins X-Text-Size header %q: %w", newOffsetStr, err)
			}
			offset = newOffset
		}

		moreData := resp.Header.Get("X-More-Data")
		if !follow || moreData != "true" {
			break
		}

		time.Sleep(pollInterval)
	}

	return nil
}

const maxLogStreamTransientErrors = 150

func waitForLogStreamRetry(ctx context.Context, pollInterval time.Duration, attempts int, lastErr error) error {
	if attempts > maxLogStreamTransientErrors {
		return fmt.Errorf("log stream did not recover after %d transient errors: %w", attempts-1, lastErr)
	}
	timer := time.NewTimer(pollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isTransientLogStreamStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusBadGateway ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout
}

func trimHTTPErrorBody(body []byte) string {
	const limit = 240
	text := strings.TrimSpace(string(body))
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "...[truncated]"
}

type consoleLogStream struct {
	out         io.Writer
	rawPending  []byte
	linePending []byte
}

type buildLogStream interface {
	io.Writer
	Flush() error
}

type rawLogStream struct {
	out io.Writer
}

var (
	jenkinsHiddenNoteStart = []byte("\x1b[8mha:")
	jenkinsHiddenNoteEnd   = []byte("\x1b[0m")
)

func newConsoleLogStream(out io.Writer) *consoleLogStream {
	return &consoleLogStream{out: out}
}

func newBuildLogStream(out io.Writer, raw bool) buildLogStream {
	if raw {
		return rawLogStream{out: out}
	}
	return newConsoleLogStream(out)
}

func (s rawLogStream) Write(p []byte) (int, error) {
	return s.out.Write(p)
}

func (s rawLogStream) Flush() error {
	return nil
}

func (s *consoleLogStream) Write(p []byte) (int, error) {
	s.rawPending = append(s.rawPending, p...)
	clean, pending := sanitizeJenkinsConsoleNotes(s.rawPending, false)
	s.rawPending = pending
	if err := s.writeCompleteLines(clean); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *consoleLogStream) Flush() error {
	clean, _ := sanitizeJenkinsConsoleNotes(s.rawPending, true)
	s.rawPending = nil
	s.linePending = append(s.linePending, clean...)
	if len(s.linePending) == 0 {
		return nil
	}
	_, err := s.out.Write(s.linePending)
	s.linePending = nil
	return err
}

func (s *consoleLogStream) writeCompleteLines(clean []byte) error {
	if len(clean) == 0 {
		return nil
	}
	s.linePending = append(s.linePending, clean...)
	lastNewline := bytes.LastIndexByte(s.linePending, '\n')
	if lastNewline < 0 {
		return nil
	}
	complete := s.linePending[:lastNewline+1]
	rest := append([]byte(nil), s.linePending[lastNewline+1:]...)
	if _, err := s.out.Write(complete); err != nil {
		return err
	}
	s.linePending = rest
	return nil
}

func sanitizeJenkinsConsoleNotes(in []byte, final bool) ([]byte, []byte) {
	if len(in) == 0 {
		return nil, nil
	}
	var out []byte
	remaining := in
	for {
		start := bytes.Index(remaining, jenkinsHiddenNoteStart)
		if start < 0 {
			keep := 0
			if !final {
				keep = hiddenNoteMarkerSuffixLen(remaining)
			}
			out = append(out, remaining[:len(remaining)-keep]...)
			if keep > 0 {
				return out, append([]byte(nil), remaining[len(remaining)-keep:]...)
			}
			return out, nil
		}
		out = append(out, remaining[:start]...)
		end := bytes.Index(remaining[start:], jenkinsHiddenNoteEnd)
		if end < 0 {
			if final {
				return out, nil
			}
			return out, append([]byte(nil), remaining[start:]...)
		}
		remaining = remaining[start+end+len(jenkinsHiddenNoteEnd):]
	}
}

func hiddenNoteMarkerSuffixLen(in []byte) int {
	max := len(jenkinsHiddenNoteStart) - 1
	if len(in) < max {
		max = len(in)
	}
	for n := max; n > 0; n-- {
		if bytes.Equal(in[len(in)-n:], jenkinsHiddenNoteStart[:n]) {
			return n
		}
	}
	return 0
}

// ExecuteGroovy runs a Groovy script on the Jenkins master and returns the output.
func (jc *JenkinsClient) ExecuteGroovy(script string) (string, error) {
	data := url.Values{}
	data.Set("script", script)

	endpoint := strings.TrimRight(jc.Client.Server, "/") + "/scriptText"
	resp, err := jc.postForm(endpoint, data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", readErr
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("jenkins returned error %d: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}

func (jc *JenkinsClient) postForm(endpoint string, data url.Values) (*http.Response, error) {
	var body io.Reader
	if data != nil {
		body = strings.NewReader(data.Encode())
	}
	req, err := http.NewRequestWithContext(jc.ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if jc.username != "" {
		req.SetBasicAuth(jc.username, jc.password)
	}
	jc.setCrumb(req)
	return http.DefaultClient.Do(req)
}

func (jc *JenkinsClient) setCrumb(req *http.Request) {
	if jc.Client == nil {
		return
	}
	crumbURL := strings.TrimRight(jc.Client.Server, "/") + "/crumbIssuer/api/json"
	crumbReq, err := http.NewRequestWithContext(jc.ctx, http.MethodGet, crumbURL, nil)
	if err != nil {
		return
	}
	if jc.username != "" {
		crumbReq.SetBasicAuth(jc.username, jc.password)
	}
	resp, err := http.DefaultClient.Do(crumbReq)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	for _, cookie := range resp.Cookies() {
		req.AddCookie(cookie)
	}
	var crumb struct {
		RequestField string `json:"crumbRequestField"`
		Crumb        string `json:"crumb"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&crumb); err != nil {
		return
	}
	if crumb.RequestField != "" && crumb.Crumb != "" {
		req.Header.Set(crumb.RequestField, crumb.Crumb)
	}
	if cookie := resp.Header.Get("Set-Cookie"); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
}

// SafeRestart triggers a safe restart of the Jenkins server.
func (jc *JenkinsClient) SafeRestart() error {
	status, body, err := jc.postSafeRestart()
	if err != nil {
		return err
	}
	if status >= 200 && status < 400 {
		return nil
	}
	if status == http.StatusServiceUnavailable && strings.Contains(body, "Jenkins is restarting") {
		return nil
	}
	return fmt.Errorf("jenkins returned error %d: %s", status, body)
}

// GetInfo returns the raw Jenkins executor response (system info).
func (jc *JenkinsClient) GetInfo() (*gojenkins.ExecutorResponse, error) {
	return jc.Client.Info(jc.ctx)
}

// GetSystemConfig retrieves the main Jenkins config.xml.
func (jc *JenkinsClient) GetSystemConfig() (string, error) {
	var output string
	_, err := jc.Client.Requester.GetXML(jc.ctx, "/config.xml", &output, nil)
	return output, err
}

// UpdateSystemConfig updates the main Jenkins config.xml.
func (jc *JenkinsClient) UpdateSystemConfig(configXML string) error {
	_, err := jc.Client.Requester.PostXML(jc.ctx, "/config.xml", configXML, nil, nil)
	return err
}

// ListWorkspace returns a list of files in the workspace of a job.
func (jc *JenkinsClient) ListWorkspace(jobName string) (string, error) {
	script := fmt.Sprintf(`
def job = Jenkins.instance.getItem('%s')
def build = job.lastBuild
if (build == null) { println "No builds found"; return }
def workspace = build.getExecutor()?.getCurrentWorkspace()
if (workspace == null) { println "Workspace not available (no active executor)"; return }
workspace.list().each { println it.name + (it.isDirectory() ? "/" : "") }
`, jobName)
	return jc.ExecuteGroovy(script)
}

// GetPipelineStages returns the stage-by-stage progress of a pipeline build.
func (jc *JenkinsClient) GetPipelineStages(jobName string, buildNumber int64) (string, error) {
	script := fmt.Sprintf(`
def job = Jenkins.instance.getItem('%s')
def build = job.getBuildByNumber(%d)
if (build == null) { println "Build not found"; return }

def action = build.getAction(org.jenkinsci.plugins.workflow.job.views.FlowGraphAction.class)
if (action == null) { println "No pipeline graph found"; return }

println "STAGE\tSTATUS"
action.nodes.findAll { it instanceof org.jenkinsci.plugins.workflow.graph.FlowNode && it.getDisplayFunctionName() == 'stage' }.each { node ->
    println node.getDisplayName() + '\tCOMPLETED'
}
`, jobName, buildNumber)
	return jc.ExecuteGroovy(script)
}

// GetTestResults returns a summary of test failures for a build.
func (jc *JenkinsClient) GetTestResults(jobName string, buildNumber int64) (string, error) {
	script := fmt.Sprintf(`
def job = Jenkins.instance.getItem('%s')
def build = job.getBuildByNumber(%d)
def action = build.getAction(hudson.tasks.test.AbstractTestResultAction.class)
if (action == null) { println "No test results found."; return }

println "TOTAL: ${action.totalCount} | FAIL: ${action.failCount} | SKIP: ${action.skipCount}"
if (action.failCount > 0) {
    println "\n--- Failures ---"
    action.failedTests.each { println "${it.fullDisplayName}\n${it.errorStackTrace}\n" }
}
`, jobName, buildNumber)
	return jc.ExecuteGroovy(script)
}

// GetBuildHistory returns the recent build history for a job.
func (jc *JenkinsClient) GetBuildHistory(jobName string) (string, error) {
	script := fmt.Sprintf(`
def job = Jenkins.instance.getItem('%s')
println "BUILD\tRESULT\tDURATION"
job.builds.take(20).each { b ->
    println b.number + '\t' + b.result + '\t' + b.durationString
}
`, jobName)
	return jc.ExecuteGroovy(script)
}

// ListSharedLibraries returns all configured shared libraries (Global and Folder level).
func (jc *JenkinsClient) ListSharedLibraries() (string, error) {
	script := `
import org.jenkinsci.plugins.workflow.libs.*
println "SCOPE\tNAME\tRETRIEVER\tDEFAULT_VERSION"

// Global
GlobalLibraries.get().libraries.each { lib ->
    println "GLOBAL\t${lib.name}\t${lib.retriever.class.simpleName}\t${lib.defaultVersion ?: 'none'}"
}

// Folders
try {
    Jenkins.instance.getAllItems(com.cloudbees.hudson.plugins.folder.AbstractFolder.class).each { folder ->
        def prop = folder.getProperties().get(FolderLibraries.class)
        if (prop && prop.libraries) {
            prop.libraries.each { lib ->
                println "FOLDER:${folder.fullName}\t${lib.name}\t${lib.retriever.class.simpleName}\t${lib.defaultVersion ?: 'none'}"
            }
        }
    }
} catch (Throwable e) {
    // AbstractFolder or FolderLibraries might not be available
}
`
	return jc.ExecuteGroovy(script)
}

// GetExecutedScript retrieves the exact Jenkinsfile script that was executed for a build.
func (jc *JenkinsClient) GetExecutedScript(jobName string, buildNumber int64) (string, error) {
	script := fmt.Sprintf(`
def job = Jenkins.instance.getItem('%s')
def build = job.getBuildByNumber(%d)
if (build == null) { println "ERROR: Build not found"; return }

def action = build.getAction(org.jenkinsci.plugins.workflow.cps.replay.ReplayAction.class)
if (action != null && action.originalScript != null) {
    println action.originalScript
} else {
    println "ERROR: Original script not available (ReplayAction missing)"
}
`, jobName, buildNumber)
	out, err := jc.ExecuteGroovy(script)
	if err == nil && strings.HasPrefix(out, "ERROR:") {
		return "", errors.New(strings.TrimSpace(out))
	}
	return out, err
}

// RestartFromStage restarts a Declarative Pipeline from a specific stage.
func (jc *JenkinsClient) RestartFromStage(jobName string, buildNumber int64, stageName string) error {
	script := fmt.Sprintf(`
def job = Jenkins.instance.getItem('%s')
def build = job.getBuildByNumber(%d)
if (build == null) { println "ERROR: Build not found"; return }

def action = build.getAction(org.jenkinsci.plugins.pipeline.modeldefinition.actions.RestartDeclarativePipelineAction.class)
if (action != null) {
    if (action.isRestartEnabled()) {
        try {
            def run = action.run('%s')
            println "SUCCESS: Restarted build as ${run.fullDisplayName}"
        } catch (Exception e) {
            println "ERROR: Failed to restart stage: ${e.message}"
        }
    } else {
        println "ERROR: Restart not enabled for this build."
    }
} else {
    println "ERROR: RestartDeclarativePipelineAction not found. Ensure this is a Declarative Pipeline."
}
`, jobName, buildNumber, stageName)
	out, err := jc.ExecuteGroovy(script)
	if err == nil && strings.HasPrefix(out, "ERROR:") {
		return errors.New(strings.TrimSpace(out))
	}
	return err
}

// CleanWorkspace completely wipes the workspace for a job, including @libs cache.
func (jc *JenkinsClient) CleanWorkspace(jobName string) error {
	script := fmt.Sprintf(`
def job = Jenkins.instance.getItem('%s')
def build = job.lastBuild
if (build == null) { println "ERROR: No builds found"; return }

def workspace = build.getExecutor()?.getCurrentWorkspace() ?: build.getWorkspace()
if (workspace == null) { println "ERROR: Workspace not available"; return }

try {
    workspace.deleteRecursive()
    println "SUCCESS: Workspace wiped"
} catch (Exception e) {
    println "ERROR: Failed to wipe workspace: ${e.message}"
}
`, jobName)
	out, err := jc.ExecuteGroovy(script)
	if err == nil && strings.HasPrefix(out, "ERROR:") {
		return errors.New(strings.TrimSpace(out))
	}
	return err
}

// AuditLibraryUsage finds all jobs that used a specific shared library in their last build.
func (jc *JenkinsClient) AuditLibraryUsage(libName string) (string, error) {
	script := fmt.Sprintf(`
import org.jenkinsci.plugins.workflow.libs.LibrariesAction
println "JOB\tVERSION\tBUILD"
Jenkins.instance.getAllItems(org.jenkinsci.plugins.workflow.job.WorkflowJob.class).each { job ->
    def lb = job.lastBuild
    if (lb != null) {
        def action = lb.getAction(LibrariesAction.class)
        if (action != null) {
            def entry = action.libraries.find { it.name == '%s' }
            if (entry != null) {
                println "${job.fullName}\t${entry.version}\t#${lb.number}"
            }
        }
    }
}
`, libName)
	return jc.ExecuteGroovy(script)
}

// GetLibrarySignatures extracts 'def call' signatures from a library's global variables.
func (jc *JenkinsClient) GetLibrarySignatures(libName string) (string, error) {
	script := fmt.Sprintf(`
def root = Jenkins.instance.rootPath.remote
// Note: This relies on the master filesystem cache.
def varsDir = new File(root, "workflow-libs/vars")
if (!varsDir.exists()) {
    println "ERROR: Library cache not found on master. Run a build that uses this library first."
    return
}

println "STEP\tSIGNATURE"
varsDir.eachFileMatch(~/.*\.groovy/) { file ->
    def name = file.name.take(file.name.lastIndexOf('.'))
    def signatures = []
    file.eachLine { line ->
        if (line.contains("def call")) {
            signatures << line.trim()
        }
    }
    if (signatures) {
        signatures.each { println "${name}\t${it}" }
    } else {
        println "${name}\t(no call method found)"
    }
}
`)
	return jc.ExecuteGroovy(script)
}

// AddFolderSharedLibrary adds a shared library to a specific folder.
func (jc *JenkinsClient) AddFolderSharedLibrary(folderName, name, url, version, credentialsId string) error {
	script := fmt.Sprintf(`
import org.jenkinsci.plugins.workflow.libs.*
import jenkins.plugins.git.*
import com.cloudbees.hudson.plugins.folder.properties.FolderLibraries

def folder = Jenkins.instance.getItemByFullName("%s")
if (folder == null) { println "ERROR: Folder not found"; return }

def scm = new SCMSourceRetriever(new GitSCMSource(null, "%s", "%s", "*", "", false))
def newLib = new LibraryConfiguration("%s", scm)
newLib.setDefaultVersion("%s")
newLib.setAllowVersionOverride(true)

def prop = folder.getProperties().get(FolderLibraries.class)
def libs = []
if (prop != null) {
    libs = new ArrayList(prop.libraries)
    libs.removeAll { it.name == "%s" }
}
libs.add(newLib)

folder.addProperty(new FolderLibraries(libs))
folder.save()
println "SUCCESS"
`, folderName, url, credentialsId, name, version, name)
	out, err := jc.ExecuteGroovy(script)
	if err == nil && strings.Contains(out, "ERROR:") {
		return errors.New(strings.TrimSpace(out))
	}
	return err
}

// ReadGlobalLibraryScript reads a script from the legacy workflow-libs/vars directory.
func (jc *JenkinsClient) ReadGlobalLibraryScript(varName string) (string, error) {
	script := fmt.Sprintf(`
def file = new File(Jenkins.instance.rootPath.remote, "workflow-libs/vars/%s.groovy")
if (file.exists()) {
    println file.text
} else {
    println "ERROR: File not found"
}
`, varName)
	output, err := jc.ExecuteGroovy(script)
	if strings.HasPrefix(output, "ERROR:") {
		return "", errors.New(strings.TrimSpace(output))
	}
	return output, err
}

// WriteGlobalLibraryScript writes a script to the legacy workflow-libs/vars directory.
func (jc *JenkinsClient) WriteGlobalLibraryScript(varName, content string) error {
	script := fmt.Sprintf(`
def dir = new File(Jenkins.instance.rootPath.remote, "workflow-libs/vars")
if (!dir.exists()) dir.mkdirs()
def file = new File(dir, "%s.groovy")
file.text = '''%s'''
println "SUCCESS"
`, varName, content)
	output, err := jc.ExecuteGroovy(script)
	if !strings.Contains(output, "SUCCESS") {
		return fmt.Errorf("failed to write script: %s", output)
	}
	return err
}

// AddSharedLibrary adds a new Git-based global shared library.
func (jc *JenkinsClient) AddSharedLibrary(name, url, version, credentialsId string) error {
	script := fmt.Sprintf(`
import org.jenkinsci.plugins.workflow.libs.*

def scm = new org.jenkinsci.plugins.workflow.libs.SCMSourceRetriever(
    new jenkins.plugins.git.GitSCMSource(null, "%s", "%s", "*", "", false)
)
def newLib = new LibraryConfiguration("%s", scm)
newLib.setDefaultVersion("%s")
newLib.setImplicit(false)
newLib.setAllowVersionOverride(true)

def globalLibs = GlobalLibraries.get()
def libs = new ArrayList(globalLibs.libraries)
libs.removeAll { it.name == "%s" }
libs.add(newLib)
globalLibs.setLibraries(libs)
globalLibs.save()
println "Library %s added/updated"
`, url, credentialsId, name, version, name, name)
	_, err := jc.ExecuteGroovy(script)
	return err
}

// DeleteSharedLibrary removes a global shared library.
func (jc *JenkinsClient) DeleteSharedLibrary(name string) error {
	script := fmt.Sprintf(`
import org.jenkinsci.plugins.workflow.libs.GlobalLibraries
def globalLibs = GlobalLibraries.get()
def libs = new ArrayList(globalLibs.libraries)
libs.removeAll { it.name == "%s" }
globalLibs.setLibraries(libs)
globalLibs.save()
println "Library %s removed"
`, name, name)
	_, err := jc.ExecuteGroovy(script)
	return err
}

// ListLibraryVars returns the custom steps (global variables) provided by a library.
func (jc *JenkinsClient) ListLibraryVars(libName string) (string, error) {
	script := fmt.Sprintf(`
import org.jenkinsci.plugins.workflow.libs.*
def lib = GlobalLibraries.get().libraries.find { it.name == '%s' }
if (lib == null) { println "Library not found"; return }
println "Library: ${lib.name}"
println "Retriever: ${lib.retriever.class.simpleName}"
println "\nAVAILABLE STEPS (vars/):"
def varsDir = new File(Jenkins.instance.rootPath.remote, "workflow-libs/vars")
if (varsDir.exists()) {
    varsDir.eachFileMatch(~/.*\.groovy/) { println " - ${it.name.take(it.name.lastIndexOf('.'))}" }
} else {
    println "Note: Library source not cached on master."
}
`, libName)
	return jc.ExecuteGroovy(script)
}

// GetBuildInfo returns an enhanced summary of a build including library usage.
func (jc *JenkinsClient) GetBuildInfo(jobName string, buildNumber int64) (string, error) {
	script := fmt.Sprintf(`
def job = Jenkins.instance.getItem('%s')
def build = job.getBuildByNumber(%d)
if (build == null) { println "Build not found"; return }

println "BUILD: ${build.fullDisplayName}"
println "RESULT: ${build.result}"
println "DURATION: ${build.durationString}"

def libs = build.getAction(org.jenkinsci.plugins.workflow.libs.LibrariesAction.class)
if (libs != null && libs.libraries) {
    println "\nSHARED LIBRARIES USED:"
    libs.libraries.each { lib ->
        println " - ${lib.name}@${lib.version}"
    }
}
`, jobName, buildNumber)
	return jc.ExecuteGroovy(script)
}

// ReplayBuild re-runs a build with a modified Groovy script.
func (jc *JenkinsClient) ReplayBuild(jobName string, buildNumber int64, script string) (string, error) {
	groovy := fmt.Sprintf(`
def job = Jenkins.instance.getItem('%s')
def build = job.getBuildByNumber(%d)
def action = build.getAction(org.jenkinsci.plugins.workflow.cps.replay.ReplayAction.class)
if (action == null) { println "ERROR: Replay not supported"; return }

def scriptMap = [:]
scriptMap.put(action.originalScript, '''%s''')
action.run("", scriptMap)
println "SUCCESS: Replay triggered"
`, jobName, buildNumber, script)
	return jc.ExecuteGroovy(groovy)
}

// GetLibraryDoc retrieves the help text (.txt file) for a custom pipeline step.
func (jc *JenkinsClient) GetLibraryDoc(libName, varName string) (string, error) {
	script := fmt.Sprintf(`
// We search for documentation in the global library cache on master.
// Note: This only works if the library has been loaded/cached.
def varName = '%s'
def root = Jenkins.instance.rootPath.remote
def found = false

// Check global workflow-libs
def docFile = new File(root, "workflow-libs/vars/${varName}.txt")
if (docFile.exists()) {
    println "DOCS FOR ${varName} (Global):"
    println "---"
    println docFile.text
    found = true
}

if (!found) {
    println "No documentation found on master cache for '${varName}'. Try running a build first."
}
`, varName)
	return jc.ExecuteGroovy(script)
}

// GetScanLogs retrieves the indexing/scan logs for a multibranch project or folder.
func (jc *JenkinsClient) GetScanLogs(jobName string) (string, error) {
	script := fmt.Sprintf(`
def item = Jenkins.instance.getItem('%s')
if (item instanceof jenkins.branch.MultiBranchProject) {
    def logFile = item.getComputation().getLogFile()
    if (logFile.exists()) {
        println logFile.text
    } else {
        println "No scan logs found."
    }
} else if (item instanceof com.cloudbees.hudson.plugins.folder.computed.ComputedFolder) {
    def logFile = item.getComputation().getLogFile()
    if (logFile.exists()) {
        println logFile.text
    } else {
        println "No scan logs found."
    }
} else {
    println "ERROR: Job is not a multibranch project or computed folder"
}
`, jobName)
	return jc.ExecuteGroovy(script)
}

// ScanJob triggers a scan/indexing of a Multibranch or Org folder.
func (jc *JenkinsClient) ScanJob(jobName string) error {
	script := fmt.Sprintf(`
def item = Jenkins.instance.getItem('%s')
if (item instanceof jenkins.branch.MultiBranchProject) {
    item.index()
    println "Scanning triggered"
} else if (item instanceof com.cloudbees.hudson.plugins.folder.computed.ComputedFolder) {
    item.triggerScan()
    println "Scanning triggered"
} else {
    println "Job is not a multibranch project or folder"
}
`, jobName)
	_, err := jc.ExecuteGroovy(script)
	return err
}

// SignalInput proceeds or aborts a pending 'input' step in a pipeline.
func (jc *JenkinsClient) SignalInput(jobName string, buildNumber int64, inputId string, abort bool) error {
	script := fmt.Sprintf(`
def job = Jenkins.instance.getItem('%s')
def build = job.getBuildByNumber(%d)
def action = build.getAction(org.jenkinsci.plugins.workflow.support.steps.input.InputAction.class)
if (action == null) { println "No pending inputs"; return }

def input = action.getExecutions().find { it.id == '%s' || '%s' == '' }
if (input == null) { println "Input not found"; return }

if (%v) {
    input.doAbort()
    println "Input aborted"
} else {
    input.doProceed(null)
    println "Input proceeded"
}
`, jobName, buildNumber, inputId, inputId, abort)
	_, err := jc.ExecuteGroovy(script)
	return err
}

// GenerateSnippet generates a Groovy DSL snippet or searches for steps.
func (jc *JenkinsClient) GenerateSnippet(query string) (string, error) {
	script := fmt.Sprintf(`
def descriptors = Jenkins.instance.getExtensionList(org.jenkinsci.plugins.workflow.steps.StepDescriptor.class)
def exact = descriptors.find { it.functionName == '%s' || it.displayName == '%s' }

if (exact != null) {
    println "HELP FOR: ${exact.functionName}"
    println "DISPLAY NAME: ${exact.displayName}"
    println "CLASS: ${exact.clazz.name}"
    return
}

println "SEARCH RESULTS FOR: %s"
descriptors.findAll { it.functionName?.contains('%s') || it.displayName?.contains('%s') }.each {
    println " - ${it.functionName} (${it.displayName})"
}
`, query, query, query, query, query)
	return jc.ExecuteGroovy(script)
}

// GetGlobalTools lists all configured tool installations (JDK, Maven, etc.)
func (jc *JenkinsClient) GetGlobalTools() (string, error) {
	script := `
println "TYPE\tNAME\tHOME"
Jenkins.instance.getExtensionList(hudson.tools.ToolDescriptor.class).each { desc ->
    try {
        desc.installations.each { tool ->
            println "${desc.displayName}\t${tool.name}\t${tool.home ?: 'Auto-install'}"
        }
    } catch (e) { }
}
`
	return jc.ExecuteGroovy(script)
}

// CloneJob copies a job from one name to another.
func (jc *JenkinsClient) CloneJob(sourceName, destName string) error {
	_, err := jc.Client.CopyJob(jc.ctx, sourceName, destName)
	return err
}

// AnalyzeFailure performs a "Doctor" analysis on a failed build.
func (jc *JenkinsClient) AnalyzeFailure(jobName string, buildNumber int64) (string, error) {
	report, err := jc.DoctorReport(jobName, buildNumber)
	if err != nil {
		return "", err
	}
	return report.FormatText(), nil
}

// CreateCredential creates a simple Secret Text credential.
func (jc *JenkinsClient) CreateCredential(id, secret, description string) error {
	script := fmt.Sprintf(`
import org.jenkinsci.plugins.plaincredentials.impl.StringCredentialsImpl
import com.cloudbees.plugins.credentials.*
import com.cloudbees.plugins.credentials.domains.Domain
import hudson.util.Secret

def cred = new StringCredentialsImpl(CredentialsScope.GLOBAL, "%s", "%s", Secret.fromString("%s"))
SystemCredentialsProvider.instance.store.addCredentials(Domain.global(), cred)
println "Credential created"
`, id, description, secret)
	_, err := jc.ExecuteGroovy(script)
	return err
}

// DeleteCredential deletes a credential by ID.
func (jc *JenkinsClient) DeleteCredential(id string) error {
	script := fmt.Sprintf(`
import com.cloudbees.plugins.credentials.*
import com.cloudbees.plugins.credentials.domains.Domain

def store = SystemCredentialsProvider.instance.store
def creds = store.getCredentials(Domain.global())
def toRemove = creds.find { it.id == '%s' }

if (toRemove) {
    store.removeCredentials(Domain.global(), toRemove)
    println "SUCCESS"
} else {
    println "ERROR: Credential not found"
}
`, id)
	out, err := jc.ExecuteGroovy(script)
	if err != nil {
		return err
	}
	if strings.HasPrefix(out, "ERROR:") {
		return errors.New(strings.TrimSpace(out))
	}
	return nil
}

// ListEnvVars lists global environment variables.
func (jc *JenkinsClient) ListEnvVars() (string, error) {
	script := `
println "KEY\tVALUE"
def props = Jenkins.instance.getGlobalNodeProperties()
def envNodes = props.getAll(hudson.slaves.EnvironmentVariablesNodeProperty.class)
if (envNodes.size() > 0) {
    envNodes.get(0).getEnvVars().each { k, v ->
        println "${k}\t${v}"
    }
}
`
	return jc.ExecuteGroovy(script)
}

// SetEnvVar sets a global environment variable.
func (jc *JenkinsClient) SetEnvVar(key, value string) error {
	script := fmt.Sprintf(`
def props = Jenkins.instance.getGlobalNodeProperties()
def envNodes = props.getAll(hudson.slaves.EnvironmentVariablesNodeProperty.class)
hudson.slaves.EnvironmentVariablesNodeProperty envProp
if (envNodes.size() == 0) {
    envProp = new hudson.slaves.EnvironmentVariablesNodeProperty()
    props.add(envProp)
} else {
    envProp = envNodes.get(0)
}
envProp.getEnvVars().put('%s', '''%s''')
Jenkins.instance.save()
println "SUCCESS"
`, key, value)
	out, err := jc.ExecuteGroovy(script)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "SUCCESS") {
		return fmt.Errorf("failed to set env var: %s", out)
	}
	return nil
}

// DeleteEnvVar deletes a global environment variable.
func (jc *JenkinsClient) DeleteEnvVar(key string) error {
	script := fmt.Sprintf(`
def props = Jenkins.instance.getGlobalNodeProperties()
def envNodes = props.getAll(hudson.slaves.EnvironmentVariablesNodeProperty.class)
if (envNodes.size() > 0) {
    def envProp = envNodes.get(0)
    if (envProp.getEnvVars().containsKey('%s')) {
        envProp.getEnvVars().remove('%s')
        Jenkins.instance.save()
        println "SUCCESS"
        return
    }
}
println "ERROR: Variable not found"
`, key, key)
	out, err := jc.ExecuteGroovy(script)
	if err != nil {
		return err
	}
	if strings.HasPrefix(out, "ERROR:") {
		return errors.New(strings.TrimSpace(out))
	}
	return nil
}

// UploadPlugin uploads a local .hpi file to Jenkins for installation.
func (jc *JenkinsClient) UploadPlugin(filePath string) error {
	// PostFiles is problematic with crumbs on some Jenkins versions.
	// We'll use a manually constructed request with the patched gojenkins requester.
	resp, err := jc.Client.Requester.PostFiles(jc.ctx, "/pluginManager/uploadPlugin", nil, nil, nil, []string{filePath})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jenkins returned error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetSystemLogs retrieves the main Jenkins system log.
func (jc *JenkinsClient) GetSystemLogs() (string, error) {
	return jc.ExecuteGroovy("println hudson.model.LogRecorder.instance.getLogRecords().join('\\n')")
}

// ListPendingApprovals lists pending script and signature approvals.
func (jc *JenkinsClient) ListPendingApprovals() (string, error) {
	script := `
import org.jenkinsci.plugins.scriptsecurity.scripts.ScriptApproval

def sa = ScriptApproval.get()
println "TYPE\tHASH/SIGNATURE"

sa.pendingSignatures.each { s ->
    println "SIGNATURE\t${s.signature}"
}

sa.pendingScripts.each { s ->
    println "SCRIPT\t${s.hash}"
}
`
	return jc.ExecuteGroovy(script)
}

// ApproveScript approves a script or signature by hash/signature.
func (jc *JenkinsClient) ApproveScript(hashOrSignature string) error {
	script := fmt.Sprintf(`
import org.jenkinsci.plugins.scriptsecurity.scripts.ScriptApproval

def sa = ScriptApproval.get()
def target = '''%s'''
def approved = false

def pendingSig = sa.pendingSignatures.find { it.signature == target }
if (pendingSig != null) {
    sa.approveSignature(pendingSig.signature)
    println "SUCCESS: Approved signature: ${target}"
    approved = true
}

def pendingScript = sa.pendingScripts.find { it.hash == target }
if (pendingScript != null) {
    sa.approveScript(pendingScript.hash)
    println "SUCCESS: Approved script hash: ${target}"
    approved = true
}

if (!approved) {
    println "ERROR: Hash or signature not found in pending queue."
}
`, hashOrSignature)
	out, err := jc.ExecuteGroovy(script)
	if err != nil {
		return err
	}
	if strings.HasPrefix(out, "ERROR:") {
		return errors.New(strings.TrimSpace(out))
	}
	return nil
}

// LintPipeline validates a Declarative Pipeline (Jenkinsfile).
func (jc *JenkinsClient) LintPipeline(content string) (string, error) {
	// We escape single quotes to pass the content safely into the groovy string.
	escapedContent := strings.ReplaceAll(content, "'", "\\'")
	script := fmt.Sprintf(`
try {
    def pipelineDef = org.jenkinsci.plugins.pipeline.modeldefinition.parser.Converter.scriptToPipelineDef('''%s''')
    println "SUCCESS: Jenkinsfile successfully validated."
} catch (Throwable e) {
    println "ERROR: Validation failed:"
    println e.message
}
`, escapedContent)
	return jc.ExecuteGroovy(script)
}

// DisablePlugin disables a plugin by ID.
func (jc *JenkinsClient) DisablePlugin(pluginId string) error {
	script := fmt.Sprintf(`
def p = Jenkins.instance.pluginManager.getPlugin('%s')
if (p == null) { println "Plugin not found"; return }
p.enabled = false
p.save()
Jenkins.instance.pluginManager.save()
println "Plugin %s disabled"
`, pluginId, pluginId)
	_, err := jc.ExecuteGroovy(script)
	return err
}

// ListPendingInputs returns all builds currently blocked on an 'input' step.
func (jc *JenkinsClient) ListPendingInputs() (string, error) {
	script := `
println "JOB\tBUILD\tID\tPROMPT"
Jenkins.instance.getAllItems(org.jenkinsci.plugins.workflow.job.WorkflowJob.class).each { job ->
    job.builds.findAll { it.isBuilding() }.each { build ->
        def action = build.getAction(org.jenkinsci.plugins.workflow.support.steps.input.InputAction.class)
        if (action != null) {
            action.getExecutions().each { input ->
                println "${job.fullName}\t#${build.number}\t${input.id}\t${input.message}"
            }
        }
    }
}
`
	return jc.ExecuteGroovy(script)
}

// ListJobBranches lists branches of a multibranch pipeline.
func (jc *JenkinsClient) ListJobBranches(jobName string) (string, error) {
	script := fmt.Sprintf(`
def item = Jenkins.instance.getItem('%s')
if (!(item instanceof jenkins.branch.MultiBranchProject)) {
    println "ERROR: Not a multibranch project"
    return
}
println "BRANCH\tSTATUS\tLAST_BUILD"
item.getItems().each { branch ->
    def lb = branch.lastBuild
    println "${branch.name}\t${branch.disabled ? 'DISABLED' : 'ACTIVE'}\t${lb ? '#' + lb.number + ' (' + lb.result + ')' : 'NONE'}"
}
`, jobName)
	return jc.ExecuteGroovy(script)
}

// --- frozen build (agent remoting) operations ---

// FrozenBuild represents a build paused on an input step, preserving its
// workspace for interactive debugging.
type FrozenBuild struct {
	Job       string `json:"job"`
	Build     int64  `json:"build"`
	Node      string `json:"node"`
	Workspace string `json:"workspace"`
	InputID   string `json:"inputId"`
	Prompt    string `json:"prompt"`
}

// AgentExecResult captures the output of a command executed on a remote agent.
type AgentExecResult struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// ListFrozenJobs returns all builds currently waiting on an input step
// (frozen), enriched with the agent node and workspace they hold open.
func (jc *JenkinsClient) ListFrozenJobs() ([]FrozenBuild, error) {
	script := `
import groovy.json.JsonBuilder
def result = []

Jenkins.instance.getAllItems(org.jenkinsci.plugins.workflow.job.WorkflowJob.class).each { job ->
    job.builds.findAll { it.isBuilding() }.each { build ->
        def action = build.getAction(org.jenkinsci.plugins.workflow.support.steps.input.InputAction.class)
        if (action != null) {
            action.getExecutions().each { input ->
                def nodeName = build.getBuiltOnStr() ?: ""
                def ws = ""
                try {
                    def computer = build.getBuiltOn()
                    if (computer != null) {
                        def wsl = computer.getWorkspaceList()
                        def executor = build.getExecutor()
                        if (executor != null && executor.getCurrentWorkspace() != null) {
                            ws = executor.getCurrentWorkspace().remote
                        }
                    }
                } catch (Throwable ignored) {}
                result << [
                    job:       job.fullName,
                    build:     build.number,
                    node:      nodeName,
                    workspace: ws,
                    inputId:   input.id,
                    prompt:    input.message ?: ""
                ]
            }
        }
    }
}
println new JsonBuilder(result).toPrettyString()
`
	out, err := jc.ExecuteGroovy(script)
	if err != nil {
		return nil, err
	}
	var builds []FrozenBuild
	if err := json.Unmarshal([]byte(out), &builds); err != nil {
		return nil, fmt.Errorf("parse frozen builds: %w (raw: %s)", err, out)
	}
	return builds, nil
}

// AgentExec runs a shell command on a remote agent's workspace via the
// remoting channel. Returns exit code, stdout, and stderr.
func (jc *JenkinsClient) AgentExec(nodeName, workspace, command string) (*AgentExecResult, error) {
	cmdB64 := base64.StdEncoding.EncodeToString([]byte(command))
	script := fmt.Sprintf(`
import groovy.json.JsonBuilder
def node = Jenkins.instance.getNode('%s')
if (node == null) { println '{"error":"node not found"}'; return }

def computer = node.toComputer()
if (computer == null) { println '{"error":"computer offline"}'; return }

def channel = computer.channel
if (channel == null) { println '{"error":"no remoting channel"}'; return }

def cmd = new String(java.util.Base64.decoder.decode('%s'), 'UTF-8')
def result = channel.call(new hudson.remoting.Callable<Map, Exception>() {
    Map call() {
        def proc = ["bash", "-c", cmd].execute(null, new File('%s'))
        def out = new StringBuilder(), err = new StringBuilder()
        proc.consumeProcessOutput(out, err)
        proc.waitFor()
        return [exitCode: proc.exitValue(), stdout: out.toString(), stderr: err.toString()]
    }
})
println new JsonBuilder(result).toPrettyString()
`, nodeName, cmdB64, workspace)

	out, err := jc.ExecuteGroovy(script)
	if err != nil {
		return nil, err
	}

	var result AgentExecResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil, fmt.Errorf("parse agent exec result: %w (raw: %s)", err, out)
	}
	return &result, nil
}

// AgentReadFile reads a file from a remote agent's workspace via the remoting
// channel and returns its content.
func (jc *JenkinsClient) AgentReadFile(nodeName, workspace, filePath string) (string, error) {
	script := fmt.Sprintf(`
def node = Jenkins.instance.getNode('%s')
if (node == null) { println '{"error":"node not found"}'; return }
def computer = node.toComputer()
if (computer == null) { println '{"error":"computer offline"}'; return }
def channel = computer.channel
if (channel == null) { println '{"error":"no remoting channel"}'; return }

def result = channel.call(new hudson.remoting.Callable<String, Exception>() {
    String call() {
        def file = new File(new File('%s'), '%s')
        if (!file.exists()) { return '{"error":"file not found"}' }
        return file.text
    }
})
println result
`, nodeName, workspace, filePath)

	out, err := jc.ExecuteGroovy(script)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(out, `{"error"`) {
		return "", errors.New(strings.TrimSpace(out))
	}
	return out, nil
}

// AgentWriteFile writes content to a file on a remote agent's workspace via
// the remoting channel.
func (jc *JenkinsClient) AgentWriteFile(nodeName, workspace, filePath, content string) error {
	contentB64 := base64.StdEncoding.EncodeToString([]byte(content))
	script := fmt.Sprintf(`
def node = Jenkins.instance.getNode('%s')
if (node == null) { println '{"error":"node not found"}'; return }
def computer = node.toComputer()
if (computer == null) { println '{"error":"computer offline"}'; return }
def channel = computer.channel
if (channel == null) { println '{"error":"no remoting channel"}'; return }

def data = java.util.Base64.decoder.decode('%s')
def result = channel.call(new hudson.remoting.Callable<String, Exception>() {
    String call() {
        def file = new File(new File('%s'), '%s')
        file.parentFile.mkdirs()
        file.bytes = data
        return 'ok'
    }
})
println result
`, nodeName, contentB64, workspace, filePath)

	out, err := jc.ExecuteGroovy(script)
	if err != nil {
		return err
	}
	if strings.HasPrefix(out, `{"error"`) {
		return errors.New(strings.TrimSpace(out))
	}
	return nil
}

// AgentSnapshotWorkspace creates a tarball of the workspace on the remote
// agent and returns its remote path. The tarball can then be retrieved via
// AgentReadFile or a build artifact upload.
func (jc *JenkinsClient) AgentSnapshotWorkspace(nodeName, workspace string) (map[string]any, error) {
	script := fmt.Sprintf(`
import groovy.json.JsonBuilder
def node = Jenkins.instance.getNode('%s')
if (node == null) { println '{"error":"node not found"}'; return }
def computer = node.toComputer()
if (computer == null) { println '{"error":"computer offline"}'; return }
def channel = computer.channel
if (channel == null) { println '{"error":"no remoting channel"}'; return }

def info = channel.call(new hudson.remoting.Callable<Map, Exception>() {
    Map call() {
        def ws = new File('%s')
        def tarFile = new File(File.createTempFile("frozen-workspace-", ".tar.gz").parentFile, "frozen-snapshot-" + System.currentTimeMillis() + ".tar.gz")
        def proc = ["tar", "czf", tarFile.absolutePath, "-C", ws.absolutePath, "."].execute()
        proc.waitFor()
        return [exitCode: proc.exitValue(), path: tarFile.absolutePath,
                size: tarFile.length(), name: tarFile.name]
    }
})
println new JsonBuilder(info).toPrettyString()
`, nodeName, workspace)

	out, err := jc.ExecuteGroovy(script)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil, fmt.Errorf("parse snapshot info: %w (raw: %s)", err, out)
	}
	if exitCode, ok := result["exitCode"].(float64); ok && exitCode != 0 {
		return nil, fmt.Errorf("tar failed with exit code %.0f", exitCode)
	}
	return result, nil
}

// AgentReattempt runs a shell command on the frozen build's agent workspace
// and returns the exit code and output. This is the fix-and-retry primitive.
func (jc *JenkinsClient) AgentReattempt(nodeName, workspace, command string) (*AgentExecResult, error) {
	return jc.AgentExec(nodeName, workspace, command)
}

// ThawFrozenBuild submits the pending input on a frozen build, resuming or
// finishing the pipeline. Returns the input ID that was submitted.
func (jc *JenkinsClient) ThawFrozenBuild(jobName string, buildNumber int64, inputId string) error {
	return jc.SignalInput(jobName, buildNumber, inputId, false)
}

// EnablePlugin enables a plugin by ID.
func (jc *JenkinsClient) EnablePlugin(pluginId string) error {
	script := fmt.Sprintf(`
def p = Jenkins.instance.pluginManager.getPlugin('%s')
if (p == null) { println "Plugin not found"; return }
p.enabled = true
p.save()
Jenkins.instance.pluginManager.save()
println "Plugin %s enabled"
`, pluginId, pluginId)
	_, err := jc.ExecuteGroovy(script)
	return err
}
