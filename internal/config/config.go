// Package config loads dot.toml, renders placeholders, and validates every known machine.
package config

import (
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/fschrhunt/dot/internal/setup"
)

// Mapping is one source's destinations and copying policy, in configuration order.
// Run is a shell command for apply to run after it changes anything under the destinations.
type Mapping struct {
	Label, Src, Run       string
	To, Exclude, Machines []string
	Template, Mirror      bool
}

// Sync holds this machine's [sync] settings, after its [sync.machine.<name>] overrides.
// Take makes sync two-way: an edit to a live file is taken into the setup and committed. Push
// sends local commits to the remote. Every is the timer's interval and AfterBoot the wait before
// its first run after boot on Linux. Timeout stops each git command and each mapping's run
// command; ConnectTimeout is given to ssh. Durations are whole seconds.
type Sync struct {
	Take, Push                                bool
	Every, AfterBoot, Timeout, ConnectTimeout time.Duration
}

// DefaultSync returns the settings dot uses when dot.toml sets none. A version 2 setup syncs
// both ways and pushes; a version 1 setup keeps the one-way sync it was written for.
func DefaultSync(version int64) Sync {
	return Sync{Take: version >= 2, Push: version >= 2, Every: 15 * time.Minute, AfterBoot: 2 * time.Minute, Timeout: time.Minute, ConnectTimeout: 5 * time.Second}
}

// Latest is the newest dot.toml version this dot reads.
const Latest = 2

// versionOf reads dot.toml's version, which defaults to 1, and returns the problem with a
// value that is not a whole number or is newer than this dot.
func versionOf(raw map[string]any) (int64, string) {
	v, exists := raw["version"]
	if !exists {
		return 1, ""
	}
	if n, ok := v.(integer); ok {
		if !strings.HasPrefix(string(n), "-") {
			return 0, "version " + string(n) + " is newer than this dot; update dot"
		}
		return 0, ""
	}
	version, ok := v.(int64)
	if !ok {
		return 0, "version must be a whole number"
	}
	if version > Latest {
		return 0, fmt.Sprintf("version %d is newer than this dot; update dot", version)
	}
	return version, ""
}

// Config is a validated setup; Names and MachineNames preserve TOML order for help output.
type Config struct {
	Paths   setup.Paths
	Version int64
	Exclude []string
	Agents  []Agent
	Sync
	Values              map[string]string
	Machines            map[string]map[string]string
	Names, MachineNames []string
	Maps                []Mapping
}

// Resolved is a mapping with rendered paths for one machine.
type Resolved struct {
	Mapping
	Source string
	Dests  []string
}

var placeholder = regexp.MustCompile(`\{\{[\s\p{Z}\x{000b}\x{001c}-\x{001f}\x{0085}]*([\pL\pN_]+)[\s\p{Z}\x{000b}\x{001c}-\x{001f}\x{0085}]*\}\}`)

// Render replaces {{name}} and reports the first undefined name with its context.
func Render(text string, values map[string]string, where string) (string, error) {
	var err error
	result := placeholder.ReplaceAllStringFunc(text, func(s string) string {
		key := placeholder.FindStringSubmatch(s)[1]
		v, ok := values[key]
		if !ok && err == nil {
			err = setup.Fail(fmt.Sprintf("%s: undefined name %q", where, key))
		}
		return v
	})
	return result, err
}

// orderedKeys returns immediate keys in the insertion order exposed by the TOML parser.
func orderedKeys(md toml.MetaData, prefix ...string) []string {
	var out []string
	for _, k := range md.Keys() {
		if len(k) <= len(prefix) {
			continue
		}
		match := true
		for i, p := range prefix {
			if k[i] != p {
				match = false
				break
			}
		}
		if match && !slices.Contains(out, k[len(prefix)]) {
			out = append(out, k[len(prefix)])
		}
	}
	return out
}

// table converts a TOML table and accepts a missing field as an empty table.
func table(v any) (map[string]any, bool) {
	if v == nil {
		return map[string]any{}, true
	}
	t, ok := v.(map[string]any)
	return t, ok
}

