package alertpopup

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
	"github.com/siliconwitch/caltui/theme"
)

type Model struct {
	source    calendar.Source
	location  *time.Location
	lastCheck time.Time
	alerts    []string
	width     int
}

func New(source calendar.Source, location *time.Location) Model {
	return Model{source: source, location: location, lastCheck: time.Now()}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Pending() int {
	return len(m.alerts)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width

		return m, nil

	case msgs.ClockTickMsg:
		now := msg.Now

		for _, event := range m.source.Events(now.AddDate(0, 0, -1), now.AddDate(0, 0, 31)) {
			for _, offset := range event.Alarms {
				alarmTime := event.Start.Add(offset)

				if !alarmTime.After(m.lastCheck) || alarmTime.After(now) {
					continue
				}

				text := event.Title + " — " + event.Start.In(m.location).Format("Mon 2 Jan 15:04")

				if minutes := int(event.Start.Sub(now).Round(time.Minute).Minutes()); minutes > 0 {
					text += fmt.Sprintf(" (in %dm)", minutes)
				}

				m.alerts = append(m.alerts, text)
			}
		}

		m.lastCheck = now

		return m, nil

	case tea.KeyMsg:
		if len(m.alerts) > 0 {
			m.alerts = m.alerts[1:]
		}

		if len(m.alerts) == 0 {
			return m, func() tea.Msg { return msgs.ClosePopupMsg{} }
		}

		return m, nil
	}

	return m, nil
}

func (m Model) View() string {
	if len(m.alerts) == 0 {
		return ""
	}

	innerWidth := min(60, max(24, m.width-8))

	heading := "Reminder"
	if len(m.alerts) > 1 {
		heading = fmt.Sprintf("Reminder (1 of %d)", len(m.alerts))
	}

	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(theme.Accent).Render(heading),
		"",
		ansi.Truncate(m.alerts[0], innerWidth, "…"),
		"",
		lipgloss.NewStyle().Foreground(theme.Muted).Render("any key dismisses"),
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Padding(0, 1).
		Width(innerWidth + 2)

	return box.Render(strings.Join(lines, "\n"))
}

func (m Model) KeyHints() []msgs.KeyHint {
	return []msgs.KeyHint{{Key: "any key", Action: "dismiss"}}
}
