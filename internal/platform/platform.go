// Package platform abstracts OS-specific path conventions used by Claude Code
// and OpenCode. The goal is to keep the rest of the codebase OS-agnostic.
//
// Conventions:
//
//	Linux  : XDG ($XDG_DATA_HOME, $XDG_CONFIG_HOME, $XDG_STATE_HOME)
//	macOS  : $HOME/Library/Application Support / $HOME/Library/Preferences
//	Windows: %AppData% / %LocalAppData%
package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// OS identifies the runtime operating system.
type OS string

const (
	Linux   OS = "linux"
	Darwin  OS = "darwin"
	Windows OS = "windows"
	Unknown OS = "unknown"
)

// Current returns the OS the binary was built for or is running on.
func Current() OS {
	switch runtime.GOOS {
	case "linux":
		return Linux
	case "darwin":
		return Darwin
	case "windows":
		return Windows
	default:
		return Unknown
	}
}

// Home returns the user's home directory or "" if it cannot be determined.
func Home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

// EnvOr returns the env var value or fallback if unset/empty.
func EnvOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// dataDir returns the OS-specific per-user data directory root.
func dataDir() string {
	switch Current() {
	case Windows:
		return EnvOr("AppData", filepath.Join(Home(), "AppData", "Roaming"))
	case Darwin:
		return filepath.Join(Home(), "Library", "Application Support")
	default:
		return EnvOr("XDG_DATA_HOME", filepath.Join(Home(), ".local", "share"))
	}
}

// configDir returns the OS-specific per-user config directory root.
func configDir() string {
	switch Current() {
	case Windows:
		return EnvOr("AppData", filepath.Join(Home(), "AppData", "Roaming"))
	case Darwin:
		return filepath.Join(Home(), "Library", "Preferences")
	default:
		return EnvOr("XDG_CONFIG_HOME", filepath.Join(Home(), ".config"))
	}
}

// stateDir returns the OS-specific per-user state directory root.
func stateDir() string {
	switch Current() {
	case Windows:
		return EnvOr("LocalAppData", filepath.Join(Home(), "AppData", "Local"))
	case Darwin:
		return filepath.Join(Home(), "Library", "Application Support")
	default:
		return EnvOr("XDG_STATE_HOME", filepath.Join(Home(), ".local", "state"))
	}
}

// ClaudeCodeHome returns the root of Claude Code's state directory.
// Override with $CLAUDE_CODE_HOME.
func ClaudeCodeHome() string {
	if v := EnvOr("CLAUDE_CODE_HOME", ""); v != "" {
		return v
	}
	return filepath.Join(Home(), ".claude")
}

// OpenCodeDataHome returns the root of OpenCode's data directory.
// Override with $OPENCODE_DATA_HOME.
func OpenCodeDataHome() string {
	if v := EnvOr("OPENCODE_DATA_HOME", ""); v != "" {
		return v
	}
	return filepath.Join(dataDir(), "opencode")
}

// OpenCodeConfigHome returns the root of OpenCode's config directory.
// Override with $OPENCODE_CONFIG_HOME.
func OpenCodeConfigHome() string {
	if v := EnvOr("OPENCODE_CONFIG_HOME", ""); v != "" {
		return v
	}
	return filepath.Join(configDir(), "opencode")
}

// OpenCodeDBPath returns the path to opencode.db.
func OpenCodeDBPath() string {
	return filepath.Join(OpenCodeDataHome(), "opencode.db")
}

// OpenCodeConfigPath returns the path to the user config file (opencode.json / opencode.jsonc).
func OpenCodeConfigPath() string {
	return filepath.Join(OpenCodeConfigHome(), "opencode.json")
}

// ClaudeCodeProjectsDir returns the path containing encoded-cwd project dirs.
func ClaudeCodeProjectsDir() string {
	return filepath.Join(ClaudeCodeHome(), "projects")
}

// ClaudeCodeSkillsDir returns the global skills directory.
func ClaudeCodeSkillsDir() string {
	return filepath.Join(ClaudeCodeHome(), "skills")
}

// ClaudeCodeSettingsPath returns the global settings file.
func ClaudeCodeSettingsPath() string {
	return filepath.Join(ClaudeCodeHome(), "settings.json")
}

// ClaudeCodeMCPPath returns the global mcp.json file.
func ClaudeCodeMCPPath() string {
	return filepath.Join(ClaudeCodeHome(), "mcp.json")
}

// EncodeCWD converts an absolute path into Claude Code's encoded-cwd form.
//
//	"/home/mirhan/works" -> "-home-mirhan-works"
func EncodeCWD(absPath string) string {
	cleaned := filepath.Clean(absPath)
	if !filepath.IsAbs(cleaned) {
		return strings.ReplaceAll(strings.ReplaceAll(cleaned, "/", "-"), "\\", "-")
	}
	// Use forward slashes regardless of host OS for cross-platform stability.
	cleaned = filepath.ToSlash(cleaned)
	return strings.ReplaceAll(cleaned, "/", "-")
}

// DecodeCWD is the inverse of EncodeCWD.
func DecodeCWD(encoded string) string {
	if encoded == "" {
		return ""
	}
	if strings.HasPrefix(encoded, "-") {
		return "/" + strings.ReplaceAll(encoded[1:], "-", "/")
	}
	return "/" + strings.ReplaceAll(encoded, "-", "/")
}