// stringsOf accepts only TOML arrays of strings, including an empty array.
func stringsOf(v any) ([]string, bool) {
	if v == nil {
		return []string{}, true
	}
	a, ok := v.([]any)
	if !ok {
		return nil, false
	}
	s := []string{}
	for _, v := range a {
		x, ok := v.(string)
		if !ok {
			return nil, false
		}
		s = append(s, x)
	}
	return s, true
}

// scalars converts strings and numbers to Python-compatible placeholder text.
func scalars(v any) (map[string]string, bool) {
	t, ok := table(v)
	if !ok {
		return nil, false
	}
	out := map[string]string{}
	for k, v := range t {
		switch x := v.(type) {
		case string:
			out[k] = x
		case int64:
			out[k] = strconv.FormatInt(x, 10)
		case integer:
			out[k] = string(x)
		case float64:
			format := byte('f')
			if x != 0 && (math.Abs(x) < 1e-4 || math.Abs(x) >= 1e16) {
				format = 'e'
			}
			s := strconv.FormatFloat(x, format, -1, 64)
			if !strings.ContainsAny(s, ".e") && s != "NaN" && s != "+Inf" && s != "-Inf" {
				s += ".0"
			}
			s = strings.ReplaceAll(s, "NaN", "nan")
			s = strings.ReplaceAll(s, "+Inf", "inf")
			s = strings.ReplaceAll(s, "-Inf", "-inf")
			out[k] = s
		default:
			return nil, false
		}
	}
	return out, true
}

// setSync applies one table of sync settings to s and returns the first bad value's problem.
func setSync(s *Sync, t map[string]any, where string) string {
	for _, f := range []struct {
		key string
		to  *bool
	}{{"push", &s.Push}, {"take", &s.Take}} {
		if v, exists := t[f.key]; exists {
			on, ok := v.(bool)
			if !ok {
				return where + " " + f.key + " must be true or false"
			}
			*f.to = on
		}
	}
	for _, f := range []struct {
		key  string
		to   *time.Duration
		zero bool
	}{{"every", &s.Every, false}, {"after_boot", &s.AfterBoot, true}, {"timeout", &s.Timeout, false}, {"connect_timeout", &s.ConnectTimeout, false}} {
		v, exists := t[f.key]
		if !exists {
			continue
		}
		text, _ := v.(string)
		d, e := time.ParseDuration(text)
		if e != nil || d < 0 || d == 0 && !f.zero || d%time.Second != 0 {
			return where + " " + f.key + " must be whole seconds, minutes or hours, such as \"90s\", \"5m\" or \"1h\""
		}
		*f.to = d
	}
	return ""
}

// syncFor reads [sync] and then machine's [sync.machine.<name>] table over it. Every machine's
// table is checked, so a mistake in one shows on all of them.
func syncFor(raw map[string]any, machine string, version int64) (Sync, string) {
	base := DefaultSync(version)
	t, ok := table(raw["sync"])
	if !ok {
		return base, "[sync] push must be true or false"
	}
	if problem := setSync(&base, t, "[sync]"); problem != "" {
		return base, problem
	}
	machines, ok := table(t["machine"])
	if !ok {
		return base, "[sync.machine] must hold [sync.machine.<name>] tables"
	}
	out := base
	for _, name := range slices.Sorted(maps.Keys(machines)) {
		where := "[sync.machine." + name + "]"
		over, ok := machines[name].(map[string]any)
		if !ok {
			return base, where + " must be a table"
		}
		s := base
		if problem := setSync(&s, over, where); problem != "" {
			return base, problem
		}
		if name == machine {
			out = s
		}
	}
	return out, ""
}

// SyncSettings reads only this machine's sync settings, without validating the rest of the
// setup. sync uses it before pulling, when dot.toml may be about to change; a file it cannot
// read yields the defaults.
func SyncSettings(paths setup.Paths) Sync {
	var raw map[string]any
	source, e := source(paths)
	if e == nil {
		_, e = decode(source, &raw)
	}
	if e != nil {
		return DefaultSync(1)
	}
	version, problem := versionOf(raw)
	if problem != "" {
		return DefaultSync(1)
	}
	s, problem := syncFor(raw, paths.Machine, version)
	if problem != "" {
		return DefaultSync(version)
	}
	return s
}

