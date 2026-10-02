package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fschrhunt/dot/internal/app"
	"github.com/fschrhunt/dot/internal/config"
	"github.com/fschrhunt/dot/internal/testutil"
)

// TestStatusReportsDifferingBinaryFiles pins the behavior: status reports differing binary files.
func TestStatusReportsDifferingBinaryFiles(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nb = \"~/b\"\n", map[string]string{"b": "\xff\x00"})
	f.Write(f.Paths.Home, map[string]string{"b": "\xfe"})
	testutil.Equal(t, f.Status("~/b"), testutil.Result{Code: 1, Output: "binary files live ~/b and dot ~/b differ\n"})
}

// TestTakeCopiesLiveEditBackToSource pins the behavior: take copies live edit back to source.
func TestTakeCopiesLiveEditBackToSource(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nd = [\"~/d\",\"~/e\"]\n", map[string]string{"d/f": "1"})
	testutil.OK(t, f.Apply(false))
	f.Write(f.Paths.Home, map[string]string{"d/f": "2", "d/g": "new"})
	testutil.OK(t, f.Take("~/d"))
	testutil.Equal(t, f.Read(f.Paths.Dot, "d/f"), "2")
	testutil.Equal(t, f.Read(f.Paths.Dot, "d/g"), "new")
}

// TestTakeRefusesTemplateDestination pins the behavior: take refuses template destination.
func TestTakeRefusesTemplateDestination(t *testing.T) {
	f := testutil.New(t)
	f.Config("[templates]\nt = \"~/t\"\n", map[string]string{"t": "{{machine}}\n"})
	testutil.OK(t, f.Apply(false))
	f.Write(f.Paths.Home, map[string]string{"t": "edited\n"})
	r := f.Take("~/t")
	testutil.Equal(t, r.Code, 1)
	if !strings.Contains(r.Output, "+edited") {
		t.Fatal(r.Output)
	}
	testutil.Equal(t, f.Read(f.Paths.Dot, "t"), "{{machine}}\n")
}

// TestTakeOfMissingFolderIsError pins the behavior: take of missing folder is error.
func TestTakeOfMissingFolderIsError(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nd = \"~/d\"\n", map[string]string{"d/f": "1"})
	testutil.OK(t, f.Apply(false))
	if e := os.RemoveAll(filepath.Join(f.Paths.Home, "d")); e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, f.Take("~/d"), testutil.Result{Code: 2, Output: "dot: ~/d does not exist\n"})
}

// TestInitUsesEmbeddedExample pins the behavior: init creates a setup that loads and has nothing to do yet.
func TestInitUsesEmbeddedExample(t *testing.T) {
	f := testutil.New(t)
	if e := os.Remove(f.Paths.Dot); e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	code, e := app.Init(f.Paths, "", &out, &out)
	if e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, code, 0)
	testutil.Equal(t, f.Read(f.Paths.Dot, ".gitignore"), ".state/\n")
	testutil.Equal(t, f.Status("").Output, "up to date\n")
}

// run captures one of the commands that manage what the setup holds.
func run(t *testing.T, call func(out *bytes.Buffer) (int, error)) string {
	t.Helper()
	var out bytes.Buffer
	code, e := call(&out)
	if e != nil || code != 0 {
		t.Fatal(code, e, out.String())
	}
	return out.String()
}

// folders makes a version 2 setup in which the named agents are installed.
func folders(t *testing.T, f *testutil.Fixture, homes ...string) {
	t.Helper()
	f.Config("version = 2\n", nil)
	for _, home := range homes {
		if e := os.MkdirAll(filepath.Join(f.Paths.Home, home), 0755); e != nil {
			t.Fatal(e)
		}
	}
}

// TestAddPutsAHomeFileUnderHome pins the behavior: dot add copies a file from the home folder into home/ and leaves nothing to do.
func TestAddPutsAHomeFileUnderHome(t *testing.T) {
	f := testutil.New(t)
	folders(t, f)
	f.Write(f.Paths.Home, map[string]string{".config/zsh/aliases.zsh": "alias g=git\n"})
	run(t, func(out *bytes.Buffer) (int, error) {
		return app.Add(f.Paths, []string{"~/.config/zsh/aliases.zsh"}, out, out)
	})
	testutil.Equal(t, f.Read(f.Paths.Dot, "home/.config/zsh/aliases.zsh"), "alias g=git\n")
	testutil.Equal(t, f.Status("").Output, "up to date\n")
}

