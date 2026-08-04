// Hand-rolled TOML parser for the small subset a2migrate needs.
//
// Schema recognized:
//
//	version = 1
//	[[endpoint]]
//	id   = "..."
//	kind = "local" | "ssh"
//	host = "..."     # ssh only
//	user = "..."     # ssh only
//	path = "..."
//	tools = ["a", "b"]
//
// When we ship the production-grade sync we'll swap this for
// BurntSushi/toml or pelletier/go-toml. For now, the schema is the
// only TOML we need and the parser is ~80 lines.
package config

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/MrMirhan/a2migrate/internal/tools"
)

// decodedFile is the in-memory form produced by parseTOML.
type decodedFile struct {
	Version   int
	Endpoints []Endpoint
}

// parseTOML decodes the subset.
func parseTOML(r io.Reader) (*decodedFile, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	out := &decodedFile{}
	var cur *Endpoint
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			name := strings.TrimSpace(line[2 : len(line)-2])
			if name != "endpoint" {
				return nil, fmt.Errorf("unsupported table: %q", name)
			}
			out.Endpoints = append(out.Endpoints, Endpoint{})
			cur = &out.Endpoints[len(out.Endpoints)-1]
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			return nil, fmt.Errorf("malformed line: %q", line)
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		v = stripQuote(v)
		var toolsList []tools.ID
		_ = toolsList
		switch {
		case cur == nil && k == "version":
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("version not int: %q", v)
			}
			out.Version = n
		case cur != nil:
			switch k {
			case "id":
				cur.ID = v
			case "kind":
				cur.Kind = EndpointKind(v)
			case "host":
				cur.Host = v
			case "user":
				cur.User = v
			case "path":
				cur.Path = v
			case "tools":
				for _, s := range splitList(v) {
					cur.Tools = append(cur.Tools, tools.ID(s))
				}
			default:
				return nil, fmt.Errorf("unknown key %q", k)
			}
		default:
			return nil, fmt.Errorf("unknown top-level key %q", k)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func stripQuote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func splitList(v string) []string {
	v = strings.Trim(v, "[]")
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		s = strings.Trim(s, "\"'")
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
