package migrate

import (
	"fmt"

	"github.com/MrMirhan/a2migrate/internal/domain"
	"github.com/MrMirhan/a2migrate/internal/source/claudecode"
	"github.com/MrMirhan/a2migrate/internal/target/opencode"
)

// Domain identifies one category of Claude Code artifact that
// ArtifactsMigrator can migrate to OpenCode.
type Domain string

const (
	DomainSkills   Domain = "skills"
	DomainCommands Domain = "commands"
	DomainAgents   Domain = "agents"
	DomainRules    Domain = "rules"
	DomainMCP      Domain = "mcp"
	DomainSystem   Domain = "system"
)

// ArtifactsReport summarises what an artifact migration run produced.
type ArtifactsReport struct {
	SkillsWritten       []string
	CommandsWritten     []string
	AgentsWritten       []string
	RulesWritten        []string
	MCPMerged           []string
	SystemPromptWritten string
	DryRun              bool
}

// ArtifactsMigrator coordinates the non-session artifact migration:
// skills, commands, agents, rules, MCP, and the top-level system prompt.
type ArtifactsMigrator struct {
	CWD    string
	DryRun bool

	// Domains restricts Migrate to the listed domains. A nil or empty
	// slice migrates every domain (the historical, unscoped default).
	Domains []Domain
}

// NewArtifactsMigrator returns an artifacts migrator rooted at cwd.
func NewArtifactsMigrator(cwd string) *ArtifactsMigrator {
	return &ArtifactsMigrator{CWD: cwd}
}

// domainMigration pairs a Domain with the function that migrates it.
type domainMigration struct {
	domain Domain
	run    func(*ArtifactsMigrator, *ArtifactsReport) error
}

// domainMigrators is the data-driven dispatch table Migrate walks. Order
// matters: it's the sequence used when Domains is left unset, and it
// matches the historical hand-written order (skills, commands, agents,
// rules, mcp, system).
var domainMigrators = []domainMigration{
	{DomainSkills, (*ArtifactsMigrator).migrateSkills},
	{DomainCommands, (*ArtifactsMigrator).migrateCommands},
	{DomainAgents, (*ArtifactsMigrator).migrateAgents},
	{DomainRules, (*ArtifactsMigrator).migrateRules},
	{DomainMCP, (*ArtifactsMigrator).migrateMCP},
	{DomainSystem, (*ArtifactsMigrator).migrateSystem},
}

// migratorFor looks up the migration function for a domain.
func migratorFor(d Domain) (domainMigration, bool) {
	for _, dm := range domainMigrators {
		if dm.domain == d {
			return dm, true
		}
	}
	return domainMigration{}, false
}

// Migrate reads everything from Claude Code, applies renames via title
// fallback (not applicable to artifacts), and writes to OpenCode. Returns
// a per-domain list of written/merged file paths. Only the domains listed
// in m.Domains run; a nil or empty Domains runs all of them.
func (m *ArtifactsMigrator) Migrate() (*ArtifactsReport, error) {
	rep := &ArtifactsReport{DryRun: m.DryRun}
	if m.DryRun {
		return rep, nil
	}

	table := domainMigrators
	if len(m.Domains) > 0 {
		table = make([]domainMigration, 0, len(m.Domains))
		for _, d := range m.Domains {
			dm, ok := migratorFor(d)
			if !ok {
				return rep, fmt.Errorf("migrate: unknown artifact domain %q", d)
			}
			table = append(table, dm)
		}
	}

	for _, dm := range table {
		if err := dm.run(m, rep); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

func (m *ArtifactsMigrator) migrateSkills(rep *ArtifactsReport) error {
	if global, err := claudecode.ReadGlobalSkills(); err != nil {
		return err
	} else if len(global) > 0 {
		w := opencode.NewSkillWriter()
		written, err := w.WriteGlobal(global)
		if err != nil {
			return err
		}
		rep.SkillsWritten = written
	}
	if project, err := claudecode.ReadProjectSkills(m.CWD); err != nil {
		return err
	} else if len(project) > 0 {
		w := opencode.NewSkillWriter()
		w.WorkDir = m.CWD
		written, err := w.WriteProject(project)
		if err != nil {
			return err
		}
		rep.SkillsWritten = append(rep.SkillsWritten, written...)
	}
	return nil
}

func (m *ArtifactsMigrator) migrateCommands(rep *ArtifactsReport) error {
	if global, err := claudecode.ReadGlobalCommands(); err != nil {
		return err
	} else if len(global) > 0 {
		w := opencode.NewCommandWriter()
		written, err := w.WriteGlobal(global)
		if err != nil {
			return err
		}
		rep.CommandsWritten = written
	}
	if project, err := claudecode.ReadProjectCommands(m.CWD); err != nil {
		return err
	} else if len(project) > 0 {
		w := opencode.NewCommandWriter()
		w.WorkDir = m.CWD
		written, err := w.WriteProject(project)
		if err != nil {
			return err
		}
		rep.CommandsWritten = append(rep.CommandsWritten, written...)
	}
	return nil
}

func (m *ArtifactsMigrator) migrateAgents(rep *ArtifactsReport) error {
	if global, err := claudecode.ReadGlobalAgents(); err != nil {
		return err
	} else if len(global) > 0 {
		w := opencode.NewAgentWriter()
		written, err := w.WriteGlobal(global)
		if err != nil {
			return err
		}
		rep.AgentsWritten = written
	}
	if project, err := claudecode.ReadProjectAgents(m.CWD); err != nil {
		return err
	} else if len(project) > 0 {
		w := opencode.NewAgentWriter()
		w.WorkDir = m.CWD
		written, err := w.WriteProject(project)
		if err != nil {
			return err
		}
		rep.AgentsWritten = append(rep.AgentsWritten, written...)
	}
	return nil
}

func (m *ArtifactsMigrator) migrateRules(rep *ArtifactsReport) error {
	if global, err := claudecode.ReadGlobalRules(); err != nil {
		return err
	} else if len(global) > 0 {
		w := opencode.NewRuleWriter()
		written, err := w.WriteGlobal(global)
		if err != nil {
			return err
		}
		rep.RulesWritten = written
	}
	if project, err := claudecode.ReadProjectRules(m.CWD); err != nil {
		return err
	} else if len(project) > 0 {
		w := opencode.NewRuleWriter()
		w.WorkDir = m.CWD
		written, err := w.WriteProject(project)
		if err != nil {
			return err
		}
		rep.RulesWritten = append(rep.RulesWritten, written...)
	}
	return nil
}

func (m *ArtifactsMigrator) migrateMCP(rep *ArtifactsReport) error {
	if servers, err := claudecode.ReadGlobalMCP(); err != nil {
		return err
	} else if len(servers) > 0 {
		w := opencode.NewMCPConfigWriter()
		patch := opencode.MCPConfigPatch{Servers: servers}
		if _, err := w.Apply(patch); err != nil {
			return err
		}
		rep.MCPMerged = serverNames(servers)
	}
	return nil
}

func (m *ArtifactsMigrator) migrateSystem(rep *ArtifactsReport) error {
	if prompt, err := claudecode.ReadGlobalSystemPrompt(); err != nil {
		return err
	} else if prompt != nil {
		w := opencode.NewSystemPromptWriter()
		if out, err := w.Write(prompt); err != nil {
			return err
		} else if out != "" {
			rep.SystemPromptWritten = out
		}
	}
	return nil
}

func serverNames(xs []domain.MCPServer) []string {
	out := make([]string, 0, len(xs))
	for _, s := range xs {
		out = append(out, s.Name)
	}
	return out
}
