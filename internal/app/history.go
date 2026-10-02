package app

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fschrhunt/dot/internal/apply"
	"github.com/fschrhunt/dot/internal/config"
	"github.com/fschrhunt/dot/internal/plan"
	"github.com/fschrhunt/dot/internal/setup"
)

// git runs git in the setup with paths taken literally, and returns its output. A failure is a
// user error carrying git's own last line.
func git(paths setup.Paths, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"--literal-pathspecs", "-C", paths.Dot}, args...)...)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if e := cmd.Run(); e != nil {
		lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
		return "", setup.Fail("git " + args[0] + ": " + lines[len(lines)-1])
	}
	return out.String(), nil
}

// sources lists where the live path p is kept, as paths inside the setup: the one file it is
// written from, the folder a shared unit is written from, or the file behind each managed path
// under p.
func sources(c *config.Config, p string) ([]string, error) {
	want, roots, _, e := plan.Wants(c)
	if e != nil {
		return nil, e
	}
	var found []string
	if w, ok := want[p]; ok && w.Kind != "dir" {
		found = []string{w.Src}
	}
	for _, r := range roots {
		if len(found) == 0 && (p == r.Path || setup.Under(p, r.Path)) {
			rel, _ := filepath.Rel(r.Path, p)
			found = []string{filepath.Join(r.Source, rel)}
		}
	}
	if len(found) == 0 {
		for q, w := range want {
			if setup.Under(q, p) && w.Kind != "dir" && !slices.Contains(found, w.Src) {
				found = append(found, w.Src)
			}
		}
	}
	if len(found) == 0 {
		return nil, setup.Fail(c.Paths.Show(p) + " is not managed by dot")
	}
	slices.Sort(found)
	for i, source := range found {
		rel, e := filepath.Rel(c.Paths.Dot, source)
		if e != nil || strings.HasPrefix(rel, "..") {
			return nil, setup.Fail(c.Paths.Show(p) + " is kept outside the setup, in " + c.Paths.Show(source))
		}
		found[i] = rel
	}
	return found, nil
}

// Log prints the setup's last twenty changes, newest first, or those of one managed path. Each
// line is a commit: its short name, when it was made, and the machine and paths it names.
func Log(c *config.Config, path string, out io.Writer) (int, error) {
	args := []string{"log", "-n", "20", "--date=format:%Y-%m-%d %H:%M", "--format=%h %ad %s"}
	if path != "" {
		kept, e := sources(c, c.Paths.Abs(path))
		if e != nil {
			return 2, e
		}
		args = append(append(args, "--"), kept...)
	}
	text, e := git(c.Paths, args...)
	if e != nil {
		return 2, e
	}
	fmt.Fprint(out, text)
	return 0, nil
}

// Undo takes one managed path back to how the setup had it before its last change, records
// that as a new commit named for the machine, and applies it; the next sync sends it on. It is
// itself a change, so a second undo brings the path forward again. It refuses a path with
// changes not yet synced, which it would otherwise bury, a path whose last change added it, and
// a one-way setup, where its commit could not be pushed.
func Undo(c *config.Config, path string, out, stderr io.Writer) (int, error) {
	// A commit made here must be able to reach the remote: a one-way setup only fast-forwards,
	// so a local commit would stop every later sync.
	if !c.Take {
		return 2, setup.Fail("dot undo needs a setup that syncs both ways (version 2, with take on); here, use git revert in " + c.Paths.Show(c.Paths.Dot))
	}
	p := c.Paths.Abs(path)
	shown := c.Paths.Show(p)
	kept, e := sources(c, p)
	if e != nil {
		return 2, e
	}
	written, e := apply.ReadWritten(c.Paths)
	if e != nil {
		return 2, e
	}
	pending, e := plan.Build(c, written, plan.Options{Take: c.Take})
	if e != nil {
		return 2, e
	}
	dirty, e := git(c.Paths, append([]string{"status", "--porcelain", "--"}, kept...)...)
	if e != nil {
		return 2, e
	}
	if dirty != "" || slices.ContainsFunc(pending.Actions, func(a plan.Action) bool {
		return a.Op != "" && (a.Path == p || setup.Under(a.Path, p))
	}) {
		return 1, setup.Fail(shown+" has changes that are not synced yet (dot status "+shown+" shows them); dot sync first, or dot apply --force to drop them", 1)
	}
	last, e := git(c.Paths, append([]string{"log", "-1", "--format=%H %h %s", "--"}, kept...)...)
	if e != nil {
		return 2, e
	}
	hash, name, _ := strings.Cut(strings.TrimSpace(last), " ")
	if hash == "" {
		return 2, setup.Fail(shown + " has no history in the setup yet")
	}
	if !slices.ContainsFunc(kept, func(source string) bool {
		_, e := git(c.Paths, "cat-file", "-e", hash+"^:"+filepath.ToSlash(source))
		return e == nil
	}) {
		return 2, setup.Fail(shown + " was added by its last change (" + name + "), so there is nothing earlier to go back to; dot forget " + shown + " stops managing it")
	}
	if _, e := git(c.Paths, append([]string{"restore", "--source=" + hash + "^", "--staged", "--worktree", "--"}, kept...)...); e != nil {
		return 2, e
	}
	commit := []string{"commit", "--quiet", "--message", c.Paths.Machine + ": undo " + shown, "--"}
	if _, e := git(c.Paths, "config", "user.email"); e != nil {
		commit = append([]string{"-c", "user.name=dot", "-c", "user.email=dot@" + c.Paths.Machine}, commit...)
	}
	if _, e := git(c.Paths, append(commit, kept...)...); e != nil {
		return 2, e
	}
	fmt.Fprintf(out, "undid %s for %s; dot sync sends it to the other machines\n", name, shown)
	if c, e = config.Load(c.Paths); e != nil {
		return 2, e
	}
	return Apply(c, false, false, out, stderr)
}
