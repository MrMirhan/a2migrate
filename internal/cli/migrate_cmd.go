package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mirhan/a2migrate/internal/migrate"
	"github.com/mirhan/a2migrate/internal/tools"
)

type migrateFlags struct {
	dryRun     bool
	yes        bool
	backup     bool
	cwd        string
	sourcePath string
	targetPath string
	renames    []string
	includes   []string
	excludes   []string
	search     string
	skipRepair bool
	skipNative bool
}

func bindMigrateFlags(c *cobra.Command) *migrateFlags {
	f := &migrateFlags{}
	fl := c.Flags()
	fl.BoolVar(&f.dryRun, "dry-run", false, "Plan only; do not write to disk")
	fl.BoolVar(&f.yes, "yes", false, "Skip confirmation prompts")
	fl.BoolVar(&f.backup, "backup", false, "Back up the target before writing")
	fl.StringVar(&f.cwd, "cwd", "", "Project root for project-scoped artifacts (default: current directory)")
	fl.StringVar(&f.sourcePath, "source-path", "", "Override where the source tool's state is read from")
	fl.StringVar(&f.targetPath, "target-path", "", "Override where the target tool's state is written to")
	fl.StringSliceVar(&f.renames, "rename", nil, "Rename a session during migration (old=new), repeatable")
	fl.StringSliceVar(&f.includes, "include", nil, "Only migrate sessions whose id matches")
	fl.StringSliceVar(&f.excludes, "exclude", nil, "Skip sessions whose id matches")
	fl.StringVar(&f.search, "search", "", "Substring filter on session id, path, or title")
	fl.BoolVar(&f.skipRepair, "skip-repair", false, "Skip post-write invariants (OpenCode target only)")
	fl.BoolVar(&f.skipNative, "skip-native", false, "Skip sessions that did not originate in the source tool")
	return f
}

// sessionOptions builds the orchestrator options shared by both
// pipelines. Direction is stamped by the migrator constructor; backupDir
// is direction-specific because the two targets are different shapes (a
// database file vs. a home directory).
func (f *migrateFlags) sessionOptions(from, to, backupDir string) (migrate.Options, error) {
	renames, err := migrate.ParsedRenames(f.renames)
	if err != nil {
		return migrate.Options{}, err
	}
	opts := migrate.Options{
		From:       from,
		To:         to,
		DryRun:     f.dryRun,
		Yes:        f.yes,
		Renames:    renames,
		Includes:   f.includes,
		Excludes:   f.excludes,
		Search:     f.search,
		SkipRepair: f.skipRepair,
		SkipNative: f.skipNative,
	}
	if f.backup && !f.dryRun {
		opts.BackupDir = backupDir
	}
	return opts, nil
}

func newMigrateCmd() *cobra.Command {
	var f *migrateFlags
	c := &cobra.Command{
		Use:   "migrate <from> <to> [domain...]",
		Short: "Migrate state from one tool to another",
		Long: "Migrates sessions and artifacts between two AI coding tools.\n\n" +
			"Omit the domain arguments to migrate every domain both tools support.\n" +
			"Swap <from> and <to> to migrate the other way.",
		Args:              cobra.MinimumNArgs(2),
		ValidArgsFunction: completeMigrateArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			from, err := resolveTool(args[0])
			if err != nil {
				return err
			}
			to, err := resolveTool(args[1])
			if err != nil {
				return err
			}
			if from.ID == to.ID {
				return fmt.Errorf("source and target are both %s", from.DisplayName)
			}
			domains, err := resolveDomains(args[2:], from, to)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "migrating %s -> %s (%s)\n",
				from.DisplayName, to.DisplayName, strings.Join(domainNames(domains), ", "))
			if !f.yes && !f.dryRun {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "(use --yes to skip this confirmation)")
			}

			switch {
			case from.ID == toolClaudeCode && to.ID == toolOpenCode:
				return runMigrateToOpenCode(cmd, f, domains)
			case from.ID == toolOpenCode && to.ID == toolClaudeCode:
				return runMigrateToClaudeCode(cmd, f, domains)
			default:
				return notImplemented(from, to)
			}
		},
	}
	f = bindMigrateFlags(c)
	return c
}

func notImplemented(from, to tools.Tool) error {
	return fmt.Errorf("migrating %s -> %s is not implemented yet; see docs/ROADMAP.md",
		from.DisplayName, to.DisplayName)
}

func runMigrateToOpenCode(cmd *cobra.Command, f *migrateFlags, domains []domainSpec) error {
	if arts := artifactDomains(domains); len(arts) > 0 {
		m := migrate.ArtifactsMigrator{CWD: f.cwd, DryRun: f.dryRun, Domains: arts}
		rep, err := m.Migrate()
		if err != nil {
			return err
		}
		printArtifactsReport(cmd, rep)
	}
	if !hasDomain(domains, "sessions") {
		return nil
	}

	db := resolveOCDB(f.targetPath)
	opts, err := f.sessionOptions(f.sourcePath, db, filepath.Join(filepath.Dir(db), "backups"))
	if err != nil {
		return err
	}
	m := migrate.NewSessionMigrator(opts)
	refs, err := m.Discover(cmd.Context())
	if err != nil {
		return err
	}
	report, err := m.Run(cmd.Context(), refs)
	if err != nil {
		return err
	}
	printSessionReport(cmd, report)
	return nil
}

func runMigrateToClaudeCode(cmd *cobra.Command, f *migrateFlags, domains []domainSpec) error {
	ccHome := resolveCCHome(f.targetPath)
	for _, d := range domains {
		if d.name == "sessions" {
			continue
		}
		if err := runReverseArtifactDomain(cmd.Context(), d, f, ccHome, cmd); err != nil {
			return err
		}
	}
	if !hasDomain(domains, "sessions") {
		return nil
	}

	opts, err := f.sessionOptions(resolveOCDB(f.sourcePath), ccHome, filepath.Join(ccHome, "backups"))
	if err != nil {
		return err
	}
	m := migrate.NewReverseMigrator(opts)
	refs, err := m.Discover(cmd.Context())
	if err != nil {
		return err
	}
	report, err := m.Run(cmd.Context(), refs)
	if err != nil {
		return err
	}
	printReverseReport(cmd, report)
	return nil
}

// runReverseArtifactDomain writes one OpenCode artifact domain back to
// Claude Code.
func runReverseArtifactDomain(ctx context.Context, d domainSpec, f *migrateFlags, ccHome string, cmd *cobra.Command) error {
	switch d.name {
	case "mcp":
		return runReverseMCP(ctx, f.dryRun, ccHome, cmd)
	case "system":
		return runReverseSystemPrompt(f.dryRun, ccHome, cmd)
	default:
		return runReverseArtifacts(d, f.dryRun, f.cwd, cmd)
	}
}
