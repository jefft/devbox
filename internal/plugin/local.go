// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package plugin

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"go.jetify.com/devbox/internal/cachehash"
	"go.jetify.com/devbox/internal/devconfig/configfile"
	"go.jetify.com/devbox/nix/flake"
)

type LocalPlugin struct {
	ref  flake.Ref
	name string
	path string
}

// newLocalPlugin resolves a local include ref to a descriptor file: a path
// naming an existing file is used directly, while a directory is probed for
// devbox.json (a project include) and then plugin.json (a plugin include).
// Project includes don't require a "name" field; the descriptor's directory
// name is the fallback canonical name.
func newLocalPlugin(ref flake.Ref, pluginDir string) (*LocalPlugin, error) {
	plugin := &LocalPlugin{ref: ref}
	base := os.ExpandEnv(ref.Path)
	if !filepath.IsAbs(base) {
		base = filepath.Join(pluginDir, base)
	}
	path, err := resolveDescriptorPath(base)
	if err != nil {
		return nil, err
	}
	plugin.path = path
	if strings.HasSuffix(path, pluginConfigName) {
		plugin.name, err = getPluginNameFromContent(plugin)
	} else {
		plugin.name, err = getProjectNameFromContent(plugin)
	}
	if err != nil {
		return nil, err
	}
	return plugin, nil
}

// resolveDescriptorPath returns the descriptor file for a local include ref:
// the ref itself if it names an existing file, otherwise devbox.json or
// plugin.json inside the referenced directory.
func resolveDescriptorPath(base string) (string, error) {
	if fi, err := os.Stat(base); err == nil && !fi.IsDir() {
		return base, nil
	}
	for _, name := range []string{configfile.DefaultName, pluginConfigName} {
		p := filepath.Join(base, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", errors.Errorf(
		"no %s or %s found in %q", configfile.DefaultName, pluginConfigName, base)
}

func (plugin *LocalPlugin) Fetch() ([]byte, error) {
	content, err := os.ReadFile(plugin.Path())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return jsonPurifyPluginContent(content)
}

func (plugin *LocalPlugin) CanonicalName() string {
	return plugin.name
}

func (plugin *LocalPlugin) IsLocal() bool {
	return true
}

func (plugin *LocalPlugin) Hash() string {
	return cachehash.Bytes([]byte(filepath.Clean(plugin.path)))
}

func (plugin *LocalPlugin) FileContent(subpath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(filepath.Dir(plugin.path), subpath))
}

func (plugin *LocalPlugin) LockfileKey() string {
	return plugin.ref.String()
}

// Path returns the absolute path to the descriptor file (devbox.json or
// plugin.json) that this includable resolves to.
func (plugin *LocalPlugin) Path() string {
	return plugin.path
}
