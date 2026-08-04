package claudecode

import "time"

// timeT is a tiny alias so tests can reference ms() helpers without
// dragging the full time package into every signature.
type timeT = time.Time

func parseTime(s string) timeT {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
