package confirm

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

func New() Model {
	return Model{}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width

		return m, nil

	case msgs.RequestDeleteMsg:
		m.event = msg.Event

		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "y", "enter":
			event := m.event

			return m, func() tea.Msg { return msgs.DeleteConfirmedMsg{Event: event} }

		case "n", "esc":
			return m, func() tea.Msg { return msgs.ClosePopupMsg{} }
		}
	}

	return m, nil
}

func (m Model) View() string {
	textLimit := 50
	if m.width > 0 {
		textLimit = min(50, max(16, m.width-8))
	}

	heading := lipgloss.NewStyle().Bold(true).Render("Delete event?")
	title := lipgloss.NewStyle().Foreground(lipgloss.Color(m.event.Color)).Render(m.event.Title)

	endInclusive := m.event.End.AddDate(0, 0, -1)

	singleDay := endInclusive.Year() == m.event.Start.Year() && endInclusive.YearDay() == m.event.Start.YearDay()

	endsSameDay := m.event.End.Year() == m.event.Start.Year() && m.event.End.YearDay() == m.event.Start.YearDay()

	var whenText string
	switch {
	case m.event.AllDay && singleDay:
		whenText = m.event.Start.Format("Mon 2 Jan") + "  All day"
	case m.event.AllDay:
		whenText = m.event.Start.Format("Mon 2 Jan") + " - " + endInclusive.Format("Mon 2 Jan") + "  All day"
	case !endsSameDay:
		whenText = m.event.Start.Format("Mon 2 Jan 15:04") + " - " + m.event.End.Format("Mon 2 Jan 15:04")
	default:
		whenText = m.event.Start.Format("Mon 2 Jan  15:04") + " - " + m.event.End.Format("15:04")
	}

	when := lipgloss.NewStyle().Foreground(theme.Muted).Render(whenText)

	lines := []string{heading, title, when}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, textLimit, "…")
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Danger).
		Padding(0, 2)

	return box.Render(strings.Join(lines, "\n"))
}

func (m Model) KeyHints() []msgs.KeyHint {
	return []msgs.KeyHint{
		{Key: "y", Action: "delete"},
		{Key: "n", Action: "cancel"},
	}
}
