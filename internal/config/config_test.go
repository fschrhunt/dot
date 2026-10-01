package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fschrhunt/dot/internal/config"
	"github.com/fschrhunt/dot/internal/testutil"
)

// invalid pins a config error's exit code and identifying diagnostic.
func invalid(t *testing.T, f *testutil.Fixture, text, message string, files map[string]string) {
	t.Helper()
	f.Config(text, files)
	r := f.Status("")
	testutil.Equal(t, r.Code, 2)
	if !strings.Contains(r.Output, message) {
		t.Fatalf("got %q; want diagnostic %q", r.Output, message)
	}
}

// TestUndefinedValueIsError pins the behavior: undefined value is error.
func TestUndefinedValueIsError(t *testing.T) {
	f := testutil.New(t)
	invalid(t, f, "[templates]\nt = \"~/t\"\n", "dot: ~/.dot/t on laptop: undefined name \"nope\"", map[string]string{"t": "{{nope}}"})
}

// TestMissingSourceOnAnotherMachineIsError pins the behavior: missing source on another machine is error.
func TestMissingSourceOnAnotherMachineIsError(t *testing.T) {
	f := testutil.New(t)
	invalid(t, f, "[machine.server]\n[files]\n\"c.{{machine}}\" = \"~/c\"\n", "dot: dot.toml: [files] \"c.{{machine}}\" on server: missing source ~/.dot/c.server", map[string]string{"c.laptop": ""})
}

// TestTwoMappingsWritingOneDestinationIsError pins the behavior: two mappings writing one destination is error.
func TestTwoMappingsWritingOneDestinationIsError(t *testing.T) {
	f := testutil.New(t)
	invalid(t, f, "[files]\na = \"~/a\"\nb = \"~/a\"\n", "[files] \"b\" on laptop: ~/a is also written by [files] \"a\"", map[string]string{"a": "", "b": ""})
}

// TestNestedDestinationsAreError pins the behavior: nested destinations are error.
func TestNestedDestinationsAreError(t *testing.T) {
	f := testutil.New(t)
	invalid(t, f, "[files]\na = \"~/d/a\"\nd = \"~/d\"\n", "[files] \"a\" on laptop: ~/d/a is inside ~/d", map[string]string{"a": "", "d/f": ""})
}

