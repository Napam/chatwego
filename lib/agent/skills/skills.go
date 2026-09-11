// Package skills loads agent skills from the markdown files in this
// directory: each is a chunk of instructions the model can fetch on demand
// (via the "skill" tool in lib/agent/tools) instead of every skill's full
// body being in the system prompt all the time. Add a skill by adding a new
// .md file here -- no code change needed.
package skills

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed *.md
var files embed.FS

// Skill is a markdown-defined capability the agent can fetch by name. Name
// and Description come from the file's YAML frontmatter and are surfaced in
// the system prompt so the model knows what's available; Body is the
// markdown after the frontmatter -- the actual instructions handed back to
// the model when it fetches this skill.
type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Body        string `yaml:"-"`
}

// All is every loaded skill, keyed by name.
var All = mustLoad()

func mustLoad() map[string]Skill {
	loaded, err := load(files)
	if err != nil {
		panic("failed to load skills: " + err.Error())
	}
	return loaded
}

func load(fsys embed.FS) (map[string]Skill, error) {
	entries, err := fsys.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	loaded := make(map[string]Skill, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		raw, err := fsys.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		skill, err := parse(raw)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", entry.Name(), err)
		}
		loaded[skill.Name] = skill
	}
	return loaded, nil
}

// parse splits a skill file into its YAML frontmatter and markdown body.
// Frontmatter is delimited by a "---" line at the very start of the file and
// a matching "---" line that ends it, e.g.:
//
//	---
//	name: example
//	description: What this skill is for.
//	---
//	The actual instructions, as markdown.
func parse(raw []byte) (Skill, error) {
	const delim = "---"
	text := string(raw)
	if !strings.HasPrefix(text, delim) {
		return Skill{}, fmt.Errorf("missing frontmatter delimiter %q", delim)
	}
	rest := text[len(delim):]
	end := strings.Index(rest, "\n"+delim)
	if end == -1 {
		return Skill{}, fmt.Errorf("missing closing frontmatter delimiter %q", delim)
	}

	var skill Skill
	if err := yaml.Unmarshal([]byte(rest[:end]), &skill); err != nil {
		return Skill{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	if skill.Name == "" {
		return Skill{}, fmt.Errorf("frontmatter missing required 'name'")
	}

	skill.Body = strings.TrimSpace(rest[end+1+len(delim):])
	return skill, nil
}

// Find looks up a loaded skill by name.
func Find(name string) (Skill, bool) {
	skill, ok := All[name]
	return skill, ok
}

// Sorted returns every loaded skill sorted by name, for deterministic
// listing (e.g. in the system prompt).
func Sorted() []Skill {
	sorted := make([]Skill, 0, len(All))
	for _, skill := range All {
		sorted = append(sorted, skill)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	return sorted
}
