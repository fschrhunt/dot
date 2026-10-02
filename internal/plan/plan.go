// Package plan derives the single ordered plan used by status and apply.
package plan

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

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
// A take copies the live file at Path into the setup at Want.Src. A run action's Path is its
// command. Note says why a refused action was refused. Live is the path's signature when the
// plan looked at it, kept only when Looked, so apply can refuse a path that changed since.
type Action struct {
	Mark, Path, Op string
	Want           setup.Want
	Note, Live     string
	Looked         bool
}

// Options says how far a plan may go. With Take, an edit to a live file whose source has not
// changed is planned as a take into the setup, and a new file in a mirrored folder as an
// addition; without it the plan is one-way, as apply needs. Settle holds back a file modified
// more recently than that, so the timer does not take a file still being written.
type Options struct {
	Take   bool
	Settle time.Duration
}

// Hook is a mapping's run command and the destinations whose changes trigger it.
type Hook struct {
	Command string
	Dests   []string
}

// Plan holds the ordered actions, desired paths, directory roots, and hooks for an invocation.
type Plan struct {
	Actions []Action
	Wants   map[string]setup.Want
	Roots   []Root
	Hooks   []Hook
}

// Touched reports whether any of actions writes or removes something at or under dests.
func Touched(dests []string, actions []Action) bool {
	for _, a := range actions {
		for _, d := range dests {
			if a.Op != "" && a.Op != "run" && a.Op != "take" && (a.Path == d || setup.Under(a.Path, d)) {
				return true
			}
		}
	}
	return false
}

