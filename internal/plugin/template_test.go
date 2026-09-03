// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package plugin

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

// runtimeDir must be deterministic for a given project and plugin, live under
// the per-user runtime dir, and be scoped per plugin (mysql and mariadb both
// use mysql.sock, so a shared dir would collide).
func TestRuntimeDirStableAndScoped(t *testing.T) {
	runtimeBase := t.TempDir() // hermetic: /run/user/<uid> may not exist on CI
	t.Setenv("XDG_RUNTIME_DIR", runtimeBase)
	dir1 := runtimeDir("/home/user/projects/myapp", "mariadb")
	dir2 := runtimeDir("/home/user/projects/myapp", "mariadb")
	if dir1 != dir2 {
		t.Fatalf("runtimeDir not stable: %q != %q", dir1, dir2)
	}
	if !strings.HasPrefix(dir1, runtimeBase+"/devbox/") {
		t.Fatalf("expected XDG_RUNTIME_DIR base, got %q", dir1)
	}
	if !strings.HasSuffix(dir1, "/mariadb") {
		t.Fatalf("expected per-plugin suffix, got %q", dir1)
	}
	if runtimeDir("/home/user/projects/myapp", "mariadb") == runtimeDir("/home/user/projects/myapp", "mysql") {
		t.Fatal("distinct plugins must not share a runtime dir")
	}
	// Different project paths must hash to different dirs.
	if runtimeDir("/home/user/projects/myapp", "mysql") == runtimeDir("/home/user/projects/other", "mysql") {
		t.Fatal("distinct projects must not share a runtime dir")
	}
}

func TestRuntimeDirFallsBackToTempDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	if dir := runtimeDir("/p", "mariadb"); strings.HasPrefix(dir, "/run/user") {
		t.Fatalf("expected temp-dir fallback when XDG_RUNTIME_DIR unset, got %q", dir)
	}

	// A set but missing XDG_RUNTIME_DIR must also fall back: the dir cannot be
	// created and would break MkdirAll if we tried to use it as a base.
	t.Setenv("XDG_RUNTIME_DIR", "/nonexistent/xyz")
	if dir := runtimeDir("/p", "mariadb"); strings.HasPrefix(dir, "/nonexistent") {
		t.Fatalf("expected fallback when XDG_RUNTIME_DIR missing, got %q", dir)
	}
}

// Unix domain socket paths are limited to 107 chars on Linux and 104 on
// macOS. A 200-char project path plus a long macOS-style TMPDIR must still
// render socket paths under the 104-char limit.
func TestRuntimePathsFitUnixLimit(t *testing.T) {
	deep := filepath.Join(strings.Repeat("a", 100), strings.Repeat("b", 100))
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("TMPDIR", "/var/folders/yn/abcdefghijklmnopqrstuvwx/T")
	for _, testCase := range []struct{ plugin, sock string }{
		{"mariadb", "mysql.sock"},
		{"mysql", "mysql.sock"},
		{"php", "php-fpm.sock"},
		{"postgresql", ".s.PGSQL.5432"},
	} {
		got := filepath.Join(runtimeDir(deep, testCase.plugin), testCase.sock)
		if len(got) > 104 {
			t.Errorf("%s socket path is %d chars (>104 limit): %s", testCase.plugin, len(got), got)
		}
	}
}

func TestRuntimeDirRenderedInPluginConfig(t *testing.T) {
	deep := filepath.Join(strings.Repeat("a", 100), strings.Repeat("b", 100))
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("TMPDIR", "/var/folders/yn/abcdefghijklmnopqrstuvwx/T")
	content := `{
	  "name": "mariadb",
	  "version": "0.0.8",
	  "env": {
	    "MYSQL_UNIX_PORT": "{{ .RuntimeDir }}/mysql.sock"
	  }
	}`
	cfg, err := buildConfig(fakeIncludable{name: "mariadb"}, deep, content)
	if err != nil {
		t.Fatalf("buildConfig: %v", err)
	}
	want := filepath.Join(runtimeDir(deep, "mariadb"), "mysql.sock")
	if got := cfg.Env["MYSQL_UNIX_PORT"]; got != want {
		t.Fatalf("rendered MYSQL_UNIX_PORT = %q, want %q", got, want)
	}
	if len(cfg.Env["MYSQL_UNIX_PORT"]) > 104 {
		t.Fatalf("rendered socket path too long: %q", cfg.Env["MYSQL_UNIX_PORT"])
	}
}

