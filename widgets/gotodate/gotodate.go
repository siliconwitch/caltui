package gotodate

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/siliconwitch/caltui/maskinput"
	"github.com/siliconwitch/caltui/msgs"
	"github.com/siliconwitch/caltui/theme"
)

type Model struct {
	field    maskinput.Field
	location *time.Location
}

func New() Model {
	return Model{field: maskinput.NewDate(true), location: time.Local}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgs.OpenGotoMsg:
		m.field = maskinput.NewDate(true).WithDate(msg.Date).Focus()
		m.location = msg.Date.Location()

		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return msgs.ClosePopupMsg{} }

		case "enter":
			return m, m.gotoCommand()
		}

		field, completed := m.field.Typed(msg.String())
		m.field = field

		if completed {
			return m, m.gotoCommand()
		}

		return m, nil
	}

	return m, nil
}

func (m Model) gotoCommand() tea.Cmd {
	year, month, day := m.field.Date()

	date := time.Date(year, month, day, 0, 0, 0, 0, m.location)

	return func() tea.Msg { return msgs.GotoDateMsg{Date: date} }
}

func (m Model) View() string {
	heading := lipgloss.NewStyle().Bold(true).Render("Go to date")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Padding(0, 2)

	return box.Render(heading + "\n\n" + m.field.View())
}

func (m Model) KeyHints() []msgs.KeyHint {
	return []msgs.KeyHint{
		{Key: "0-9", Action: "type date"},
		{Key: "←/→", Action: "part"},
		{Key: "enter", Action: "go"},
		{Key: "esc", Action: "cancel"},
	}
}
