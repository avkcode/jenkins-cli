package client

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bndr/gojenkins"
)

func TestConsoleLogStreamStripsJenkinsHiddenNotesAcrossChunks(t *testing.T) {
	var out bytes.Buffer
	stream := newConsoleLogStream(&out)

	chunks := []string{
		"+ make -C /tmp/src O=/tmp/out-kernel-09 defconfig\n\x1b[8mha:////4Jid",
		"esDDY8TWW9fXElGydWNeViDNK2hL8qvcWQS5TaiaAAAAoh+LCAAAAAAAAP9tjTE",
		"OwjAQBM8BClpKHuFItIiK1krDC0x8GCfWnbEdkooX8TX+gCESFVvtrLSa5wtWKcK\x1b[0m",
		"+ make -C /tmp/src O=/tmp/out-kernel-10 -j2 bzImage\n",
	}
	for _, chunk := range chunks {
		if _, err := stream.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if err := stream.Flush(); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	if strings.Contains(got, "ha:////") || strings.Contains(got, "\x1b[8m") || strings.Contains(got, "\x1b[0m") {
		t.Fatalf("hidden Jenkins note leaked into output: %q", got)
	}
	for _, want := range []string{
		"+ make -C /tmp/src O=/tmp/out-kernel-09 defconfig",
		"+ make -C /tmp/src O=/tmp/out-kernel-10 -j2 bzImage",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing clean console line %q in %q", want, got)
		}
	}
}

func TestConsoleLogStreamKeepsSplitHiddenNotePrefixBuffered(t *testing.T) {
	var out bytes.Buffer
	stream := newConsoleLogStream(&out)

	if _, err := stream.Write([]byte("+ echo before\n\x1b[8m")); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "+ echo before\n" {
		t.Fatalf("unexpected output before marker completes: %q", got)
	}
	if _, err := stream.Write([]byte("ha:////hidden\x1b[0m+ echo after\n")); err != nil {
		t.Fatal(err)
	}
	if err := stream.Flush(); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	if got != "+ echo before\n+ echo after\n" {
		t.Fatalf("unexpected sanitized output: %q", got)
	}
}

func TestConsoleLogStreamWritesCompleteLines(t *testing.T) {
	var out bytes.Buffer
	stream := newConsoleLogStream(&out)

	if _, err := stream.Write([]byte("+ make -C /tmp/src O=/tmp/out-kernel-01")); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("partial line was written early: %q", out.String())
	}
	if _, err := stream.Write([]byte(" -j2 bzImage\n+ echo done")); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "+ make -C /tmp/src O=/tmp/out-kernel-01 -j2 bzImage\n" {
		t.Fatalf("unexpected complete-line output: %q", got)
	}
	if err := stream.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "+ make -C /tmp/src O=/tmp/out-kernel-01 -j2 bzImage\n+ echo done" {
		t.Fatalf("flush did not write trailing partial line: %q", got)
	}
}

func TestRawLogStreamKeepsJenkinsHiddenNotes(t *testing.T) {
	var out bytes.Buffer
	stream := newBuildLogStream(&out, true)

	raw := "\x1b[8mha:////hidden\x1b[0m+ make bzImage\n"
	if _, err := stream.Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	if err := stream.Flush(); err != nil {
		t.Fatal(err)
	}
	if out.String() != raw {
		t.Fatalf("raw stream changed output: %q", out.String())
	}
}

func TestStreamLogsHTTPRetriesJenkinsRestartAndResumes(t *testing.T) {
	var starts []string
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/logText/progressiveText") {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		starts = append(starts, r.URL.Query().Get("start"))
		requests++
		switch requests {
		case 1:
			w.Header().Set("X-Text-Size", "7")
			w.Header().Set("X-More-Data", "true")
			_, _ = w.Write([]byte("line 1\n"))
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("<html><title>Starting Jenkins</title></html>"))
		case 3:
			w.Header().Set("X-Text-Size", "14")
			w.Header().Set("X-More-Data", "false")
			_, _ = w.Write([]byte("line 2\n"))
		default:
			t.Fatalf("unexpected request %d", requests)
		}
	}))
	defer server.Close()

	jc := &JenkinsClient{
		Client: gojenkins.CreateJenkins(nil, server.URL, "", ""),
		ctx:    context.Background(),
	}
	var out bytes.Buffer
	err := jc.streamLogsHTTP("kernel-build", 2, &out, time.Millisecond, false, true)
	if err != nil {
		t.Fatal(err)
	}

	if got := out.String(); got != "line 1\nline 2\n" {
		t.Fatalf("unexpected streamed output: %q", got)
	}
	wantStarts := []string{"0", "7", "7"}
	if strings.Join(starts, ",") != strings.Join(wantStarts, ",") {
		t.Fatalf("unexpected resume offsets: got %v want %v", starts, wantStarts)
	}
}

func TestStreamLogsWithOptionsFollowsJenkinsConsoleWhenNATSIsConfigured(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/logText/progressiveText") {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		requests++
		switch requests {
		case 1:
			w.Header().Set("X-Text-Size", "7")
			w.Header().Set("X-More-Data", "true")
			_, _ = w.Write([]byte("line 1\n"))
		case 2:
			w.Header().Set("X-Text-Size", "14")
			w.Header().Set("X-More-Data", "false")
			_, _ = w.Write([]byte("line 2\n"))
		default:
			t.Fatalf("unexpected request %d", requests)
		}
	}))
	defer server.Close()

	jc := &JenkinsClient{
		Client: gojenkins.CreateJenkins(nil, server.URL, "", ""),
		ctx:    context.Background(),
	}
	var out bytes.Buffer
	err := jc.StreamLogsWithOptions("kernel-build", 2, &out, LogStreamOptions{
		PollInterval: time.Millisecond,
		Follow:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := out.String(); got != "line 1\nline 2\n" {
		t.Fatalf("unexpected streamed output: %q", got)
	}
}

func TestStreamLogsHTTPNoFollowReturnsAfterFirstChunk(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("X-Text-Size", "7")
		w.Header().Set("X-More-Data", "true")
		_, _ = w.Write([]byte("line 1\n"))
	}))
	defer server.Close()

	jc := &JenkinsClient{
		Client: gojenkins.CreateJenkins(nil, server.URL, "", ""),
		ctx:    context.Background(),
	}
	var out bytes.Buffer
	err := jc.streamLogsHTTP("kernel-build", 2, &out, time.Millisecond, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("expected one snapshot request, got %d", requests)
	}
	if got := out.String(); got != "line 1\n" {
		t.Fatalf("unexpected streamed output: %q", got)
	}
}
