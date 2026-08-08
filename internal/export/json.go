package export

import (
	"encoding/json"
	"io"
)

func writeJSON(w io.Writer, b Bundle) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	// Tool output is read by humans and by scripts; escaping < and & to
	// < makes both worse and buys nothing outside HTML.
	enc.SetEscapeHTML(false)
	return enc.Encode(b)
}
