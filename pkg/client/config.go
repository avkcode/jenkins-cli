package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GetJobConfigByURL retrieves a job config.xml from an absolute Jenkins job URL.
func (jc *JenkinsClient) GetJobConfigByURL(ctx context.Context, jobURL string) (string, error) {
	if strings.TrimSpace(jobURL) == "" {
		return "", fmt.Errorf("job url is required")
	}
	endpoint := strings.TrimRight(jobURL, "/") + "/config.xml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	if jc.username != "" {
		req.SetBasicAuth(jc.username, jc.password)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("jenkins returned error %d while reading %s: %s", resp.StatusCode, endpoint, string(body))
	}
	return string(body), nil
}
