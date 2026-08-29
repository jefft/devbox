package plugin

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"

	"go.jetify.com/devbox/internal/boxcli/usererr"
	"go.jetify.com/devbox/nix/flake"
)

type Includable interface {
	CanonicalName() string
	FileContent(subpath string) ([]byte, error)
	Hash() string
	LockfileKey() string
}

func parseIncludable(includableRef, workingDir string) (Includable, error) {
	ref, err := flake.ParseRef(includableRef)
	if err != nil {
		return nil, err
	}
	switch ref.Type {
	case flake.TypePath:
		return newLocalPlugin(ref, workingDir)
	case flake.TypeIndirect:
		// Bare names (e.g. "inner" or "./../base") parse as indirect flake
		// registry refs. Devbox treats them as local paths relative to the
		// including config's directory.
		ref.Type = flake.TypePath
		ref.Path = includableRef
		return newLocalPlugin(ref, workingDir)
	case flake.TypeGitHub:
		return newGithubPlugin(ref)
	case flake.TypeGit:
		return newGitPlugin(ref)
	default:
		return nil, fmt.Errorf("unsupported ref type %q", ref.Type)
	}
}

type fetcher interface {
	Includable
	Fetch() ([]byte, error)
}

var (
	nameRegex        = regexp.MustCompile(`^[a-zA-Z0-9_\- ]+$`)
	nameRegexInvalid = regexp.MustCompile(`[^a-zA-Z0-9_\- ]`)
	errNameMissing   = usererr.New("'name' is missing")
)

func getPluginNameFromContent(plugin fetcher) (string, error) {
	content, err := plugin.Fetch()
	if err != nil {
		return "", err
	}
	m := map[string]any{}
	if err := json.Unmarshal(content, &m); err != nil {
		return "", err
	}
	name, ok := m["name"].(string)
	if !ok || name == "" {
		return "",
			fmt.Errorf("%w in plugin %s", errNameMissing, plugin.LockfileKey())
	}
	if !nameRegex.MatchString(name) {
		return "", usererr.New(
			"plugin %s has an invalid name %q. Name must match %s",
			plugin.LockfileKey(), name, nameRegex,
		)
	}
	return name, nil
}

// getProjectNameFromContent reads the optional "name" field of a project
// descriptor (devbox.json), falling back to a sanitized form of the
// descriptor's directory name.
func getProjectNameFromContent(plugin fetcher) (string, error) {
	content, err := plugin.Fetch()
	if err != nil {
		return "", err
	}
	m := map[string]any{}
	if err := json.Unmarshal(content, &m); err != nil {
		return "", err
	}
	if name, ok := m["name"].(string); ok && nameRegex.MatchString(name) {
		return name, nil
	}
	base := filepath.Base(filepath.Dir(plugin.(*LocalPlugin).path))
	return nameRegexInvalid.ReplaceAllString(base, "-"), nil
}
