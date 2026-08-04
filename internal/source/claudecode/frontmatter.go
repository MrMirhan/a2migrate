package claudecode

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter splits a markdown file into a YAML frontmatter map and the
// remaining body. Files without a frontmatter block return nil, body, nil.
type Frontmatter struct {
	Raw  map[string]any
	Body string
}

const frontmatterDelim = "---"

// ParseFrontmatter extracts YAML frontmatter. Recognized delimiters are
// "---" on both ends; only the first block is consumed.
func ParseFrontmatter(text string) (Frontmatter, error) {
	// Normalize line endings.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, frontmatterDelim+"\n") && !strings.HasPrefix(text, frontmatterDelim+"\r\n") {
		return Frontmatter{Body: text}, nil
	}
	// Find closing delimiter on its own line.
	rest := text[len(frontmatterDelim)+1:]
	idx := strings.Index(rest, "\n"+frontmatterDelim+"\n")
	if idx < 0 {
		idx = strings.Index(rest, "\n"+frontmatterDelim)
		if idx < 0 {
			return Frontmatter{Body: text}, nil
		}
	}
	yamlText := rest[:idx]
	body := rest[idx+len("\n"+frontmatterDelim):]
	body = strings.TrimLeft(body, "\n")

	var raw map[string]any
	if err := yaml.Unmarshal([]byte(yamlText), &raw); err != nil {
		return Frontmatter{Body: text}, err
	}
	return Frontmatter{Raw: raw, Body: body}, nil
}

// StringField returns a string field from frontmatter, or "" if missing/wrong type.
func (f Frontmatter) StringField(key string) string {
	if f.Raw == nil {
		return ""
	}
	v, ok := f.Raw[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// StringSliceField returns a []string field, or nil.
func (f Frontmatter) StringSliceField(key string) []string {
	if f.Raw == nil {
		return nil
	}
	v, ok := f.Raw[key]
	if !ok {
		return nil
	}
	switch xs := v.(type) {
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return xs
	}
	return nil
}