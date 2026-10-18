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
	"strings"

	"github.com/pkg/errors"

	"go.jetify.com/devbox/internal/devbox/devopt"
	"go.jetify.com/devbox/internal/devpkg"
	"go.jetify.com/devbox/internal/nix"
	"go.jetify.com/devbox/internal/xdg"
	"go.jetify.com/devbox/nix/flake"
)

// processComposeRev is the github.com/jefft/process-compose commit that runs
// as the services daemon. It must be the commit the go.mod replace of
// github.com/f1bonacc1/process-compose points at, so the daemon and the
// library types internal/services decodes its responses into come from the
// same source (TestProcessComposeRevMatchesGoMod). The full 40-character rev
// keeps the flake reference immutable.
const processComposeRev = "7c5c2422b33a988891b38e8ea814a1ad839fc448"

const processComposeFlake = "github:jefft/process-compose/" + processComposeRev

// processComposeName is the canonical package name and flake attribute/repo
// name shared by every packaging of process-compose.
const processComposeName = "process-compose"

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

	// Remove previous process-compose packages (e.g. the nixhub pin or an
	// older fork revision): two process-compose packages would collide in the
	// profile's bin directory. Only process-compose is removed; the utility
	// project is shared by every devbox version and is user-writable, so
	// unrelated utilities are left alone.
	topLevel := box.TopLevelPackages()
	existing := []string{}
	for _, p := range topLevel {
		existing = append(existing, p.Raw)
	}
	stale := utilityPackagesToRemove(topLevel, utilities)

	// Skip Add for utilities already in the config; calling Add anyway would
	// print noisy "Package already in devbox.json" messages on every services
	// interaction.
	toAdd := []string{}
	for _, u := range utilities {
		if !slices.Contains(existing, u) {
			toAdd = append(toAdd, u)
		}
	}

	// Fetch the new flake before removing anything. The profile is shared by
	// every project, so an offline or otherwise failed fetch must not leave it
	// without process-compose.
	if len(toAdd) > 0 {
		if err = prefetchUtilities(ctx, toAdd); err != nil {
			return err
		}
	}

	if len(stale) > 0 {
		if err = box.Remove(ctx, stale...); err != nil {
			return err
		}
	}
	if len(toAdd) > 0 {
		if err = box.Add(ctx, toAdd, devopt.AddOpts{}); err != nil {
			return err
		}
	}

	return box.Install(ctx)
}

// utilityPackagesToRemove returns the configured packages that must be
// removed before the desired utilities are added. Only previous
// process-compose packages are returned: the utility project is shared by all
// devbox versions and is user-writable, so unrelated utilities are preserved.
func utilityPackagesToRemove(pkgs []*devpkg.Package, desired []string) []string {
	stale := []string{}
	for _, p := range pkgs {
		if slices.Contains(desired, p.Raw) {
			continue
		}
		if isProcessComposePackage(p) {
			stale = append(stale, p.Raw)
		}
	}
	return stale
}

// isProcessComposePackage reports whether p is any process-compose package,
// whether named canonically (the nixhub package "process-compose@<version>")
// or through a flake's attribute path or repository name (a fork revision or
// an installable such as nixpkgs#process-compose).
func isProcessComposePackage(p *devpkg.Package) bool {
	if p.CanonicalName() == processComposeName {
		return true
	}

	installable, err := flake.ParseInstallable(p.Raw)
	if err != nil {
		return false
	}
	if attrPath := installable.AttrPath; attrPath != "" {
		if attrPath[strings.LastIndex(attrPath, ".")+1:] == processComposeName {
			return true
		}
	}
	return installable.Ref.Repo == processComposeName
}

// prefetchUtilities resolves each remote flake in utilities so that a fetch
// failure happens before the shared utility profile is modified. Package
// types other than remote flakes are validated by Add itself.
func prefetchUtilities(ctx context.Context, utilities []string) error {
	for _, u := range utilities {
		installable, err := flake.ParseInstallable(u)
		if err != nil || installable.Ref.Type == flake.TypePath {
			continue
		}
		if _, err := nix.ResolveFlake(ctx, installable.Ref, false); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
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
