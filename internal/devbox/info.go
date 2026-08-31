// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package devbox

import (
	"go.jetify.com/devbox/internal/plugin"
)

// ProjectPaths describes the devbox-managed directories of the consuming
// project and of every includable in its include tree. All values are
// resolved (absolute) and match exactly what plugin templates see via
// plugin.TemplateVars, so this cannot drift from what services and file
// materialization actually use.
type ProjectPaths struct {
	ProjectDir    string            `json:"projectDir"`
	DevboxDirRoot string            `json:"devboxDirRoot"`
	ProfilePath   string            `json:"profilePath"`
	Includables   []IncludablePaths `json:"includables"`
}

// IncludablePaths are the name-scoped directories of one includable.
type IncludablePaths struct {
	Name       string `json:"name"`
	DevboxDir  string `json:"devboxDir"`
	Virtenv    string `json:"virtenv"`
	DataDir    string `json:"dataDir"`
	LogDir     string `json:"logDir"`
	RuntimeDir string `json:"runtimeDir"`
}

// ProjectPaths resolves the devbox-managed directories for the project and
// each includable in the include tree. It is a pure function of the include
// tree and requires no nix, packages, or installed state.
func (d *Devbox) ProjectPaths() ProjectPaths {
	paths := ProjectPaths{Includables: []IncludablePaths{}}

	vars := plugin.TemplateVars(d.projectDir, "")
	paths.ProjectDir = vars["DevboxProjectDir"].(string)
	paths.DevboxDirRoot = vars["DevboxDirRoot"].(string)
	paths.ProfilePath = vars["DevboxProfileDefault"].(string)

	seen := map[string]bool{}
	for _, cfg := range d.cfg.IncludedConfigs() {
		if cfg.Source == nil {
			continue
		}
		name := cfg.Source.CanonicalName()
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		vars := plugin.TemplateVars(d.projectDir, name)
		paths.Includables = append(paths.Includables, IncludablePaths{
			Name:       name,
			DevboxDir:  vars["DevboxDir"].(string),
			Virtenv:    vars["Virtenv"].(string),
			DataDir:    vars["DataDir"].(string),
			LogDir:     vars["LogDir"].(string),
			RuntimeDir: vars["RuntimeDir"].(string),
		})
	}
	return paths
}
