package monthview

import (
	"fmt"
	"sort"
	"strconv"
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

const ConfigSection = "monthview"

type Config struct {
	ShowWeekNumbers bool `toml:"show_week_numbers"`
}

func DefaultConfig() Config {
	return Config{ShowWeekNumbers: false}
}

var todayForeground = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#16161E"}

var allDayForeground = lipgloss.Color("#16161E")

type Model struct {
	source             calendar.Source
	config             Config
	location           *time.Location
	topWeek            time.Time
	selectedDate       time.Time
	selectedEventIndex int
	yankedEventID      string
	width              int
	height             int
}

func New(source calendar.Source, config Config, location *time.Location) Model {
	now := time.Now().In(location)

	return Model{
		source:             source,
		config:             config,
		location:           location,
		selectedDate:       time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()),
		selectedEventIndex: -1,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if m.topWeek.IsZero() {
			anchorWeeks := len(rowHeights(m.height)) / 3

			m.topWeek = mondayOf(m.selectedDate).AddDate(0, 0, -7*anchorWeeks)

			return m, nil
		}

		m.topWeek = m.scrolledTopWeek()

		return m, nil

	case msgs.FocusDateMsg:
		m.selectedDate = time.Date(msg.Date.Year(), msg.Date.Month(), msg.Date.Day(), 0, 0, 0, 0, msg.Date.Location())
		m.selectedEventIndex = -1
		m.topWeek = m.scrolledTopWeek()

		return m, nil

	case msgs.EventsChangedMsg:
		m.selectedEventIndex = -1

		return m, nil

	case msgs.YankedMsg:
		m.yankedEventID = msg.EventID

		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "h", "left":
			return m.movedTo(m.selectedDate.AddDate(0, 0, -1))

		case "l", "right":
			return m.movedTo(m.selectedDate.AddDate(0, 0, 1))

		case "j", "down":
			return m.movedTo(m.selectedDate.AddDate(0, 0, 7))

		case "k", "up":
			return m.movedTo(m.selectedDate.AddDate(0, 0, -7))

		case "ctrl+d":
			return m.movedTo(m.selectedDate.AddDate(0, 0, 7*max(1, len(rowHeights(m.height))/2)))

		case "ctrl+u":
			return m.movedTo(m.selectedDate.AddDate(0, 0, -7*max(1, len(rowHeights(m.height))/2)))

		case "t":
			now := time.Now().In(m.location)

			return m.movedTo(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()))

		case "tab":
			return m.cycledSelection(1)

		case "shift+tab":
			return m.cycledSelection(-1)

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

			start := time.Date(
				m.selectedDate.Year(), m.selectedDate.Month(), m.selectedDate.Day(),
				9, 0, 0, 0, m.selectedDate.Location(),
			)

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
			date := m.selectedDate

			return m, func() tea.Msg { return msgs.PasteMsg{Date: date} }
		}
	}

	return m, nil
}

func (m Model) movedTo(date time.Time) (tea.Model, tea.Cmd) {
	m.selectedDate = date
	m.topWeek = m.scrolledTopWeek()

	if m.selectedEventIndex < 0 {
		return m, nil
	}

	m.selectedEventIndex = -1

	return m, func() tea.Msg { return msgs.EventSelectedMsg{} }
}

func (m Model) cycledSelection(step int) (tea.Model, tea.Cmd) {
	events := m.dayEvents(m.selectedDate)

	if len(events) == 0 {
		return m, nil
	}

	switch {
	case m.selectedEventIndex < 0 && step > 0:
		m.selectedEventIndex = 0
	case m.selectedEventIndex < 0:
		m.selectedEventIndex = len(events) - 1
	default:
		m.selectedEventIndex = (m.selectedEventIndex + step + len(events)) % len(events)
	}

	selectedEvent := events[m.selectedEventIndex]

	return m, func() tea.Msg { return msgs.EventSelectedMsg{Event: &selectedEvent} }
}

func (m Model) scrolledTopWeek() time.Time {
	if m.topWeek.IsZero() {
		return m.topWeek
	}

	selectedWeek := mondayOf(m.selectedDate)

	if selectedWeek.Before(m.topWeek) {
		return selectedWeek
	}

	visibleWeeks := len(rowHeights(m.height))

	bottomWeek := m.topWeek.AddDate(0, 0, 7*(visibleWeeks-1))

	if selectedWeek.After(bottomWeek) {
		return selectedWeek.AddDate(0, 0, -7*(visibleWeeks-1))
	}

	return m.topWeek
}