// Wants expands active mappings into desired files, symlinks and directories, and lists the
// mappings' run commands in configuration order.
func Wants(c *config.Config) (map[string]setup.Want, []Root, []Hook, error) {
	out := map[string]setup.Want{}
	var roots []Root
	var hooks []Hook
	maps, e := c.Resolve(c.Paths.Machine)
	if e != nil {
		return nil, nil, nil, e
	}
	for _, r := range maps {
		if r.Run != "" {
			hooks = append(hooks, Hook{r.Run, r.Dests})
		}
		excl := append(slices.Clone(c.Exclude), r.Exclude...)
		items := map[string]setup.Want{}
		if r.Template {
			b, e := os.ReadFile(r.Source)
			if e != nil {
				return nil, nil, nil, e
			}
			text, e := config.Render(strings.ReplaceAll(strings.ReplaceAll(string(b), "\r\n", "\n"), "\r", "\n"), c.ValuesFor(c.Paths.Machine), c.Paths.Show(r.Source))
			if e != nil {
				return nil, nil, nil, e
			}
			w, e := setup.WantOf(r.Source, []byte(text))
			if e != nil {
				return nil, nil, nil, e
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
				return nil, nil, nil, e
			}
			for _, d := range r.Dests {
				roots = append(roots, Root{d, r.Source, r.Mirror, excl})
			}
		} else {
			w, e := setup.WantOf(r.Source, nil)
			if e != nil {
				return nil, nil, nil, e
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
	return out, roots, hooks, nil
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

// credentials are the shapes dot will not take into a setup on its own: private key blocks and
// the common token prefixes. The check is best effort; it is a seat belt, not a scanner.
var credentials = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----|\b(?:sk|rk)-[A-Za-z0-9_-]{20,}|\bsk_(?:live|test)_[A-Za-z0-9]{16,}|\bgh[pousr]_[A-Za-z0-9]{30,}|\bgithub_pat_[A-Za-z0-9_]{30,}|\bAKIA[0-9A-Z]{16}\b|\bxox[baprs]-[A-Za-z0-9-]{10,}|\bAIza[0-9A-Za-z_-]{35}\b|\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)

// Secret reports whether edited adds a line that looks like a credential and that base lacks.
func Secret(base, edited []byte) bool {
	known := map[string]bool{}
	for _, line := range bytes.Split(base, []byte("\n")) {
		known[string(line)] = true
	}
	for _, line := range bytes.Split(edited, []byte("\n")) {
		if !known[string(line)] && credentials.Match(line) {
			return true
		}
	}
	return false
}

// found is a live regular file a take could copy into the setup.
type found struct {
	path, sig string
	data      []byte
	mode      os.FileMode
	fresh     bool
}

// look reads a live path for a take. ok is false for anything but a regular file.
func look(q string, settle time.Duration) (found, bool, error) {
	i, e := os.Lstat(q)
	if e != nil || !i.Mode().IsRegular() {
		return found{}, false, nil
	}
	data, e := os.ReadFile(q)
	if e != nil {
		return found{}, false, e
	}
	mode := i.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	return found{q, setup.Hash(data), data, mode, settle > 0 && time.Since(i.ModTime()) < settle}, true, nil
}

// take decides one group of live files that would all become the same source file. They are
// taken when they agree, have settled, and add nothing that looks like a credential; then every
// destination of that source wants the taken bytes. Otherwise each is held back: quietly when it
// is only unsettled, and as a refused take with its reason when it needs the user.
func take(p *Plan, src string, base []byte, group []found, dests []string, held map[string]bool) {
	first := group[0]
	note := ""
	if slices.ContainsFunc(group, func(f found) bool { return f.sig != first.sig }) {
		note = "edited differently under another of its names"
	} else if Secret(base, first.data) {
		note = "looks like a credential"
	}
	for _, f := range group {
		if note != "" {
			held[f.path] = true
			p.Actions = append(p.Actions, Action{Mark: "!", Path: f.path, Op: "take", Note: note})
		} else if f.fresh {
			held[f.path] = true
		}
	}
	if note != "" || slices.ContainsFunc(group, func(f found) bool { return f.fresh }) {
		return
	}
	w := setup.Want{Kind: "file", Data: first.data, Mode: first.mode, Sig: first.sig, Src: src}
	p.Actions = append(p.Actions, Action{Mark: "<", Path: first.path, Op: "take", Want: w})
	for _, d := range dests {
		if old, ok := p.Wants[d]; ok {
			w.Mode = old.Mode
		}
		p.Wants[d] = w
	}
}

// Build orders takes, then deletions, then writes, protecting edited paths and honoring
// exclusions. opt says whether live edits may be taken; see Options.
func Build(c *config.Config, written map[string]string, opt Options) (Plan, error) {
	want, roots, hooks, e := Wants(c)
	p := Plan{Wants: want, Roots: roots, Hooks: hooks}
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
	// held are live paths this plan leaves alone: an edit or addition that is not taken yet.
	held := map[string]bool{}
	if opt.Take {
		// An edit: the live file differs from what dot wrote, and the source still is what dot wrote.
		bySource := map[string][]string{}
		for _, q := range Keys(want) {
			bySource[want[q].Src] = append(bySource[want[q].Src], q)
		}
		for _, src := range Keys(bySource) {
			var group []found
			for _, q := range bySource[src] {
				w := want[q]
				if w.Kind != "file" || w.Template || written[q] != w.Sig || ViaLink(q, tops) {
					continue
				}
				f, ok, e := look(q, opt.Settle)
				if e != nil {
					return p, e
				}
				if ok && f.sig != w.Sig {
					group = append(group, f)
				}
			}
			if len(group) > 0 {
				take(&p, src, want[bySource[src][0]].Data, group, bySource[src], held)
			}
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
			p.Actions = append(p.Actions, Action{Mark: mark, Path: q, Op: "delete", Live: live, Looked: true})
		}
	}
	// A file in a mirrored folder that dot neither wants nor wrote is an addition when taking,
	// and otherwise an extra to remove. Additions are gathered by the source file they would
	// become, since several destinations of one folder can each hold the new file.
	type addition struct {
		source string
		files  []found
		dests  []string
	}
	var additions []*addition
	bySource := map[string]*addition{}
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
				if wanted || recorded || setup.Excluded(rel, r.Exclude) {
					continue
				}
				if !r.Mirror {
					p.Actions = append(p.Actions, Action{Mark: "?", Path: q})
					continue
				}
				if !opt.Take {
					live, e := setup.LiveSig(q)
					if e != nil {
						return e
					}
					p.Actions = append(p.Actions, Action{Mark: "-", Path: q, Op: "delete", Live: live, Looked: true})
					continue
				}
				file, ok, e := look(q, opt.Settle)
				if e != nil {
					return e
				}
				if !ok {
					held[q] = true
					p.Actions = append(p.Actions, Action{Mark: "!", Path: q, Op: "take", Note: "not a regular file"})
					continue
				}
				source := filepath.Join(r.Source, rel)
				a := bySource[source]
				if a == nil {
					a = &addition{source: source}
					bySource[source] = a
					additions = append(additions, a)
					for _, other := range roots {
						if other.Source == r.Source {
							a.dests = append(a.dests, filepath.Join(other.Path, rel))
						}
					}
				}
				a.files = append(a.files, file)
			}
			return nil
		})
		if e != nil {
			return p, e
		}
	}
	for _, a := range additions {
		take(&p, a.source, nil, a.files, a.dests, held)
	}
	for _, q := range Keys(p.Wants) {
		w := p.Wants[q]
		if held[q] {
			continue
		}
		live := ""
		looked := !ViaLink(q, tops)
		if looked {
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
		p.Actions = append(p.Actions, Action{Mark: mark, Path: q, Op: op, Want: w, Live: live, Looked: looked})
	}
	// Name each command a plain apply would run: one whose mapping has a change that is not an
	// edit apply refuses. These lines are informational; apply decides from what it really did.
	var plain []Action
	for _, a := range p.Actions {
		if a.Mark != "!" {
			plain = append(plain, a)
		}
	}
	for _, h := range hooks {
		if Touched(h.Dests, plain) {
			p.Actions = append(p.Actions, Action{Mark: ">", Path: h.Command})
		}
	}
	return p, nil
}

// Print writes the exact plan labels, with a trailing slash for directory creations.
func Print(out io.Writer, paths setup.Paths, actions []Action) {
	labels := map[string]string{"+": "new", "~": "changed", "-": "removed", "!": "edited here", "?": "extra", ">": "run", "<": "take"}
	for _, a := range actions {
		suffix := ""
		if a.Op == "mkdir" {
			suffix = "/"
		}
		shown := paths.Show(a.Path)
		if a.Mark == ">" {
			shown = a.Path
		}
		fmt.Fprintf(out, "%s %-11s %s%s\n", a.Mark, labels[a.Mark], shown, suffix)
	}
}

// Refusal tells the user how to resolve a refused write or deletion, or why a command failed.
func Refusal(paths setup.Paths, a Action) string {
	if a.Op == "run" {
		return "run failed: " + a.Path + " (" + a.Note + ")"
	}
	q, op := paths.Show(a.Path), a.Op
	if op == "take" {
		return "not taken: " + q + " (" + a.Note + "; dot take " + q + " takes it anyway)"
	}
	if a.Note != "" {
		return "edited here: " + q + " (" + a.Note + ")"
	}
	if op == "delete" {
		return "edited here: " + q + " (no longer in the setup; delete it, or dot apply --force)"
	}
	return "edited here: " + q + " (dot take " + q + ", or dot apply --force)"
}
