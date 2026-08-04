package migrate

import (
	"github.com/mirhan/a2migrate/internal/domain"
	"github.com/mirhan/a2migrate/internal/source/claudecode"
	"github.com/mirhan/a2migrate/internal/target/opencode"
)

// ArtifactsReport summarises what an artifact migration run produced.
type ArtifactsReport struct {
	SkillsWritten   []string
	CommandsWritten []string
	AgentsWritten   []string
	RulesWritten    []string
	MCPMerged       []string
	DryRun          bool
}

// ArtifactsMigrator coordinates the non-session artifact migration:
// skills, commands, agents, rules, and MCP servers.
type ArtifactsMigrator struct {
	CWD    string
	DryRun bool
}

// NewArtifactsMigrator returns an artifacts migrator rooted at cwd.
func NewArtifactsMigrator(cwd string) *ArtifactsMigrator {
	return &ArtifactsMigrator{CWD: cwd}
}

// Migrate reads everything from Claude Code, applies renames via title
// fallback (not applicable to artifacts), and writes to OpenCode. Returns
// a per-domain list of written/merged file paths.
func (m *ArtifactsMigrator) Migrate() (*ArtifactsReport, error) {
	rep := &ArtifactsReport{DryRun: m.DryRun}
	if m.DryRun {
		return rep, nil
	}

	// Skills.
	if global, err := claudecode.ReadGlobalSkills(); err != nil {
		return rep, err
	} else if len(global) > 0 {
		w := opencode.NewSkillWriter()
		written, err := w.WriteGlobal(global)
		if err != nil {
			return rep, err
		}
		rep.SkillsWritten = written
	}
	if project, err := claudecode.ReadProjectSkills(m.CWD); err != nil {
		return rep, err
	} else if len(project) > 0 {
		w := opencode.NewSkillWriter()
		w.WorkDir = m.CWD
		written, err := w.WriteProject(project)
		if err != nil {
			return rep, err
		}
		rep.SkillsWritten = append(rep.SkillsWritten, written...)
	}

	// Commands.
	if global, err := claudecode.ReadGlobalCommands(); err != nil {
		return rep, err
	} else if len(global) > 0 {
		w := opencode.NewCommandWriter()
		written, err := w.WriteGlobal(global)
		if err != nil {
			return rep, err
		}
		rep.CommandsWritten = written
	}
	if project, err := claudecode.ReadProjectCommands(m.CWD); err != nil {
		return rep, err
	} else if len(project) > 0 {
		w := opencode.NewCommandWriter()
		w.WorkDir = m.CWD
		written, err := w.WriteProject(project)
		if err != nil {
			return rep, err
		}
		rep.CommandsWritten = append(rep.CommandsWritten, written...)
	}

	// Agents.
	if global, err := claudecode.ReadGlobalAgents(); err != nil {
		return rep, err
	} else if len(global) > 0 {
		w := opencode.NewAgentWriter()
		written, err := w.WriteGlobal(global)
		if err != nil {
			return rep, err
		}
		rep.AgentsWritten = written
	}
	if project, err := claudecode.ReadProjectAgents(m.CWD); err != nil {
		return rep, err
	} else if len(project) > 0 {
		w := opencode.NewAgentWriter()
		w.WorkDir = m.CWD
		written, err := w.WriteProject(project)
		if err != nil {
			return rep, err
		}
		rep.AgentsWritten = append(rep.AgentsWritten, written...)
	}

	// Rules.
	if global, err := claudecode.ReadGlobalRules(); err != nil {
		return rep, err
	} else if len(global) > 0 {
		w := opencode.NewRuleWriter()
		written, err := w.WriteGlobal(global)
		if err != nil {
			return rep, err
		}
		rep.RulesWritten = written
	}
	if project, err := claudecode.ReadProjectRules(m.CWD); err != nil {
		return rep, err
	} else if len(project) > 0 {
		w := opencode.NewRuleWriter()
		w.WorkDir = m.CWD
		written, err := w.WriteProject(project)
		if err != nil {
			return rep, err
		}
		rep.RulesWritten = append(rep.RulesWritten, written...)
	}

	// MCP.
	if servers, err := claudecode.ReadGlobalMCP(); err != nil {
		return rep, err
	} else if len(servers) > 0 {
		w := opencode.NewMCPConfigWriter()
		patch := opencode.MCPConfigPatch{Servers: servers}
		if _, err := w.Apply(patch); err != nil {
			return rep, err
		}
		rep.MCPMerged = serverNames(servers)
	}

	return rep, nil
}

func serverNames(xs []domain.MCPServer) []string {
	out := make([]string, 0, len(xs))
	for _, s := range xs {
		out = append(out, s.Name)
	}
	return out
}