func (m Model) dayEvents(date time.Time) []calendar.Event {
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())

	weekEvents, lanes := m.weekLayout(mondayOf(dayStart))

	column := (int(dayStart.Weekday()) + 6) % 7

	var events []calendar.Event
	for _, lane := range lanes {
		if lane[column] >= 0 {
			events = append(events, weekEvents[lane[column]].event)
		}
	}

	return events
}

type weekEvent struct {
	event     calendar.Event
	firstCol  int
	lastCol   int
	multiDay  bool
	continued bool
}

func (m Model) weekLayout(week time.Time) ([]weekEvent, [][7]int) {
	var weekEvents []weekEvent

	for _, event := range m.source.Events(week, week.AddDate(0, 0, 7)) {
		localStart := event.Start.In(week.Location())

		localEnd := event.End.In(week.Location())

		startDay := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, week.Location())

		endDay := time.Date(localEnd.Year(), localEnd.Month(), localEnd.Day(), 0, 0, 0, 0, week.Location())
		if localEnd.Equal(endDay) {
			endDay = endDay.AddDate(0, 0, -1)
		}
		if endDay.Before(startDay) {
			endDay = startDay
		}

		firstCol, lastCol := 0, 6
		for column := range 7 {
			day := week.AddDate(0, 0, column)

			if day.Equal(startDay) {
				firstCol = column
			}
			if day.Equal(endDay) {
				lastCol = column
			}
		}

		weekEvents = append(weekEvents, weekEvent{
			event:     event,
			firstCol:  firstCol,
			lastCol:   lastCol,
			multiDay:  endDay.After(startDay),
			continued: startDay.Before(week),
		})
	}

	sort.SliceStable(weekEvents, func(i, j int) bool {
		spanI := weekEvents[i].lastCol - weekEvents[i].firstCol

		spanJ := weekEvents[j].lastCol - weekEvents[j].firstCol

		switch {
		case spanI != spanJ:
			return spanI > spanJ
		case weekEvents[i].event.AllDay != weekEvents[j].event.AllDay:
			return weekEvents[i].event.AllDay
		case !weekEvents[i].event.Start.Equal(weekEvents[j].event.Start):
			return weekEvents[i].event.Start.Before(weekEvents[j].event.Start)
		default:
			return weekEvents[i].event.Title < weekEvents[j].event.Title
		}
	})

	var lanes [][7]int

	for index, entry := range weekEvents {
		assignedLane := -1

		for laneIndex := range lanes {
			free := true
			for column := entry.firstCol; column <= entry.lastCol; column++ {
				if lanes[laneIndex][column] >= 0 {
					free = false

					break
				}
			}

			if free {
				assignedLane = laneIndex

				break
			}
		}

		if assignedLane < 0 {
			lanes = append(lanes, [7]int{-1, -1, -1, -1, -1, -1, -1})
			assignedLane = len(lanes) - 1
		}

		for column := entry.firstCol; column <= entry.lastCol; column++ {
			lanes[assignedLane][column] = index
		}
	}

	return weekEvents, lanes
}

func (m Model) FocusedDate() time.Time {
	return m.selectedDate
}

func (m Model) SelectedEvent() *calendar.Event {
	if m.selectedEventIndex < 0 {
		return nil
	}

	events := m.dayEvents(m.selectedDate)

	if m.selectedEventIndex >= len(events) {
		return nil
	}

	selectedEvent := events[m.selectedEventIndex]

	return &selectedEvent
}

func (m Model) KeyHints() []msgs.KeyHint {
	if m.selectedEventIndex < 0 {
		return []msgs.KeyHint{
			{Key: "hjkl", Action: "move"},
			{Key: "tab", Action: "select"},
			{Key: "n", Action: "new"},
			{Key: "p", Action: "paste"},
			{Key: "t", Action: "today"},
		}
	}

	return []msgs.KeyHint{
		{Key: "tab", Action: "next"},
		{Key: "S-tab", Action: "prev"},
		{Key: "e", Action: "edit"},
		{Key: "d", Action: "delete"},
		{Key: "y", Action: "yank"},
		{Key: "esc", Action: "deselect"},
	}
}

