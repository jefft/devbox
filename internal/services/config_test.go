// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFromProcessComposeParsesServiceEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "process-compose.yaml")
	content := `
version: "0.5"
environment:
  - PROJECT_LEVEL=yes
processes:
  php-fpm:
    environment:
      - DEVBOX_RUNTIME_DIR=/run/user/1000/devbox/abc123/php
      - PROJECT_LEVEL=overridden
  plain:
    command: echo hi
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	svcs, err := FromProcessCompose(path)
	require.NoError(t, err)

	fpm := svcs["php-fpm"]
	require.Equal(t, "/run/user/1000/devbox/abc123/php", fpm.Env["DEVBOX_RUNTIME_DIR"])
	// Per-process env overlays the project-level one...
	require.Equal(t, "overridden", fpm.Env["PROJECT_LEVEL"])

	// ...and processes without their own env still inherit it.
	plain := svcs["plain"]
	require.Equal(t, "yes", plain.Env["PROJECT_LEVEL"])
	require.Empty(t, plain.Env["DEVBOX_RUNTIME_DIR"])
}

func TestEnsureRuntimeDirs(t *testing.T) {
	base := t.TempDir()
	runtimeDir := filepath.Join(base, "devbox", "abc123", "php")

	svcs := Services{
		// Two services sharing one runtime dir: created once.
		"php-fpm": {Name: "php-fpm", Env: map[string]string{
			"DEVBOX_RUNTIME_DIR": runtimeDir,
		}},
		"nginx": {Name: "nginx", Env: map[string]string{
			"DEVBOX_RUNTIME_DIR": runtimeDir,
		}},
		// No DEVBOX_RUNTIME_DIR: skipped.
		"plain": {Name: "plain"},
		// Relative paths are never materialized relative to the cwd.
		"relative": {Name: "relative", Env: map[string]string{
			"DEVBOX_RUNTIME_DIR": "relative/runtime",
		}},
		// Empty value: skipped.
		"empty": {Name: "empty", Env: map[string]string{
			"DEVBOX_RUNTIME_DIR": "",
		}},
	}

	require.NoError(t, ensureRuntimeDirs(svcs))

	info, err := os.Stat(runtimeDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	require.NoDirExists(t, filepath.Join(base, "relative/runtime"))

	// Idempotent: a second pass over existing dirs succeeds.
	require.NoError(t, ensureRuntimeDirs(svcs))
}