// TestShippedPluginsRuntimePathsFitUnixLimit renders the real plugin.json files
// shipped in this repo (the ones users actually install) with a deeply nested
// project dir and asserts every socket-related env var stays under the unix
// socket path limit (104 chars on macOS, the stricter of the two).
func TestShippedPluginsRuntimePathsFitUnixLimit(t *testing.T) {
	deep := filepath.Join(strings.Repeat("a", 100), strings.Repeat("b", 100))
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("TMPDIR", "/var/folders/yn/abcdefghijklmnopqrstuvwx/T")
	// sockFile is the filename the server appends to the env value: PGHOST is
	// a directory (psql appends .s.PGSQL.<port>); the others are full socket
	// paths already.
	for _, testCase := range []struct{ file, plugin, env, sockFile string }{
		{"mariadb.json", "mariadb", "MYSQL_UNIX_PORT", ""},
		{"mysql.json", "mysql", "MYSQL_UNIX_PORT", ""},
		{"php.json", "php", "PHPFPM_UNIX_SOCKET", ""},
		{"postgresql.json", "postgresql", "PGHOST", ".s.PGSQL.5432"},
	} {
		content, err := os.ReadFile(filepath.Join("..", "..", "plugins", testCase.file))
		if err != nil {
			t.Fatalf("read %s: %v", testCase.file, err)
		}
		cfg, err := buildConfig(fakeIncludable{name: testCase.plugin}, deep, string(content))
		if err != nil {
			t.Fatalf("buildConfig(%s): %v", testCase.file, err)
		}
		got := cfg.Env[testCase.env]
		if got == "" {
			t.Fatalf("%s: env %s not rendered", testCase.file, testCase.env)
		}
		if testCase.sockFile != "" {
			got = filepath.Join(got, testCase.sockFile)
		}
		if len(got) > 104 {
			t.Errorf("%s: %s socket path is %d chars (>104): %s", testCase.file, testCase.env, len(got), got)
		}
	}
}

// TestShippedPluginsDataAndLogDirs pins the DataDir/LogDir contracts for the
// shipped plugins: persistent state under .devbox/data/<plugin> (a single
// backup target) and logs under .devbox/logs/<plugin>.
func TestShippedPluginsDataAndLogDirs(t *testing.T) {
	deep := "/home/user/projects/myapp"
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	for _, testCase := range []struct {
		file, plugin, dataEnv, logEnv, logFile string
	}{
		{"apacheHttpd.json", "apache", "", "HTTPD_ERROR_LOG_FILE", "error.log"},
		{"caddy.json", "caddy", "", "CADDY_LOG_DIR", ""},
		{"mariadb.json", "mariadb", "MYSQL_DATADIR", "", ""},
		{"mysql.json", "mysql", "MYSQL_DATADIR", "", ""},
		{"php.json", "php", "", "PHPFPM_ERROR_LOG_FILE", "php-fpm.log"},
		{"postgresql.json", "postgresql", "PGDATA", "", ""},
	} {
		content, err := os.ReadFile(filepath.Join("..", "..", "plugins", testCase.file))
		if err != nil {
			t.Fatalf("read %s: %v", testCase.file, err)
		}
		cfg, err := buildConfig(fakeIncludable{name: testCase.plugin}, deep, string(content))
		if err != nil {
			t.Fatalf("buildConfig(%s): %v", testCase.file, err)
		}
		if testCase.dataEnv != "" {
			want := filepath.Join(deep, ".devbox", "data", testCase.plugin)
			if got := cfg.Env[testCase.dataEnv]; got != want {
				t.Errorf("%s: %s = %q, want %q", testCase.file, testCase.dataEnv, got, want)
			}
		}
		if testCase.logEnv != "" {
			want := filepath.Join(deep, ".devbox", "logs", testCase.plugin, testCase.logFile)
			if got := cfg.Env[testCase.logEnv]; got != want {
				t.Errorf("%s: %s = %q, want %q", testCase.file, testCase.logEnv, got, want)
			}
		}
	}
}

