package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/f1bonacc1/process-compose/src/types"
)

// PrintServicePorts waits briefly for startup, then queries the running services,
// including dependencies and replicas, using the same process-compose binary.
// Port reporting is a snapshot, not a readiness check or a port-change monitor.
func PrintServicePorts(ctx context.Context, w io.Writer, projectDir, binPath string) error {
	port, err := GetProcessManagerPort(projectDir)
	if err != nil {
		return err
	}

	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	states, err := waitForPortReport(startupCtx, port)
	if err != nil {
		return err
	}
	slices.SortFunc(states.States, func(a, b types.ProcessState) int {
		return strings.Compare(a.Name, b.Name)
	})
	for _, state := range states.States {
		if !state.IsRunning {
			continue
		}
		queryCtx, queryCancel := context.WithTimeout(ctx, 2*time.Second)
		cmd := exec.CommandContext(queryCtx, binPath, "-p", strconv.Itoa(port), "process", "ports", "--", state.Name)
		cmd.Stdout = w
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		queryCancel()
		if err != nil {
			return fmt.Errorf("querying ports for %s: %w: %s", state.Name, err, stderr.String())
		}
	}
	return nil
}

// waitForPortReport tolerates the interval between spawning the manager and its
// API becoming available. Services with readiness checks must become ready;
// without one, process-compose considers a running process ready immediately.
func waitForPortReport(ctx context.Context, port int) (*processStates, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://localhost:%d/processes", port), nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err == nil {
			var states processStates
			err = json.NewDecoder(resp.Body).Decode(&states)
			resp.Body.Close()
			starting := slices.ContainsFunc(states.States, func(state types.ProcessState) bool {
				return state.Status == types.ProcessStatePending ||
					state.Status == types.ProcessStateLaunching ||
					(state.IsRunning && (!state.IsReady() || state.ProcessReadyTime == nil))
			})
			if resp.StatusCode == http.StatusOK && err == nil && !starting {
				return &states, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for service startup: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
