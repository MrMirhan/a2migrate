package opencode

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	filenameBadChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)
	frontmatterDelim = "---"
)

// sanitizeFilename strips characters that would break a cross-platform
// filename. Lowercases to match the a2migrate conventions seen in the
// existing OC config.
func sanitizeFilename(s string) string {
	s = filenameBadChars.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "untitled"
	}
	return strings.ToLower(s)
}

// renderFrontmatter turns a map into a YAML frontmatter block. Empty
// maps return "".
func renderFrontmatter(fm map[string]any) string {
	if len(fm) == 0 {
		return ""
	}
	// Use the most natural JSON-ish rendering for primitives; nested objects
	// are serialized as compact JSON which is valid YAML.
	var b strings.Builder
	b.WriteString(frontmatterDelim + "\n")
	for _, k := range sortedKeys(fm) {
		v := fm[k]
		switch tv := v.(type) {
		case string:
			fmt.Fprintf(&b, "%s: %q\n", k, tv)
		case []string:
			fmt.Fprintf(&b, "%s: %s\n", k, renderStringSlice(tv))
		default:
			jb, err := json.Marshal(v)
			if err != nil {
				continue
			}
			fmt.Fprintf(&b, "%s: %s\n", k, string(jb))
		}
	}
	b.WriteString(frontmatterDelim + "\n")
	return b.String()
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Insertion sort for stability across Go versions.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}

func renderStringSlice(xs []string) string {
	if len(xs) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteString("[")
	for i, s := range xs {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", s)
	}
	b.WriteString("]")
	return b.String()
}

// extractName / extractBody are reflection-free helpers using type switches
// over the small union of markdown-writeable items.

// extractName returns the canonical name field for any of the supported
// writer item types.
func extractName(v any) string {
	switch x := v.(type) {
	case domainSkill:
		return x.Name
	case domainCommand:
		return x.Name
	case domainAgent:
		return x.Name
	case domainRule:
		return x.Name
	}
	return ""
}

// extractBody returns the markdown body for any of the supported types.
func extractBody(v any) string {
	switch x := v.(type) {
	case domainSkill:
		return x.Body
	case domainCommand:
		return x.Body
	case domainAgent:
		return x.Body
	case domainRule:
		return x.Body
	}
	return ""
}
