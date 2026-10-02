package setup_test

import (
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
