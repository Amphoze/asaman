package memory

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderIndexExcludesSupersededAndIsDeterministic(t *testing.T) {
	facts, err := LoadDir(filepath.Join("..", "..", "testdata", "memory"))
	if err != nil {
		t.Fatal(err)
	}
	out1 := RenderIndex(facts, "full")
	out2 := RenderIndex(facts, "full")
	if out1 != out2 {
		t.Error("RenderIndex not deterministic")
	}
	// superseded fact excluded
	if strings.Contains(out1, "old_plan") {
		t.Error("superseded fact old_plan should be excluded")
	}
	// current facts present
	for _, name := range []string{"never_guess", "hosting_map", "feedback-destructive-commands"} {
		if !strings.Contains(out1, name) {
			t.Errorf("missing fact %q in index", name)
		}
	}
	// grouped: Feedback heading precedes References heading (category order)
	fi := strings.Index(out1, "## Feedback")
	ri := strings.Index(out1, "## References")
	if fi < 0 || ri < 0 || fi > ri {
		t.Errorf("category ordering wrong:\n%s", out1)
	}
}

func lineFor(out, name string) string {
	for _, ln := range strings.Split(out, "\n") {
		if strings.HasPrefix(ln, "- ") && strings.Contains(ln, name) {
			return ln
		}
	}
	return ""
}

func TestRenderIndexHybridDensity(t *testing.T) {
	longDesc := strings.Repeat("x", 100) // >60 runes
	facts := []Fact{
		{Name: "fb_long", Description: longDesc, Category: "Feedback"},
		{Name: "ref_one", Description: "some reference detail", Category: "References"},
		{Name: "fb_empty", Description: "", Category: "Feedback"},
	}
	out := RenderIndex(facts, "hybrid")

	// Feedback long-desc: has " — ", ends with "…", hook ≤ 60 runes before ellipsis.
	long := lineFor(out, "fb_long")
	if !strings.Contains(long, " — ") {
		t.Errorf("fb_long line missing separator: %q", long)
	}
	if !strings.HasSuffix(long, "…") {
		t.Errorf("fb_long line should end with ellipsis: %q", long)
	}
	hook := strings.TrimSuffix(strings.SplitN(long, " — ", 2)[1], "…")
	if n := len([]rune(hook)); n > 60 {
		t.Errorf("fb_long hook = %d runes, want <= 60", n)
	}

	// References: name-only, no separator.
	ref := lineFor(out, "ref_one")
	if ref != "- ref_one" {
		t.Errorf("ref_one line = %q, want %q", ref, "- ref_one")
	}
	if strings.Contains(ref, " — ") {
		t.Errorf("ref_one should have no separator: %q", ref)
	}

	// Empty-desc Feedback: name-only.
	empty := lineFor(out, "fb_empty")
	if empty != "- fb_empty" {
		t.Errorf("fb_empty line = %q, want %q", empty, "- fb_empty")
	}
}

func TestRenderIndexNamesDensity(t *testing.T) {
	facts := []Fact{
		{Name: "fb_one", Description: "a feedback description", Category: "Feedback"},
		{Name: "ref_one", Description: "a reference description", Category: "References"},
		{Name: "usr_one", Description: "a user description", Category: "User"},
	}
	out := RenderIndex(facts, "names")
	for _, ln := range strings.Split(out, "\n") {
		if strings.HasPrefix(ln, "- ") && strings.Contains(ln, " — ") {
			t.Errorf("names density line has separator: %q", ln)
		}
	}
}

func TestRenderStub(t *testing.T) {
	s := RenderStub()
	if !strings.Contains(s, "asaman mem") || len(strings.Split(strings.TrimSpace(s), "\n")) > 6 {
		t.Errorf("stub not tiny/pointered:\n%s", s)
	}
}
