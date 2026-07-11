package agenda

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
	"github.com/siliconwitch/caltui/theme"
)

const ConfigSection = "agenda"

type Config struct {
	LookaheadDays int `toml:"lookahead_days"`
}

func DefaultConfig() Config {
	return Config{LookaheadDays: 365}
}

var todayForeground = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#16161E"}

var allDayForeground = lipgloss.Color("#16161E")

type Model struct {
	source             calendar.Source
	config             Config
	location           *time.Location
	anchorDate         time.Time
	selectedEventIndex int
	scrollOffset       int
	yankedEventID      string
	width              int
	height             int
}

func New(source calendar.Source, config Config, location *time.Location) Model {
	if config.LookaheadDays < 1 {
		config.LookaheadDays = DefaultConfig().LookaheadDays
	}

	now := time.Now().In(location)

	return Model{
		source:             source,
		config:             config,
		location:           location,
		anchorDate:         time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location),
		selectedEventIndex: -1,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) upcomingEvents() []calendar.Event {
	return m.source.Events(m.anchorDate, m.anchorDate.AddDate(0, 0, m.config.LookaheadDays))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		return m, nil

	case msgs.FocusDateMsg:
		date := msg.Date.In(m.location)

		m.anchorDate = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, m.location)
		m.selectedEventIndex = -1
		m.scrollOffset = 0

		return m, nil

	case msgs.EventsChangedMsg:
		m.selectedEventIndex = -1

		return m, nil

	case msgs.YankedMsg:
		m.yankedEventID = msg.EventID

		return m, nil

	case tea.KeyMsg:
		events := m.upcomingEvents()

		switch msg.String() {
		case "j", "down", "tab":
			return m.selected(events, m.selectedEventIndex+1)

		case "k", "up", "shift+tab":
			return m.selected(events, m.selectedEventIndex-1)

		case "ctrl+d":
			return m.selected(events, m.selectedEventIndex+max(1, m.height/3))

		case "ctrl+u":
			return m.selected(events, m.selectedEventIndex-max(1, m.height/3))

		case "t":
			now := time.Now().In(m.location)

			m.anchorDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, m.location)
			m.selectedEventIndex = -1
			m.scrollOffset = 0

			return m, func() tea.Msg { return msgs.EventSelectedMsg{} }

		case "esc":
			if m.selectedEventIndex < 0 {
				return m, nil
			}

			m.selectedEventIndex = -1

			return m, func() tea.Msg { return msgs.EventSelectedMsg{} }

		case "n":
			calendars := m.source.Calendars()

			if len(calendars) == 0 {
				return m, nil
			}

			day := m.FocusedDate()

			start := time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, m.location)

			template := calendar.Event{
				Start:    start,
				End:      start.Add(time.Hour),
				Calendar: calendars[0].Name,
				Color:    calendars[0].Color,
			}

			return m, func() tea.Msg { return msgs.OpenEventFormMsg{Event: template, IsNew: true} }

		case "e":
			selected := m.SelectedEvent()

			if selected == nil {
				return m, nil
			}

			event := *selected

			return m, func() tea.Msg { return msgs.OpenEventFormMsg{Event: event, IsNew: false} }

		case "d":
			selected := m.SelectedEvent()

			if selected == nil {
				return m, nil
			}

			event := *selected

			return m, func() tea.Msg { return msgs.RequestDeleteMsg{Event: event} }

		case "y":
			selected := m.SelectedEvent()

			if selected == nil {
				return m, nil
			}

			event := *selected

			return m, func() tea.Msg { return msgs.YankMsg{Event: event} }

		case "p":
			date := m.FocusedDate()

			return m, func() tea.Msg { return msgs.PasteMsg{Date: date} }
		}
	}

	return m, nil
}

func (m Model) selected(events []calendar.Event, index int) (tea.Model, tea.Cmd) {
	if len(events) == 0 {
		return m, nil
	}

	m.selectedEventIndex = max(0, min(index, len(events)-1))

	selectedEvent := events[m.selectedEventIndex]

	return m, func() tea.Msg { return msgs.EventSelectedMsg{Event: &selectedEvent} }
}

