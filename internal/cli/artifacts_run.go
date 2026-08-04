package cli

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/MrMirhan/a2migrate/internal/migrate"
	"github.com/MrMirhan/a2migrate/internal/target/opencode"
)

func openTargetDB(ctx context.Context, path string) (*sql.DB, error) {
	return opencode.OpenDatabase(ctx, path)
}

func migrateRepair(ctx context.Context, db *sql.DB) (opencode.RepairReport, error) {
	return opencode.Repair(ctx, db, nil)
}

func printArtifactsReport(cmd *cobra.Command, rep *migrate.ArtifactsReport) {
	out := cmd.OutOrStdout()
	if rep.DryRun {
		_, _ = fmt.Fprintln(out, "dry-run: nothing written")
		return
	}
	for _, g := range []struct {
		label string
		paths []string
	}{
		{"skills", rep.SkillsWritten},
		{"commands", rep.CommandsWritten},
		{"agents", rep.AgentsWritten},
		{"rules", rep.RulesWritten},
		{"mcp", rep.MCPMerged},
	} {
		if len(g.paths) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(out, "%s: %d\n", g.label, len(g.paths))
		for _, p := range g.paths {
			_, _ = fmt.Fprintf(out, "  %s\n", p)
		}
	}
	if rep.SystemPromptWritten != "" {
		_, _ = fmt.Fprintf(out, "system: wrote %s\n", rep.SystemPromptWritten)
	}
}
