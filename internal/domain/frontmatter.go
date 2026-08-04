package domain

// FrontmatterString returns a string field from the frontmatter, or "".
func (s Skill) FrontmatterString(key string) string {
	if s.Frontmatter == nil {
		return ""
	}
	if v, ok := s.Frontmatter[key].(string); ok {
		return v
	}
	return ""
}

// FrontmatterStringSlice returns a []string field, or nil.
func (s Skill) FrontmatterStringSlice(key string) []string {
	if s.Frontmatter == nil {
		return nil
	}
	v, ok := s.Frontmatter[key]
	if !ok {
		return nil
	}
	switch xs := v.(type) {
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			if str, ok := x.(string); ok {
				out = append(out, str)
			}
		}
		return out
	case []string:
		return xs
	}
	return nil
}
