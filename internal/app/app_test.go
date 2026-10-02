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

// TestTakeRefusesAnEditToAFilledLine pins the behavior: take refuses an edit to a line the template fills in, and shows the diff.
func TestTakeRefusesAnEditToAFilledLine(t *testing.T) {
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
	if _, e := os.Stat(filepath.Join(f.Paths.Dot, "home/.zshrc")); !os.IsNotExist(e) {
		t.Fatal("the source is still in the setup")
	}
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, ".zshrc"), "export A=1\n")
}

// forget loads the setup and forgets one live path.
func forget(t *testing.T, f *testutil.Fixture, path string) testutil.Result {
	t.Helper()
	c, e := config.Load(f.Paths)
	if e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	code, e := app.Forget(c, []string{path}, &out)
	if e != nil {
		out.WriteString(e.Error())
	}
	return testutil.Result{Code: code, Output: out.String()}
}

// skill makes a version 2 setup with claude and codex installed and one shared skill applied.
func skill(t *testing.T, f *testutil.Fixture, files map[string]string) {
	t.Helper()
	folders(t, f, ".claude", ".codex")
	f.Write(filepath.Join(f.Paths.Dot, "agents/skills/review"), files)
	testutil.OK(t, f.Apply(false))
}

// TestForgetRefusesAFileInsideASharedSkill pins the behavior: one file of a shared skill cannot be forgotten alone, since the skill would take it back.
func TestForgetRefusesAFileInsideASharedSkill(t *testing.T) {
	f := testutil.New(t)
	skill(t, f, map[string]string{"SKILL.md": "review\n"})
	r := forget(t, f, "~/.claude/skills/review/SKILL.md")
	testutil.Equal(t, r.Code, 2)
	if !strings.Contains(r.Output, "is part of ~/.claude/skills/review, which dot manages whole") {
		t.Fatal(r.Output)
	}
	testutil.Equal(t, f.Read(f.Paths.Dot, "agents/skills/review/SKILL.md"), "review\n")
}

// TestForgetOfASkillKeepsFilesItsSourceAlreadyLost pins the behavior: forgetting a skill also drops the record of a file removed from the setup but not yet from the machine, so no later apply deletes it.
func TestForgetOfASkillKeepsFilesItsSourceAlreadyLost(t *testing.T) {
	f := testutil.New(t)
	skill(t, f, map[string]string{"SKILL.md": "review\n", "old.md": "old\n"})
	if e := os.Remove(filepath.Join(f.Paths.Dot, "agents/skills/review/old.md")); e != nil {
		t.Fatal(e)
	}
	testutil.OK(t, forget(t, f, "~/.claude/skills/review"))
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, ".claude/skills/review/old.md")+f.Read(f.Paths.Home, ".codex/skills/review/old.md"), "old\nold\n")
}

// TestForgetRefusesAPathMappedInDotToml pins the behavior: a path dot.toml maps is not forgotten; its line is the user's to remove.
func TestForgetRefusesAPathMappedInDotToml(t *testing.T) {
	f := testutil.New(t)
	f.Config("version = 2\n[files]\na = \"~/a\"\n", map[string]string{"a": "1\n"})
	testutil.OK(t, f.Apply(false))
	r := forget(t, f, "~/a")
	testutil.Equal(t, r.Code, 2)
	testutil.Equal(t, f.Read(f.Paths.Dot, "a"), "1\n")
}

