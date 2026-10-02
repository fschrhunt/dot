package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fschrhunt/dot/internal/apply"
	"github.com/fschrhunt/dot/internal/config"
	"github.com/fschrhunt/dot/internal/plan"
	"github.com/fschrhunt/dot/internal/setup"
)

// Agents prints each agent dot knows, whether it is installed on this machine, and where it
// keeps each kind of thing, so the table that decides a shared file's names can be read.
func Agents(c *config.Config, out io.Writer) (int, error) {
	for _, a := range c.Agents {
		state := "not installed"
		if a.Home == "" {
			state = "turned off"
		} else if a.Installed(c.Paths) {
			state = "installed"
		}
		fmt.Fprintf(out, "%-10s %s  (%s)\n", a.Name, a.Home, state)
		for _, kind := range plan.Keys(a.Kinds) {
			fmt.Fprintf(out, "  %-14s %s\n", kind, c.Paths.Show(a.Place(c.Paths, kind)))
		}
	}
	return 0, nil
}

// home finds where a live path belongs in a version 2 setup. A path that is an agent's place
// for a kind, or inside one, belongs under agents/ and is shared: live is then the whole unit,
// such as one skill's folder. Anything else under the home folder belongs under home/. only
// keeps an agent's path to that agent, by treating it as an ordinary home path.
func home(c *config.Config, p string, only bool) (source, live string, shared bool, err error) {
	for _, a := range c.Agents {
		for _, kind := range plan.Keys(a.Kinds) {
			place := a.Place(c.Paths, kind)
			if only || place == "" {
				continue
			}
			if p == place {
				return filepath.Join("agents", kind+filepath.Ext(p)), p, true, nil
			}
			if setup.Under(p, place) {
				rel, _ := filepath.Rel(place, p)
				unit, _, _ := strings.Cut(rel, "/")
				return filepath.Join("agents", kind, unit), filepath.Join(place, unit), true, nil
			}
		}
	}
	if !setup.Under(p, c.Paths.Home) {
		return "", "", false, setup.Fail(c.Paths.Show(p) + " is outside your home folder; map it under [files] in dot.toml")
	}
	rel, _ := filepath.Rel(c.Paths.Home, p)
	return filepath.Join("home", rel), p, false, nil
}

// copyIn copies a live file, link or folder into the setup, skipping excluded names. A file
// that looks like it holds a credential is left out and named on out: a single one is an error.
func copyIn(c *config.Config, live, source string, out io.Writer) error {
	secret := func(w setup.Want) bool { return w.Kind == "file" && plan.Secret(nil, w.Data) }
	if !setup.IsDir(live) || setup.IsLink(live) {
		w, e := setup.WantOf(live, nil)
		if e != nil {
			return e
		}
		if secret(w) {
			return setup.Fail("it looks like it holds a credential; copy it into the setup yourself if it belongs there")
		}
		return setup.Write(source, w)
	}
	return setup.Walk(live, c.Exclude, func(dir string, files []string) error {
		for _, name := range files {
			rel, _ := filepath.Rel(live, filepath.Join(dir, name))
			if setup.Excluded(rel, c.Exclude) {
				continue
			}
			w, e := setup.WantOf(filepath.Join(dir, name), nil)
			if e != nil {
				return e
			}
			if secret(w) {
				fmt.Fprintf(out, "left out %s: it looks like it holds a credential\n", c.Paths.Show(filepath.Join(dir, name)))
				continue
			}
			if e := setup.Write(filepath.Join(source, rel), w); e != nil {
				return e
			}
		}
		return nil
	})
}

