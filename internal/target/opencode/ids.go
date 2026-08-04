// Package opencode writes migrated state into an OpenCode SQLite database.
//
// Two responsibilities live here:
//
//	SessionWriter — builds a Plan of INSERTs for a batch of sessions and
//	                applies them in a single transaction with a backup.
//	Repair        — runs the four post-fix invariants the renderer requires
//	                (reparent, pad step parts, step-start time, tool state
//	                time) against already-migrated rows.
package opencode

import (
	"crypto/sha1"
	"encoding/base32"
	"encoding/hex"
	"hash/fnv"
	"strings"

	"github.com/MrMirhan/a2migrate/internal/source/claudecode"
)

const (
	idBodyLen     = 26
	bodyLowerBase = "abcdefghijklmnopqrstuvwxyz234567"
)

var enc = base32.NewEncoding(bodyLowerBase).WithPadding(base32.NoPadding)

// GenID returns the canonical OpenCode id "<prefix>_<26-char lower-base32>".
//
// The body is derived from sha1(seed + ":" + prefix). If the resulting id
// is already in `existing`, a counter-suffix is appended to disambiguate.
// `existing` is mutated.
func GenID(prefix, seed string, existing map[string]struct{}) string {
	h := sha1Sum(seed + ":" + prefix)
	body := strings.ToLower(enc.EncodeToString(h))[:idBodyLen]
	id := prefix + "_" + body
	if _, ok := existing[id]; !ok {
		existing[id] = struct{}{}
		return id
	}
	for i := 0; i < 1000; i++ {
		suffix := collisionSuffix(i)
		if len(suffix) >= idBodyLen {
			break
		}
		truncated := body[:idBodyLen-len(suffix)] + suffix
		id = prefix + "_" + truncated
		if _, ok := existing[id]; !ok {
			existing[id] = struct{}{}
			return id
		}
	}
	// Last-resort fallback: append a 6-char fragment derived from hash+counter.
	for i := 1000; ; i++ {
		suffix := collisionSuffix(i)
		if idBodyLen <= len(suffix) {
			suffix = suffix[:idBodyLen]
		}
		body2 := body
		if len(body2) > idBodyLen-len(suffix) {
			body2 = body2[:idBodyLen-len(suffix)]
		}
		id = prefix + "_" + body2 + suffix
		if _, ok := existing[id]; !ok {
			existing[id] = struct{}{}
			return id
		}
	}
}

// collisionSuffix returns a deterministic, monotonically-growing suffix for
// disambiguating colliding ids. "-00" → "-99" → "-0a0" → …
func collisionSuffix(i int) string {
	if i < 100 {
		const digits = "0123456789"
		return "-" + string(digits[i/10]) + string(digits[i%10])
	}
	// For >= 100, use lowercase base32 fragment.
	return "-" + strings.ToLower(enc.EncodeToString([]byte{
		byte(i >> 8), byte(i & 0xff),
	}))[:3]
}

func sha1Sum(s string) []byte {
	h := sha1.Sum([]byte(s))
	return h[:]
}

// ProjectIDForWorktree is re-exported here so target writers and source
// readers agree on the same id derivation.
func ProjectIDForWorktree(worktree string) string {
	return claudecode.ProjectIDForWorktree(worktree)
}

// ProjectHash returns the OpenCode project id for a non-global worktree.
// Worktree "/" yields "global" (no hash).
func ProjectHash(worktree string) string {
	return ProjectIDForWorktree(worktree)
}

// IsProjectGlobal returns true when the project id is the literal "global"
// placeholder used for the root worktree.
func IsProjectGlobal(id string) bool {
	return id == "global"
}

// Hash16 returns a stable hex digest for arbitrary input. Used as part of
// seed material for IDs derived from non-UUID sources.
func Hash16(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
