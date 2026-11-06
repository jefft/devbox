package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForPortReportWaitsForStartupNotFailedServices(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			fmt.Fprint(w, `{"data":[{"name":"web","status":"Pending"}]}`)
			return
		}
		if requests.Load() == 2 {
			fmt.Fprint(w, `{"data":[{"name":"web","status":"Running","is_running":true,"is_ready":"-"}]}`)
			return
		}
		fmt.Fprint(w, `{"data":[
			{"name":"web","status":"Running","is_running":true,"is_ready":"Ready","process_ready_time":"2026-01-01T00:00:00Z"},
			{"name":"failed","status":"Skipped","exit_code":1},
			{"name":"disabled","status":"Disabled"}
		]}`)
	}))
	defer server.Close()
	_, portString, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	states, err := waitForServiceStartup(ctx, &bytes.Buffer{}, port, false)
	if err != nil {
		t.Fatal(err)
	}
	if !states.States[0].IsRunning || states.States[1].ExitCode != 1 {
		t.Fatalf("reported before startup settled: %+v", states.States)
	}
}

func TestWaitForServiceStartupStreamsDelayedHooksExactlyOnce(t *testing.T) {
	var requests atomic.Int32
	var output bytes.Buffer
	cursors := []string{"0", "1", "1", "2"}
	snapshots := []string{
		`{"lines":[{"sequence":1,"process":"nginx","event":"on_ready","message":"http://localhost:8080"}],"next_sequence":1,"pending":2,"startup_complete":false}`,
		`{"lines":[{"sequence":1,"process":"nginx","event":"on_ready","message":"http://localhost:8080"}],"next_sequence":1,"pending":1,"startup_complete":true}`,
		`{"lines":[{"sequence":2,"event":"on_ready","message":"Project is ready"}],"next_sequence":2,"pending":1,"startup_complete":true}`,
		`{"lines":[{"sequence":2,"event":"on_ready","message":"Project is ready"},{"sequence":3,"process":"nginx","event":"on_ready","message":"Hook stderr"}],"next_sequence":3,"pending":0,"startup_complete":true}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/processes":
			fmt.Fprint(w, `{"data":[{"name":"nginx","status":"Running","is_running":true,"is_ready":"Ready","process_ready_time":"2026-01-01T00:00:00Z"}]}`)
		case "/project/hooks":
			index := int(requests.Add(1)) - 1
			if index >= len(snapshots) {
				t.Error("kept polling after startup and hooks finished")
				http.Error(w, "unexpected poll", http.StatusInternalServerError)
				return
			}
			if got := r.URL.Query().Get("after"); got != cursors[index] {
				t.Errorf("poll %d: cursor = %q, want %q", index, got, cursors[index])
			}
			fmt.Fprint(w, snapshots[index])
		default:
			t.Errorf("unexpected request for non-hook output: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	states, err := waitForServiceStartup(ctx, &output, reportServerPort(t, server), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(states.States) != 1 || !states.States[0].IsRunning {
		t.Fatalf("missing running service snapshot: %+v", states)
	}
	if requests.Load() != int32(len(snapshots)) {
		t.Fatalf("returned before delayed hooks completed: %d polls", requests.Load())
	}
	want := "[nginx] http://localhost:8080\n[project] Project is ready\n[nginx] Hook stderr\n"
	if output.String() != want {
		t.Fatalf("hook output = %q, want %q", output.String(), want)
	}
}

func TestWaitForServiceStartupReportsWriterErrors(t *testing.T) {
	writeErr := errors.New("terminal closed")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/project/hooks" {
			t.Errorf("continued polling after writer error: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"lines":[{"sequence":1,"process":"nginx","event":"on_ready","message":"URL"}],"next_sequence":1,"pending":0,"startup_complete":true}`)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := waitForServiceStartup(ctx, hookErrorWriter{err: writeErr}, reportServerPort(t, server), true)
	if !errors.Is(err, writeErr) {
		t.Fatalf("writer error = %v, want %v", err, writeErr)
	}
}

func TestWaitForServiceStartupPreservesPartialOutputOnDeadline(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/project/hooks":
			if requests.Add(1) == 1 {
				fmt.Fprint(w, `{"lines":[{"sequence":1,"process":"nginx","event":"on_ready","message":"URL before slow hook"}],"next_sequence":1,"pending":1,"startup_complete":false}`)
				return
			}
			<-r.Context().Done()
		case "/processes":
			fmt.Fprint(w, `{"data":[{"name":"nginx","status":"Running","is_running":true,"is_ready":"Ready","process_ready_time":"2026-01-01T00:00:00Z"}]}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var output bytes.Buffer
	_, err := waitForServiceStartup(ctx, &output, reportServerPort(t, server), true)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("startup error = %v, want deadline exceeded", err)
	}
	if output.String() != "[nginx] URL before slow hook\n" {
		t.Fatalf("lost or duplicated partial hook output: %q", output.String())
	}
	if requests.Load() < 2 {
		t.Fatal("did not wait for the slow hook after services became ready")
	}
}

func TestWaitForServiceStartupForegroundDoesNotReplayHookHistory(t *testing.T) {
	var hookRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/processes":
			fmt.Fprint(w, `{"data":[{"name":"nginx","status":"Running","is_running":true,"is_ready":"Ready","process_ready_time":"2026-01-01T00:00:00Z"}]}`)
		case "/project/hooks":
			hookRequests.Add(1)
			fmt.Fprint(w, `{"lines":[{"sequence":1,"process":"nginx","event":"on_ready","message":"already visible in TUI"}],"next_sequence":1,"pending":0,"startup_complete":true}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var output bytes.Buffer
	states, err := waitForServiceStartup(ctx, &output, reportServerPort(t, server), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(states.States) != 1 {
		t.Fatalf("foreground lost port-report snapshot: %+v", states)
	}
	if hookRequests.Load() != 0 || output.Len() != 0 {
		t.Fatalf("replayed foreground hook history: requests=%d, output=%q", hookRequests.Load(), output.String())
	}
}

func reportServerPort(t *testing.T, server *httptest.Server) int {
	t.Helper()
	_, portString, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

type hookErrorWriter struct {
	err error
}

func (w hookErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
