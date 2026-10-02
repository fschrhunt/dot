package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fschrhunt/dot/internal/testutil"
)

// agents creates the home folders of the named agents, as an install would.
func agents(t *testing.T, f *testutil.Fixture, homes ...string) {
	t.Helper()
	for _, home := range homes {
		if e := os.MkdirAll(filepath.Join(f.Paths.Home, home), 0755); e != nil {
			t.Fatal(e)
		}
	}
}

// TestHomeFolderIsWrittenToHome pins the behavior: a file under home/ is written to the same path under the home folder, with no mapping.
func TestHomeFolderIsWrittenToHome(t *testing.T) {
	f := testutil.New(t)
	f.Write(f.Paths.Dot, map[string]string{"home/.config/zsh/aliases.zsh": "alias g=git\n"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, ".config/zsh/aliases.zsh"), "alias g=git\n")
}

// TestSharedFileReachesEachInstalledAgentUnderItsOwnName pins the behavior: agents/instructions.md is written under each installed agent's own name, and an agent that is not installed gets no folder.
func TestSharedFileReachesEachInstalledAgentUnderItsOwnName(t *testing.T) {
	f := testutil.New(t)
	agents(t, f, ".claude", ".codex")
	f.Config("version = 2\n", map[string]string{"agents/instructions.md": "rules\n", "agents/skills/review/SKILL.md": "review\n"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, ".claude/CLAUDE.md")+f.Read(f.Paths.Home, ".codex/AGENTS.md")+f.Read(f.Paths.Home, ".codex/skills/review/SKILL.md"), "rules\nrules\nreview\n")
	if _, e := os.Stat(filepath.Join(f.Paths.Home, ".cursor")); !os.IsNotExist(e) {
		t.Fatal("dot created the folder of an agent that is not installed")
	}
}

// TestOnlyLimitsASharedPathToSomeAgents pins the behavior: an [only] rule keeps a shared skill from the agents it does not name.
func TestOnlyLimitsASharedPathToSomeAgents(t *testing.T) {
	f := testutil.New(t)
	agents(t, f, ".claude", ".codex")
	f.Config("version = 2\n[only]\n\"agents/skills/review\" = { agents = [\"codex\"] }\n", map[string]string{"agents/skills/review/SKILL.md": "review\n"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, ".codex/skills/review/SKILL.md"), "review\n")
	if _, e := os.Stat(filepath.Join(f.Paths.Home, ".claude/skills")); !os.IsNotExist(e) {
		t.Fatal("the skill reached an agent the rule leaves out")
	}
}

// TestAnAgentTableAddsAnAgent pins the behavior: an [agent.<name>] table gives a new agent its places.
func TestAnAgentTableAddsAnAgent(t *testing.T) {
	f := testutil.New(t)
	agents(t, f, ".mine")
	f.Config("version = 2\n[agent.mine]\nhome = \"~/.mine\"\ninstructions = \"RULES.md\"\n", map[string]string{"agents/instructions.md": "rules\n"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, ".mine/RULES.md"), "rules\n")
}

// TestTmplSuffixMarksATemplate pins the behavior: a name ending in .tmpl is rendered and written without the suffix.
func TestTmplSuffixMarksATemplate(t *testing.T) {
	f := testutil.New(t)
	f.Config("version = 2\n", map[string]string{"home/.note.tmpl": "on {{machine}}\n"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, ".note"), "on laptop\n")
}

// TestSharedEntryWithNoPlaceIsError pins the behavior: an entry under agents/ that no agent has a place for is an error naming it.
func TestSharedEntryWithNoPlaceIsError(t *testing.T) {
	f := testutil.New(t)
	f.Config("version = 2\n", map[string]string{"agents/hooks/x.sh": ""})
	r := f.Status("")
	testutil.Equal(t, r.Code, 2)
	if !strings.Contains(r.Output, "dot: agents/hooks: no agent has a place for hooks") {
		t.Fatal(r.Output)
	}
}

// TestVersionOneIgnoresTheFolders pins the behavior: a version 1 setup is not changed by a home/ folder it happens to have.
func TestVersionOneIgnoresTheFolders(t *testing.T) {
	f := testutil.New(t)
	f.Config("", map[string]string{"home/.x": "1\n"})
	testutil.OK(t, f.Apply(false))
	if _, e := os.Stat(filepath.Join(f.Paths.Home, ".x")); !os.IsNotExist(e) {
		t.Fatal("a version 1 setup applied its home/ folder")
	}
}