// Add starts managing live paths: each is copied into the setup's home/ or agents/ folder, and
// the setup is applied so the path is recorded and a shared path reaches its other names. A
// path the setup already holds is left as it is; status says how the two differ.
func Add(paths setup.Paths, args []string, out, stderr io.Writer) (int, error) {
	only := slices.Contains(args, "--only")
	args = slices.DeleteFunc(slices.Clone(args), func(s string) bool { return s == "--only" })
	if len(args) == 0 {
		return 2, setup.Fail("usage: dot add [--only] <path>...")
	}
	c, e := config.Load(paths)
	if e != nil {
		return 2, e
	}
	if c.Version < 2 {
		return 2, setup.Fail("dot add needs a version 2 setup: set version = 2 in " + paths.Show(paths.Dot) + "/dot.toml")
	}
	want, _, _, e := plan.Wants(c)
	if e != nil {
		return 2, e
	}
	for _, arg := range args {
		p := paths.Abs(arg)
		if !setup.Exists(p) {
			return 2, setup.Fail(paths.Show(p) + " does not exist")
		}
		if _, managed := want[p]; managed {
			fmt.Fprintf(out, "%s is already managed\n", paths.Show(p))
			continue
		}
		if slices.ContainsFunc(c.Agents, func(a config.Agent) bool { return a.Home != "" && p == paths.Expand(a.Home) }) {
			return 2, setup.Fail(paths.Show(p) + " is an agent's whole folder, sessions and credentials included; add the files and skills you want from it")
		}
		source, live, shared, e := home(c, p, only)
		if e != nil {
			return 2, e
		}
		target := filepath.Join(paths.Dot, source)
		if setup.Exists(target) {
			fmt.Fprintf(out, "%s is already in the setup as %s; dot status shows how they differ\n", paths.Show(live), source)
			continue
		}
		if e := copyIn(c, live, target, out); e != nil {
			return 2, setup.Fail("cannot add " + paths.Show(live) + ": " + setup.Reason(e))
		}
		note := ""
		if shared {
			note = " (shared with every agent that has a place for it)"
		}
		fmt.Fprintf(out, "added %s as %s%s\n", paths.Show(live), source, note)
	}
	if c, e = config.Load(paths); e != nil {
		return 2, e
	}
	return Apply(c, false, false, out, stderr)
}

// Forget stops managing live paths: their sources leave the setup and dot forgets it wrote
// them, so the live files stay where they are. A path with several names is forgotten under
// all of them. A path mapped in dot.toml is not touched; its line is the user's to remove. A
// path inside a shared unit is refused, since the unit would only take it back.
func Forget(c *config.Config, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		return 2, setup.Fail("usage: dot forget <path>...")
	}
	want, roots, _, e := plan.Wants(c)
	if e != nil {
		return 2, e
	}
	maps, e := c.Resolve(c.Paths.Machine)
	if e != nil {
		return 2, e
	}
	// A source a line in dot.toml names stays, even when it sits under home/ or agents/.
	mapped := func(source string) bool {
		return slices.ContainsFunc(maps, func(r config.Resolved) bool {
			return strings.HasPrefix(r.Label, "[") && (r.Source == source || setup.Under(source, r.Source))
		})
	}
	var gone []string
	for _, arg := range args {
		p := c.Paths.Abs(arg)
		found := false
		for q, w := range want {
			if q != p && !setup.Under(q, p) {
				continue
			}
			found = true
			rel, e := filepath.Rel(c.Paths.Dot, w.Src)
			if e != nil || !strings.HasPrefix(rel, "home/") && !strings.HasPrefix(rel, "agents/") || mapped(w.Src) {
				return 2, setup.Fail(c.Paths.Show(p) + " is mapped in dot.toml; remove its line there")
			}
			if !slices.Contains(gone, w.Src) {
				gone = append(gone, w.Src)
			}
		}
		if !found {
			return 2, setup.Fail(c.Paths.Show(p) + " is not managed by dot")
		}
		for _, r := range roots {
			if setup.Under(p, r.Path) {
				return 2, setup.Fail(c.Paths.Show(p) + " is part of " + c.Paths.Show(r.Path) + ", which dot manages whole; forget that, or remove the file from " + c.Paths.Show(r.Source))
			}
		}
		fmt.Fprintf(out, "forgot %s; it stays where it is\n", c.Paths.Show(p))
	}
	var names []string
	for q, w := range want {
		if slices.Contains(gone, w.Src) {
			names = append(names, q)
		}
	}
	if e := apply.Disown(c.Paths, names); e != nil {
		return 2, e
	}
	for _, source := range gone {
		if e := os.RemoveAll(source); e != nil {
			return 2, e
		}
		// home/ and agents/ themselves stay: they are what makes the folder a setup.
		for dir := filepath.Dir(source); setup.Under(filepath.Dir(dir), c.Paths.Dot); dir = filepath.Dir(dir) {
			if os.Remove(dir) != nil {
				break
			}
		}
	}
	return 0, nil
}
