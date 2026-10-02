// Package app implements user commands over validated configurations and shared plans.
package app

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	dot "github.com/fschrhunt/dot"
	"github.com/fschrhunt/dot/internal/apply"
	"github.com/fschrhunt/dot/internal/config"
	"github.com/fschrhunt/dot/internal/plan"
	"github.com/fschrhunt/dot/internal/setup"
)

// Usage describes the compatible commands and the added binary version command.
const Usage = `dot: the dotfiles manager.

usage:
  dot [status [path]]       show what apply would do; with a path, a diff for it
                            (+ new, ~ changed, - removed, ! edited here, ? extra)
  dot apply [-n] [--force]  write the plan; -n only prints it; --force overwrites files edited here
                            (> run is a mapping's command, run after its files change)
  dot sync                  pull, push when ahead and push is on, then apply (the timer runs this)
  dot take <path>           copy a live file or folder back to its source in the setup
  dot init [remote]         create the setup from the example, or clone it from a git remote
  dot timer [--remove]      run dot sync on a timer on this machine, every 15 minutes unless
                            [sync] every says otherwise (or stop it)
  dot help                  this text and a summary of the setup
  dot version               print the binary version (also --version)

The setup is ~/.dot (or DOT_HOME): dot.toml plus the sources it names. The machine name is
hostname -s (or DOT_MACHINE). Edit sources there and commit; never edit the destinations.`

// Status prints the last sync and plan, or the diff under path; actionable changes exit 1.
func Status(c *config.Config, path string, out io.Writer) (int, error) {
	written, e := apply.ReadWritten(c.Paths)
	if e != nil {
		return 2, e
	}
	p, e := plan.Build(c, written)
	if e != nil {
		return 2, e
	}
	actions := p.Actions
	if path != "" {
		q := c.Paths.Abs(path)
		var hits []plan.Action
		for _, a := range actions {
			if a.Path == q || setup.Under(a.Path, q) {
				hits = append(hits, a)
			}
		}
		managed := len(hits) > 0
		for s := range p.Wants {
			if s == q || setup.Under(s, q) {
				managed = true
			}
		}
		if !managed {
			return 2, setup.Fail(c.Paths.Show(q) + " is not managed by dot")
		}
		code := 0
		for _, a := range hits {
			if a.Op != "" {
				code = 1
			}
			if a.Op == "write" || a.Op == "delete" {
				if e := plan.Diff(out, c.Paths, a.Path, a.Want, false); e != nil {
					return 2, e
				}
			}
		}
		return code, nil
	}
	if b, e := os.ReadFile(filepath.Join(c.Paths.State, "last")); e == nil {
		fmt.Fprintln(out, "last sync:", strings.TrimSpace(string(b)))
	} else if !os.IsNotExist(e) {
		return 2, e
	}
	plan.Print(out, c.Paths, actions)
	if len(actions) == 0 {
		fmt.Fprintln(out, "up to date")
	}
	for _, a := range actions {
		if a.Op != "" {
			return 1, nil
		}
	}
	return 0, nil
}

// Apply prints a dry plan or executes it and reports refused edits on stderr.
func Apply(c *config.Config, dry, force bool, out, stderr io.Writer) (int, error) {
	if dry {
		written, e := apply.ReadWritten(c.Paths)
		if e != nil {
			return 2, e
		}
		p, e := plan.Build(c, written)
		if e != nil {
			return 2, e
		}
		plan.Print(out, c.Paths, p.Actions)
		return 0, nil
	}
	done, refused, e := apply.Run(c, force)
	if e != nil {
		return 2, e
	}
	plan.Print(out, c.Paths, done)
	for _, a := range refused {
		fmt.Fprintln(stderr, "dot: "+plan.Refusal(c.Paths, a))
	}
	if len(refused) > 0 {
		return 1, nil
	}
	return 0, nil
}

// Take copies live managed paths back to their source, refusing rendered destinations.
func Take(c *config.Config, path string, out io.Writer) (int, error) {
	p := c.Paths.Abs(path)
	want, roots, _, e := plan.Wants(c)
	if e != nil {
		return 2, e
	}
	hits := map[string]setup.Want{}
	for q, w := range want {
		if q == p || setup.Under(q, p) {
			hits[q] = w
		}
	}
	if len(hits) == 0 {
		return 2, setup.Fail(c.Paths.Show(p) + " is not managed by dot")
	}
	maps, e := c.Resolve(c.Paths.Machine)
	if e != nil {
		return 2, e
	}
	for _, mp := range maps {
		if !mp.Template {
			continue
		}
		for _, q := range mp.Dests {
			w, ok := hits[q]
			if ok {
				if e := plan.Diff(out, c.Paths, q, w, true); e != nil {
					return 2, e
				}
				return 1, setup.Fail(c.Paths.Show(q)+" is rendered from "+c.Paths.Show(w.Src)+"; edit the template instead", 1)
			}
		}
	}
	var root *plan.Root
	for i, r := range roots {
		if (p == r.Path || setup.Under(p, r.Path)) && (root == nil || len(r.Path) > len(root.Path)) {
			root = &roots[i]
		}
	}
	type pair struct{ live, src string }
	var pairs []pair
	if w, ok := hits[p]; ok && w.Kind != "dir" {
		pairs = append(pairs, pair{p, w.Src})
	}
	if len(pairs) == 0 && root == nil {
		return 2, setup.Fail(c.Paths.Show(p) + " holds several destinations; take them one at a time")
	}
	if len(pairs) == 0 {
		if !setup.IsDir(p) {
			s := " is not a folder"
			if !setup.Exists(p) {
				s = " does not exist"
			}
			return 2, setup.Fail(c.Paths.Show(p) + s)
		}
		e := setup.Walk(p, root.Exclude, func(dir string, files []string) error {
			for _, f := range files {
				q := filepath.Join(dir, f)
				rel, _ := filepath.Rel(root.Path, q)
				if !setup.Excluded(rel, root.Exclude) {
					pairs = append(pairs, pair{q, filepath.Join(root.Source, rel)})
				}
			}
			return nil
		})
		if e != nil {
			return 2, e
		}
	}
	for _, pair := range pairs {
		if !setup.Exists(pair.live) {
			return 2, setup.Fail(c.Paths.Show(pair.live) + " does not exist")
		}
		w, e := setup.WantOf(pair.live, nil)
		if e != nil {
			return 2, e
		}
		sig, e := setup.LiveSig(pair.src)
		if e != nil {
			return 2, e
		}
		if sig != w.Sig {
			if setup.IsDir(pair.src) && !setup.IsLink(pair.src) {
				return 2, setup.Fail("cannot take " + c.Paths.Show(pair.live) + ": " + c.Paths.Show(pair.src) + " is a folder")
			}
			if e := setup.Write(pair.src, w); e != nil {
				return 2, e
			}
			fmt.Fprintf(out, "took %s -> %s\n", c.Paths.Show(pair.live), c.Paths.Show(pair.src))
		}
	}
	return 0, nil
}

