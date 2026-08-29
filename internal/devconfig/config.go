package devconfig

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/samber/lo/mutable"
	"go.jetify.com/devbox/internal/boxcli/usererr"
	"go.jetify.com/devbox/internal/build"
	"go.jetify.com/devbox/internal/cachehash"
	"go.jetify.com/devbox/internal/devbox/shellcmd"
	"go.jetify.com/devbox/internal/devconfig/configfile"
	"go.jetify.com/devbox/internal/devpkg"
	"go.jetify.com/devbox/internal/lock"
	"go.jetify.com/devbox/internal/plugin"
)

// ErrNotFound occurs when [Open] or [Find] cannot find a devbox config file
// after searching a directory (and possibly its parent directories).
var ErrNotFound = errors.New("no devbox config file found")

// errIsDirectory indicates that a file can't be opened because it's a
// directory.
const errIsDirectory = syscall.EISDIR

// errNotDirectory indicates that a file can't be opened because the directory
// portion of its path is not a directory.
const errNotDirectory = syscall.ENOTDIR

// Config represents a base devbox.json as well as any included plugins it may have.
type Config struct {
	Root configfile.ConfigFile

	// Source is the includable that produced this config; nil for the root
	// config of a project.
	Source plugin.Includable

	included []*Config
}

const defaultInitHook = "echo 'Welcome to devbox!' > /dev/null"

const defaultConfig = `{
	"$schema": "https://raw.githubusercontent.com/jetify-com/devbox/%s/.schema/devbox.schema.json",
	"packages": [],
	"shell": {
		"init_hook": [
			"%s"
		],
		"scripts": {
			"test": [
				"echo \"Error: no test specified\" && exit 1"
			]
		}
	}
}
`

func DefaultConfig() *Config {
	schemaVersion := lo.Ternary(build.IsDev, "main", build.Version)

	cfg, err := loadBytes([]byte(fmt.Sprintf(defaultConfig, schemaVersion, defaultInitHook)))
	if err != nil {
		panic("default devbox.json is invalid: " + err.Error())
	}
	return cfg
}

func IsDefault(path string) bool {
	cfg, err := readFromFile(path)
	if err != nil {
		return false
	}
	return cfg.Root.Equals(&DefaultConfig().Root)
}

// Open loads a Devbox config from a file or project directory. If path is a
// directory, Open looks for a well-known config name (such as devbox.json)
// within it. The error will be [ErrNotFound] if path is a valid directory
// without a config file.
//
// Open does not recursively search outside of path. See [Find] to load a config
// by walking up the directory tree.
func Open(path string) (*Config, error) {
	start := time.Now()
	slog.Debug("searching for config file (excluding parent directories)", "path", path)

	cfg, err := open(path)

	if err == nil {
		slog.Debug("config file found", "path", cfg.Root.AbsRootPath, "dur", time.Since(start))
	} else {
		slog.Error("config file search error", "err", err.Error(), "dur", time.Since(start))
	}
	return cfg, err
}

func open(path string) (*Config, error) {
	// First try the happy path by assuming that path is a directory
	// containing a devbox.json.
	cfg, err := searchDir(path)
	if errors.Is(err, ErrNotFound) || errors.Is(err, errNotDirectory) {
		// Try reading path directly as a config file.
		slog.Debug("trying config file", "path", path)
		cfg, err = readFromFile(path)
		if errors.Is(err, errIsDirectory) {
			return nil, ErrNotFound
		}
	}
	return cfg, err
}

// Find is like [Open] except it recursively searches up the directory tree,
// starting in path. It returns [ErrNotFound] if path is a valid directory and
// neither it nor any of its parents contain a config file.
//
// Find stops searching as soon as it encounters a file with a well-known config
// name (such as devbox.json), even if that config fails to load.
func Find(path string) (*Config, error) {
	start := time.Now()
	slog.Debug("searching for config file (including parent directories)", "path", path)

	cfg, err := open(path)
	if errors.Is(err, ErrNotFound) {
		cfg, err = searchParentDirs(path)
	}

	if err == nil {
		slog.Debug("config file found", "path", cfg.Root.AbsRootPath, "dur", time.Since(start))
	} else {
		slog.Error("config file search error", "err", err.Error(), "dur", time.Since(start))
	}
	return cfg, err
}