// TestForgetRefusesAHomeSourceThatDotTomlAlsoMaps pins the behavior: a file under home/ that a line in dot.toml also writes elsewhere is not removed, so that line does not lose its source.
func TestForgetRefusesAHomeSourceThatDotTomlAlsoMaps(t *testing.T) {
	f := testutil.New(t)
	f.Config("version = 2\n[files]\n\"home/a\" = \"~/alias\"\n", map[string]string{"home/a": "1\n"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, forget(t, f, "~/a").Code, 2)
	testutil.Equal(t, f.Read(f.Paths.Dot, "home/a"), "1\n")
}

// TestForgetKeepsASetupThatHasNoDotToml pins the behavior: forgetting the last file leaves home/ in place, so a setup configured only by its folders still loads.
func TestForgetKeepsASetupThatHasNoDotToml(t *testing.T) {
	f := testutil.New(t)
	f.Write(f.Paths.Dot, map[string]string{"home/.zshrc": "export A=1\n"})
	testutil.OK(t, f.Apply(false))
	testutil.OK(t, forget(t, f, "~/.zshrc"))
	testutil.Equal(t, f.Status("").Output, "up to date\n")
}

// TestAddRefusesAnAgentsWholeFolder pins the behavior: dot add of an agent's home folder is refused, since it holds sessions and credentials.
func TestAddRefusesAnAgentsWholeFolder(t *testing.T) {
	f := testutil.New(t)
	folders(t, f, ".claude")
	var out bytes.Buffer
	code, e := app.Add(f.Paths, []string{"~/.claude"}, &out, &out)
	if code != 2 || e == nil || !strings.Contains(e.Error(), "is an agent's whole folder") {
		t.Fatal(code, e)
	}
}

// TestAddLeavesOutAFileThatHoldsACredential pins the behavior: dot add of a folder copies everything but a file shaped like a credential, and names it.
func TestAddLeavesOutAFileThatHoldsACredential(t *testing.T) {
	f := testutil.New(t)
	folders(t, f)
	f.Write(f.Paths.Home, map[string]string{".config/tool/config": "a = 1\n", ".config/tool/auth": "token = ghp_0123456789abcdefghijklmnopqrstuvwxyz\n"})
	got := run(t, func(out *bytes.Buffer) (int, error) { return app.Add(f.Paths, []string{"~/.config/tool"}, out, out) })
	if !strings.Contains(got, "left out ~/.config/tool/auth: it looks like it holds a credential\n") {
		t.Fatal(got)
	}
	testutil.Equal(t, f.Read(f.Paths.Dot, "home/.config/tool/config"), "a = 1\n")
	if _, e := os.Stat(filepath.Join(f.Paths.Dot, "home/.config/tool/auth")); !os.IsNotExist(e) {
		t.Fatal("the credential was copied into the setup")
	}
}

// TestAddKeepsANewFileInsideASkillTheSetupHolds pins the behavior: dot add of a new file in a skill the setup already has leaves the file for sync to take; the apply it runs does not remove it.
func TestAddKeepsANewFileInsideASkillTheSetupHolds(t *testing.T) {
	f := testutil.New(t)
	skill(t, f, map[string]string{"SKILL.md": "review\n"})
	f.Write(f.Paths.Home, map[string]string{".claude/skills/review/notes.md": "new\n"})
	run(t, func(out *bytes.Buffer) (int, error) {
		return app.Add(f.Paths, []string{"~/.claude/skills/review/notes.md"}, out, out)
	})
	testutil.Equal(t, f.Read(f.Paths.Home, ".claude/skills/review/notes.md"), "new\n")
	if r := f.Status(""); !strings.Contains(r.Output, "< take        ~/.claude/skills/review/notes.md\n") {
		t.Fatal(r.Output)
	}
}

// TestTakeCarriesAnEditIntoItsTemplate pins the behavior: dot take of a rendered file puts the edit into the template and keeps its placeholders.
func TestTakeCarriesAnEditIntoItsTemplate(t *testing.T) {
	f := testutil.New(t)
	f.Config("[templates]\nt = \"~/t\"\n", map[string]string{"t": "Machine {{machine}}\nRules.\n"})
	testutil.OK(t, f.Apply(false))
	f.Write(f.Paths.Home, map[string]string{"t": "Machine laptop\nRules.\nMore.\n"})
	testutil.OK(t, f.Take("~/t"))
	testutil.Equal(t, f.Read(f.Paths.Dot, "t"), "Machine {{machine}}\nRules.\nMore.\n")
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

// edited seeds a version 2 setup with a remote, then syncs an edit to ~/a so its history has two changes.
func edited(t *testing.T, f *testutil.Fixture) {
	t.Helper()
	f.Remote("version = 2\n[files]\na = \"~/a\"\n", map[string]string{"a": "1\n"})
	testutil.OK(t, f.Sync())
	f.Write(f.Paths.Home, map[string]string{"a": "2\n"})
	testutil.OK(t, f.Sync())
}

// undo loads the setup and undoes one live path's last change.
func undo(t *testing.T, f *testutil.Fixture, path string) testutil.Result {
	t.Helper()
	c, e := config.Load(f.Paths)
	if e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	code, e := app.Undo(c, path, &out, &out)
	if e != nil {
		out.WriteString(e.Error())
	}
	return testutil.Result{Code: code, Output: out.String()}
}

// TestUndoTakesAPathBackOneChange pins the behavior: dot undo restores a path to before its last change, in the setup and live, as a new commit named for the machine.
func TestUndoTakesAPathBackOneChange(t *testing.T) {
	f := testutil.New(t)
	edited(t, f)
	testutil.OK(t, undo(t, f, "~/a"))
	testutil.Equal(t, f.Read(f.Paths.Home, "a")+f.Read(f.Paths.Dot, "a"), "1\n1\n")
	testutil.Equal(t, strings.TrimSpace(f.Git(f.Paths.Dot, "log", "-1", "--format=%s")), "laptop: undo ~/a")
}

// TestUndoRefusesAPathWithUnsyncedChanges pins the behavior: dot undo does not bury an edit that has not been synced.
func TestUndoRefusesAPathWithUnsyncedChanges(t *testing.T) {
	f := testutil.New(t)
	edited(t, f)
	f.Write(f.Paths.Home, map[string]string{"a": "3\n"})
	testutil.Equal(t, undo(t, f, "~/a").Code, 1)
	testutil.Equal(t, f.Read(f.Paths.Home, "a")+f.Read(f.Paths.Dot, "a"), "3\n2\n")
}

// TestUndoRefusesAPathItsLastChangeAdded pins the behavior: dot undo does not remove a path whose only change is the one that added it.
func TestUndoRefusesAPathItsLastChangeAdded(t *testing.T) {
	f := testutil.New(t)
	f.Remote("version = 2\n[files]\na = \"~/a\"\n", map[string]string{"a": "1\n"})
	testutil.OK(t, f.Sync())
	r := undo(t, f, "~/a")
	testutil.Equal(t, r.Code, 2)
	if !strings.Contains(r.Output, "was added by its last change") {
		t.Fatal(r.Output)
	}
	testutil.Equal(t, f.Read(f.Paths.Home, "a"), "1\n")
}

// TestLogListsTheChangesToAPath pins the behavior: dot log with a path prints that path's changes, newest first, each with its machine.
func TestLogListsTheChangesToAPath(t *testing.T) {
	f := testutil.New(t)
	edited(t, f)
	c, e := config.Load(f.Paths)
	if e != nil {
		t.Fatal(e)
	}
	lines := strings.Split(strings.TrimSpace(run(t, func(out *bytes.Buffer) (int, error) { return app.Log(c, "~/a", out) })), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], " laptop: ~/a") || !strings.HasSuffix(lines[1], " setup") {
		t.Fatal(lines)
	}
}

// TestAddSharesAClonedSkillWithoutItsRepository pins the behavior: a skill that is a git clone is added without its .git folder, which stays where it is and reaches no other agent.
func TestAddSharesAClonedSkillWithoutItsRepository(t *testing.T) {
	f := testutil.New(t)
	folders(t, f, ".claude", ".codex")
	f.Write(f.Paths.Home, map[string]string{".claude/skills/review/SKILL.md": "review\n", ".claude/skills/review/.git/HEAD": "ref: refs/heads/main\n"})
	run(t, func(out *bytes.Buffer) (int, error) {
		return app.Add(f.Paths, []string{"~/.claude/skills/review"}, out, out)
	})
	testutil.Equal(t, f.Read(f.Paths.Home, ".codex/skills/review/SKILL.md")+f.Read(f.Paths.Home, ".claude/skills/review/.git/HEAD"), "review\nref: refs/heads/main\n")
	for _, copied := range []string{filepath.Join(f.Paths.Dot, "agents/skills/review/.git"), filepath.Join(f.Paths.Home, ".codex/skills/review/.git")} {
		if _, e := os.Stat(copied); !os.IsNotExist(e) {
			t.Fatal("the repository was copied to " + copied)
		}
	}
	testutil.Equal(t, f.Status("").Output, "up to date\n")
}

// TestAddRefusesThePartsOfTheSetupAndTheWholeHome pins the behavior: dot add refuses a path inside the setup, and a folder that holds the setup or the whole home.
func TestAddRefusesThePartsOfTheSetupAndTheWholeHome(t *testing.T) {
	f := testutil.New(t)
	folders(t, f)
	for _, path := range []string{"~/.dot/dot.toml", "~"} {
		var out bytes.Buffer
		if code, e := app.Add(f.Paths, []string{path}, &out, &out); code != 2 || e == nil {
			t.Fatal(path, code, e, out.String())
		}
	}
	if _, e := os.Stat(filepath.Join(f.Paths.Dot, "home")); !os.IsNotExist(e) {
		t.Fatal("something was copied into the setup")
	}
}

// TestStatusOfAPathShowsWhatATakeWouldChange pins the behavior: dot status with a path prints the diff a pending take would make in the setup.
func TestStatusOfAPathShowsWhatATakeWouldChange(t *testing.T) {
	f := testutil.New(t)
	f.Config("version = 2\n[files]\na = \"~/a\"\n", map[string]string{"a": "1\n"})
	testutil.OK(t, f.Apply(false))
	f.Write(f.Paths.Home, map[string]string{"a": "1\n2\n"})
	r := f.Status("~/a")
	testutil.Equal(t, r.Code, 1)
	if !strings.Contains(r.Output, "--- dot ~/a\n+++ live ~/a\n") || !strings.Contains(r.Output, "+2\n") {
		t.Fatal(r.Output)
	}
}
