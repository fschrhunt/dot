package plan_test

import (
	"bytes"
	"testing"

	"github.com/fschrhunt/dot/internal/plan"
	"github.com/fschrhunt/dot/internal/setup"
	"github.com/fschrhunt/dot/internal/testutil"
)

// TestStatusShowsExactlyWhatApplyDoes pins the behavior: status shows exactly what apply does.
func TestStatusShowsExactlyWhatApplyDoes(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\na = \"~/a\"\nb = \"~/b\"\n", map[string]string{"a": "1", "b": "1"})
	testutil.OK(t, f.Apply(false))
	f.Config("[files]\na = \"~/a\"\nc = \"~/c\"\n", map[string]string{"a": "2", "c": "1"})
	r := f.Status("")
	testutil.Equal(t, r, testutil.Result{Code: 1, Output: "- removed     ~/b\n~ changed     ~/a\n+ new         ~/c\n"})
	testutil.Equal(t, f.Apply(false).Output, r.Output)
	testutil.Equal(t, f.Status(""), testutil.Result{Code: 0, Output: "up to date\n"})
}

// TestDiffUsesThreeContextLines protects the shared diff's hunk boundaries and missing newline output.
func TestDiffUsesThreeContextLines(t *testing.T) {
	f := testutil.New(t)
	f.Write(f.Paths.Home, map[string]string{"a": "0\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n16"})
	var out bytes.Buffer
	err := plan.Diff(&out, f.Paths, f.Paths.Home+"/a", setup.Want{Kind: "file", Data: []byte("new\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\nlast")}, false)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Equal(t, out.String(), "--- live ~/a\n+++ dot ~/a\n@@ -1,4 +1,4 @@\n-0\n+new\n 1\n 2\n 3\n@@ -14,4 +14,4 @@\n 13\n 14\n 15\n-16+last")
}

// TestPrintSaysWhyATakeIsHeldBack protects the held-back take's explanation: the plan line
// without its note cannot tell a waiting take from an edit that is at war with another name.
func TestPrintSaysWhyATakeIsHeldBack(t *testing.T) {
	f := testutil.New(t)
	var out bytes.Buffer
	plan.Print(&out, f.Paths, []plan.Action{{Mark: "!", Path: f.Paths.Home + "/a", Op: "take", Note: "looks like a credential; add it yourself"}})
	testutil.Equal(t, out.String(), "! edited here ~/a (looks like a credential; add it yourself)\n")
}

// TestBrokenReadsJSONCAndTOML pins the behavior: JSONC's comments and trailing commas parse, and
// an edit that breaks JSONC or TOML, or any edit to a base that never parsed, is judged as such.
func TestBrokenReadsJSONCAndTOML(t *testing.T) {
	jsonc := "{\n  // a comment, with a comma,\n  \"url\": \"http://x/*y*/\", /* block */\n  \"a\": [1, 2,],\n}\n"
	for _, c := range []struct {
		path, base, edited string
		broken             bool
	}{
		{"a.jsonc", jsonc, jsonc + "\n// more\n", false},
		{"a.jsonc", jsonc, "{\n  \"a\": 1\n  \"b\": 2\n}\n", true},
		{"a.toml", "x = 1\n", "x = 1\ny = \n", true},
		{"a.json", "not json", "still not json", false},
		{"a.txt", "1", "{", false},
	} {
		if got := plan.Broken(c.path, []byte(c.base), []byte(c.edited)) != ""; got != c.broken {
			t.Errorf("%s %q: broken = %v", c.path, c.edited, got)
		}
	}
}