// source returns dot.toml's text. The file is optional once a setup has a home/ or agents/
// folder: its layout is then the configuration, and it reads as a version 2 setup.
func source(paths setup.Paths) (string, error) {
	b, e := os.ReadFile(filepath.Join(paths.Dot, "dot.toml"))
	if os.IsNotExist(e) && (setup.IsDir(filepath.Join(paths.Dot, "home")) || setup.IsDir(filepath.Join(paths.Dot, "agents"))) {
		return "version = 2\n", nil
	}
	if os.IsNotExist(e) {
		return "", setup.Fail("no setup at " + paths.Show(paths.Dot) + " (run dot init)")
	}
	return string(b), e
}

// Load parses and validates all accepted dot.toml keys, including machines not running here,
// and adds the mappings a version 2 setup's home/ and agents/ folders stand for.
func Load(paths setup.Paths) (*Config, error) {
	source, err := source(paths)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	md, err := decode(source, &raw)
	if err != nil {
		return nil, setup.Fail("dot.toml: " + syntaxError(source, err))
	}
	bad := func(s string) (*Config, error) { return nil, setup.Fail("dot.toml: " + s) }
	version, problem := versionOf(raw)
	if problem != "" {
		return bad(problem)
	}
	c := &Config{Paths: paths, Version: version, Machines: map[string]map[string]string{}}
	var ok bool
	c.Exclude, ok = stringsOf(raw["exclude"])
	if !ok {
		return bad("exclude must be a list of strings")
	}
	if c.Sync, problem = syncFor(raw, paths.Machine, version); problem != "" {
		return bad(problem)
	}
	c.Values, ok = scalars(raw["values"])
	if !ok {
		return bad("[values] must be strings or numbers")
	}
	c.Names = orderedKeys(md, "values")
	machines, ok := table(raw["machine"])
	if !ok {
		return bad("machine must be a table of [machine.<name>] tables")
	}
	c.MachineNames = orderedKeys(md, "machine")
	for _, m := range c.MachineNames {
		values, ok := scalars(machines[m])
		if !ok {
			return bad("[machine." + m + "] values must be strings or numbers")
		}
		c.Machines[m] = values
		for _, k := range orderedKeys(md, "machine", m) {
			if !slices.Contains(c.Names, k) {
				c.Names = append(c.Names, k)
			}
		}
	}
	for _, section := range []string{"templates", "files"} {
		t, ok := table(raw[section])
		if !ok {
			return bad("[" + section + "] must be a table")
		}
		for _, src := range orderedKeys(md, section) {
			val := t[src]
			mp := Mapping{Label: fmt.Sprintf("[%s] \"%s\"", section, src), Src: src, Template: section == "templates", Mirror: true}
			if opt, yes := val.(map[string]any); yes {
				for _, k := range orderedKeys(md, section, src) {
					if !slices.Contains([]string{"to", "mirror", "exclude", "machines", "run"}, k) {
						return bad(mp.Label + ": unknown key \"" + k + "\"")
					}
				}
				if v, exists := opt["mirror"]; exists {
					mp.Mirror, ok = v.(bool)
					if !ok {
						return bad(mp.Label + ": mirror must be true or false")
					}
				}
				mp.Exclude, ok = stringsOf(opt["exclude"])
				if !ok {
					return bad(mp.Label + ": exclude must be a list of strings")
				}
				if v, exists := opt["machines"]; exists {
					mp.Machines, ok = stringsOf(v)
					if !ok {
						return bad(mp.Label + ": machines must be a list of strings")
					}
				}
				if v, exists := opt["run"]; exists {
					mp.Run, ok = v.(string)
					if !ok || strings.TrimSpace(mp.Run) == "" {
						return bad(mp.Label + ": run must be a command")
					}
				}
				val = opt["to"]
			}
			if s, yes := val.(string); yes {
				mp.To = []string{s}
			} else {
				mp.To, ok = stringsOf(val)
				if !ok || len(mp.To) == 0 {
					return bad(mp.Label + ": destination must be a string or a list of strings")
				}
			}
			c.Maps = append(c.Maps, mp)
		}
	}
	if c.Agents, problem = agentsFrom(raw, md); problem != "" {
		return bad(problem)
	}
	if version >= 2 {
		rules, problem := onlyFrom(raw, md)
		if problem != "" {
			return bad(problem)
		}
		problem, e := c.discover(rules)
		if e != nil {
			return nil, e
		}
		if problem != "" {
			return nil, setup.Fail(problem)
		}
	}
	return c, c.validate()
}

