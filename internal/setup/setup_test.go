package setup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fschrhunt/dot/internal/setup"
	"github.com/fschrhunt/dot/internal/testutil"
)

// TestExcludesUseFnmatch protects literal backslashes and character classes in user patterns.
func TestExcludesUseFnmatch(t *testing.T) {
	for _, c := range []struct {
		path, pattern string
		excluded      bool
	}{
		{"outer/cache/f", "cache", true},
		{"outer/x.tmp", "*.tmp", true},
		{"outer/f", "outer/f", false},
		{"outer/\\", `[\]`, true},
		{"outer/^", "[^abc]", true},
		{"outer/\\", "[^abc]", false},
		{"outer/z", "[!abc]", true},
		{"outer/b", "[!abc]", false},
		{"outer/[", "[[]", true},
		{"outer/é", "?", true},
		{"outer/café", "café", true},
		{"outer/cafe", "café", false},
	} {
		testutil.Equal(t, setup.Excluded(c.path, []string{c.pattern}), c.excluded)
	}
}

// TestWriteWithZeroModeForNewFiles protects a file creation whose Want carries no mode.
func TestWriteWithZeroModeForNewFiles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "new")
	if e := setup.Write(p, setup.Want{Kind: "file", Data: []byte("x")}); e != nil {
		t.Fatal(e)
	}
	i, e := os.Stat(p)
	if e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, i.Mode()&0777, os.FileMode(0644))
}
