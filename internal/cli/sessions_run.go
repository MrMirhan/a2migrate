package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mirhan/a2migrate/internal/interactive"
	"github.com/mirhan/a2migrate/internal/migrate"
	"github.com/mirhan/a2migrate/internal/platform"
	"github.com/mirhan/a2migrate/internal/source/claudecode"
)

type sessionsMigrateFlags struct {
	dryRun     bool
	yes        bool
	backup     bool
	from       string
	to         string
	renames    []string
	includes   []string
	excludes   []string
	search     string
	skipRepair bool
}

func bindSessionsMigrateFlags(c *cobra.Command) *sessionsMigrateFlags {
	f := &sessionsMigrateFlags{}
	flags := c.Flags()
	flags.BoolVar(&f.dryRun, "dry-run", false, "Plan only; do not write to disk")
	flags.BoolVar(&f.yes, "yes", false, "Skip confirmation prompts")
	flags.BoolVar(&f.backup, "backup", false, "Create a timestamped DB backup before apply")
	flags.StringVar(&f.from, "from", "", "Override Claude Code home (default ~/.claude)")
	flags.StringVar(&f.to, "to", "", "Override OpenCode database path (default ~/.local/share/opencode/opencode.db)")
	flags.StringSliceVar(&f.renames, "rename", nil, "Rename a session during migration (old=new), repeatable")
	flags.StringSliceVar(&f.includes, "include", nil, "Only migrate sessions whose id matches")
	flags.StringSliceVar(&f.excludes, "exclude", nil, "Skip sessions whose id matches")
	flags.StringVar(&f.search, "search", "", "Substring filter on session id or file path")
	flags.BoolVar(&f.skipRepair, "skip-repair", false, "Skip post-fix invariants (reparent, pad step parts, etc.)")
	return f
}

func resolveTarget(to string) string {
	if to != "" {
		return to
	}
	return platform.OpenCodeDBPath()
}

func runSessionsMigrate(ctx context.Context, f *sessionsMigrateFlags, extra migrate.Options) (*migrate.SessionReport, error) {
	renames, err := migrate.ParsedRenames(f.renames)
	if err != nil {
		return nil, err
	}
	opts := extra
	opts.From = f.from
	opts.To = resolveTarget(f.to)
	opts.DryRun = f.dryRun
	opts.Yes = f.yes
	opts.Renames = renames
	opts.Includes = f.includes
	opts.Excludes = f.excludes
	opts.Search = f.search
	opts.SkipRepair = f.skipRepair
	if f.backup && !f.dryRun {
		opts.BackupDir = filepath.Join(filepath.Dir(opts.To), "backups")
	}

	m := migrate.NewSessionMigrator(opts)
	refs, err := m.Discover(ctx)
	if err != nil {
		return nil, err
	}
	return m.Run(ctx, refs)
}

func newSessionsListCmd() *cobra.Command {
	var search string
	c := &cobra.Command{
		Use:   "list",
		Short: "List discovered Claude Code sessions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := migrate.NewSessionMigrator(migrate.Options{Search: search})
			refs, err := m.Discover(cmd.Context())
			if err != nil {
				return err
			}
			refs = m.Selected(refs)
			if len(refs) == 0 {
				cmd.Println("No sessions found.")
				return nil
			}
			for _, r := range refs {
				tag := ""
				if r.IsSubagent {
					tag = " [subagent]"
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s%s\t%s\n", r.OriginID, tag, r.Worktree)
			}
			return nil
		},
	}
	c.Flags().StringVar(&search, "search", "", "Substring filter on session id or file path")
	return c
}

func newSessionsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show details of one session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			r := claudecode.NewSessionReader("")
			refs, err := r.DiscoverSessions()
			if err != nil {
				return err
			}
			for _, ref := range refs {
				if ref.OriginID != id {
					continue
				}
				sess, err := r.ParseSession(ref.FilePath)
				if err != nil {
					return err
				}
				n := 0
				for _, m := range sess.Messages {
					n += len(m.Parts)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(),
					"id:        %s\ntitle:     %s\nproject:   %s\nsubagent:  %v\nmessages:  %d\nparts:     %d\n",
					sess.OriginID, sess.Title, sess.ProjectDir, sess.IsSubagent,
					len(sess.Messages), n)
				return nil
			}
			return fmt.Errorf("session %q not found", id)
		},
	}
}

func newSessionsSelectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "select",
		Short: "Interactively select sessions to migrate",
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := migrate.NewSessionMigrator(migrate.Options{})
			refs, err := m.Discover(cmd.Context())
			if err != nil {
				return err
			}
			items := make([]interactive.Item, 0, len(refs))
			for _, r := range refs {
				title := r.OriginID
				if r.IsSubagent {
					title = "↳ " + r.OriginID
				}
				items = append(items, interactive.Item{
					Title:    title,
					Subtitle: r.Worktree,
					ID:       r.OriginID,
					Sub:      r.IsSubagent,
				})
			}
			picked, err := interactive.Run(items, cmd.InOrStdin(), cmd.OutOrStdout())
			if err != nil {
				return err
			}
			if picked == nil {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "non-interactive: use --include/--search instead")
				return nil
			}
			for _, it := range picked {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "selected: %s\n", it.ID)
			}
			return nil
		},
	}
}

func newSessionsMigrateCmd() *cobra.Command {
	flags := bindSessionsMigrateFlags(&cobra.Command{Use: "migrate"})
	c := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate one or many sessions to OpenCode",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := runSessionsMigrate(cmd.Context(), flags, migrate.Options{})
			if err != nil {
				return err
			}
			printSessionReport(cmd, report)
			return nil
		},
	}
	// Re-attach the same flags onto c (since bindSessionsMigrateFlags bound
	// onto a temporary command above).
	bindAllFlags(c, flags)
	return c
}

func bindAllFlags(dst *cobra.Command, src *sessionsMigrateFlags) {
	f := dst.Flags()
	f.BoolVar(&src.dryRun, "dry-run", false, "Plan only; do not write to disk")
	f.BoolVar(&src.yes, "yes", false, "Skip confirmation prompts")
	f.BoolVar(&src.backup, "backup", false, "Create a timestamped DB backup before apply")
	f.StringVar(&src.from, "from", "", "Override Claude Code home")
	f.StringVar(&src.to, "to", "", "Override OpenCode database path")
	f.StringSliceVar(&src.renames, "rename", nil, "Rename a session during migration (old=new)")
	f.StringSliceVar(&src.includes, "include", nil, "Only migrate sessions whose id matches")
	f.StringSliceVar(&src.excludes, "exclude", nil, "Skip sessions whose id matches")
	f.StringVar(&src.search, "search", "", "Substring filter")
	f.BoolVar(&src.skipRepair, "skip-repair", false, "Skip post-fix invariants")
}

func newSessionsVerifyCmd() *cobra.Command {
	var to string
	c := &cobra.Command{
		Use:   "verify",
		Short: "Verify migrated sessions in the OpenCode database",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := migrate.Verify(cmd.Context(), resolveTarget(to))
			if err != nil {
				return err
			}
			if len(report.Migrated) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no migrated sessions found")
				return nil
			}
			for _, r := range report.Migrated {
				tag := ""
				if r.IsSubagent {
					tag = " [subagent]"
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(),
					"%s\torigin=%s parent=%s%s title=%q\n",
					r.OCSessionID, r.OriginID, r.ParentOrigin, tag, r.Title)
			}
			return nil
		},
	}
	c.Flags().StringVar(&to, "to", "", "Override OpenCode database path")
	return c
}

func newSessionsRepairCmd() *cobra.Command {
	var to string
	c := &cobra.Command{
		Use:   "repair",
		Short: "Re-run post-migration invariants on already-migrated sessions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := openTargetDB(cmd.Context(), resolveTarget(to))
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			report, err := migrateRepair(cmd.Context(), db)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"scanned=%d reparent=%d pad=%d step-start-time=%d tool-state-time=%d\n",
				report.SessionsScanned, report.Reparents, report.PadsStepParts,
				report.AddedStepStartTimes, report.AddedToolStateTimes)
			return nil
		},
	}
	c.Flags().StringVar(&to, "to", "", "Override OpenCode database path")
	return c
}

func printSessionReport(cmd *cobra.Command, r *migrate.SessionReport) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "discovered=%d selected=%d projects=%d successes=%d failures=%d\n",
		r.Discovered, r.Selected, r.Projects, r.Successes, r.Failures)
	if r.BackupPath != "" {
		_, _ = fmt.Fprintf(out, "backup=%s\n", r.BackupPath)
	}
	for _, s := range r.Results {
		switch {
		case s.Error != nil:
			_, _ = fmt.Fprintf(out, "FAIL %s: %v\n", s.OriginID, s.Error)
		case s.AlreadyMigrated:
			_, _ = fmt.Fprintf(out, "SKIP %s\talready migrated as %s\n", s.OriginID, s.OCSessionID)
		default:
			_, _ = fmt.Fprintf(out, "OK   %s\t%s (%d messages, %d parts)\n",
				s.OCSessionID, s.Title, s.MessageCount, s.PartCount)
		}
	}
	if r.Reparents+r.PadsStep+r.StepStarts+r.ToolTimes > 0 {
		_, _ = fmt.Fprintf(out, "repair: reparent=%d pad=%d step-start=%d tool-time=%d\n",
			r.Reparents, r.PadsStep, r.StepStarts, r.ToolTimes)
	}
}
