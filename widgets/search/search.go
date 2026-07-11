package search

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
	"github.com/siliconwitch/caltui/theme"
)

const maxResults = 50

const visibleResults = 10

type Model struct {
	source        calendar.Source
	location      *time.Location
	queryInput    textinput.Model
	results       []calendar.Event
	selectedIndex int
	width         int
}

func New(source calendar.Source, location *time.Location) Model {
	queryInput := textinput.New()
	queryInput.Prompt = ""

	return Model{source: source, location: location, queryInput: queryInput}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.queryInput.Width = m.innerWidth() - 2

		return m, nil

	case msgs.OpenSearchMsg:
		m.queryInput.SetValue("")
		m.queryInput.Focus()
		m.queryInput.Width = m.innerWidth() - 2
		m.results = nil
		m.selectedIndex = 0

		return m, textinput.Blink

	case msgs.EventsChangedMsg:
		return m.withResults(), nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return msgs.ClosePopupMsg{} }

		case "enter":
			if len(m.results) == 0 {
				return m, nil
			}

			date := m.results[m.selectedIndex].Start

			return m, func() tea.Msg { return msgs.GotoDateMsg{Date: date} }

		case "down", "ctrl+n":
			if len(m.results) > 0 {
				m.selectedIndex = (m.selectedIndex + 1) % len(m.results)
			}

			return m, nil

		case "up", "ctrl+p":
			if len(m.results) > 0 {
				m.selectedIndex = (m.selectedIndex + len(m.results) - 1) % len(m.results)
			}

			return m, nil
		}

		input, cmd := m.queryInput.Update(msg)
		m.queryInput = input

		return m.withResults(), cmd

	default:
		input, cmd := m.queryInput.Update(msg)
		m.queryInput = input

		return m, cmd
	}
}

func (m Model) withResults() Model {
	query := strings.ToLower(strings.TrimSpace(m.queryInput.Value()))
	m.results = nil
	m.selectedIndex = 0

	if query == "" {
		return m
	}

	now := time.Now().In(m.location)

	for _, event := range m.source.Events(now.AddDate(-1, 0, 0), now.AddDate(1, 0, 0)) {
		matchesTitle := strings.Contains(strings.ToLower(event.Title), query)

		matchesLocation := strings.Contains(strings.ToLower(event.Location), query)

		if !matchesTitle && !matchesLocation {
			continue
		}

		m.results = append(m.results, event)

		if len(m.results) == maxResults {
			break
		}
	}

	return m
}

func (m Model) innerWidth() int {
	return min(60, max(24, m.width-8))
}

func (m Model) View() string {
	innerWidth := m.innerWidth()

	lines := []string{
		lipgloss.NewStyle().Bold(true).Render("Search"),
		"",
		m.queryInput.View(),
	}

	if len(m.results) > 0 {
		lines = append(lines, "")

		first := max(0, min(m.selectedIndex-visibleResults/2, len(m.results)-visibleResults))

		for index := first; index < min(first+visibleResults, len(m.results)); index++ {
			event := m.results[index]

			when := event.Start.In(m.location).Format("Mon 2 Jan 15:04")
			if event.AllDay {
				when = event.Start.In(m.location).Format("Mon 2 Jan") + " all day"
			}

			bullet := lipgloss.NewStyle().Foreground(lipgloss.Color(event.Color)).Render("●")

			row := ansi.Truncate(bullet+" "+when+"  "+event.Title, innerWidth, "…")

			if index == m.selectedIndex {
				row = lipgloss.NewStyle().Background(theme.SelectionBg).Render(row)
			}

			lines = append(lines, row)
		}

		if len(m.results) == maxResults {
			lines = append(lines, lipgloss.NewStyle().Foreground(theme.Muted).Render("further matches not shown"))
		}
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
		{Key: "type", Action: "search"},
		{Key: "↑/↓", Action: "choose"},
		{Key: "enter", Action: "go"},
		{Key: "esc", Action: "cancel"},
	}
}
