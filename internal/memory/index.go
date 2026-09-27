package memory

import (
	"sort"
	"strings"
)

// categoryOrder gives stable, sensible grouping; unknown categories sort last.
var categoryOrder = []string{"User", "Feedback", "References", "Projects", "Incidents", "Other"}

func catRank(c string) int {
	for i, name := range categoryOrder {
		if name == c {
			return i
		}
	}
	return len(categoryOrder)
}

// RenderIndex renders the one-line-per-fact index, grouped by category.
// Superseded facts are excluded; output is deterministic (stable ordering).
func RenderIndex(facts []Fact) string {
	byCat := map[string][]Fact{}
	for _, f := range facts {
		if f.Superseded() {
			continue
		}
		byCat[f.Category] = append(byCat[f.Category], f)
	}
	cats := make([]string, 0, len(byCat))
	for c := range byCat {
		cats = append(cats, c)
	}
	sort.Slice(cats, func(i, j int) bool {
		ri, rj := catRank(cats[i]), catRank(cats[j])
		if ri != rj {
			return ri < rj
		}
		return cats[i] < cats[j]
	})
	var b strings.Builder
	for i, c := range cats {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## " + c + "\n")
		fs := byCat[c]
		sort.Slice(fs, func(a, z int) bool { return fs[a].Name < fs[z].Name })
		for _, f := range fs {
			b.WriteString("- **" + f.Name + "** — " + f.Description + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// RenderStub renders the tiny MEMORY.md stub that replaces the fat hand-edited
// index on the Claude side (the real index lives in the canonical rules file).
func RenderStub() string {
	return strings.Join([]string{
		"# Memory",
		"",
		"This machine uses **asaman**. The full fact index lives in the project rules file.",
		"- Search a fact: `asaman mem <query>`",
		"- Session history (all agents): `asaman sessions`",
	}, "\n") + "\n"
}
