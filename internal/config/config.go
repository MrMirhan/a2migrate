// Package config loads the multi-endpoint sync configuration a2migrate
// uses to push state to (or pull state from) remote machines and
// multiple co-installed CLIs.
//
// Status: experimental. The schema documented here is what we'll cut
// when `a2migrate remote sync` ships. Until then, only single-endpoint
// migrations work; the file format is forward-compatible.
//
// File format: TOML. Schema version 1.
//
//	version = 1
//	[[endpoint]]
//	id   = "vds1"
//	kind = "ssh"
//	host = "10.0.0.5"
//	user = "alice"
//	path = "/home/alice/.local/share/a2migrate"
//	tools = ["claude_code", "opencode"]
//
//	[[endpoint]]
//	id   = "workstation-b"
//	kind = "local"
//	path = "/Volumes/external/a2migrate"
//	tools = ["opencode"]
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/MrMirhan/a2migrate/internal/tools"
)

// Version is the schema revision.
const Version = 1

// EndpointKind enumerates how a2migrate reaches an endpoint.
type EndpointKind string

const (
	KindLocal EndpointKind = "local" // same machine, just a different path
	KindSSH   EndpointKind = "ssh"   // remote machine over SSH
)

// Endpoint is one target machine, optionally scoped to a subset of tools.
type Endpoint struct {
	ID    string       `toml:"id"`
	Kind  EndpointKind `toml:"kind"`
	Host  string       `toml:"host,omitempty"`  // SSH only
	User  string       `toml:"user,omitempty"`  // SSH only
	Path  string       `toml:"path"`            // a2migrate data root on the endpoint
	Tools []tools.ID   `toml:"tools,omitempty"` // empty = all known tools
}

// Validate enforces required fields.
func (e Endpoint) Validate() error {
	if e.ID == "" {
		return errors.New("endpoint.id required")
	}
	switch e.Kind {
	case KindLocal:
		// path required; host/user ignored
	case KindSSH:
		if e.Host == "" {
			return fmt.Errorf("endpoint %q: kind=ssh requires host", e.ID)
		}
	default:
		return fmt.Errorf("endpoint %q: unknown kind %q", e.ID, e.Kind)
	}
	if e.Path == "" {
		return fmt.Errorf("endpoint %q: path required", e.ID)
	}
	return nil
}

// File is the top-level config.
type File struct {
	Version   int        `toml:"version"`
	Endpoints []Endpoint `toml:"endpoint"`
}

// Load reads and parses the given TOML file.
func Load(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return Parse(f, path)
}

// Parse reads and parses TOML from r. source is used only in errors.
func Parse(r io.Reader, source string) (*File, error) {
	decoded, err := parseTOML(r)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	out := &File{Version: decoded.Version, Endpoints: decoded.Endpoints}
	if out.Version != Version {
		return nil, fmt.Errorf("%s: unsupported version %d (want %d)", source, out.Version, Version)
	}
	for _, e := range out.Endpoints {
		if err := e.Validate(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// LookupTool returns the endpoints that are configured for the given
// tool. Endpoints with an empty Tools list match every tool.
func (f *File) LookupTool(t tools.ID) []Endpoint {
	var out []Endpoint
	for _, e := range f.Endpoints {
		if len(e.Tools) == 0 {
			out = append(out, e)
			continue
		}
		for _, want := range e.Tools {
			if want == t {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

// ExamplePath returns where the example.toml would live beside the
// running binary. Useful for `a2migrate init` writing a template.
func ExamplePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "./example.toml"
	}
	return filepath.Join(filepath.Dir(exe), "example.toml")
}