// TestOnlyNamingAnUnknownAgentIsError pins the behavior: a rule naming an agent dot does not know is an error, not a skill silently dropped from every agent.
func TestOnlyNamingAnUnknownAgentIsError(t *testing.T) {
	f := testutil.New(t)
	agents(t, f, ".claude")
	f.Config("version = 2\n[only]\n\"agents/skills/review\" = { agents = [\"claud\"] }\n", map[string]string{"agents/skills/review/SKILL.md": "review\n"})
	r := f.Status("")
	testutil.Equal(t, r.Code, 2)
	if !strings.Contains(r.Output, "no agent is named claud") {
		t.Fatal(r.Output)
	}
}

// TestOnlyLimitsAHomePathToSomeMachines pins the behavior: an [only] rule on a folder under home/ keeps everything in it from other machines.
func TestOnlyLimitsAHomePathToSomeMachines(t *testing.T) {
	f := testutil.New(t)
	f.Config("version = 2\n[only]\n\"home/.config/hypr\" = { machines = [\"desktop\"] }\n", map[string]string{"home/.config/hypr/hypr.conf": "x\n", "home/.zshrc": "y\n"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, ".zshrc"), "y\n")
	if _, e := os.Stat(filepath.Join(f.Paths.Home, ".config/hypr")); !os.IsNotExist(e) {
		t.Fatal("the folder reached a machine the rule leaves out")
	}
}

// TestAnEmptyHomeTurnsAnAgentOff pins the behavior: [agent.<name>] with home = "" stops dot writing to an installed agent.
func TestAnEmptyHomeTurnsAnAgentOff(t *testing.T) {
	f := testutil.New(t)
	agents(t, f, ".claude", ".codex")
	f.Config("version = 2\n[agent.codex]\nhome = \"\"\n", map[string]string{"agents/instructions.md": "rules\n"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, ".claude/CLAUDE.md"), "rules\n")
	if _, e := os.Stat(filepath.Join(f.Paths.Home, ".codex/AGENTS.md")); !os.IsNotExist(e) {
		t.Fatal("dot wrote to an agent that is turned off")
	}
}

// TestAgentsThatShareAFolderGetOneCopy pins the behavior: when one agent's skills folder is a link to another's, the setup still loads and the skill is written once.
func TestAgentsThatShareAFolderGetOneCopy(t *testing.T) {
	f := testutil.New(t)
	agents(t, f, ".claude/skills", ".codex")
	if e := os.Symlink(filepath.Join(f.Paths.Home, ".claude/skills"), filepath.Join(f.Paths.Home, ".codex/skills")); e != nil {
		t.Fatal(e)
	}
	f.Config("version = 2\n", map[string]string{"agents/skills/review/SKILL.md": "review\n"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, ".codex/skills/review/SKILL.md"), "review\n")
	testutil.Equal(t, f.Status("").Output, "up to date\n")
}

// TestAHiddenFileInAgentsIsIgnored pins the behavior: a stray .DS_Store beside the shared entries does not stop the setup loading.
func TestAHiddenFileInAgentsIsIgnored(t *testing.T) {
	f := testutil.New(t)
	agents(t, f, ".claude")
	f.Config("version = 2\n", map[string]string{"agents/.DS_Store": "x", "agents/instructions.md": "rules\n"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, ".claude/CLAUDE.md"), "rules\n")
}

// TestASkillRemovedFromTheSetupLeavesEveryAgent pins the behavior: a shared skill deleted from agents/ is removed from each agent that had it.
func TestASkillRemovedFromTheSetupLeavesEveryAgent(t *testing.T) {
	f := testutil.New(t)
	agents(t, f, ".claude", ".codex")
	f.Config("version = 2\n", map[string]string{"agents/skills/review/SKILL.md": "review\n", "agents/skills/keep/SKILL.md": "keep\n"})
	testutil.OK(t, f.Apply(false))
	if e := os.RemoveAll(filepath.Join(f.Paths.Dot, "agents/skills/review")); e != nil {
		t.Fatal(e)
	}
	testutil.OK(t, f.Apply(false))
	for _, agent := range []string{".claude", ".codex"} {
		if _, e := os.Stat(filepath.Join(f.Paths.Home, agent, "skills/review")); !os.IsNotExist(e) {
			t.Fatal("the skill is still in " + agent)
		}
		testutil.Equal(t, f.Read(f.Paths.Home, agent+"/skills/keep/SKILL.md"), "keep\n")
	}
}

// TestAFileExcludedAfterItWasWrittenStays pins the behavior: excluding a name under home/ that dot already wrote leaves the live file where it is.
func TestAFileExcludedAfterItWasWrittenStays(t *testing.T) {
	f := testutil.New(t)
	f.Config("version = 2\n", map[string]string{"home/.config/tool/local": "mine\n"})
	testutil.OK(t, f.Apply(false))
	f.Config("version = 2\nexclude = [\"local\"]\n", nil)
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, ".config/tool/local"), "mine\n")
}
