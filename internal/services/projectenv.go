// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package services

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ProjectEnv is the ordered chain of project directories that the services
// daemon resolves relative references against: the consuming project first,
// then each including project, innermost last. It exists so that services
// defined in an included project can reference files relative to their own
// project root (e.g. devbox.d/bin/setup) without the consuming project
// having to copy them, while still letting the consuming project override
// any of them.
//
// The environment is materialized as a symlink forest (see Dir) rather than
// by rewriting service commands: a process-compose command is a free-form
// shell line, so path references cannot be identified and rewritten
// reliably, and a POSIX shell resolves slash-paths against a single
// directory with no search order.
type ProjectEnv struct {
	Roots []string
}

// NewProjectEnv returns an environment for the given directories, which must
// be ordered innermost (defining project) first, as returned by
// devconfig.Config.LocalProjectDirs. The returned environment resolves the
// consumer first.
func NewProjectEnv(innermostFirstDirs []string) ProjectEnv {
	roots := make([]string, 0, len(innermostFirstDirs))
	for i := len(innermostFirstDirs) - 1; i >= 0; i-- {
		roots = append(roots, innermostFirstDirs[i])
	}
	return ProjectEnv{Roots: roots}
}

// top-level directories that are devbox's own state or version-control
// internals; they never take part in reference resolution.
var projectEnvExcludes = map[string]bool{
	".devbox": true,
	".git":    true,
	".jj":     true,
}

// Dir returns the directory the services daemon should run in. For a single
// root that is the root itself (byte-for-byte today's behavior); for a chain
// it is a symlink forest under projectDir that deep-merges every root,
// consumer first: the first project providing a given file wins, and files
// missing upstream are filled in by projects further down the chain. The
// forest is rebuilt on every call so newly added files are picked up.
func (e ProjectEnv) Dir(projectDir string) (string, error) {
	if len(e.Roots) == 0 {
		return projectDir, nil
	}
	if len(e.Roots) == 1 && e.Roots[0] == projectDir {
		return projectDir, nil
	}

	sum := sha256.Sum256([]byte(strings.Join(e.Roots, "\x00")))
	dir := filepath.Join(projectDir, ".devbox", "gen", "project-env", hex.EncodeToString(sum[:6]))
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for _, root := range e.Roots {
		if err := mergeTree(dir, root); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// mergeTree links every entry of src into the forest at dst. Entries already
// present shadow the rest of that subtree from src, so projects further out
// win per file while inner projects fill the gaps. Symlinks are linked as
// symlinks and never followed.
func mergeTree(dst, src string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() && rel == filepath.Base(src) && filepath.Dir(src) == filepath.Dir(dst) {
			// Unreachable in practice; guard against self-merge.
			return filepath.SkipDir
		}
		if projectEnvExcludes[rel] && !strings.Contains(rel, string(filepath.Separator)) {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if _, err := os.Lstat(target); err == nil {
			// Provided by a project further out: its entry wins.
			if entry.IsDir() {
				return nil // keep walking src to fill gaps inside the dir
			}
			return filepath.SkipDir
		} else if !os.IsNotExist(err) {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return os.Symlink(path, target)
	})
}