func (m Model) SelectedEvent() *calendar.Event {
	events := m.upcomingEvents()

	if m.selectedEventIndex < 0 || m.selectedEventIndex >= len(events) {
		return nil
	}

	return &events[m.selectedEventIndex]
}

func (m Model) FocusedDate() time.Time {
	if selected := m.SelectedEvent(); selected != nil {
		start := selected.Start.In(m.location)

		return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, m.location)
	}

	return m.anchorDate
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	events := m.upcomingEvents()

	if len(events) == 0 {
		return lipgloss.NewStyle().Foreground(theme.Muted).Render(" No events in the next " +
			m.anchorDate.AddDate(0, 0, m.config.LookaheadDays).Sub(m.anchorDate).String())
	}

	now := time.Now().In(m.location)

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, m.location)

	type renderedRow struct {
		text       string
		eventIndex int
	}

	var rows []renderedRow

	previousDay := time.Time{}
	selectedRow := 0

	for index, event := range events {
		start := event.Start.In(m.location)

		day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, m.location)

		if !day.Equal(previousDay) {
			if len(rows) > 0 {
				rows = append(rows, renderedRow{text: "", eventIndex: -1})
			}

			header := day.Format("Monday 2 January")

			headerStyle := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
			if day.Equal(today) {
				headerStyle = lipgloss.NewStyle().Background(theme.Accent).Foreground(todayForeground).Bold(true)
				header = " " + header + " "
			}

			rows = append(rows, renderedRow{text: headerStyle.Render(header), eventIndex: -1})
			previousDay = day
		}

		when := start.Format("15:04") + "–" + event.End.In(m.location).Format("15:04")
		if event.AllDay {
			when = "all day"
		}

		style := lipgloss.NewStyle().Foreground(lipgloss.Color(event.Color))

		switch {
		case event.ID != "" && event.ID == m.yankedEventID:
			style = lipgloss.NewStyle().Foreground(theme.Yank).Italic(true)

		case event.AllDay:
			style = lipgloss.NewStyle().Background(lipgloss.Color(event.Color)).Foreground(allDayForeground)
		}

		text := style.Render(ansi.Truncate("  "+when+"  "+event.Title, m.width-1, "…"))

		if index == m.selectedEventIndex {
			text = lipgloss.NewStyle().Reverse(true).Render(ansi.Truncate("  "+when+"  "+event.Title, m.width-1, "…"))
			selectedRow = len(rows)
		}

		rows = append(rows, renderedRow{text: text, eventIndex: index})
	}

	scrollOffset := m.scrollOffset
	if m.selectedEventIndex >= 0 {
		scrollOffset = max(0, min(selectedRow-m.height/2, len(rows)-m.height))
	}
	scrollOffset = max(0, min(scrollOffset, max(0, len(rows)-m.height)))

	var lines []string
	for _, row := range rows[scrollOffset:min(scrollOffset+m.height, len(rows))] {
		lines = append(lines, row.text)
	}

	for len(lines) < m.height {
		lines = append(lines, "")
	}

	joined := ""
	for index, line := range lines {
		if index > 0 {
			joined += "\n"
		}
		joined += line
	}

	return joined
}

func (m Model) KeyHints() []msgs.KeyHint {
	if m.selectedEventIndex >= 0 {
		return []msgs.KeyHint{
			{Key: "j/k", Action: "select"},
			{Key: "e", Action: "edit"},
			{Key: "y", Action: "yank"},
			{Key: "d", Action: "delete"},
			{Key: "esc", Action: "deselect"},
		}
	}

	return []msgs.KeyHint{
		{Key: "j/k", Action: "select"},
		{Key: "n", Action: "new"},
		{Key: "p", Action: "paste"},
		{Key: "t", Action: "today"},
	}
}
