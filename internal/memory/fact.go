// Package memory parses the atomic fact store and renders its index.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// stringList accepts either a YAML scalar or a sequence and yields []string.
type stringList []string

func (s *stringList) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		if n.Value == "" {
			*s = nil
			return nil
		}
		*s = stringList{n.Value}
		return nil
	}
	var seq []string
	if err := n.Decode(&seq); err != nil {
		return err
	}
	*s = seq
	return nil
}

// Fact is one atomic memory record.
type Fact struct {
	Name         string
	Description  string
	Type         string
	Category     string
	Origin       map[string]string
	Supersedes   []string
	SupersededBy []string
	Body         string
	Path         string
}

// Superseded reports whether this fact has been replaced by a newer one.
func (f Fact) Superseded() bool { return len(f.SupersededBy) > 0 }

type frontmatter struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	Type         string            `yaml:"type"`
	Category     string            `yaml:"category"`
	Origin       map[string]string `yaml:"origin"`
	Supersedes   stringList        `yaml:"supersedes"`
	SupersededBy stringList        `yaml:"superseded_by"`
	Metadata     map[string]any    `yaml:"metadata"`
}

// ParseFact parses one file's bytes into a Fact.
func ParseFact(path string, data []byte) (Fact, error) {
	s := string(data)
	var fmText, body string
	if strings.HasPrefix(s, "---\n") {
		rest := s[4:]
		if i := strings.Index(rest, "\n---"); i >= 0 {
			fmText = rest[:i]
			body = strings.TrimLeft(rest[i+4:], "\n")
		} else {
			fmText = rest
		}
	} else {
		body = s
	}
	var fm frontmatter
	if fmText != "" {
		if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
			return Fact{}, fmt.Errorf("%s: frontmatter: %w", filepath.Base(path), err)
		}
	}
	f := Fact{
		Name:         fm.Name,
		Description:  fm.Description,
		Type:         fm.Type,
		Category:     fm.Category,
		Origin:       fm.Origin,
		Supersedes:   fm.Supersedes,
		SupersededBy: fm.SupersededBy,
		Body:         strings.TrimRight(body, "\n"),
		Path:         path,
	}
	// Nested-metadata shape: fall back to metadata.type when top-level is empty.
	if f.Type == "" && fm.Metadata != nil {
		if t, ok := fm.Metadata["type"].(string); ok {
			f.Type = t
		}
	}
	if f.Name == "" {
		f.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	if f.Category == "" {
		f.Category = defaultCategory(f.Type)
	}
	return f, nil
}

func defaultCategory(t string) string {
	switch t {
	case "feedback":
		return "Feedback"
	case "reference":
		return "References"
	case "project":
		return "Projects"
	case "incident":
		return "Incidents"
	case "user":
		return "User"
	default:
		return "Other"
	}
}

// LoadDir loads every top-level *.md fact in dir, skipping the MEMORY.md index.
func LoadDir(dir string) ([]Fact, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var facts []Fact
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "MEMORY.md" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		f, err := ParseFact(p, data)
		if err != nil {
			return nil, err
		}
		facts = append(facts, f)
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].Name < facts[j].Name })
	return facts, nil
}
