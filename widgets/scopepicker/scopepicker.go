package scopepicker

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
	"github.com/siliconwitch/caltui/theme"
)

type Model struct {
	event calendar.Event
	width int
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width

		return m, nil

	case msgs.OpenEventFormMsg:
		m.event = msg.Event

		return m, nil

	case tea.KeyMsg:
		event := m.event

		switch msg.String() {
		case "o":
			return m, func() tea.Msg {
				return msgs.OpenEventFormMsg{Event: event, Scope: msgs.ScopeOccurrence}
			}

		case "s":
			return m, func() tea.Msg {
				return msgs.OpenEventFormMsg{Event: event, Scope: msgs.ScopeSeries}
			}

		case "esc", "n", "q":
			return m, func() tea.Msg { return msgs.ClosePopupMsg{} }
		}

		return m, nil
	}

	return m, nil
}

func (m Model) View() string {
	innerWidth := min(46, max(20, m.width-4))

	lines := []string{
		lipgloss.NewStyle().Bold(true).Render("Edit repeating event"),
		"",
		ansi.Truncate(m.event.Title, innerWidth, "…"),
		"",
		"Edit this occurrence or the whole series?",
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Padding(0, 1).
		Width(innerWidth + 2)

	return box.Render(strings.Join(lines, "\n"))
}

func (m Model) KeyHints() []msgs.KeyHint {
	return []msgs.KeyHint{
		{Key: "o", Action: "this occurrence"},
		{Key: "s", Action: "whole series"},
		{Key: "esc", Action: "cancel"},
	}
}
