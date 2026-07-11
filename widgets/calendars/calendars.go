package calendars

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
	"github.com/siliconwitch/caltui/theme"
)

type Toggler interface {
	All() []calendar.CalendarVisibility
	Toggle(name string)
}

type Model struct {
	toggler       Toggler
	selectedIndex int
	width         int
}

func New(toggler Toggler) Model {
	return Model{toggler: toggler}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width

		return m, nil

	case msgs.OpenCalendarsMsg:
		m.selectedIndex = 0

		return m, nil

	case tea.KeyMsg:
		entries := m.toggler.All()

		switch msg.String() {
		case "j", "down", "tab":
			if len(entries) > 0 {
				m.selectedIndex = (m.selectedIndex + 1) % len(entries)
			}

			return m, nil

		case "k", "up", "shift+tab":
			if len(entries) > 0 {
				m.selectedIndex = (m.selectedIndex + len(entries) - 1) % len(entries)
			}

			return m, nil

		case " ", "enter":
			if len(entries) == 0 {
				return m, nil
			}

			m.selectedIndex = min(m.selectedIndex, len(entries)-1)
			m.toggler.Toggle(entries[m.selectedIndex].Name)

			return m, func() tea.Msg { return msgs.EventsChangedMsg{} }

		case "esc", "c", "q":
			return m, func() tea.Msg { return msgs.ClosePopupMsg{} }
		}
	}

	return m, nil
}

func (m Model) View() string {
	innerWidth := min(46, max(20, m.width-8))

	lines := []string{lipgloss.NewStyle().Bold(true).Render("Calendars"), ""}

	entries := m.toggler.All()

	if len(entries) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Muted).Render("No calendars yet"))
	}

	for index, entry := range entries {
		marker := "[x]"
		if entry.Hidden {
			marker = "[ ]"
		}

		plain := ansi.Truncate(marker+" ● "+entry.Name, innerWidth, "…")

		row := strings.Replace(plain, "●",
			lipgloss.NewStyle().Foreground(lipgloss.Color(entry.Color)).Render("●"), 1)

		if index == m.selectedIndex {
			row = lipgloss.NewStyle().Background(theme.SelectionBg).Render(plain)
		}

		lines = append(lines, row)
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
		{Key: "j/k", Action: "choose"},
		{Key: "space", Action: "show/hide"},
		{Key: "esc", Action: "close"},
	}
}
