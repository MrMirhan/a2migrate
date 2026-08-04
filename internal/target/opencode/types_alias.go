package opencode

import "github.com/MrMirhan/a2migrate/internal/domain"

// Type aliases give the render helper a stable interface to type-switch on,
// without forcing every writer to import domain just to satisfy the same
// union.
type (
	domainSkill   = domain.Skill
	domainCommand = domain.Command
	domainAgent   = domain.AgentDef
	domainRule    = domain.Rule
)
