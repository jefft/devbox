package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/f1bonacc1/process-compose/src/types"
)

// PrintStartupHooks streams startup hook messages for background launches;
// foreground sessions already display hooks, so detaching prints nothing.
// The entire reporting flow shares one ten-second deadline.
func PrintStartupHooks(ctx context.Context, w io.Writer, projectDir string, background bool) error {
	port, err := GetProcessManagerPort(projectDir)
	if err != nil {
		return err
	}

	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err = waitForServiceStartup(startupCtx, w, port, background)
	return err
}

// waitForServiceStartup tolerates the interval between spawning the manager and
// its API becoming available, and waits until the selected services are ready.
// Background launches also wait for startup hooks: readiness alone can precede
// their output, including the project on_ready hook.
func waitForServiceStartup(ctx context.Context, w io.Writer, port int, background bool) (*processStates, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: time.Second}
	var after uint64
	for {
		hooksComplete := !background
		if background {
			var err error
			hooksComplete, after, err = forwardStartupHooks(ctx, w, client, port, after)
			if err != nil {
				return nil, err
			}
		}

		var states processStates
		if err := startupSnapshot(ctx, client, port, "/processes", &states); err == nil {
			starting := slices.ContainsFunc(states.States, func(state types.ProcessState) bool {
				return state.Status == types.ProcessStatePending ||
					state.Status == types.ProcessStateLaunching ||
					(state.IsRunning && (!state.IsReady() || state.ProcessReadyTime == nil))
			})
			if !starting && hooksComplete {
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

// forwardStartupHooks writes newly retained hook output once and reports whether
// startup notifications have finished. Unavailable snapshots are retried.
func forwardStartupHooks(ctx context.Context, w io.Writer, client *http.Client, port int, after uint64) (bool, uint64, error) {
	var snapshot types.HookOutputSnapshot
	path := "/project/hooks?after=" + strconv.FormatUint(after, 10)
	if !startupSnapshotAvailable(ctx, client, port, path, &snapshot) {
		return false, after, nil
	}
	for _, line := range snapshot.Lines {
		if line.Sequence <= after {
			continue
		}
		scope := line.Process
		if scope == "" {
			scope = "project"
		}
		if _, err := fmt.Fprintf(w, "[%s] %s\n", scope, line.Message); err != nil {
			return false, after, fmt.Errorf("writing service hook output: %w", err)
		}
		after = line.Sequence
	}
	return snapshot.StartupComplete && snapshot.Pending == 0, max(after, snapshot.NextSequence), nil
}

// startupSnapshotAvailable treats API failures as not-yet-available; the caller's
// shared startup deadline bounds retries.
func startupSnapshotAvailable(ctx context.Context, client *http.Client, port int, path string, target any) bool {
	return startupSnapshot(ctx, client, port, path, target) == nil
}

func startupSnapshot(ctx context.Context, client *http.Client, port int, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://localhost:%d%s", port, path), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("querying %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}