// searchDir looks for a config file in dir. It does not search parent
// directories.
func searchDir(dir string) (*Config, error) {
	try := []string{configfile.DefaultName}
	for _, name := range try {
		path := filepath.Join(dir, name)
		slog.Debug("trying config file", "path", path)

		cfg, err := readFromFile(path)
		if err == nil {
			return cfg, nil
		}

		// Keep searching for other valid config filenames.
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		// Ignore directories named devbox.json.
		if errors.Is(err, errIsDirectory) {
			continue
		}
		// Stop if we found a config but couldn't load it.
		return cfg, err
	}
	return nil, ErrNotFound
}

// searchParentDirs recursively searches parent directories for a config. It
// starts with filepath.Dir(path) and does not search path itself.
func searchParentDirs(path string) (cfg *Config, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("devconfig: search parent directories: %v", err)
	}

	err = ErrNotFound
	for abs != "/" && errors.Is(err, ErrNotFound) {
		abs = filepath.Dir(abs)
		cfg, err = searchDir(abs)
	}
	return cfg, err
}

func readFromFile(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	config, err := loadBytes(b)
	if err != nil {
		return nil, err
	}
	config.Root.AbsRootPath, err = filepath.Abs(path)
	if err == nil {
		// Resolve symlinks so the project dir is canonical regardless of the
		// path devbox was invoked from. Nix rejects path: flake refs (e.g. the
		// php plugin's virtenv flake) whose components traverse a symlink:
		// "path '...' is a symlink" (jetify-com/devbox#2160).
		if resolved, resolveErr := filepath.EvalSymlinks(config.Root.AbsRootPath); resolveErr == nil {
			config.Root.AbsRootPath = resolved
		}
	}
	return config, err
}

func LoadConfigFromURL(ctx context.Context, url string) (*Config, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return loadBytes(data)
}

func loadBytes(b []byte) (*Config, error) {
	root, err := configfile.LoadBytes(b)
	if err != nil {
		return nil, err
	}

	return &Config{
		Root: *root,
	}, nil
}

func (c *Config) LoadRecursive(lockfile *lock.File) error {
	if err := c.loadRecursive(lockfile, map[string]bool{}, "" /*cyclePath*/); err != nil {
		return err
	}
	return c.validateIncludeTree()
}

// validateIncludeTree enforces invariants that are only checkable once the
// whole include tree is loaded: trigger-package removal is reserved for
// built-in plugins, nixpkgs pins must agree with the root, and no two
// distinct includables may share a canonical name.
func (c *Config) validateIncludeTree() error {
	type includeInfo struct {
		key            string
		hasCreateFiles bool
		cfg            *Config
	}
	rootPinned := c.Root.NixPkgsCommitHash()
	canonNames := map[string]includeInfo{} // CanonicalName -> source info
	var walk func(c *Config) error
	walk = func(c *Config) error {
		if c.Root.RemoveTriggerPackage {
			if _, ok := c.Source.(*devpkg.Package); !ok {
				return usererr.New(
					"__remove_trigger_package in %s is only valid for built-in plugins",
					c.Root.AbsRootPath)
			}
		}
		if pinned := c.Root.NixPkgsCommitHash(); pinned != "" &&
			rootPinned != "" && pinned != rootPinned {
			return usererr.New(
				"%s pins nixpkgs %q but the project pins %q",
				c.Root.AbsRootPath, pinned, rootPinned)
		}
		name := c.Source.CanonicalName()
		key := c.Source.LockfileKey()
		hasCreateFiles := len(c.Root.CreateFiles) > 0
		if prev, ok := canonNames[name]; ok && prev.key != key &&
			prev.hasCreateFiles && hasCreateFiles &&
			!sameCreateFiles(prev.cfg, c) {
			return usererr.New(
				"two different includes named %q both create files and would "+
					"collide in the same directory: %q and %q",
				name, prev.key, key)
		}
		canonNames[name] = includeInfo{key: key, hasCreateFiles: hasCreateFiles, cfg: c}
		for _, i := range c.included {
			if err := walk(i); err != nil {
				return err
			}
		}
		return nil
	}
	for _, i := range c.included {
		if err := walk(i); err != nil {
			return err
		}
	}
	return nil
}

