package ux

import (
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
)

// LogLevelEnv carries the diagnostic threshold into nested Devbox invocations.
const LogLevelEnv = "DEVBOX_LOG_LEVEL"

var logLevel atomic.Int64

func init() {
	logLevel.Store(int64(slog.LevelWarn))
}

// SetLogLevel sets the process-wide threshold for severity-labelled UX messages.
// Success messages, command results, and hook output are not diagnostic logs.
func SetLogLevel(value string) error {
	var level slog.Level
	switch strings.ToUpper(value) {
	case "INFO":
		level = slog.LevelInfo
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		return fmt.Errorf("invalid log level %q: expected INFO, WARN, or ERROR", value)
	}
	logLevel.Store(int64(level))
	return nil
}

func logEnabled(level slog.Level) bool {
	return int64(level) >= logLevel.Load()
}
