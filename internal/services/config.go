// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package services

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/f1bonacc1/process-compose/src/types"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"go.jetify.com/devbox/internal/boxcli/usererr"
	"go.jetify.com/devbox/internal/cuecfg"
)

func FromUserProcessCompose(projectDir, userProcessCompose string) Services {
	processComposeYaml := lookupProcessCompose(projectDir, userProcessCompose)
	if processComposeYaml == "" {
		return nil
	}

	userSvcs, err := FromProcessCompose(processComposeYaml)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading process-compose.yaml: %s, skipping", err)
		return nil
	}
	return userSvcs
}

// FromProjectProcessComposes merges the root-level process-compose file of
// each project directory, innermost first, so that a project including
// another project inherits its user services. Two distinct files defining
// the same service is an error.
func FromProjectProcessComposes(dirs []string) (Services, error) {
	all := Services{}
	origins := map[string]string{} // service name -> defining file
	for _, dir := range dirs {
		path := lookupProcessCompose(dir, "")
		if path == "" {
			continue
		}
		svcs, err := FromProcessCompose(path)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		for name := range svcs {
			if prev, ok := origins[name]; ok {
				return nil, usererr.New(
					"service %q is defined by two project process-compose files: %q and %q",
					name, prev, path)
			}
			origins[name] = path
		}
		maps.Copy(all, svcs)
	}
	return all, nil
}

func FromProcessCompose(path string) (Services, error) {
	processCompose := &types.Project{}
	services := Services{}
	err := errors.WithStack(cuecfg.ParseFile(path, processCompose))
	if err != nil {
		return nil, err
	}

	// Every process inherits the project-level environment, overlaid with
	// its own. Devbox relies on this to find structural paths like
	// DEVBOX_RUNTIME_DIR regardless of which process declared them.
	projectEnv := envMap(processCompose.Environment)
	for name, proc := range processCompose.Processes {
		env := maps.Clone(projectEnv)
		maps.Copy(env, envMap(proc.Environment))
		svc := Service{
			Name:               name,
			ProcessComposePath: path,
			Env:                env,
		}
		services[name] = svc
	}

	return services, nil
}

// envMap parses a process-compose Environment list of "KEY=VALUE" entries
// into a map. Entries without "=" are ignored.
func envMap(env types.Environment) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[k] = v
		}
	}
	return m
}

func NamesFromProcessCompose(content []byte) ([]string, error) {
	var processCompose types.Project
	if err := yaml.Unmarshal(content, &processCompose); err != nil {
		return nil, err
	}
	names := []string{}
	for name := range processCompose.Processes {
		names = append(names, name)
	}
	return names, nil
}

func lookupProcessCompose(projectDir, path string) string {
	if path == "" {
		path = projectDir
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(projectDir, path)
	}

	pathsToCheck := []string{
		path,
		filepath.Join(path, "process-compose.yaml"),
		filepath.Join(path, "process-compose.yml"),
	}

	for _, p := range pathsToCheck {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}

	return ""
}
