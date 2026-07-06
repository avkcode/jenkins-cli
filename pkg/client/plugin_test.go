package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bndr/gojenkins"
)

func TestInstallPluginAcceptsHTMLUpdateCenterResponse(t *testing.T) {
	var installBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/crumbIssuer/api/json":
			w.Header().Set("Content-Type", "application/json")
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "crumb-session"})
			_, _ = w.Write([]byte(`{"crumbRequestField":"Jenkins-Crumb","crumb":"test-crumb"}`))
		case "/pluginManager/installNecessaryPlugins":
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			if got := r.Header.Get("Jenkins-Crumb"); got != "test-crumb" {
				t.Fatalf("expected crumb header, got %q", got)
			}
			session, err := r.Cookie("JSESSIONID")
			if err != nil || session.Value != "crumb-session" {
				t.Fatalf("expected crumb session cookie, got cookie=%v err=%v", session, err)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			installBody = string(body)
			w.Header().Set("Content-Type", "text/html;charset=utf-8")
			_, _ = w.Write([]byte(`<html><title>Update Center</title></html>`))
		case "/pluginManager/checkUpdatesServer":
			w.WriteHeader(http.StatusOK)
		case "/pluginManager/api/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"plugins":[{"shortName":"matrix-auth"}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	jc := &JenkinsClient{
		Client: &gojenkins.Jenkins{
			Requester: &gojenkins.Requester{
				Base:   server.URL,
				Client: server.Client(),
			},
		},
	}
	if err := jc.InstallPlugin(context.Background(), "matrix-auth", "latest"); err != nil {
		t.Fatalf("install plugin: %v", err)
	}
	if !strings.Contains(installBody, `plugin="matrix-auth@latest"`) {
		t.Fatalf("expected plugin install XML, got %q", installBody)
	}
}

func TestInstallPluginReportsUpdateCenterFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/crumbIssuer/api/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"crumbRequestField":"Jenkins-Crumb","crumb":"c"}`))
		case "/pluginManager/checkUpdatesServer":
			w.WriteHeader(http.StatusOK)
		case "/pluginManager/installNecessaryPlugins":
			w.Header().Set("Content-Type", "text/html;charset=utf-8")
			_, _ = w.Write([]byte(`<html></html>`))
		case "/pluginManager/api/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"plugins":[]}`))
		case "/updateCenter/api/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jobs":[{"plugin":{"name":"allure-jenkins-plugin"},"errorMessage":"download failed","status":{"type":"Failure"}}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	jc := &JenkinsClient{
		Client: &gojenkins.Jenkins{
			Requester: &gojenkins.Requester{
				Base:   server.URL,
				Client: server.Client(),
			},
		},
	}
	err := jc.InstallPlugin(context.Background(), "allure-jenkins-plugin", "latest")
	if err == nil {
		t.Fatal("expected install failure to be reported")
	}
	if !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("expected update-center failure message, got %v", err)
	}
}

func TestInstallPluginRequiresID(t *testing.T) {
	jc := &JenkinsClient{}
	if err := jc.InstallPlugin(context.Background(), "", "latest"); err == nil {
		t.Fatal("expected missing plugin id error")
	}
}

func TestSafeRestartAcceptsRestartingPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/crumbIssuer/api/json":
			w.Header().Set("Content-Type", "application/json")
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "restart-session"})
			_, _ = w.Write([]byte(`{"crumbRequestField":"Jenkins-Crumb","crumb":"restart-crumb"}`))
		case "/safeRestart":
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			if got := r.Header.Get("Jenkins-Crumb"); got != "restart-crumb" {
				t.Fatalf("expected crumb header, got %q", got)
			}
			session, err := r.Cookie("JSESSIONID")
			if err != nil || session.Value != "restart-session" {
				t.Fatalf("expected crumb session cookie, got cookie=%v err=%v", session, err)
			}
			w.Header().Set("Content-Type", "text/html;charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`<html><title>Restarting Jenkins</title><body>Jenkins is restarting</body></html>`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	jc := &JenkinsClient{
		Client: &gojenkins.Jenkins{Server: server.URL},
		ctx:    context.Background(),
	}
	if err := jc.SafeRestart(); err != nil {
		t.Fatalf("safe restart: %v", err)
	}
}

func TestGetJobConfigByURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/job/example/config.xml" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "token" {
			t.Fatalf("expected basic auth, got user=%q ok=%v", user, ok)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<project><description>backup</description></project>`))
	}))
	defer server.Close()

	jc := &JenkinsClient{username: "admin", password: "token"}
	config, err := jc.GetJobConfigByURL(context.Background(), server.URL+"/job/example/")
	if err != nil {
		t.Fatalf("get job config: %v", err)
	}
	if !strings.Contains(config, "<description>backup</description>") {
		t.Fatalf("unexpected config: %s", config)
	}
}
