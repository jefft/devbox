// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectEnvDirSingleRootIsIdentity(t *testing.T) {
	root := t.TempDir()
	env := ProjectEnv{Roots: []string{root}}
	dir, err := env.Dir(root)
	require.NoError(t, err)
	require.Equal(t, root, dir)
}

func TestProjectEnvDirConsumerShadowsDefiner(t *testing.T) {
	// a consumes b consumes c. Roots are consumer first, as the services
	// daemon receives them.
	rootC := t.TempDir()
	rootB := t.TempDir()
	rootA := t.TempDir()
	consumer := t.TempDir()

	write := func(dir, rel, content string) {
		path := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	// a overrides the same relative path that b defines.
	write(rootB, "devbox.d/bin/from-b", "b")
	write(rootA, "devbox.d/bin/from-b", "a")
	// Files only defined innermost must resolve down the chain.
	write(rootC, "devbox.d/bin/only-c", "c")
	write(rootB, "only-b", "b")
	// a's devbox state must never enter the environment.
	write(rootA, ".devbox/secret", "secret")
	write(rootB, "only-b", "b")
	write(rootA, "only-a", "a")
	env := ProjectEnv{Roots: []string{consumer, rootA, rootB, rootC}}
	dir, err := env.Dir(consumer)
	require.NoError(t, err)

	read := func(rel string) string {
		path := filepath.Join(dir, rel)
		target, err := os.ReadFile(path)
		require.NoError(t, err, rel)
		return string(target)
	}

	// Consumer first: a's file shadows b's same-named file...
	require.Equal(t, "a", read("devbox.d/bin/from-b"))
	// ...but only per file: files a lacks are filled in from b and c.
	require.Equal(t, "b", read("only-b"))
	require.Equal(t, "c", read("devbox.d/bin/only-c"))
	require.Equal(t, "a", read("only-a"))
	require.NoFileExists(t, filepath.Join(dir, ".devbox"))
}

func TestProjectEnvDirFillsFromDefiner(t *testing.T) {
	// LocalProjectDirs order: innermost (definer) first.
	rootC := t.TempDir()
	rootB := t.TempDir()
	rootA := t.TempDir()

	write := func(dir, rel, content string) {
		path := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	write(rootC, "devbox.d/bin/setup", "from-c")
	write(rootB, "devbox.d/other", "from-b")

	env := NewProjectEnv([]string{rootC, rootB, rootA})
	dir, err := env.Dir(rootA)
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(dir, "devbox.d", "bin", "setup"))
	require.NoError(t, err)
	require.Equal(t, "from-c", string(got))

	got, err = os.ReadFile(filepath.Join(dir, "devbox.d", "other"))
	require.NoError(t, err)
	require.Equal(t, "from-b", string(got))
}
