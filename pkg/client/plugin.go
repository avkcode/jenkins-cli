package client

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bndr/gojenkins"
)

// InstallPlugin installs a plugin and its dependencies through Jenkins' update
// center, then waits for the asynchronous installation to finish. It first
// refreshes the update-center catalog so the plugin and its transitive
// dependencies can be resolved -- without that refresh, installNecessaryPlugins
// is a silent no-op against a stale or empty catalog, which is why complex
// plugins such as allure-jenkins-plugin appeared to install but never did.
func (jc *JenkinsClient) InstallPlugin(ctx context.Context, id, version string) error {
	if id == "" {
		return fmt.Errorf("plugin id is required")
	}
	if version == "" {
		version = "latest"
	}
	baseURL := jc.jenkinsBaseURL()
	if baseURL == "" {
		return fmt.Errorf("jenkins base URL is not configured")
	}
	base := strings.TrimRight(baseURL, "/")

	// Refresh the catalog so dependencies are resolvable before requesting install.
	jc.refreshUpdateCenter(ctx, base)

	var plugin bytes.Buffer
	if err := xml.EscapeText(&plugin, []byte(id+"@"+version)); err != nil {
		return err
	}
	payload := fmt.Sprintf(`<jenkins><install plugin="%s" /></jenkins>`, plugin.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/pluginManager/installNecessaryPlugins", strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml;charset=utf-8")
	if jc.username != "" {
		req.SetBasicAuth(jc.username, jc.password)
	}
	jc.addCrumbForBase(ctx, req, base)

	resp, err := jc.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jenkins returned status %d while installing plugin %s: %s", resp.StatusCode, id, strings.TrimSpace(string(body)))
	}

	// installNecessaryPlugins schedules the download asynchronously; wait until
	// the plugin (and its dependencies) actually finish installing so we report a
	// truthful result instead of fire-and-forget success.
	return jc.waitForPluginInstall(ctx, base, id)
}

// refreshUpdateCenter asks Jenkins to re-download the update-center catalog so
// plugin dependencies can be resolved. Best-effort: errors are ignored.
func (jc *JenkinsClient) refreshUpdateCenter(ctx context.Context, base string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/pluginManager/checkUpdatesServer", nil)
	if err != nil {
		return
	}
	if jc.username != "" {
		req.SetBasicAuth(jc.username, jc.password)
	}
	jc.addCrumbForBase(ctx, req, base)
	resp, err := jc.httpClient().Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// waitForPluginInstall polls until the plugin is installed, an update-center job
// reports a failure for it, or the context is canceled.
func (jc *JenkinsClient) waitForPluginInstall(ctx context.Context, base, id string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if installed, _ := jc.pluginInstalled(ctx, base, id); installed {
			return nil
		}
		if failure := jc.updateCenterFailure(ctx, base, id); failure != "" {
			return fmt.Errorf("installing plugin %s failed: %s", id, failure)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for plugin %s to install (it may still be downloading): %w", id, ctx.Err())
		case <-ticker.C:
		}
	}
}

// pluginInstalled reports whether a plugin with the given short name is present
// in Jenkins (installed; it may still require a restart to become active).
func (jc *JenkinsClient) pluginInstalled(ctx context.Context, base, id string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/pluginManager/api/json?depth=1", nil)
	if err != nil {
		return false, err
	}
	if jc.username != "" {
		req.SetBasicAuth(jc.username, jc.password)
	}
	resp, err := jc.httpClient().Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("status %d", resp.StatusCode)
	}
	var payload struct {
		Plugins []struct {
			ShortName string `json:"shortName"`
		} `json:"plugins"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, err
	}
	for _, p := range payload.Plugins {
		if p.ShortName == id {
			return true, nil
		}
	}
	return false, nil
}

// updateCenterFailure returns a non-empty message if an update-center job for
// the plugin has failed. Best-effort and lenient about the JSON shape.
func (jc *JenkinsClient) updateCenterFailure(ctx context.Context, base, id string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/updateCenter/api/json?depth=2", nil)
	if err != nil {
		return ""
	}
	if jc.username != "" {
		req.SetBasicAuth(jc.username, jc.password)
	}
	resp, err := jc.httpClient().Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	var payload struct {
		Jobs []struct {
			Plugin struct {
				Name string `json:"name"`
			} `json:"plugin"`
			ErrorMessage string `json:"errorMessage"`
			Status       struct {
				Type string `json:"type"`
			} `json:"status"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ""
	}
	for _, j := range payload.Jobs {
		if j.Plugin.Name != id {
			continue
		}
		if j.ErrorMessage != "" {
			return j.ErrorMessage
		}
		if strings.Contains(strings.ToLower(j.Status.Type), "fail") {
			return j.Status.Type
		}
	}
	return ""
}

func (jc *JenkinsClient) jenkinsBaseURL() string {
	if jc != nil && jc.Client != nil && jc.Client.Server != "" {
		return jc.Client.Server
	}
	if jc != nil && jc.Client != nil && jc.Client.Requester != nil {
		if requester, ok := jc.Client.Requester.(*gojenkins.Requester); ok {
			return requester.Base
		}
	}
	return ""
}

func (jc *JenkinsClient) httpClient() *http.Client {
	if jc != nil && jc.Client != nil && jc.Client.Requester != nil {
		if requester, ok := jc.Client.Requester.(*gojenkins.Requester); ok && requester.Client != nil {
			return requester.Client
		}
	}
	return http.DefaultClient
}

func (jc *JenkinsClient) addCrumbForBase(ctx context.Context, req *http.Request, baseURL string) {
	crumbReq, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/crumbIssuer/api/json", nil)
	if err != nil {
		return
	}
	if jc.username != "" {
		crumbReq.SetBasicAuth(jc.username, jc.password)
	}
	resp, err := jc.httpClient().Do(crumbReq)
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
}
