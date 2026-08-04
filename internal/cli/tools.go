package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mirhan/a2migrate/internal/tools"
)

// newToolsCmd wires `a2migrate tools` — surface the registry so users
// can see which CLIs a2migrate knows about and where each one stores
// its state on disk.
func newToolsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "tools",
		Short: "List AI coding CLIs that a2migrate knows about",
	}
	c.AddCommand(newToolsListCmd())
	c.AddCommand(newToolsShowCmd())
	return c
}

func newToolsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all known tools",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rows := make([][]string, 0, 16)
			for _, t := range tools.All() {
				caps := "—"
				if len(t.Capabilities) > 0 {
					caps = joinCaps(t.Capabilities)
				}
				rows = append(rows, []string{string(t.ID), t.DisplayName, caps})
			}
			return writeTable(cmd, []string{"ID", "Tool", "Capabilities"}, rows)
		},
	}
}

func newToolsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show on-disk paths and capabilities for one tool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t, ok := tools.Get(tools.ID(args[0]))
			if !ok {
				return &unknownToolError{id: args[0]}
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintln(out, "id:          ", t.ID)
			_, _ = fmt.Fprintln(out, "name:        ", t.DisplayName)
			_, _ = fmt.Fprintln(out, "config:      ", t.ConfigPath())
			_, _ = fmt.Fprintln(out, "data:        ", t.DataPath())
			_, _ = fmt.Fprintln(out, "capabilities:", joinCaps(t.Capabilities))
			return nil
		},
	}
}

// unknownToolError is returned by `tools show <id>` for missing IDs.
type unknownToolError struct{ id string }

func (e *unknownToolError) Error() string {
	return "unknown tool: " + e.id
}

func joinCaps(caps []tools.Capability) string {
	if len(caps) == 0 {
		return "—"
	}
	out := ""
	for i, c := range caps {
		if i > 0 {
			out += ", "
		}
		out += string(c)
	}
	return out
}

// writeTable renders a fixed-width text table. Avoid pulling in
// text/tabwriter for what is a one-page CLI surface.
func writeTable(cmd *cobra.Command, header []string, rows [][]string) error {
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	out := cmd.OutOrStdout()
	for i, h := range header {
		if i > 0 {
			_, _ = fmt.Fprint(out, "  ")
		}
		_, _ = fmt.Fprintf(out, "%-*s", widths[i], h)
	}
	_, _ = fmt.Fprintln(out)
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				_, _ = fmt.Fprint(out, "  ")
			}
			_, _ = fmt.Fprintf(out, "%-*s", widths[i], cell)
		}
		_, _ = fmt.Fprintln(out)
	}
	return nil
}