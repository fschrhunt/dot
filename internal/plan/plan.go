// Package plan derives the single ordered plan used by status and apply.
package plan

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fschrhunt/dot/internal/config"
	"github.com/fschrhunt/dot/internal/setup"
)

// Root records the policy and source for a directory destination.
type Root struct {
	Path, Source string
	Mirror       bool
	Exclude      []string
}

// Action describes a change; an empty Op is informational and must not mutate files.
type Action struct {
	Mark, Path, Op string
	Want           setup.Want
}

// Plan holds the ordered actions, desired paths, and directory roots for an invocation.
type Plan struct {
	Actions []Action
	Wants   map[string]setup.Want
	Roots   []Root
}

// Wants expands active mappings into desired files, symlinks and directories.
func Wants(c *config.Config) (map[string]setup.Want, []Root, error) {
	out := map[string]setup.Want{}
	var roots []Root
	maps, e := c.Resolve(c.Paths.Machine)
	if e != nil {
		return nil, nil, e
	}
	for _, r := range maps {
		excl := append(slices.Clone(c.Exclude), r.Exclude...)
		items := map[string]setup.Want{}
		if r.Template {
			b, e := os.ReadFile(r.Source)
			if e != nil {
				return nil, nil, e
			}
			text, e := config.Render(strings.ReplaceAll(strings.ReplaceAll(string(b), "\r\n", "\n"), "\r", "\n"), c.ValuesFor(c.Paths.Machine), c.Paths.Show(r.Source))
			if e != nil {
				return nil, nil, e
			}
			w, e := setup.WantOf(r.Source, []byte(text))
			if e != nil {
				return nil, nil, e
			}
			items["."] = w
		} else if setup.IsDir(r.Source) && !setup.IsLink(r.Source) {
			walkExclude := excl
			if setup.Excluded(".", excl) {
				walkExclude = []string{"*"}
			}
			e := setup.Walk(r.Source, walkExclude, func(dir string, files []string) error {
				rel, _ := filepath.Rel(r.Source, dir)
				w, e := setup.WantOf(dir, nil)
				if e != nil {
					return e
				}
				items[rel] = w
				for _, f := range files {
					q := filepath.Join(dir, f)
					rel, _ := filepath.Rel(r.Source, q)
					if setup.Excluded(".", excl) || setup.Excluded(rel, excl) {
						continue
					}
					w, e := setup.WantOf(q, nil)
					if e != nil {
						return e
					}
					items[rel] = w
				}
				return nil
			})
			if e != nil {
				return nil, nil, e
			}
			for _, d := range r.Dests {
				roots = append(roots, Root{d, r.Source, r.Mirror, excl})
			}
		} else {
			w, e := setup.WantOf(r.Source, nil)
			if e != nil {
				return nil, nil, e
			}
			items["."] = w
		}
		for _, dest := range r.Dests {
			for rel, w := range items {
				w.Template = r.Template
				out[filepath.Join(dest, rel)] = w
			}
		}
	}
	return out, roots, nil
}

// ViaLink detects a symlink between the deepest managed root and a destination's parent.
func ViaLink(p string, tops []string) bool {
	top := ""
	for _, t := range tops {
		if setup.Under(p, t) && len(t) > len(top) {
			top = t
		}
	}
	for d := filepath.Dir(p); top != "" && (d == top || setup.Under(d, top)); d = filepath.Dir(d) {
		if setup.IsLink(d) {
			return true
		}
	}
	return false
}

// Keys returns sorted path keys so plan and state ordering stay stable.
func Keys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// Build orders deletions before writes, protecting edited paths and honoring exclusions.
func Build(c *config.Config, written map[string]string) (Plan, error) {
	want, roots, e := Wants(c)
	p := Plan{Wants: want, Roots: roots}
	if e != nil {
		return p, e
	}
	var tops []string
	for _, r := range roots {
		tops = append(tops, r.Path)
	}
	for q, s := range written {
		if s == "dir" {
			tops = append(tops, q)
		}
	}
	for _, q := range Keys(written) {
		s := written[q]
		_, wanted := want[q]
		skip := false
		for _, r := range roots {
			if setup.Under(q, r.Path) {
				rel, _ := filepath.Rel(r.Path, q)
				skip = setup.Excluded(rel, r.Exclude)
				break
			}
		}
		if wanted || s == "dir" || ViaLink(q, tops) || skip {
			continue
		}
		live, e := setup.LiveSig(q)
		if e != nil {
			return p, e
		}
		if live != "" {
			mark := "!"
			if live == s {
				mark = "-"
			}
			p.Actions = append(p.Actions, Action{Mark: mark, Path: q, Op: "delete"})
		}
	}
	for _, r := range roots {
		if !setup.IsDir(r.Path) || setup.IsLink(r.Path) {
			continue
		}
		e := setup.Walk(r.Path, r.Exclude, func(dir string, files []string) error {
			slices.Sort(files)
			for _, f := range files {
				q := filepath.Join(dir, f)
				_, wanted := want[q]
				_, recorded := written[q]
				rel, _ := filepath.Rel(r.Path, q)
				if !wanted && !recorded && !setup.Excluded(rel, r.Exclude) {
					a := Action{Mark: "?", Path: q}
					if r.Mirror {
						a.Mark = "-"
						a.Op = "delete"
					}
					p.Actions = append(p.Actions, a)
				}
			}
			return nil
		})
		if e != nil {
			return p, e
		}
	}
	for _, q := range Keys(want) {
		w := want[q]
		live := ""
		if !ViaLink(q, tops) {
			live, e = setup.LiveSig(q)
			if e != nil {
				return p, e
			}
		}
		if live == w.Sig {
			continue
		}
		replace := live == written[q]
		if live == "dir" {
			for _, r := range roots {
				if setup.Under(q, r.Path) {
					replace = true
					break
				}
			}
		}
		mark := "!"
		if live == "" {
			mark = "+"
		} else if replace {
			mark = "~"
		}
		op := "write"
		if w.Kind == "dir" {
			op = "mkdir"
		}
		p.Actions = append(p.Actions, Action{mark, q, op, w})
	}
	return p, nil
}

// Print writes the exact plan labels, with a trailing slash for directory creations.
func Print(out io.Writer, paths setup.Paths, actions []Action) {
	labels := map[string]string{"+": "new", "~": "changed", "-": "removed", "!": "edited here", "?": "extra"}
	for _, a := range actions {
		suffix := ""
		if a.Op == "mkdir" {
			suffix = "/"
		}
		fmt.Fprintf(out, "%s %-11s %s%s\n", a.Mark, labels[a.Mark], paths.Show(a.Path), suffix)
	}
}

// EditedNote tells the user how to resolve a refused write or deletion.
func EditedNote(paths setup.Paths, p, op string) string {
	q := paths.Show(p)
	if op == "delete" {
		return "edited here: " + q + " (no longer in the setup; delete it, or dot apply --force)"
	}
	return "edited here: " + q + " (dot take " + q + ", or dot apply --force)"
}
