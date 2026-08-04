package opencode

import (
	"time"

	"gopkg.in/yaml.v3"
)

// yamlUnmarshal is the indirection that lets the OC source reader reuse
// the same YAML frontmatter semantics as the CC reader.
func yamlUnmarshal(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}

// unixMillis converts unix-ms to time.Time (UTC).
func unixMillis(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}
