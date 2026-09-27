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
	out1 := RenderIndex(facts)
	out2 := RenderIndex(facts)
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

func TestRenderStub(t *testing.T) {
	s := RenderStub()
	if !strings.Contains(s, "asaman mem") || len(strings.Split(strings.TrimSpace(s), "\n")) > 6 {
		t.Errorf("stub not tiny/pointered:\n%s", s)
	}
}