// ValuesFor combines base values, machine overrides, and the reserved machine placeholder.
func (c *Config) ValuesFor(machine string) map[string]string {
	out := map[string]string{}
	for k, v := range c.Values {
		out[k] = v
	}
	for k, v := range c.Machines[machine] {
		out[k] = v
	}
	out["machine"] = machine
	return out
}

// Resolve renders the active mappings on machine, requiring absolute destinations.
func (c *Config) Resolve(machine string) ([]Resolved, error) {
	var out []Resolved
	values := c.ValuesFor(machine)
	for _, mp := range c.Maps {
		if mp.Machines != nil && !slices.Contains(mp.Machines, machine) {
			continue
		}
		where := "dot.toml: " + mp.Label + " on " + machine
		src, e := Render(mp.Src, values, where)
		if e != nil {
			return nil, e
		}
		if !filepath.IsAbs(src) {
			src = c.Paths.Dot + "/" + src
		}
		r := Resolved{Mapping: mp, Source: src}
		for _, d := range mp.To {
			s, e := Render(d, values, where)
			if e != nil {
				return nil, e
			}
			s = filepath.Clean(c.Paths.Expand(s))
			if !filepath.IsAbs(s) {
				return nil, setup.Fail(where + ": destination " + s + " must be absolute or start with ~")
			}
			r.Dests = append(r.Dests, s)
		}
		out = append(out, r)
	}
	return out, nil
}

// validate checks sources, templates, aliases, and nested destinations on every known machine.
func (c *Config) validate() error {
	machines := []string{c.Paths.Machine}
	for m := range c.Machines {
		if !slices.Contains(machines, m) {
			machines = append(machines, m)
		}
	}
	for _, mp := range c.Maps {
		for _, m := range mp.Machines {
			if !slices.Contains(machines, m) {
				machines = append(machines, m)
			}
		}
	}
	slices.Sort(machines)
	var problems []string
	for _, m := range machines {
		maps, e := c.Resolve(m)
		if e != nil {
			problems = append(problems, e.Error())
			continue
		}
		type owner struct{ real, label, dest string }
		var owners []owner
		for _, r := range maps {
			where := "dot.toml: " + r.Label + " on " + m
			if !setup.Exists(r.Source) {
				problems = append(problems, where+": missing source "+c.Paths.Show(r.Source))
			} else if r.Template {
				if !setup.IsFile(r.Source) {
					problems = append(problems, where+": a template must be a file")
				} else {
					b, e := os.ReadFile(r.Source)
					if e != nil {
						return e
					}
					_, e = Render(strings.ReplaceAll(string(b), "\r\n", "\n"), c.ValuesFor(m), c.Paths.Show(r.Source)+" on "+m)
					if e != nil {
						problems = append(problems, e.Error())
					}
				}
			}
			for _, d := range r.Dests {
				real := filepath.Join(setup.Real(filepath.Dir(d)), filepath.Base(d))
				if d == "/" {
					real = "/"
				}
				idx := -1
				for i, o := range owners {
					if o.real == real {
						problems = append(problems, where+": "+c.Paths.Show(d)+" is also written by "+o.label)
						idx = i
						break
					}
				}
				o := owner{real, r.Label, d}
				if idx >= 0 {
					owners[idx] = o
				} else {
					owners = append(owners, o)
				}
			}
		}
		for _, a := range owners {
			for _, b := range owners {
				if setup.Under(b.real, a.real) {
					problems = append(problems, "dot.toml: "+b.label+" on "+m+": "+c.Paths.Show(b.dest)+" is inside "+c.Paths.Show(a.dest)+" from "+a.label)
				}
			}
		}
	}
	var unique []string
	for _, p := range problems {
		if !slices.Contains(unique, p) {
			unique = append(unique, p)
		}
	}
	if len(unique) > 0 {
		return setup.Fail(strings.Join(unique, "\n"))
	}
	return nil
}