// sameCreateFiles reports whether two includables that share a canonical name
// would materialize identical files into the shared virtenv directory. Two
// refs of the same plugin (for example different branches) are allowed to
// coexist; two genuinely different configs must not silently overwrite each
// other's files.
func sameCreateFiles(a, b *Config) bool {
	if len(a.Root.CreateFiles) != len(b.Root.CreateFiles) {
		return false
	}
	for dest, contentPathA := range a.Root.CreateFiles {
		contentPathB, ok := b.Root.CreateFiles[dest]
		if !ok {
			return false
		}
		if contentPathA == "" || contentPathB == "" {
			if contentPathA != contentPathB {
				return false
			}
			continue
		}
		aContent, errA := a.Source.FileContent(contentPathA)
		bContent, errB := b.Source.FileContent(contentPathB)
		if errA != nil || errB != nil || !bytes.Equal(aContent, bContent) {
			// Unreadable or differing content is treated as a collision so
			// we never silently merge files we can't prove are identical.
			return false
		}
	}
	return true
}

// loadRecursive loads all the included plugins and their included plugins, etc.
// seen should be a cloned map because loading plugins twice is allowed if they
// are in different paths.
func (c *Config) loadRecursive(
	lockfile *lock.File,
	seen map[string]bool,
	cyclePath string,
) error {
	included := make([]*Config, 0, len(c.Root.Include))

	for _, includeRef := range c.Root.Include {
		pluginConfig, err := plugin.LoadConfigFromInclude(
			includeRef, lockfile, filepath.Dir(c.Root.AbsRootPath))
		if err != nil {
			return errors.WithStack(err)
		}

		newCyclePath := fmt.Sprintf("%s -> %s", cyclePath, includeRef)
		if seen[pluginConfig.Source.Hash()] {
			// Note that duplicate includes are allowed if they are in different paths
			// e.g. 2 different plugins can include the same plugin.
			// We do not allow a single plugin to include duplicates.
			return errors.Errorf(
				"circular or duplicate include detected:\n%s", newCyclePath)
		}
		seen[pluginConfig.Source.Hash()] = true

		includable := &Config{
			Root:   pluginConfig.ConfigFile,
			Source: pluginConfig.Source,
		}
		if localPlugin, ok := pluginConfig.Source.(*plugin.LocalPlugin); ok {
			includable.Root.AbsRootPath = localPlugin.Path()
		}

		if err := includable.loadRecursive(
			lockfile, maps.Clone(seen), newCyclePath); err != nil {
			return errors.WithStack(err)
		}

		included = append(included, includable)
	}

	builtIns, err := plugin.GetBuiltinsForPackages(
		c.Root.TopLevelPackages(),
		lockfile,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, builtIn := range builtIns {
		includable := &Config{
			Root:   builtIn.ConfigFile,
			Source: builtIn.Source,
		}
		newCyclePath := fmt.Sprintf("%s -> %s", cyclePath, builtIn.Source.LockfileKey())
		if err := includable.loadRecursive(
			lockfile, maps.Clone(seen), newCyclePath); err != nil {
			return errors.WithStack(err)
		}
		included = append(included, includable)
	}

	c.included = included
	return nil
}

func (c *Config) PackageMutator() *configfile.PackagesMutator {
	return &c.Root.PackagesMutator
}

// IncludedConfigs returns a plugin.Config for every includable in this
// config's include tree, excluding the root config itself. Configs deeper in
// the tree come first.
func (c *Config) IncludedConfigs() []*plugin.Config {
	configs := []*plugin.Config{}
	for _, i := range c.included {
		configs = append(configs, i.IncludedConfigs()...)
		configs = append(configs, &plugin.Config{
			ConfigFile: i.Root,
			Source:     i.Source,
		})
	}
	return configs
}

// EnvFromConfigs returns every config file in the include tree, including
// the root, innermost first, so env_from can be applied with provenance.
func (c *Config) EnvFromConfigs() []*configfile.ConfigFile {
	configs := []*configfile.ConfigFile{}
	for _, i := range c.included {
		configs = append(configs, i.EnvFromConfigs()...)
	}
	return append(configs, &c.Root)
}

// LocalProjectDirs returns the directory of the root config and of every
// local-path include in the include tree, innermost first, so user services
// (root-level process-compose files) can be applied with provenance.
// Remote includes (git, github, nix packages) have no checked-out project
// directory and are skipped: their services reach the project through
// create_files instead.
func (c *Config) LocalProjectDirs() []string {
	dirs := []string{}
	for _, i := range c.included {
		dirs = append(dirs, i.LocalProjectDirs()...)
	}
	switch src := c.Source.(type) {
	case nil:
		dirs = append(dirs, filepath.Dir(c.Root.AbsRootPath))
	case *plugin.LocalPlugin:
		dirs = append(dirs, filepath.Dir(src.Path()))
	}
	return dirs
}

// Returns all packages including those from included plugins.
// If includeRemovedTriggerPackages is true, then trigger packages that have
// been removed will also be returned. These are only used for built-ins
// (e.g. php) when the plugin creates a flake that is meant to replace the
// original package.
func (c *Config) Packages(
	includeRemovedTriggerPackages bool,
) []configfile.Package {
	packages := []configfile.Package{}
	packagesToRemove := map[string]bool{}

	for _, i := range c.included {
		packages = append(packages, i.Packages(includeRemovedTriggerPackages)...)
		if i.Root.RemoveTriggerPackage && !includeRemovedTriggerPackages {
			packagesToRemove[i.Source.LockfileKey()] = true
		}
	}

	// Packages to remove in built ins only affect the devbox.json where they are defined.
	// They should not remove packages that are part of other imports.
	for _, pkg := range c.Root.TopLevelPackages() {
		if !packagesToRemove[pkg.VersionedName()] {
			packages = append(packages, pkg)
		}
	}

	// Keep only the last occurrence of each package (by name).
	mutable.Reverse(packages)
	packages = lo.UniqBy(
		packages,
		func(p configfile.Package) string { return p.Name },
	)
	mutable.Reverse(packages)

	return packages
}

func (c *Config) NixPkgsCommitHash() string {
	return c.Root.NixPkgsCommitHash()
}

func (c *Config) Env() map[string]string {
	env := map[string]string{}
	for _, i := range c.included {
		expandedEnvFromPlugin := OSExpandIfPossible(i.Env(), env)
		maps.Copy(env, expandedEnvFromPlugin)
	}
	rootConfigEnv := OSExpandIfPossible(c.Root.Env, env)
	maps.Copy(env, rootConfigEnv)
	return env
}

func (c *Config) InitHook() *shellcmd.Commands {
	commands := shellcmd.Commands{}
	for _, i := range c.included {
		commands.Cmds = append(commands.Cmds, i.InitHook().Cmds...)
	}
	commands.Cmds = append(commands.Cmds, c.Root.InitHook().Cmds...)
	return &commands
}

// Aliases returns the merged shell aliases from this config and any included
// configs (plugins). Aliases defined in the root config take precedence over
// those from included configs.
func (c *Config) Aliases() map[string]string {
	aliases := map[string]string{}
	for _, i := range c.included {
		maps.Copy(aliases, i.Aliases())
	}
	maps.Copy(aliases, c.Root.Aliases)
	return aliases
}

func (c *Config) Scripts() configfile.Scripts {
	scripts := configfile.Scripts{}
	for _, i := range c.included {
		maps.Copy(scripts, i.Scripts())
	}
	maps.Copy(scripts, c.Root.Scripts())
	return scripts
}

func (c *Config) Hash() (string, error) {
	data := []byte{}
	for _, i := range c.included {
		hash, err := i.Hash()
		if err != nil {
			return "", err
		}
		data = append(data, hash...)
	}
	hash, err := c.Root.Hash()
	if err != nil {
		return "", err
	}
	data = append(data, hash...)
	return cachehash.Bytes(data), nil
}

func (c *Config) IsJetifyCloudEnvFrom() bool {
	for _, i := range c.included {
		if i.IsJetifyCloudEnvFrom() {
			return true
		}
	}
	return c.Root.IsJetifyCloudEnvFrom()
}

func OSExpandIfPossible(env, existingEnv map[string]string) map[string]string {
	mapping := func(value string) string {
		// If the value is not set in existingEnv, return the value wrapped in ${...}
		if existingEnv == nil || existingEnv[value] == "" {
			return fmt.Sprintf("${%s}", value)
		}
		return existingEnv[value]
	}

	res := map[string]string{}
	for k, v := range env {
		res[k] = os.Expand(v, mapping)
	}
	return res
}