func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	gutterWidth, cellWidths := columnWidths(m.width, m.config.ShowWeekNumbers)

	heights := rowHeights(m.height)

	now := time.Now().In(m.location)

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	gridStyle := lipgloss.NewStyle().Foreground(theme.Grid)
	mutedStyle := lipgloss.NewStyle().Foreground(theme.Muted)

	weekdayNames := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

	headerCells := make([]string, 0, len(weekdayNames))
	for column, name := range weekdayNames {
		style := lipgloss.NewStyle()
		if column == (int(today.Weekday())+6)%7 {
			style = style.Foreground(theme.Accent)
		}

		label := ansi.Truncate(" "+name, max(0, cellWidths[column]), "")

		padding := strings.Repeat(" ", max(0, cellWidths[column]-ansi.StringWidth(label)))

		headerCells = append(headerCells, style.Render(label)+padding)
	}

	lines := []string{
		strings.Repeat(" ", gutterWidth) + strings.Join(headerCells, " "),
		gridStyle.Render(strings.Repeat("─", m.width)),
	}

	week := m.topWeek
	if week.IsZero() {
		week = mondayOf(m.selectedDate)
	}

	selectedID := ""
	if selected := m.SelectedEvent(); selected != nil {
		selectedID = selected.ID
	}

	for _, weekRowHeight := range heights {
		if weekRowHeight <= 0 {
			continue
		}

		contentLineCount := weekRowHeight - 1
		laneCapacity := contentLineCount - 1

		weekEvents, lanes := m.weekLayout(week)

		var weekDays [7]time.Time
		for column := range weekDays {
			weekDays[column] = week.AddDate(0, 0, column)
		}

		selectedColumn := -1
		for column, day := range weekDays {
			if day.Equal(m.selectedDate) {
				selectedColumn = column
			}
		}

		var overflowing [7]bool
		var hiddenCounts [7]int

		if laneCapacity >= 1 {
			for column := range 7 {
				for laneIndex := laneCapacity; laneIndex < len(lanes); laneIndex++ {
					if lanes[laneIndex][column] >= 0 {
						overflowing[column] = true
					}
				}

				if !overflowing[column] {
					continue
				}

				for laneIndex := laneCapacity - 1; laneIndex < len(lanes); laneIndex++ {
					if lanes[laneIndex][column] >= 0 {
						hiddenCounts[column]++
					}
				}
			}
		}

		for lineIndex := range contentLineCount {
			var row strings.Builder

			switch {
			case gutterWidth > 0 && lineIndex == 0:
				_, weekNumber := week.ISOWeek()

				row.WriteString(mutedStyle.Render(fmt.Sprintf("%2d ", weekNumber)))
			case gutterWidth > 0:
				row.WriteString(strings.Repeat(" ", gutterWidth))
			}

			if lineIndex == 0 {
				for column, day := range weekDays {
					if column > 0 {
						row.WriteString(gridStyle.Render("│"))
					}

					cellWidth := max(0, cellWidths[column])

					base := lipgloss.NewStyle()
					if day.Equal(m.selectedDate) {
						base = base.Background(theme.SelectionBg)
					}

					piece := " " + strconv.Itoa(day.Day())
					pieceStyle := base

					switch {
					case day.Equal(today):
						piece = " " + strconv.Itoa(day.Day()) + " "
						if day.Day() == 1 {
							piece = " " + day.Format("2 Jan") + " "
						}

						pieceStyle = lipgloss.NewStyle().Background(theme.Accent).Foreground(todayForeground)

					case day.Day() == 1:
						piece = " " + day.Format("2 Jan")
						pieceStyle = base.Foreground(theme.Accent).Bold(true)
					}

					truncated := ansi.Truncate(piece, cellWidth, "…")

					padding := base.Render(strings.Repeat(" ", max(0, cellWidth-ansi.StringWidth(truncated))))

					row.WriteString(pieceStyle.Render(truncated) + padding)
				}

				lines = append(lines, row.String())

				continue
			}

			laneIndex := lineIndex - 1

			column := 0
			for column < 7 {
				if column > 0 {
					row.WriteString(gridStyle.Render("│"))
				}

				cellWidth := max(0, cellWidths[column])

				base := lipgloss.NewStyle()
				if weekDays[column].Equal(m.selectedDate) {
					base = base.Background(theme.SelectionBg)
				}

				if overflowing[column] && lineIndex == contentLineCount-1 {
					piece := fmt.Sprintf(" +%d more", hiddenCounts[column])

					truncated := ansi.Truncate(piece, cellWidth, "…")

					padding := base.Render(strings.Repeat(" ", max(0, cellWidth-ansi.StringWidth(truncated))))

					row.WriteString(base.Foreground(theme.Muted).Render(truncated) + padding)

					column++

					continue
				}

				eventIndex := -1
				if laneIndex < len(lanes) && (!overflowing[column] || laneIndex < laneCapacity-1) {
					eventIndex = lanes[laneIndex][column]
				}

				if eventIndex < 0 {
					row.WriteString(base.Render(strings.Repeat(" ", cellWidth)))

					column++

					continue
				}

				entry := weekEvents[eventIndex]
				event := entry.event

				runEnd := column
				for runEnd+1 <= entry.lastCol && (!overflowing[runEnd+1] || laneIndex < laneCapacity-1) {
					runEnd++
				}

				runWidth := runEnd - column
				for spanned := column; spanned <= runEnd; spanned++ {
					runWidth += max(0, cellWidths[spanned])
				}

				piece := ""
				switch {
				case entry.continued || column > entry.firstCol:
					piece = " ↳ " + event.Title
				case event.AllDay:
					piece = " " + event.Title + " "
				default:
					timeLabel := event.Start.Format("15:04")
					if marker := timezone.Marker(event.Start, m.location); marker != "" {
						timeLabel += " " + marker
					}

					piece = " " + timeLabel + " " + event.Title
				}

				pieceBase := base
				if runEnd > column {
					pieceBase = lipgloss.NewStyle()
				}

				pieceStyle := pieceBase
				switch {
				case event.ID == m.yankedEventID:
					pieceStyle = pieceBase.Foreground(theme.Yank).Italic(true)
				case event.AllDay || entry.multiDay:
					pieceStyle = lipgloss.NewStyle().Background(lipgloss.Color(event.Color)).Foreground(allDayForeground)
				default:
					pieceStyle = pieceBase.Foreground(lipgloss.Color(event.Color))
				}

				if selectedID != "" && event.ID == selectedID && column <= selectedColumn && selectedColumn <= runEnd {
					pieceStyle = pieceStyle.Reverse(true)
				}

				truncated := ansi.Truncate(piece, runWidth, "…")

				fillWidth := max(0, runWidth-ansi.StringWidth(truncated))

				if entry.multiDay && event.ID != m.yankedEventID {
					row.WriteString(pieceStyle.Render(truncated + strings.Repeat(" ", fillWidth)))
				} else {
					row.WriteString(pieceStyle.Render(truncated) + pieceBase.Render(strings.Repeat(" ", fillWidth)))
				}

				column = runEnd + 1
			}

			lines = append(lines, row.String())
		}

		lines = append(lines, gridStyle.Render(strings.Repeat("─", m.width)))

		week = week.AddDate(0, 0, 7)
	}

	for len(lines) < m.height {
		lines = append(lines, "")
	}

	return strings.Join(lines[:m.height], "\n")
}

func mondayOf(date time.Time) time.Time {
	offset := (int(date.Weekday()) + 6) % 7

	monday := date.AddDate(0, 0, -offset)

	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, monday.Location())
}

func rowHeights(height int) []int {
	gridHeight := height - 2

	weekRowCount := max(2, min(10, gridHeight/6))

	base := gridHeight / weekRowCount
	remainder := gridHeight % weekRowCount

	heights := make([]int, weekRowCount)
	for i := range heights {
		heights[i] = base
		if i < remainder {
			heights[i]++
		}
	}

	return heights
}

func columnWidths(width int, showWeekNumbers bool) (int, []int) {
	gutterWidth := 0
	if showWeekNumbers {
		gutterWidth = 3
	}

	available := width - gutterWidth - 6

	base := available / 7
	remainder := available % 7

	cellWidths := make([]int, 7)
	for i := range cellWidths {
		cellWidths[i] = base
		if i < remainder {
			cellWidths[i]++
		}
	}

	return gutterWidth, cellWidths
}
