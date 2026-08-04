package cli

import (
	"github.com/mirhan/a2migrate/internal/target/claudecode"
)

// platformOpen returns a small facade for writing CC-side artifact files.
// Implemented as a free function so the cli package doesn't have to know
// about target/claudecode's internal layout.
func platformOpen() *ccFacade {
	return &ccFacade{}
}

type ccFacade struct{}

func (ccFacade) SkillWriterFor(cwd string) *claudecode.SkillWriter {
	w := claudecode.NewSkillWriter()
	w.WorkDir = cwd
	return w
}

func (ccFacade) CommandWriterFor(cwd string) *claudecode.CommandWriter {
	w := claudecode.NewCommandWriter()
	w.WorkDir = cwd
	return w
}

func (ccFacade) AgentWriterFor(cwd string) *claudecode.AgentWriter {
	w := claudecode.NewAgentWriter()
	w.WorkDir = cwd
	return w
}

func (ccFacade) RuleWriterFor(cwd string) *claudecode.RuleWriter {
	w := claudecode.NewRuleWriter()
	w.WorkDir = cwd
	return w
}

// targetCCPath returns a CC-side MCP writer pointing at the given home.
func targetCCPath(ccHome string) *claudecode.MCPConfigWriter {
	w := claudecode.NewMCPConfigWriter()
	if ccHome != "" {
		// Adjust the path so it lands in the requested CC home.
		// The default path uses platform.ClaudeCodeMCPPath(); we
		// rewrite it here for tests and overrides.
		w.Path = ccHome + "/mcp.json"
	}
	return w
}