// TestCollisionThroughSymlinkedParentIsError pins the behavior: collision through symlinked parent is error.
func TestCollisionThroughSymlinkedParentIsError(t *testing.T) {
	f := testutil.New(t)
	if e := os.Mkdir(filepath.Join(f.Paths.Home, "real"), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(filepath.Join(f.Paths.Home, "real"), filepath.Join(f.Paths.Home, "alias")); e != nil {
		t.Fatal(e)
	}
	invalid(t, f, "[files]\na = \"~/alias/f\"\nb = \"~/real/f\"\n", "~/real/f is also written by", map[string]string{"a": "", "b": ""})
}

// TestDestinationInsideRootFolderIsError pins the behavior: destination inside root folder is error.
func TestDestinationInsideRootFolderIsError(t *testing.T) {
	f := testutil.New(t)
	invalid(t, f, "[files]\na = \"/\"\nb = \"/nested\"\n", "[files] \"b\" on laptop: /nested is inside /", map[string]string{"a": "", "b": ""})
}

// TestWrongConfigTypesAreErrors pins the behavior: wrong config types are errors.
func TestWrongConfigTypesAreErrors(t *testing.T) {
	f := testutil.New(t)
	invalid(t, f, "[files]\na = { to = \"~/a\", mirror = \"false\" }\n", "dot: dot.toml: [files] \"a\": mirror must be true or false", map[string]string{"a": ""})
	invalid(t, f, "version = \"1\"\n", "dot: dot.toml: version must be a whole number", nil)
}

// TestUnknownMappingKeyIsError pins the behavior: unknown mapping key is error.
func TestUnknownMappingKeyIsError(t *testing.T) {
	f := testutil.New(t)
	invalid(t, f, "[files]\na = { to = \"~/a\", mirorr = false }\n", "dot: dot.toml: [files] \"a\": unknown key \"mirorr\"", map[string]string{"a": ""})
}

// TestNumericValuesUsePythonText protects placeholders that change paths or template contents.
func TestNumericValuesUsePythonText(t *testing.T) {
	f := testutil.New(t)
	f.Config("[values]\na = 1000000.0\nb = 0.0001\nc = 1e16\nd = 1e-5\ne = -0.0\n[templates]\nt = \"~/t\"\n", map[string]string{"t": "{{a}} {{b}} {{c}} {{d}} {{e}}"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, "t"), "1000000.0 0.0001 1e+16 1e-05 -0.0")
}

// TestSyntaxErrorsKeepTomllibDiagnostics protects errors users already see for malformed TOML.
func TestSyntaxErrorsKeepTomllibDiagnostics(t *testing.T) {
	f := testutil.New(t)
	for _, c := range []struct{ text, message string }{
		{"version =\n", "Invalid value (at line 1, column 10)"},
		{"hello\n", "Expected '=' after a key in a key/value pair (at line 1, column 6)"},
		{"[files\n", "Expected ']' at the end of a table declaration (at line 1, column 7)"},
		{"a = tru\n", "Invalid value (at line 1, column 5)"},
		{"a = [1 2]\n", "Unclosed array (at line 1, column 8)"},
		{"a = \"unterminated", "Unterminated string (at end of document)"},
		{"a = \"bad\\q\"\n", "Unescaped '\\' in a string (at line 1, column 11)"},
		{"a = { x = 1, }\n", "Invalid initial character for a key part (at line 1, column 14)"},
		{"[x]\n[x]\n", "Cannot declare ('x',) twice (at line 2, column 3)"},
		{"a = []\n[a]\n", "Cannot declare ('a',) twice (at line 2, column 3)"},
		{"a = 01\n", "Expected newline or end of document after a statement (at line 1, column 6)"},
		{"a = 1 x\n", "Expected newline or end of document after a statement (at line 1, column 7)"},
		{"a = { b = true\n}", "Unclosed inline table (at line 1, column 15)"},
		{"a = \"one\ntwo\"\n", "Illegal character '\\n' (at line 1, column 9)"},
	} {
		f.Config(c.text, nil)
		testutil.Equal(t, f.Status(""), testutil.Result{Code: 2, Output: "dot: dot.toml: " + c.message + "\n"})
	}
}

// TestCollisionThroughDanglingParentIsError protects alias validation before destinations exist.
func TestCollisionThroughDanglingParentIsError(t *testing.T) {
	f := testutil.New(t)
	if e := os.Symlink("not-created", filepath.Join(f.Paths.Home, "alias")); e != nil {
		t.Fatal(e)
	}
	invalid(t, f, "[files]\na = \"~/alias/f\"\nb = \"~/not-created/f\"\n", "~/not-created/f is also written by", map[string]string{"a": "", "b": ""})
}

// TestNumbersOutsideGoRangeRemainAccepted protects Python setups containing large numeric values.
func TestNumbersOutsideGoRangeRemainAccepted(t *testing.T) {
	f := testutil.New(t)
	f.Config("version = -9223372036854775809\n[values]\na = 9223372036854775808\nb = 0xffffffffffffffff\nc = 1e400\n[templates]\nt = \"~/t\"\n", map[string]string{"t": "{{a}} {{b}} {{c}}"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, "t"), "9223372036854775808 18446744073709551615 inf")
}

// TestSyncEverySetsTheInterval pins the behavior: sync every sets the interval.
func TestSyncEverySetsTheInterval(t *testing.T) {
	f := testutil.New(t)
	f.Config("[sync]\nevery = \"5m\"\n", nil)
	c, e := config.Load(f.Paths)
	if e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, c.Every, 5*time.Minute)
}

// TestSyncEveryMustBeWholeSeconds pins the behavior: sync every must be whole seconds.
func TestSyncEveryMustBeWholeSeconds(t *testing.T) {
	f := testutil.New(t)
	invalid(t, f, "[sync]\nevery = \"1.5s\"\n", "dot: dot.toml: [sync] every must be whole seconds, minutes or hours", nil)
}

// TestSyncMachineTableOverridesSync pins the behavior: a machine's sync table overrides [sync] on that machine only.
func TestSyncMachineTableOverridesSync(t *testing.T) {
	f := testutil.New(t)
	f.Config("[sync]\nevery = \"5m\"\n[sync.machine.server]\nevery = \"1m\"\npush = true\n", nil)
	for machine, want := range map[string]config.Sync{"laptop": {Every: 5 * time.Minute}, "server": {Every: time.Minute, Push: true}} {
		f.Paths.Machine = machine
		c, e := config.Load(f.Paths)
		if e != nil {
			t.Fatal(e)
		}
		testutil.Equal(t, c.Every, want.Every)
		testutil.Equal(t, c.Push, want.Push)
	}
}

// TestBadSyncMachineTableIsErrorEverywhere pins the behavior: a bad sync table for another machine is an error here too.
func TestBadSyncMachineTableIsErrorEverywhere(t *testing.T) {
	f := testutil.New(t)
	invalid(t, f, "[sync.machine.server]\ntimeout = \"never\"\n", "dot: dot.toml: [sync.machine.server] timeout must be whole seconds, minutes or hours", nil)
}
