// Package testutil provides temporary homes and offline git remotes for behavior tests.
package testutil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fschrhunt/dot/internal/app"
	"github.com/fschrhunt/dot/internal/config"
	"github.com/fschrhunt/dot/internal/setup"
	dotsync "github.com/fschrhunt/dot/internal/sync"
)

// Fixture isolates a setup, live home, and git identity for one test.
type Fixture struct {
	T     *testing.T
	Temp  string
	Paths setup.Paths
}

// New creates a temp HOME and DOT_HOME; tests using it must not run in parallel.
func New(t *testing.T) *Fixture {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	dot := filepath.Join(home, ".dot")
	if e := os.MkdirAll(dot, 0755); e != nil {
		t.Fatal(e)
	}
	for k, v := range map[string]string{"HOME": home, "DOT_HOME": dot, "DOT_MACHINE": "laptop", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_AUTHOR_NAME": "test", "GIT_AUTHOR_EMAIL": "test@example.com", "GIT_COMMITTER_NAME": "test", "GIT_COMMITTER_EMAIL": "test@example.com", "GIT_CONFIG_COUNT": "1", "GIT_CONFIG_KEY_0": "init.defaultBranch", "GIT_CONFIG_VALUE_0": "main"} {
		t.Setenv(k, v)
	}
	return &Fixture{t, tmp, setup.FromEnv()}
}

// Write creates relative files under root, failing the test on an IO error.
func (f *Fixture) Write(root string, files map[string]string) {
	f.T.Helper()
	for rel, data := range files {
		p := filepath.Join(root, rel)
		if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
			f.T.Fatal(e)
		}
		if e := os.WriteFile(p, []byte(data), 0644); e != nil {
			f.T.Fatal(e)
		}
	}
}

// Config writes dot.toml and any supplied sources into the setup.
func (f *Fixture) Config(text string, files map[string]string) {
	f.T.Helper()
	f.Write(f.Paths.Dot, map[string]string{"dot.toml": text})
	f.Write(f.Paths.Dot, files)
}

// Read returns a file's text, failing on an IO error.
func (f *Fixture) Read(root, rel string) string {
	f.T.Helper()
	b, e := os.ReadFile(filepath.Join(root, rel))
	if e != nil {
		f.T.Fatal(e)
	}
	return string(b)
}

// Result is a command's exit code and combined stdout and diagnostics.
type Result struct {
	Code   int
	Output string
}

// finish formats handler errors in the CLI's one-prefix-per-line format.
func finish(code int, err error, b *bytes.Buffer) Result {
	if err != nil {
		code = setup.ExitCode(err)
		for _, line := range strings.Split(err.Error(), "\n") {
			fmt.Fprintln(b, "dot: "+line)
		}
	}
	return Result{code, b.String()}
}

// invoke loads the config and captures one command handler.
func (f *Fixture) invoke(fn func(*config.Config, *bytes.Buffer) (int, error)) Result {
	var b bytes.Buffer
	c, e := config.Load(f.Paths)
	if e != nil {
		return finish(2, e, &b)
	}
	code, e := fn(c, &b)
	return finish(code, e, &b)
}

// Apply calls apply with the real state and an optional force flag.
func (f *Fixture) Apply(force bool) Result {
	return f.invoke(func(c *config.Config, b *bytes.Buffer) (int, error) { return app.Apply(c, false, force, b, b) })
}

// Status calls status, optionally requesting a path diff.
func (f *Fixture) Status(path string) Result {
	return f.invoke(func(c *config.Config, b *bytes.Buffer) (int, error) { return app.Status(c, path, b) })
}

// Take copies a live path back into its configured source.
func (f *Fixture) Take(path string) Result {
	return f.invoke(func(c *config.Config, b *bytes.Buffer) (int, error) { return app.Take(c, path, b) })
}

// Sync runs the quiet git-sync handler.
func (f *Fixture) Sync() Result {
	var b bytes.Buffer
	code, e := dotsync.Run(f.Paths)
	return finish(code, e, &b)
}

// Git runs git with the fixture's isolated identity and configuration.
func (f *Fixture) Git(cwd string, args ...string) string {
	f.T.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	b, e := cmd.CombinedOutput()
	if e != nil {
		f.T.Fatalf("git %v: %v: %s", args, e, b)
	}
	return string(b)
}

// Commit writes files and commits them in a local clone.
func (f *Fixture) Commit(cwd string, files map[string]string, message string) {
	f.T.Helper()
	f.Write(cwd, files)
	f.Git(cwd, "add", "-A")
	f.Git(cwd, "commit", "-qm", message)
}

// Remote seeds a local bare repo and clones it as DOT_HOME; no network is used.
func (f *Fixture) Remote(text string, files map[string]string) string {
	f.T.Helper()
	bare, seed := filepath.Join(f.Temp, "remote.git"), filepath.Join(f.Temp, "seed")
	f.Git(f.Temp, "init", "--bare", "-q", bare)
	f.Git(f.Temp, "clone", "-q", bare, seed)
	f.Write(seed, map[string]string{"dot.toml": text, ".gitignore": ".state/\n"})
	f.Commit(seed, files, "setup")
	f.Git(seed, "push", "-q", "-u", "origin", "main")
	if e := os.RemoveAll(f.Paths.Dot); e != nil {
		f.T.Fatal(e)
	}
	f.Git(f.Temp, "clone", "-q", bare, f.Paths.Dot)
	return bare
}

// Equal compares one protected result and reports the values on failure.
func Equal[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}

// OK requires a handler to succeed, showing its diagnostic on failure.
func OK(t *testing.T, r Result) {
	t.Helper()
	if r.Code != 0 {
		t.Fatalf("exit %d: %s", r.Code, r.Output)
	}
}
