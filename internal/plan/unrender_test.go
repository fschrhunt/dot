package plan_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/fschrhunt/dot/internal/config"
	"github.com/fschrhunt/dot/internal/plan"
)

// render renders text with fixed values, as Untemplate does for each line.
func render(values map[string]string) func(string) (string, bool) {
	return func(text string) (string, bool) {
		s, e := config.Render(text, values, "t")
		return s, e == nil
	}
}

// TestUnrenderRefusesAPlaceholderWrittenAcrossLines pins the behavior: a placeholder split over several lines is not replaced by this machine's value; the edit is refused.
func TestUnrenderRefusesAPlaceholderWrittenAcrossLines(t *testing.T) {
	if out, ok := plan.Unrender("a\n{{\nwho\n}}\nc\n", "a2\nann\nc\n", render(map[string]string{"who": "ann"})); ok {
		t.Fatalf("carried the edit and lost the placeholder: %q", out)
	}
}

// TestUnrenderCarriesEditsBesideACommonLineInALongFile pins the behavior: in a file of 200 lines or more, edits around a placeholder that renders to a common line are still carried.
func TestUnrenderCarriesEditsBesideACommonLineInALongFile(t *testing.T) {
	var template, live strings.Builder
	for i := 0; i < 120; i++ {
		fmt.Fprintf(&template, "k%d = 1\n{{gap}}\n", i)
		value := 1
		if i == 50 || i == 51 {
			value = 2
		}
		fmt.Fprintf(&live, "k%d = %d\n\n", i, value)
	}
	out, ok := plan.Unrender(template.String(), live.String(), render(map[string]string{"gap": ""}))
	if !ok || strings.Count(out, "{{gap}}") != 120 || !strings.Contains(out, "k50 = 2\n") {
		t.Fatalf("ok=%v out=%.80q", ok, out)
	}
}
