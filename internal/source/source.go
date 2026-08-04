// Package source reads Claude Code state from disk and produces
// domain.Session, domain.Skill, domain.Command, domain.AgentDef,
// domain.Rule, and domain.MCPServer values.
//
// Each subpackage under source/ is a pure reader: it knows the source
// system's on-disk layout and produces domain types. It does not write
// anywhere.
package source