// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package devbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

const (
	processComposeModulePath = "github.com/f1bonacc1/process-compose"
	processComposeForkPath   = "github.com/jefft/process-compose"
)

// TestProcessComposeRevMatchesGoMod guards against drift between the
// process-compose daemon we install into the global utility project and the
// process-compose library we compile against. The two are separate pins that
// nothing else keeps in sync, and they silently diverged once already: go.mod
// sat at v1.64.1 while the installed binary had moved on to 1.110.0.
//
// Skew matters because internal/services decodes the running daemon's REST
// responses into the library's types (types.ProcessesState and friends). If the
// two disagree about a JSON tag, `devbox services ls` misreports state instead
// of failing loudly. Both pins name a commit of the fork, so they must name the
// same one.
func TestProcessComposeRevMatchesGoMod(t *testing.T) {
	goModPath := findGoMod(t)

	data, err := os.ReadFile(goModPath)
	require.NoError(t, err, "read %s", goModPath)

	f, err := modfile.Parse(goModPath, data, nil)
	require.NoError(t, err, "parse %s", goModPath)

	var replacement *module.Version
	for _, r := range f.Replace {
		if r.Old.Path == processComposeModulePath {
			replacement = &r.New
			break
		}
	}
	require.NotNil(t, replacement, "no replace for %s in %s", processComposeModulePath, goModPath)
	require.Equal(t, processComposeForkPath, replacement.Path)

	rev, err := module.PseudoVersionRev(replacement.Version)
	require.NoError(t, err, "replace target %s is not a pseudo-version", replacement.Version)

	require.Len(t, processComposeRev, 40, "processComposeRev must be a full commit hash")
	require.True(t, strings.HasPrefix(processComposeRev, rev),
		"process-compose pin skew: processComposeRev is %s (internal/devbox/util.go) "+
			"but go.mod replaces %s with %s@%s. Point both at the same commit.",
		processComposeRev, processComposeModulePath, processComposeForkPath, replacement.Version)
}

// findGoMod walks up from the working directory to locate the module's go.mod,
// so the test keeps working if this package is ever moved.
func findGoMod(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		candidate := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod found above %s", dir)
		}
		dir = parent
	}
}
