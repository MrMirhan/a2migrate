package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/MrMirhan/a2migrate/internal/domain"
	"github.com/MrMirhan/a2migrate/internal/migrate"
	"github.com/MrMirhan/a2migrate/internal/platform"
	ocsrc "github.com/MrMirhan/a2migrate/internal/source/opencode"
	cctgt "github.com/MrMirhan/a2migrate/internal/target/claudecode"
)

func printReverseReport(cmd *cobra.Command, r *migrate.ReverseReport) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "discovered=%d selected=%d successes=%d failures=%d\n",
		r.Discovered, r.Selected, r.Successes, r.Failures)
	if r.BackupDir != "" {
		_, _ = fmt.Fprintf(out, "backup=%s\n", r.BackupDir)
	}
	for _, s := range r.Results {
		switch {
		case s.Error != nil:
			_, _ = fmt.Fprintf(out, "FAIL %s: %v\n", s.OCSessionID, s.Error)
		case r.DryRun:
			_, _ = fmt.Fprintf(out, "DRY  %s\t%s\n", s.OCSessionID, s.OutputPath)
		default:
			tag := ""
			if s.IsSubagent {
				tag = " [subagent]"
			}
			_, _ = fmt.Fprintf(out, "OK   %s\torigin=%s%s\t%s\n",
				s.OCSessionID, s.OriginID, tag, s.OutputPath)
		}
	}
}

func printVerifyGroup(cmd *cobra.Command, label string, rows []migrate.ReverseVerifyRow) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "[%s] %d session(s)\n", label, len(rows))
	for _, r := range rows {
		tag := ""
		if r.IsSubagent {
			tag = " [subagent]"
		}
		origin := r.OriginID
		if origin == "" {
			origin = "(native)"
		}
		_, _ = fmt.Fprintf(out, "  %s\torigin=%s%s\t%s\n",
			r.OCSessionID, origin, tag, r.Worktree)
	}
}

func resolveOCDB(path string) string {
	if path != "" {
		return path
	}
	return platform.OpenCodeDBPath()
}

func resolveCCHome(path string) string {
	if path != "" {
		return path
	}
	return platform.ClaudeCodeHome()
}

// reverseArtifactWriters maps an artifact domain onto the read-then-write
// pass that copies it from OpenCode back to Claude Code. Sessions, MCP,
// and the system prompt have their own pipelines and are absent here.
var reverseArtifactWriters = map[string]func(cwd string) ([]string, error){
	"skills": func(cwd string) ([]string, error) {
		items, err := ocsrc.ReadGlobalSkills()
		if err != nil {
			return nil, err
		}
		if p, err := ocsrc.ReadProjectSkills(cwd); err == nil {
			items = append(items, p...)
		}
		return platformOpen().SkillWriterFor(cwd).WriteGlobal(items)
	},
	"commands": func(cwd string) ([]string, error) {
		items, err := ocsrc.ReadGlobalCommands()
		if err != nil {
			return nil, err
		}
		if p, err := ocsrc.ReadProjectCommands(cwd); err == nil {
			items = append(items, p...)
		}
		return platformOpen().CommandWriterFor(cwd).WriteGlobal(items)
	},
	"agents": func(cwd string) ([]string, error) {
		items, err := ocsrc.ReadGlobalAgents()
		if err != nil {
			return nil, err
		}
		if p, err := ocsrc.ReadProjectAgents(cwd); err == nil {
			items = append(items, p...)
		}
		return platformOpen().AgentWriterFor(cwd).WriteGlobal(items)
	},
	"rules": func(cwd string) ([]string, error) {
		items, err := ocsrc.ReadGlobalRules()
		if err != nil {
			return nil, err
		}
		if p, err := ocsrc.ReadProjectRules(cwd); err == nil {
			items = append(items, p...)
		}
		return platformOpen().RuleWriterFor(cwd).WriteGlobal(items)
	},
}

func runReverseArtifacts(d domainSpec, dryRun bool, cwd string, cmd *cobra.Command) error {
	write, ok := reverseArtifactWriters[d.name]
	if !ok {
		return fmt.Errorf("no OpenCode -> Claude Code pipeline for %s", d.name)
	}
	if dryRun {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: dry-run, nothing written\n", d.name)
		return nil
	}
	written, err := write(cwd)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %d\n", d.name, len(written))
	for _, p := range written {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", p)
	}
	return nil
}

func runReverseMCP(_ context.Context, dryRun bool, ccHome string, cmd *cobra.Command) error {
	servers, err := ocsrc.ReadGlobalMCP()
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "mcp: no servers found")
		return nil
	}
	if dryRun {
		for _, s := range servers {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "mcp: would merge %s\n", s.Name)
		}
		return nil
	}
	if _, err := targetCCPath(ccHome).Apply(servers); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "mcp: merged %d server(s)\n", len(servers))
	return nil
}

func runReverseSystemPrompt(dryRun bool, ccHome string, cmd *cobra.Command) error {
	prompt, err := ocsrc.ReadGlobalSystemPrompt()
	if err != nil {
		return err
	}
	if prompt == nil {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "system: no AGENTS.md found")
		return nil
	}
	if dryRun {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "system: would write %s -> %s\n",
			prompt.SourcePath, filepath.Join(ccHome, "CLAUDE.md"))
		return nil
	}
	w := cctgt.NewSystemPromptWriter()
	if ccHome != "" {
		w.Home = ccHome
	}
	out, err := w.Write(prompt)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "system: wrote %s\n", out)
	return nil
}

var _ = domain.Hook{}
