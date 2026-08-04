// Package interactive provides terminal UI helpers built on bubbletea.
// The primary widget is a multi-select session picker used by
// `a2migrate sessions select`.
package interactive

import (
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Item is one row in the picker.
type Item struct {
	Title    string
	Subtitle string
	ID       string
	Sub      bool
}

// Model is the bubbletea model for the multi-select picker.
type Model struct {
	items    []Item
	cursor   int
	selected map[int]bool
	quitting bool
	choice   []Item
	width    int
	height   int
}

// New returns a picker with the given items.
func New(items []Item) Model {
	return Model{
		items:    items,
		selected: make(map[int]bool, len(items)),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case " ":
			m.selected[m.cursor] = !m.selected[m.cursor]
		case "a":
			for i := range m.items {
				m.selected[i] = true
			}
		case "n":
			for i := range m.items {
				m.selected[i] = false
			}
		case "enter":
			for i, it := range m.items {
				if m.selected[i] {
					m.choice = append(m.choice, it)
				}
			}
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	var sb strings.Builder
	header := lipgloss.NewStyle().Bold(true).Render(
		"a2migrate: select sessions (space=toggle, enter=confirm, q=cancel, a=all, n=none)")
	sb.WriteString(header + "\n\n")
	for i, it := range m.items {
		marker := "  "
		if m.selected[i] {
			marker = "✓ "
		}
		row := fmt.Sprintf("%s%s", marker, it.Title)
		if it.Subtitle != "" {
			row += "  " + lipgloss.NewStyle().Faint(true).Render(it.Subtitle)
		}
		if i == m.cursor {
			row = lipgloss.NewStyle().Bold(true).Reverse(true).Render("> " + row)
		}
		sb.WriteString(row + "\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

// Selected returns the items the user picked.
func (m Model) Selected() []Item { return m.choice }

// Run launches the picker on stdin/stdout and returns the chosen items.
// If stdin is not a TTY, returns nil with no error (callers should fall
// back to non-interactive flow).
func Run(items []Item, in io.Reader, out io.Writer) ([]Item, error) {
	if !isatty(in) {
		return nil, nil
	}
	p := tea.NewProgram(New(items), tea.WithInput(in), tea.WithOutput(out))
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	m := final.(Model)
	return m.Selected(), nil
}

func isatty(_ io.Reader) bool {
	// Stub: real implementation reads from the terminal. For now assume
	// stdin is a TTY; tests bypass this entry point.
	return true
}
