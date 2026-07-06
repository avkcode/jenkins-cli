package cmd

import (
	"strings"
	"testing"
)

func TestDiffLinesMarksChanges(t *testing.T) {
	got := diffLines([]string{"a", "b", "c"}, []string{"a", "x", "c"})
	want := []string{"  a", "- b", "+ x", "  c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestRenderDiffUnchanged(t *testing.T) {
	changed, diff := renderDiff("<x>1</x>\n", "<x>1</x>")
	if changed {
		t.Fatalf("expected no change, got diff:\n%s", diff)
	}
	if diff != "" {
		t.Fatalf("expected empty diff, got %q", diff)
	}
}

func TestRenderDiffChangedOnlyShowsDeltas(t *testing.T) {
	changed, diff := renderDiff("line1\nline2\nline3", "line1\nCHANGED\nline3")
	if !changed {
		t.Fatal("expected change to be detected")
	}
	if !strings.Contains(diff, "- line2") || !strings.Contains(diff, "+ CHANGED") {
		t.Fatalf("expected +/- deltas, got:\n%s", diff)
	}
	if strings.Contains(diff, "line1") || strings.Contains(diff, "line3") {
		t.Fatalf("expected unchanged context to be omitted, got:\n%s", diff)
	}
}

func TestJobApplyHasDiffAndFileFlags(t *testing.T) {
	c, _, err := rootCmd.Find([]string{"job", "apply"})
	if err != nil {
		t.Fatalf("find job apply: %v", err)
	}
	for _, name := range []string{"file", "diff"} {
		if c.Flags().Lookup(name) == nil {
			t.Fatalf("job apply missing --%s flag", name)
		}
	}
}
