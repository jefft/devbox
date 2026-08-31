// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package plugin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"github.com/tailscale/hujson"
	"go.jetify.com/devbox/internal/devconfig/configfile"
	"go.jetify.com/devbox/internal/devpkg"
	"go.jetify.com/devbox/internal/lock"
	"go.jetify.com/devbox/internal/nix"
	"go.jetify.com/devbox/internal/services"
)

const (
	// TODO rename to devboxPluginUserConfigDirName
	devboxDirName       = "devbox.d"
	devboxHiddenDirName = ".devbox"
	pluginConfigName    = "plugin.json"
)

var (
	VirtenvPath    = filepath.Join(devboxHiddenDirName, "virtenv")
	VirtenvBinPath = filepath.Join(VirtenvPath, "bin")
)

// runtimeDir returns a short, stable, per-plugin directory for ephemeral
// runtime files (unix domain sockets, pid files). Socket paths are limited to
// 107 chars on Linux (104 on macOS), which paths under deeply nested project
// directories can exceed, so we place them in a per-user runtime directory
// keyed by a hash of the project directory instead.
//
// The returned path is stable for a given (projectDir, pluginName, user): the
// hash is plain SHA-256 truncated to 12 hex chars (48 bits), which is not
// susceptible to the cachehash package's stability disclaimer.
//
// The directory is on tmpfs in the common case ($XDG_RUNTIME_DIR, i.e.
// /run/user/<uid> on Linux) and may be wiped on reboot; callers must create
// it before writing a socket or pid file (create_files entries and plugin
// scripts do so).
func runtimeDir(projectDir, pluginName string) string {
	digest := sha256.Sum256([]byte(projectDir))
	base := os.Getenv("XDG_RUNTIME_DIR")
	if info, err := os.Stat(base); err != nil || !info.IsDir() {
		base = os.TempDir()
	}
	return filepath.Join(base, "devbox", hex.EncodeToString(digest[:6]), pluginName)
}

// TemplateData is the set of values available to plugin templates. Both the
// plugin.json render (buildConfig) and the file-content render (createFile)
// build their values from TemplateDataFor, so path values can never drift
// between the two render sites. Go templates execute against struct fields,
// so the field names are the template variables ({{ .LogDir }} etc.).
type TemplateData struct {
	DevboxDir            string
	DevboxDirRoot        string
	DevboxProfileDefault string
	DevboxProjectDir     string
	Virtenv              string
	DataDir              string
	LogDir               string
	RuntimeDir           string

	// Extra values only relevant to flake-file renders.
	PackageAttributePath string
	Packages             []string
	System               string
	URLForInput          string
}

// TemplateDataFor returns the template data for the given project directory
// and plugin (or includable) name.
func TemplateDataFor(projectDir, name string) TemplateData {
	return TemplateData{
		DevboxDir:            filepath.Join(projectDir, devboxDirName, name),
		DevboxDirRoot:        filepath.Join(projectDir, devboxDirName),
		DevboxProfileDefault: filepath.Join(projectDir, nix.ProfilePath),
		DevboxProjectDir:     projectDir,
		Virtenv:              filepath.Join(projectDir, VirtenvPath, name),
		DataDir:              filepath.Join(projectDir, devboxHiddenDirName, "data", name),
		LogDir:               filepath.Join(projectDir, devboxHiddenDirName, "logs", name),
		RuntimeDir:           runtimeDir(projectDir, name),
	}
}

// Config is a resolved plugin or project fragment: a unified devbox config
// plus the includable source it was loaded from.
type Config struct {
	configfile.ConfigFile

	// Source is the includable that triggered this config. There are two ways
	// a config is included:
	// 1. Built-in plugins are triggered by packages (See plugins.builtInMap)
	// 2. Plugins can be added via the "include" field in devbox.json or plugin.json
	Source Includable
}

func (c *Config) ProcessComposeYaml() (string, string) {
	for file, contentPath := range c.CreateFiles {
		if strings.HasSuffix(file, "process-compose.yaml") || strings.HasSuffix(file, "process-compose.yml") {
			return file, contentPath
		}
	}
	return "", ""
}

func (c *Config) Services() (services.Services, error) {
	if file, _ := c.ProcessComposeYaml(); file != "" {
		return services.FromProcessCompose(file)
	}
	return nil, nil
}