// TestShippedPluginYamlsRenderCleanly renders every shipped process-compose.yaml
// with the same template variables createFile uses and asserts no stray
// template artifacts leak into the output. Regression: a '$' left in front of
// {{ .LogDir }} rendered as a literal '$' in the command, so the log-tail
// process followed a nonexistent path and crash-looped.
func TestShippedPluginYamlsRenderCleanly(t *testing.T) {
	deep := filepath.Join(strings.Repeat("a", 100), strings.Repeat("b", 100))
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("TMPDIR", "/var/folders/yn/abcdefghijklmnopqrstuvwx/T")
	dirs, err := os.ReadDir(filepath.Join("..", "..", "plugins"))
	if err != nil {
		t.Fatalf("read plugins dir: %v", err)
	}
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		path := filepath.Join("..", "..", "plugins", dir.Name(), "process-compose.yaml")
		content, err := os.ReadFile(path)
		if err != nil {
			continue // plugin without a process-compose.yaml
		}
		tmpl, err := template.New(dir.Name() + "-yaml").Parse(string(content))
		if err != nil {
			t.Fatalf("%s: parse: %v", dir.Name(), err)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, TemplateDataFor(deep, dir.Name())); err != nil {
			t.Fatalf("%s: render: %v", dir.Name(), err)
		}
		rendered := buf.String()
		if strings.Contains(rendered, "${{") || strings.Contains(rendered, "$/") {
			t.Errorf("%s: stray '$' artifact in rendered yaml:\n%s", dir.Name(), rendered)
		}
	}
}

type fakeIncludable struct{ name string }

func (f fakeIncludable) CanonicalName() string              { return f.name }
func (f fakeIncludable) FileContent(string) ([]byte, error) { return nil, nil }
func (f fakeIncludable) Hash() string                       { return "" }
func (f fakeIncludable) LockfileKey() string                { return f.name }

// TestShippedPluginsServiceEnvContract pins the DEVBOX_* injection: every
// shipped plugin's process-compose.yaml carries an environment block that
// devbox renders per includable, so plugin templates can reference their
// structural directories without declaring per-plugin aliases.
func TestShippedPluginsServiceEnvContract(t *testing.T) {
	deep := "/home/user/projects/myapp"
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	want := map[string]string{
		"DEVBOX_DEV_DIR":     filepath.Join(deep, "devbox.d", "nginx"),
		"DEVBOX_LOG_DIR":     filepath.Join(deep, ".devbox", "logs", "nginx"),
		"DEVBOX_DATA_DIR":    filepath.Join(deep, ".devbox", "data", "nginx"),
		"DEVBOX_RUNTIME_DIR": runtimeDir(deep, "nginx"),
		"DEVBOX_VIRTENV":     filepath.Join(deep, ".devbox", "virtenv", "nginx"),
		"DEVBOX_PROJECT_DIR": deep,
	}
	content, err := os.ReadFile(filepath.Join("..", "..", "plugins", "nginx", "process-compose.yaml"))
	if err != nil {
		t.Fatalf("read nginx process-compose.yaml: %v", err)
	}
	tmpl, err := template.New("pc").Parse(string(content))
	if err != nil {
		t.Fatalf("parse nginx process-compose.yaml: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, TemplateDataFor(deep, "nginx")); err != nil {
		t.Fatalf("render nginx process-compose.yaml: %v", err)
	}
	for key, value := range want {
		line := key + "=" + value
		if !strings.Contains(buf.String(), line) {
			t.Errorf("rendered nginx process-compose.yaml missing %q", line)
		}
	}
	confContent, err := os.ReadFile(filepath.Join("..", "..", "plugins", "nginx", "nginx.conf"))
	if err != nil {
		t.Fatalf("read nginx.conf: %v", err)
	}
	// The shipped default conf renders via Go templates and must keep using
	// {{ .LogDir }}, never the $DEVBOX_LOG_DIR envsubst mechanism that the
	// customization template resolves at service start.
	if !strings.Contains(string(confContent), "{{ .LogDir }}/nginx-error.log") {
		t.Error("nginx.conf must reference {{ .LogDir }}/nginx-error.log")
	}
	if strings.Contains(buf.String(), "NGINX_LOGDIR") {
		t.Error("nginx process-compose.yaml still references the removed NGINX_LOGDIR alias")
	}
}