// TestAddSharesAnAgentsFileWithTheOtherAgents pins the behavior: dot add of one agent's instructions stores them under agents/ and writes them under the other agents' names.
func TestAddSharesAnAgentsFileWithTheOtherAgents(t *testing.T) {
	f := testutil.New(t)
	folders(t, f, ".claude", ".codex")
	f.Write(f.Paths.Home, map[string]string{".claude/CLAUDE.md": "rules\n"})
	run(t, func(out *bytes.Buffer) (int, error) {
		return app.Add(f.Paths, []string{"~/.claude/CLAUDE.md"}, out, out)
	})
	testutil.Equal(t, f.Read(f.Paths.Dot, "agents/instructions.md")+f.Read(f.Paths.Home, ".codex/AGENTS.md"), "rules\nrules\n")
}

// TestAddOnlyKeepsAnAgentsFileToItsAgent pins the behavior: dot add --only stores an agent's file under home/, so no other agent gets it.
func TestAddOnlyKeepsAnAgentsFileToItsAgent(t *testing.T) {
	f := testutil.New(t)
	folders(t, f, ".claude", ".codex")
	f.Write(f.Paths.Home, map[string]string{".claude/CLAUDE.md": "rules\n"})
	run(t, func(out *bytes.Buffer) (int, error) {
		return app.Add(f.Paths, []string{"--only", "~/.claude/CLAUDE.md"}, out, out)
	})
	testutil.Equal(t, f.Read(f.Paths.Dot, "home/.claude/CLAUDE.md"), "rules\n")
	if _, e := os.Stat(filepath.Join(f.Paths.Home, ".codex/AGENTS.md")); !os.IsNotExist(e) {
		t.Fatal("the file reached another agent")
	}
}

// TestAddTakesTheWholeSkillFromAPathInsideIt pins the behavior: dot add of a file inside a skill adds the skill's folder as one shared unit.
func TestAddTakesTheWholeSkillFromAPathInsideIt(t *testing.T) {
	f := testutil.New(t)
	folders(t, f, ".claude", ".codex")
	f.Write(f.Paths.Home, map[string]string{".codex/skills/review/SKILL.md": "review\n", ".codex/skills/review/notes/a.md": "a\n"})
	run(t, func(out *bytes.Buffer) (int, error) {
		return app.Add(f.Paths, []string{"~/.codex/skills/review/SKILL.md"}, out, out)
	})
	testutil.Equal(t, f.Read(f.Paths.Dot, "agents/skills/review/notes/a.md")+f.Read(f.Paths.Home, ".claude/skills/review/SKILL.md"), "a\nreview\n")
}

// TestForgetLeavesTheLiveFile pins the behavior: dot forget removes the source and the record, so the live file stays through a later apply.
func TestForgetLeavesTheLiveFile(t *testing.T) {
	f := testutil.New(t)
	folders(t, f)
	f.Write(f.Paths.Home, map[string]string{".zshrc": "export A=1\n"})
	run(t, func(out *bytes.Buffer) (int, error) { return app.Add(f.Paths, []string{"~/.zshrc"}, out, out) })
	c, e := config.Load(f.Paths)
	if e != nil {
		t.Fatal(e)
	}
	run(t, func(out *bytes.Buffer) (int, error) { return app.Forget(c, []string{"~/.zshrc"}, out) })
	if _, e := os.Stat(filepath.Join(f.Paths.Dot, "home")); !os.IsNotExist(e) {
		t.Fatal("the source is still in the setup")
	}
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, ".zshrc"), "export A=1\n")
}

// TestAgentsShowsWhereEachAgentKeepsThings pins the behavior: dot agents lists each agent, whether it is installed, and its places.
func TestAgentsShowsWhereEachAgentKeepsThings(t *testing.T) {
	f := testutil.New(t)
	folders(t, f, ".codex")
	c, e := config.Load(f.Paths)
	if e != nil {
		t.Fatal(e)
	}
	got := run(t, func(out *bytes.Buffer) (int, error) { return app.Agents(c, out) })
	for _, want := range []string{"claude     ~/.claude  (not installed)\n", "codex      ~/.codex  (installed)\n  instructions   ~/.codex/AGENTS.md\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("got %q; want %q", got, want)
		}
	}
}
