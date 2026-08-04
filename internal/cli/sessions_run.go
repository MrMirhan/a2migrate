package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/MrMirhan/a2migrate/internal/migrate"
)

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