// Init clones a remote or copies the embedded example, refusing an existing setup.
func Init(paths setup.Paths, remote string, out, stderr io.Writer) (int, error) {
	if setup.Exists(paths.Dot) {
		return 2, setup.Fail(paths.Show(paths.Dot) + " already exists")
	}
	if remote != "" {
		cmd := exec.Command("git", "clone", "--quiet", remote, paths.Dot)
		cmd.Stdout = out
		cmd.Stderr = stderr
		if e := cmd.Run(); e != nil {
			return 2, setup.Fail("could not clone " + remote)
		}
		b, e := os.ReadFile(filepath.Join(paths.Dot, ".gitignore"))
		if e != nil && !os.IsNotExist(e) {
			return 2, e
		}
		found := false
		for _, s := range strings.Fields(string(b)) {
			if s == ".state/" {
				found = true
			}
		}
		if !found {
			f, e := os.OpenFile(filepath.Join(paths.Dot, ".git", "info", "exclude"), os.O_APPEND|os.O_WRONLY, 0666)
			if e != nil {
				return 2, e
			}
			_, e = f.WriteString("\n.state/\n")
			ce := f.Close()
			if e == nil {
				e = ce
			}
			if e != nil {
				return 2, e
			}
		}
	} else {
		e := fs.WalkDir(dot.Example, "example", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel("example", path)
			dest := filepath.Join(paths.Dot, rel)
			if d.IsDir() {
				return os.MkdirAll(dest, 0755)
			}
			b, e := dot.Example.ReadFile(path)
			if e != nil {
				return e
			}
			return os.WriteFile(dest, b, 0644)
		})
		if e != nil {
			return 2, e
		}
		cmd := exec.Command("git", "init", "--quiet", paths.Dot)
		cmd.Stdout = out
		cmd.Stderr = stderr
		if e := cmd.Run(); e != nil {
			return 2, e
		}
		if e := os.WriteFile(filepath.Join(paths.Dot, ".gitignore"), []byte(".state/\n"), 0666); e != nil {
			return 2, e
		}
	}
	fmt.Fprintf(out, "Created %s. Next: edit %s/dot.toml, run dot apply, then dot timer.\n", paths.Show(paths.Dot), paths.Show(paths.Dot))
	return 0, nil
}

// Help prints usage and an ordered summary of values and mappings, even before init.
func Help(paths setup.Paths, out io.Writer) (int, error) {
	fmt.Fprintln(out, Usage)
	if !setup.Exists(filepath.Join(paths.Dot, "dot.toml")) {
		fmt.Fprintf(out, "\nNo setup at %s yet: run dot init.\n", paths.Show(paths.Dot))
		return 0, nil
	}
	c, e := config.Load(paths)
	if e != nil {
		return 2, e
	}
	fmt.Fprintf(out, "\nSetup %s on machine %s.\n", paths.Show(paths.Dot), paths.Machine)
	values := c.ValuesFor(paths.Machine)
	if len(c.Names) > 0 {
		fmt.Fprintln(out, "Values ({{name}} in templates, sources and destinations):")
	}
	for _, k := range c.Names {
		v, ok := values[k]
		if !ok {
			v = "(unset here)"
		}
		fmt.Fprintf(out, "  %s = %s\n", k, v)
		var over []string
		for _, m := range c.MachineNames {
			if v, ok := c.Machines[m][k]; ok {
				over = append(over, m+": "+v)
			}
		}
		if len(over) > 0 {
			base, ok := c.Values[k]
			if !ok {
				base = "unset"
			}
			fmt.Fprintf(out, "    set per machine (%s; base: %s): differs on purpose; do not unify\n", strings.Join(over, ", "), base)
		}
	}
	fmt.Fprintln(out, "Sources (edit these in the setup, not the destinations):")
	for _, mp := range c.Maps {
		var notes []string
		if mp.Template {
			notes = append(notes, "template")
		}
		if !mp.Mirror {
			notes = append(notes, "mirror off")
		}
		if mp.Machines != nil {
			notes = append(notes, "only on "+strings.Join(mp.Machines, ", "))
		}
		if mp.Run != "" {
			notes = append(notes, "then runs "+mp.Run)
		}
		suffix := ""
		if len(notes) > 0 {
			suffix = "  (" + strings.Join(notes, "; ") + ")"
		}
		fmt.Fprintf(out, "  %s -> %s%s\n", mp.Src, strings.Join(mp.To, ", "), suffix)
	}
	return 0, nil
}
