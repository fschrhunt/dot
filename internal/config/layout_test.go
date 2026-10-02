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
