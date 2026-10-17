// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package devbox

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/pkg/errors"

	"go.jetify.com/devbox/internal/devbox/devopt"
	"go.jetify.com/devbox/internal/xdg"
)

// processComposeRev is the github.com/jefft/process-compose commit that runs
// as the services daemon. It must be the commit the go.mod replace of
// github.com/f1bonacc1/process-compose points at, so the daemon and the
// library types internal/services decodes its responses into come from the
// same source (TestProcessComposeRevMatchesGoMod). The full 40-character rev
// keeps the flake reference immutable.
const processComposeRev = "067cb68aceba85edf3c7ce8fd6bc6b5616410e9d"

const processComposeFlake = "github:jefft/process-compose/" + processComposeRev

var utilProjectConfigPath string

func initDevboxUtilityProject(ctx context.Context, stderr io.Writer) error {
	devboxUtilityProjectPath, err := ensureDevboxUtilityConfig()
	if err != nil {
		return err
	}

	box, err := Open(&devopt.Opts{
		Dir:    devboxUtilityProjectPath,
		Stderr: stderr,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Add all utilities here.
	utilities := []string{
		processComposeFlake,
	}

	// The utility project holds exactly the utilities above. Remove anything
	// else (e.g. the previous process-compose pin after a bump): two
	// process-compose packages would collide in the profile's bin directory.
	// Skip Add for utilities already in the config; calling Add anyway would
	// print noisy "Package already in devbox.json" messages on every services
	// interaction.
	existing := []string{}
	stale := []string{}
	for _, p := range box.TopLevelPackages() {
		existing = append(existing, p.Raw)
		if !slices.Contains(utilities, p.Raw) {
			stale = append(stale, p.Raw)
		}
	}
	if len(stale) > 0 {
		if err = box.Remove(ctx, stale...); err != nil {
			return err
		}
	}
	toAdd := []string{}
	for _, u := range utilities {
		if !slices.Contains(existing, u) {
			toAdd = append(toAdd, u)
		}
	}
	if len(toAdd) > 0 {
		if err = box.Add(ctx, toAdd, devopt.AddOpts{}); err != nil {
			return err
		}
	}

	return box.Install(ctx)
}

func ensureDevboxUtilityConfig() (string, error) {
	if utilProjectConfigPath != "" {
		return utilProjectConfigPath, nil
	}

	path, err := utilityDataPath()
	if err != nil {
		return "", err
	}

	err = EnsureConfig(path)
	if err != nil {
		return "", err
	}

	// Avoids unnecessarily initializing the config again by caching the path
	utilProjectConfigPath = path

	return path, nil
}

func utilityLookPath(binName string) (string, error) {
	binPath, err := utilityBinPath()
	if err != nil {
		return "", err
	}
	absPath := filepath.Join(binPath, binName)
	_, err = os.Stat(absPath)
	if errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	return absPath, nil
}

func utilityDataPath() (string, error) {
	path := xdg.DataSubpath("devbox/util")
	return path, errors.WithStack(os.MkdirAll(path, 0o755))
}

func utilityNixProfilePath() (string, error) {
	path, err := utilityDataPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(path, ".devbox/nix/profile"), nil
}

func utilityBinPath() (string, error) {
	nixProfilePath, err := utilityNixProfilePath()
	if err != nil {
		return "", err
	}

	return filepath.Join(nixProfilePath, "default/bin"), nil
}
