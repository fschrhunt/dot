package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fschrhunt/dot/internal/setup"
)

// Agent is one coding agent: the folder it lives in and where, inside that folder, it keeps each
// kind of thing dot can share. A kind is only a name, such as instructions or skills; an entry
// under agents/ in the setup with that name is written to every agent that has a place for it.
type Agent struct {
	Name, Home string
	Kinds      map[string]string
}

// Builtin lists the agents dot knows without being told, in the order it reports them. Each
// place was checked against a real install. dot.toml's [agent.<name>] tables add to this list
// and override it, so a wrong or missing entry is the user's to fix without a new dot.
func Builtin() []Agent {
	return []Agent{
		{"claude", "~/.claude", map[string]string{"instructions": "CLAUDE.md", "skills": "skills"}},
		{"codex", "~/.codex", map[string]string{"instructions": "AGENTS.md", "skills": "skills"}},
		{"opencode", "~/.config/opencode", map[string]string{"instructions": "AGENTS.md", "skills": "skills"}},
		{"pi", "~/.pi/agent", map[string]string{"instructions": "AGENTS.md", "skills": "skills"}},
		{"cursor", "~/.cursor", map[string]string{"skills": "skills"}},
	}
}

// Installed reports whether the agent's folder exists on this machine. dot writes an agent's
// places only then, and never creates the folder itself.
func (a Agent) Installed(paths setup.Paths) bool {
	return a.Home != "" && setup.IsDir(paths.Expand(a.Home))
}

// Place is where the agent keeps kind, or "" when it has no place for it.
func (a Agent) Place(paths setup.Paths, kind string) string {
	if a.Kinds[kind] == "" || a.Home == "" {
		return ""
	}
	return filepath.Join(paths.Expand(a.Home), a.Kinds[kind])
}

// only limits a path in the setup, and everything under it, to some agents or some machines.
type only struct{ agents, machines []string }

// agentsFrom lays dot.toml's [agent.<name>] tables over the built-in agents. A table changes the
// agent of that name or adds a new one, which needs a home; an empty home turns an agent off.
func agentsFrom(raw map[string]any, md toml.MetaData) ([]Agent, string) {
	agents := Builtin()
	tables, ok := table(raw["agent"])
	if !ok {
		return nil, "agent must be a table of [agent.<name>] tables"
	}
	for _, name := range orderedKeys(md, "agent") {
		where := "[agent." + name + "]"
		t, ok := tables[name].(map[string]any)
		if !ok {
			return nil, where + " must be a table"
		}
		i := slices.IndexFunc(agents, func(a Agent) bool { return a.Name == name })
		if i < 0 {
			agents = append(agents, Agent{Name: name, Kinds: map[string]string{}})
			i = len(agents) - 1
		}
		for _, key := range orderedKeys(md, "agent", name) {
			value, ok := t[key].(string)
			if !ok {
				return nil, where + " " + key + " must be a path in quotes"
			}
			if key == "home" {
				agents[i].Home = value
			} else if value == "" {
				delete(agents[i].Kinds, key)
			} else {
				agents[i].Kinds[key] = value
			}
		}
		if _, set := t["home"]; !set && agents[i].Home == "" {
			return nil, where + " needs home, the folder the agent lives in"
		}
	}
	return agents, ""
}

// onlyFrom reads the [only] table: a path in the setup, then the agents or machines it is for.
func onlyFrom(raw map[string]any, md toml.MetaData) (map[string]only, string) {
	rules := map[string]only{}
	tables, ok := table(raw["only"])
	if !ok {
		return nil, "[only] must be a table"
	}
	for _, path := range orderedKeys(md, "only") {
		where := "[only] \"" + path + "\""
		t, ok := tables[path].(map[string]any)
		if !ok {
			return nil, where + " must be a table such as { agents = [\"claude\"] }"
		}
		var rule only
		for _, key := range orderedKeys(md, "only", path) {
			list, ok := stringsOf(t[key])
			if !ok {
				return nil, where + ": " + key + " must be a list of strings"
			}
			switch key {
			case "agents":
				rule.agents = list
			case "machines":
				rule.machines = list
			default:
				return nil, where + ": unknown key \"" + key + "\""
			}
		}
		rules[filepath.Clean(path)] = rule
	}
	return rules, ""
}

// ruleFor finds the rule for a path in the setup: its own, or the nearest folder's above it.
func ruleFor(rules map[string]only, path string) only {
	for p := path; p != "." && p != "/"; p = filepath.Dir(p) {
		if rule, ok := rules[p]; ok {
			return rule
		}
	}
	return only{}
}

// discover turns the setup's folders into mappings, the same kind [files] and [templates] make.
// home/X is written to ~/X, one mapping per file, so dot owns those files and nothing else in
// the folders around them. An entry of agents/ is written to every installed agent that has a
// place for its kind: a file as it is, and each child of a folder as a mirrored unit of its own.
// A name ending in .tmpl is a template and loses the suffix where it is written.
func (c *Config) discover(rules map[string]only) (string, error) {
	plain := func(name string) (string, bool) {
		short, is := strings.CutSuffix(name, ".tmpl")
		return short, is
	}
	home := filepath.Join(c.Paths.Dot, "home")
	e := setup.Walk(home, c.Exclude, func(dir string, files []string) error {
		slices.Sort(files)
		for _, name := range files {
			rel, _ := filepath.Rel(c.Paths.Dot, filepath.Join(dir, name))
			if setup.Excluded(rel, c.Exclude) {
				continue
			}
			short, template := plain(strings.TrimPrefix(rel, "home/"))
			c.Maps = append(c.Maps, Mapping{Label: rel, Src: rel, To: []string{filepath.Join(c.Paths.Home, short)}, Template: template, Mirror: true, Machines: ruleFor(rules, rel).machines})
		}
		return nil
	})
	if e != nil {
		return "", e
	}
	shared := filepath.Join(c.Paths.Dot, "agents")
	entries, e := os.ReadDir(shared)
	if e != nil && !os.IsNotExist(e) {
		return "", e
	}
	for _, entry := range entries {
		name := entry.Name()
		if setup.Excluded(name, c.Exclude) {
			continue
		}
		kind, _, _ := strings.Cut(name, ".")
		if !slices.ContainsFunc(c.Agents, func(a Agent) bool { return a.Kinds[kind] != "" }) {
			return "agents/" + name + ": no agent has a place for " + kind + " (dot agents lists them; [agent.<name>] adds one)", nil
		}
		units := []string{""}
		if entry.IsDir() {
			children, e := os.ReadDir(filepath.Join(shared, name))
			if e != nil {
				return "", e
			}
			units = nil
			for _, child := range children {
				if !setup.Excluded(child.Name(), c.Exclude) {
					units = append(units, child.Name())
				}
			}
		}
		for _, unit := range units {
			rel := filepath.Join("agents", name, unit)
			rule := ruleFor(rules, rel)
			short, template := plain(unit)
			m := Mapping{Label: rel, Src: rel, Template: template && unit != "", Mirror: true, Machines: rule.machines}
			if unit == "" {
				_, m.Template = plain(name)
			}
			for _, a := range c.Agents {
				place := a.Place(c.Paths, kind)
				if place == "" || !a.Installed(c.Paths) || rule.agents != nil && !slices.Contains(rule.agents, a.Name) {
					continue
				}
				m.To = append(m.To, filepath.Join(place, short))
			}
			if len(m.To) > 0 {
				c.Maps = append(c.Maps, m)
			}
		}
	}
	return "", nil
}
