package client

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (jc *JenkinsClient) postSafeRestart() (int, string, error) {
	endpoint := strings.TrimRight(jc.Client.Server, "/") + "/safeRestart"
	req, err := http.NewRequestWithContext(jc.ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if jc.username != "" {
		req.SetBasicAuth(jc.username, jc.password)
	}
	jc.addSafeRestartCrumb(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", err
	}
	return resp.StatusCode, string(body), nil
}

func (jc *JenkinsClient) addSafeRestartCrumb(req *http.Request) {
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