func (m *Manager) CreateFilesForConfig(cfg *Config) error {
	virtenvPath := filepath.Join(m.ProjectDir(), VirtenvPath)
	pkg := cfg.Source
	locked := m.lockfile.Packages[pkg.LockfileKey()]

	name := pkg.CanonicalName()

	// Always create this dir because some plugins depend on it.
	if err := createDir(filepath.Join(virtenvPath, name)); err != nil {
		return err
	}

	// Devbox owns the plugin directory lifecycle: every includable gets its
	// data, log, and runtime directories regardless of whether the plugin
	// remembered to create them. RuntimeDir is user-private per XDG.
	data := TemplateDataFor(m.ProjectDir(), name)
	if err := createDir(data.DataDir); err != nil {
		return err
	}
	if err := createDir(data.LogDir); err != nil {
		return err
	}
	if err := os.MkdirAll(data.RuntimeDir, 0o700); err != nil {
		return errors.WithStack(err)
	}

	slog.Debug("creating files for package", "pkg", pkg)
	for filePath, contentPath := range cfg.CreateFiles {
		if !m.shouldCreateFile(locked, filePath) {
			continue
		}

		dirPath := filepath.Dir(filePath)
		if contentPath == "" {
			dirPath = filePath
		}
		if err := createDir(dirPath); err != nil {
			return errors.WithStack(err)
		}

		if contentPath == "" {
			continue
		}

		if err := m.createFile(pkg, filePath, contentPath); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) UpdateLockfileVersion(cfg *Config) error {
	pkg := cfg.Source
	locked := m.lockfile.Packages[pkg.LockfileKey()]
	// plugins that are not triggered by packages don't have a lockfile entry
	// this may change if we decide to store all plugins in the lockfile
	if locked == nil {
		return nil
	}
	locked.PluginVersion = cfg.Version
	return m.lockfile.Save()
}

func (m *Manager) createFile(
	pkg Includable,
	filePath, contentPath string,
) error {
	name := pkg.CanonicalName()
	slog.Debug("Creating file %q from contentPath: %q", filePath, contentPath)
	content, err := pkg.FileContent(contentPath)
	if err != nil {
		return errors.WithStack(err)
	}
	tmpl, err := template.New(filePath + "-template").Parse(string(content))
	if err != nil {
		return errors.WithStack(err)
	}

	var urlForInput, attributePath string

	if pkg, ok := pkg.(*devpkg.Package); ok {
		attributePath, err = pkg.PackageAttributePath()
		if err != nil {
			return err
		}
		urlForInput = pkg.URLForFlakeInput()
	}

	data := TemplateDataFor(m.ProjectDir(), name)
	data.PackageAttributePath = attributePath
	data.Packages = m.AllPackageNamesIncludingRemovedTriggerPackages()
	data.System = nix.System()
	data.URLForInput = urlForInput
	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, data); err != nil {
		return errors.WithStack(err)
	}
	var fileMode fs.FileMode = 0o644
	if strings.Contains(filePath, "bin/") {
		fileMode = 0o755
	}

	if err := os.WriteFile(filePath, buf.Bytes(), fileMode); err != nil {
		return errors.WithStack(err)
	}
	if fileMode == 0o755 {
		if err := createSymlink(m.ProjectDir(), filePath); err != nil {
			return err
		}
	}
	return nil
}

// buildConfig returns a plugin.Config
func buildConfig(pkg Includable, projectDir, content string) (*Config, error) {
	cfg := &Config{Source: pkg}
	name := pkg.CanonicalName()
	t, err := template.New(name + "-template").Parse(content)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var buf bytes.Buffer
	if err = t.Execute(&buf, TemplateDataFor(projectDir, name)); err != nil {
		return nil, errors.WithStack(err)
	}

	jsonb, err := jsonPurifyPluginContent(buf.Bytes())
	if err != nil {
		return nil, err
	}

	return cfg, errors.WithStack(json.Unmarshal(jsonb, cfg))
}

func jsonPurifyPluginContent(content []byte) ([]byte, error) {
	return hujson.Standardize(slices.Clone(content))
}

func createDir(path string) error {
	if path == "" {
		return nil
	}
	return errors.WithStack(os.MkdirAll(path, 0o755))
}

func createSymlink(root, filePath string) error {
	name := filepath.Base(filePath)
	newname := filepath.Join(root, VirtenvBinPath, name)

	// Create bin path just in case it doesn't exist
	if err := os.MkdirAll(filepath.Join(root, VirtenvBinPath), 0o755); err != nil {
		return errors.WithStack(err)
	}

	if _, err := os.Lstat(newname); err == nil {
		if err = os.Remove(newname); err != nil {
			return errors.WithStack(err)
		}
	}

	return errors.WithStack(os.Symlink(filePath, newname))
}

func (m *Manager) shouldCreateFile(
	pkg *lock.Package,
	filePath string,
) bool {
	sep := string(filepath.Separator)

	// Only create files in devbox.d directory if they are not in the lockfile
	pluginInstalled := pkg != nil && pkg.PluginVersion != ""
	if strings.Contains(filePath, sep+devboxDirName+sep) && pluginInstalled {
		return false
	}

	// Hidden .devbox files are always replaceable, so ok to recreate
	if strings.Contains(filePath, sep+devboxHiddenDirName+sep) {
		return true
	}
	_, err := os.Stat(filePath)
	// File doesn't exist, so we should create it.
	return errors.Is(err, fs.ErrNotExist)
}

func (c *Config) Description() string {
	if c == nil {
		return ""
	}
	return c.ConfigFile.Description
}
