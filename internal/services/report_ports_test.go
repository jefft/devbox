package services

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	states, err := waitForPortReport(ctx, port)
	if err != nil {
		t.Fatal(err)
	}
	if !states.States[0].IsRunning || states.States[1].ExitCode != 1 {
		t.Fatalf("reported before startup settled: %+v", states.States)
	}
}
