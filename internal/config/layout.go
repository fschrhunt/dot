package config

import (
	"maps"
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
			} else if value != "" && !filepath.IsLocal(value) {
				return nil, where + " " + key + " must be a path inside the agent's folder"
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

// checkOnly rejects a rule that could not mean what it says: a path outside the two folders,
// agents on a path that is not shared, a path deeper than one shared unit, or an agent dot does
// not know. A rule that matched nothing would otherwise drop a mapping without a word.
func checkOnly(rules map[string]only, agents []Agent) string {
	for _, path := range slices.Sorted(maps.Keys(rules)) {
		where := "[only] \"" + path + "\""
		parts := strings.Split(path, "/")
		switch {
		case parts[0] != "home" && parts[0] != "agents":
			return where + " must be a path under home/ or agents/"
		case parts[0] == "home" && rules[path].agents != nil:
			return where + ": agents applies only to a path under agents/"
		case parts[0] == "agents" && len(parts) > 3:
			return where + " reaches inside a shared unit; a rule stops at the unit, such as agents/skills/<name>"
		}
		for _, name := range rules[path].agents {
			if !slices.ContainsFunc(agents, func(a Agent) bool { return a.Name == name }) {
				return where + ": no agent is named " + name + " (dot agents lists them)"
			}
		}
	}
	return ""
}

// ruleFor finds what limits a path in the setup: its agents from the nearest rule that names
// agents, at the path or a folder above it, and its machines from the nearest that names
// machines. A rule on a skill therefore keeps the machines a rule on its folder set.
func ruleFor(rules map[string]only, path string) only {
	var out only
	for p := path; p != "." && p != "/"; p = filepath.Dir(p) {
		if out.agents == nil {
			out.agents = rules[p].agents
		}
		if out.machines == nil {
			out.machines = rules[p].machines
		}
	}
	return out
}

// discover turns the setup's folders into mappings, the same kind [files] and [templates] make.
// home/X is written to ~/X, one mapping per file, so dot owns those files and nothing else in
// the folders around them. An entry of agents/ is written to every installed agent that has a
// place for its kind: a file as it is, and each child of a folder as a mirrored unit of its own.
// Two agents whose places are the same real path, one being a link to the other, get one copy.
// A name ending in .tmpl is a template and loses the suffix where it is written; files inside a
// unit are copied as they are, whatever their names. An excluded file or entry makes no mapping,
// and where it would have been written is recorded in Kept.
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
			short, template := plain(strings.TrimPrefix(rel, "home/"))
			if setup.Excluded(rel, c.Exclude) {
				c.Kept = append(c.Kept, filepath.Join(c.Paths.Home, short))
				continue
			}
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
		if strings.HasPrefix(name, ".") {
			continue
		}
		left := setup.Excluded(name, c.Exclude)
		kind, _, _ := strings.Cut(name, ".")
		if !left && !slices.ContainsFunc(c.Agents, func(a Agent) bool { return a.Kinds[kind] != "" }) {
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
				units = append(units, child.Name())
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
			seen := map[string]bool{}
			for _, a := range c.Agents {
				place := a.Place(c.Paths, kind)
				if place == "" || !a.Installed(c.Paths) || rule.agents != nil && !slices.Contains(rule.agents, a.Name) {
					continue
				}
				to := filepath.Join(place, short)
				if real := setup.Real(to); !seen[real] {
					seen[real] = true
					m.To = append(m.To, to)
				}
			}
			if left || unit != "" && setup.Excluded(unit, c.Exclude) {
				c.Kept = append(c.Kept, m.To...)
			} else if len(m.To) > 0 {
				c.Maps = append(c.Maps, m)
			}
		}
	}
	return "", nil
}
