package detail

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
	"github.com/siliconwitch/caltui/timezone"
)

type Model struct {
	event    *calendar.Event
	location *time.Location
	width    int
}

func New(location *time.Location) Model {
	return Model{location: location}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width

	case msgs.EventSelectedMsg:
		m.event = msg.Event

	case msgs.EventsChangedMsg:
		m.event = nil
	}

	return m, nil
}

func (m Model) View() string {
	if m.event == nil || m.width == 0 {
		return ""
	}

	innerWidth := min(78, max(16, m.width-6))

	textWidth := innerWidth - 2

	minutes := int(m.event.End.Sub(m.event.Start).Minutes())

	hours, remainder := minutes/60, minutes%60

	var duration string
	switch {
	case hours == 0:
		duration = fmt.Sprintf("%dm", minutes)
	case remainder == 0:
		duration = fmt.Sprintf("%dh", hours)
	default:
		duration = fmt.Sprintf("%dh %dm", hours, remainder)
	}

	muted := lipgloss.NewStyle().Foreground(theme.Muted)
	bullet := lipgloss.NewStyle().Foreground(lipgloss.Color(m.event.Color)).Render("●")
	title := lipgloss.NewStyle().Bold(true).Render(m.event.Title)

	startDay := time.Date(m.event.Start.Year(), m.event.Start.Month(), m.event.Start.Day(), 0, 0, 0, 0, time.UTC)
	endDay := time.Date(m.event.End.Year(), m.event.End.Month(), m.event.End.Day(), 0, 0, 0, 0, time.UTC)

	startMarker := timezone.Marker(m.event.Start, m.location)

	endMarker := timezone.Marker(m.event.End, m.location)

	startLabel := m.event.Start.Format("15:04")
	if startMarker != "" {
		startLabel += " " + startMarker
	}

	endLabel := m.event.End.Format("15:04")
	if endMarker != "" {
		endLabel += " " + endMarker
	}

	var when string
	switch {
	case m.event.AllDay:
		days := max(int(endDay.Sub(startDay).Hours()/24), 1)

		when = m.event.Start.Format("Mon 2 Jan") + "  All day"
		if days > 1 {
			when = m.event.Start.Format("Mon 2 Jan") + " - " +
				m.event.End.AddDate(0, 0, -1).Format("Mon 2 Jan") +
				fmt.Sprintf("  All day  %d days", days)
		}

	case !endDay.Equal(startDay):
		when = m.event.Start.Format("Mon 2 Jan ") + startLabel + " - " + m.event.End.Format("Mon 2 Jan ") + endLabel + "  " + duration

	default:
		when = m.event.Start.Format("Mon 2 Jan  ") + startLabel + " - " + endLabel + "  " + duration
	}

	localWhen := ""
	if startMarker != "" || endMarker != "" {
		localStart := m.event.Start.In(m.location)

		localEnd := m.event.End.In(m.location)

		localWhen = "Local: " + localStart.Format("Mon 2 Jan 15:04") + " - " + localEnd.Format("15:04")
		if localEnd.YearDay() != localStart.YearDay() || localEnd.Year() != localStart.Year() {
			localWhen = "Local: " + localStart.Format("Mon 2 Jan 15:04") + " - " + localEnd.Format("Mon 2 Jan 15:04")
		}
	}

	calendarLine := m.event.Calendar
	if m.event.Location != "" {
		calendarLine += " · " + m.event.Location
	}

	lines := []string{
		bullet + " " + title,
		when,
	}

	if localWhen != "" {
		lines = append(lines, muted.Render(localWhen))
	}

	lines = append(lines, muted.Render(calendarLine))

	if len(m.event.Attendees) > 0 {
		lines = append(lines, muted.Render("Attendees: "+strings.Join(m.event.Attendees, ", ")))
	}

	if m.event.Description != "" {
		descriptionLines := strings.Split(m.event.Description, "\n")
		if len(descriptionLines) > 4 {
			descriptionLines = append(descriptionLines[:3], "…")
		}

		lines = append(lines, "")
		lines = append(lines, descriptionLines...)
	}

	for i, line := range lines {
		lines[i] = ansi.Truncate(line, textWidth, "…")
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Grid).
		Padding(0, 1).
		Width(innerWidth)

	return box.Render(strings.Join(lines, "\n"))
}
