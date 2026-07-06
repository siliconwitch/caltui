package weekview

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

const ConfigSection = "weekview"

const (
	gutterWidth      = 6
	halfHourRowCount = 48
	maxLanes         = 3
)

var (
	gridStyle               = lipgloss.NewStyle().Foreground(theme.Grid)
	mutedStyle              = lipgloss.NewStyle().Foreground(theme.Muted)
	dangerStyle             = lipgloss.NewStyle().Foreground(theme.Danger)
	yankedStyle             = lipgloss.NewStyle().Foreground(theme.Yank).Italic(true)
	selectedEventForeground = lipgloss.Color("#16161E")
)

type Config struct {
	DayStart string `toml:"day_start"`
	DayEnd   string `toml:"day_end"`
}

func DefaultConfig() Config {
	return Config{DayStart: "00:00", DayEnd: "24:00"}
}

type Model struct {
	source         calendar.Source
	location       *time.Location
	dayStartRow    int
	dayEndRow      int
	weekMonday     time.Time
	selectedDate   time.Time
	scrollOffset   int
	selectionIndex int
	yankedEventID  string
	width          int
	height         int
}

func New(source calendar.Source, config Config, location *time.Location) Model {
	defaults := DefaultConfig()

	if _, ok := parseHalfHourRow(config.DayStart); !ok {
		config.DayStart = defaults.DayStart
	}
	if _, ok := parseHalfHourRow(config.DayEnd); !ok {
		config.DayEnd = defaults.DayEnd
	}

	dayStartRow, _ := parseHalfHourRow(config.DayStart)

	dayEndRow, _ := parseHalfHourRow(config.DayEnd)

	if dayEndRow <= dayStartRow {
		dayStartRow = 0
		dayEndRow = halfHourRowCount
	}

	now := time.Now().In(location)

	return Model{
		source:         source,
		location:       location,
		dayStartRow:    dayStartRow,
		dayEndRow:      dayEndRow,
		weekMonday:     mondayOf(now),
		selectedDate:   startOfDay(now),
		scrollOffset:   dayStartRow,
		selectionIndex: -1,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		unsized := m.height <= 0

		m.width = msg.Width
		m.height = msg.Height

		if unsized {
			m.scrollOffset = m.initialScroll()

			return m, nil
		}

		m.scrollOffset = m.clampScroll(m.scrollOffset)

		return m, nil

	case msgs.FocusDateMsg:
		m.selectedDate = startOfDay(msg.Date)
		m.weekMonday = mondayOf(m.selectedDate)
		m.selectionIndex = -1

		return m, nil

	case msgs.EventsChangedMsg:
		m.selectionIndex = -1

		return m, nil

	case msgs.YankedMsg:
		m.yankedEventID = msg.EventID

		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "h", "left":
			return m.moveCursor(m.selectedDate.AddDate(0, 0, -1))

		case "l", "right":
			return m.moveCursor(m.selectedDate.AddDate(0, 0, 1))

		case "j", "down":
			m.scrollOffset = m.clampScroll(m.scrollOffset + 1)

			return m, nil

		case "k", "up":
			m.scrollOffset = m.clampScroll(m.scrollOffset - 1)

			return m, nil

		case "ctrl+d":
			m.scrollOffset = m.clampScroll(m.scrollOffset + max(m.visibleRows()/2, 1))

			return m, nil

		case "ctrl+u":
			m.scrollOffset = m.clampScroll(m.scrollOffset - max(m.visibleRows()/2, 1))

			return m, nil

		case "t":
			moved, cmd := m.moveCursor(startOfDay(time.Now().In(m.location)))
			moved.scrollOffset = moved.initialScroll()

			return moved, cmd

		case "tab", "shift+tab":
			events := m.selectedDayEvents()

			if len(events) == 0 {
				return m, nil
			}

			switch {
			case msg.String() == "tab":
				m.selectionIndex = (m.selectionIndex + 1) % len(events)
			case m.selectionIndex < 0:
				m.selectionIndex = len(events) - 1
			default:
				m.selectionIndex = (m.selectionIndex + len(events) - 1) % len(events)
			}

			selected := events[m.selectionIndex]

			if !selected.AllDay {
				startRow, endRow := eventRowSpan(selected, m.selectedDate, m.selectedDate.AddDate(0, 0, 1))

				visibleRows := m.visibleRows()
				switch {
				case visibleRows <= 0:
				case startRow < m.scrollOffset || endRow-startRow >= visibleRows:
					m.scrollOffset = m.clampScroll(startRow)
				case endRow > m.scrollOffset+visibleRows:
					m.scrollOffset = m.clampScroll(endRow - visibleRows)
				}
			}

			return m, func() tea.Msg { return msgs.EventSelectedMsg{Event: &selected} }

		case "esc":
			if m.selectionIndex < 0 {
				return m, nil
			}

			m.selectionIndex = -1

			return m, func() tea.Msg { return msgs.EventSelectedMsg{} }

		case "n":
			year, month, day := m.selectedDate.Date()

			template := calendar.Event{
				Start: time.Date(year, month, day, 9, 0, 0, 0, m.selectedDate.Location()),
				End:   time.Date(year, month, day, 10, 0, 0, 0, m.selectedDate.Location()),
			}

			if calendars := m.source.Calendars(); len(calendars) > 0 {
				template.Calendar = calendars[0].Name
				template.Color = calendars[0].Color
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

func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	availableWidth := max(m.width-gutterWidth-6, 7)

	baseColumnWidth := availableWidth / 7

	widthRemainder := availableWidth % 7

	var columnWidths [7]int
	for dayIndex := range columnWidths {
		columnWidths[dayIndex] = baseColumnWidth
		if dayIndex < widthRemainder {
			columnWidths[dayIndex]++
		}
	}

	now := time.Now().In(m.location)

	nowRow := now.Hour()*2 + now.Minute()/30

	var selectedEventID string
	if selected := m.SelectedEvent(); selected != nil {
		selectedEventID = selected.ID
	}

	type eventBlock struct {
		event    calendar.Event
		startRow int
		endRow   int
		lane     int
	}

	type eventCluster struct {
		startRow    int
		endRow      int
		laneEnds    []int
		blocks      []eventBlock
		overflow    int
		overflowRow int
	}

	var headerCells [7]string
	var bannerCells [7]string
	var dayCells [7][halfHourRowCount]string

	for dayIndex := range 7 {
		columnWidth := columnWidths[dayIndex]

		day := m.weekMonday.AddDate(0, 0, dayIndex)

		nextDay := day.AddDate(0, 0, 1)

		isToday := sameDay(day, now)

		labelStyle := lipgloss.NewStyle().Width(columnWidth).Align(lipgloss.Center)
		if isToday {
			labelStyle = labelStyle.Foreground(theme.Accent).Bold(true)
		}
		if sameDay(day, m.selectedDate) {
			labelStyle = labelStyle.Background(theme.SelectionBg)
		}

		headerCells[dayIndex] = labelStyle.Render(ansi.Truncate(day.Format("Mon 2"), columnWidth, ""))

		events := m.source.Events(day, nextDay)

		sort.SliceStable(events, func(i, j int) bool { return events[i].Start.Before(events[j].Start) })

		var allDayEvents []calendar.Event
		var timedEvents []calendar.Event
		for _, event := range events {
			if event.AllDay {
				allDayEvents = append(allDayEvents, event)

				continue
			}

			timedEvents = append(timedEvents, event)
		}

		bannerCells[dayIndex] = strings.Repeat(" ", columnWidth)

		if len(allDayEvents) > 0 {
			first := allDayEvents[0]

			marker := ""
			if len(allDayEvents) > 1 {
				marker = fmt.Sprintf(" +%d", len(allDayEvents)-1)
			}

			chip := ansi.Truncate(" "+first.Title+" ", max(columnWidth-ansi.StringWidth(marker), 0), "…")

			chipStyle := lipgloss.NewStyle().Background(lipgloss.Color(first.Color)).Foreground(selectedEventForeground)
			if first.ID == m.yankedEventID {
				chipStyle = yankedStyle
			}
			if selectedEventID != "" && first.ID == selectedEventID {
				chipStyle = chipStyle.Reverse(true)
			}

			filler := strings.Repeat(" ", max(columnWidth-ansi.StringWidth(chip)-ansi.StringWidth(marker), 0))

			bannerCells[dayIndex] = chipStyle.Render(chip) + mutedStyle.Render(marker) + filler
		}

		var clusters []eventCluster

		for _, event := range timedEvents {
			startRow, endRow := eventRowSpan(event, day, nextDay)

			if len(clusters) == 0 || startRow >= clusters[len(clusters)-1].endRow {
				clusters = append(clusters, eventCluster{startRow: startRow, endRow: endRow, overflowRow: -1})
			}

			current := &clusters[len(clusters)-1]
			current.endRow = max(current.endRow, endRow)

			lane := len(current.laneEnds)
			for laneIndex, laneEnd := range current.laneEnds {
				if laneEnd <= startRow {
					lane = laneIndex
					break
				}
			}

			if lane == len(current.laneEnds) {
				current.laneEnds = append(current.laneEnds, endRow)
			} else {
				current.laneEnds[lane] = endRow
			}

			if lane >= maxLanes {
				current.overflow++
				continue
			}

			if lane == maxLanes-1 && current.overflowRow < 0 {
				current.overflowRow = startRow
			}

			current.blocks = append(current.blocks, eventBlock{event: event, startRow: startRow, endRow: endRow, lane: lane})
		}

		for row := range halfHourRowCount {
			var covering *eventCluster
			for clusterIndex := range clusters {
				if clusters[clusterIndex].startRow <= row && row < clusters[clusterIndex].endRow {
					covering = &clusters[clusterIndex]
					break
				}
			}

			nowIndicator := isToday && row == nowRow

			if covering == nil {
				switch {
				case nowIndicator:
					dayCells[dayIndex][row] = dangerStyle.Render(strings.Repeat("╌", columnWidth))
				case row%2 == 0:
					dayCells[dayIndex][row] = gridStyle.Render(strings.Repeat("─", columnWidth))
				default:
					dayCells[dayIndex][row] = strings.Repeat(" ", columnWidth)
				}

				continue
			}

			laneCount := min(len(covering.laneEnds), maxLanes)

			baseLaneWidth := columnWidth / laneCount

			var cell strings.Builder
			for lane := range laneCount {
				laneWidth := baseLaneWidth
				if lane == laneCount-1 {
					laneWidth = columnWidth - baseLaneWidth*(laneCount-1)
				}

				var laneBlock *eventBlock
				for blockIndex := range covering.blocks {
					candidate := &covering.blocks[blockIndex]
					if candidate.lane == lane && candidate.startRow <= row && row < candidate.endRow {
						laneBlock = candidate
						break
					}
				}

				if laneBlock == nil {
					if nowIndicator {
						cell.WriteString(dangerStyle.Render(strings.Repeat("╌", laneWidth)))
					} else {
						cell.WriteString(strings.Repeat(" ", laneWidth))
					}

					continue
				}

				overflowLabel := ""
				if lane == laneCount-1 && covering.overflow > 0 && row == covering.overflowRow {
					overflowLabel = fmt.Sprintf("+%d", covering.overflow)
				}
				if ansi.StringWidth(overflowLabel) >= laneWidth {
					overflowLabel = ""
				}

				textWidth := laneWidth - ansi.StringWidth(overflowLabel)

				text := ""
				if textWidth > 0 {
					text = "▎"
					if row == laneBlock.startRow {
						timeLabel := laneBlock.event.Start.Format("15:04")
						if marker := timezone.Marker(laneBlock.event.Start, m.location); marker != "" {
							timeLabel += " " + marker
						}

						text = "▎" + timeLabel + " " + laneBlock.event.Title
					}

					text = ansi.Truncate(text, textWidth, "…")

					text += strings.Repeat(" ", max(textWidth-ansi.StringWidth(text), 0))
				}

				blockStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(laneBlock.event.Color))
				switch {
				case selectedEventID != "" && laneBlock.event.ID == selectedEventID:
					blockStyle = lipgloss.NewStyle().
						Background(lipgloss.Color(laneBlock.event.Color)).
						Foreground(selectedEventForeground).
						Italic(laneBlock.event.ID == m.yankedEventID)
				case m.yankedEventID != "" && laneBlock.event.ID == m.yankedEventID:
					blockStyle = yankedStyle
				}

				cell.WriteString(blockStyle.Render(text))

				if overflowLabel != "" {
					cell.WriteString(mutedStyle.Render(overflowLabel))
				}
			}

			dayCells[dayIndex][row] = cell.String()
		}
	}

	separator := gridStyle.Render("│")

	lines := make([]string, 0, m.height)

	headerLine := strings.Repeat(" ", gutterWidth) + strings.Join(headerCells[:], " ")

	lines = append(lines, ansi.Truncate(headerLine, m.width, ""))
	lines = append(lines, gridStyle.Render(strings.Repeat("─", m.width)))

	if m.bannerRowCount() > 0 {
		var banner strings.Builder

		banner.WriteString(strings.Repeat(" ", gutterWidth))
		for dayIndex := range 7 {
			if dayIndex > 0 {
				banner.WriteString(separator)
			}

			banner.WriteString(bannerCells[dayIndex])
		}

		lines = append(lines, ansi.Truncate(banner.String(), m.width, ""))
	}

	scrollOffset := m.clampScroll(m.scrollOffset)

	for rowIndex := range m.visibleRows() {
		row := scrollOffset + rowIndex
		if row >= halfHourRowCount {
			lines = append(lines, "")
			continue
		}

		gutter := strings.Repeat(" ", gutterWidth)
		if row%2 == 0 {
			gutter = mutedStyle.Render(fmt.Sprintf("%*s", gutterWidth, fmt.Sprintf("%02d:00", row/2)))
		}

		var line strings.Builder
		line.WriteString(gutter)
		for dayIndex := range 7 {
			if dayIndex > 0 {
				line.WriteString(separator)
			}
			line.WriteString(dayCells[dayIndex][row])
		}

		lines = append(lines, ansi.Truncate(line.String(), m.width, ""))
	}

	if len(lines) > m.height {
		lines = lines[:m.height]
	}

	return strings.Join(lines, "\n")
}

func (m Model) FocusedDate() time.Time {
	return m.selectedDate
}

func (m Model) SelectedEvent() *calendar.Event {
	events := m.selectedDayEvents()

	if m.selectionIndex < 0 || m.selectionIndex >= len(events) {
		return nil
	}

	selected := events[m.selectionIndex]

	return &selected
}

func (m Model) KeyHints() []msgs.KeyHint {
	if m.SelectedEvent() == nil {
		return []msgs.KeyHint{
			{Key: "h/l", Action: "day"},
			{Key: "j/k", Action: "scroll"},
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

func (m Model) moveCursor(target time.Time) (Model, tea.Cmd) {
	m.selectedDate = target
	m.weekMonday = mondayOf(target)

	if m.selectionIndex < 0 {
		return m, nil
	}

	m.selectionIndex = -1

	return m, func() tea.Msg { return msgs.EventSelectedMsg{} }
}

func (m Model) selectedDayEvents() []calendar.Event {
	events := m.source.Events(m.selectedDate, m.selectedDate.AddDate(0, 0, 1))

	sort.SliceStable(events, func(i, j int) bool { return events[i].Start.Before(events[j].Start) })
	sort.SliceStable(events, func(i, j int) bool { return events[i].AllDay && !events[j].AllDay })

	return events
}

func (m Model) clampScroll(offset int) int {
	maxOffset := max(halfHourRowCount-m.visibleRows(), 0)

	return min(max(offset, 0), maxOffset)
}

func (m Model) visibleRows() int {
	return max(m.height-2-m.bannerRowCount(), 0)
}

func (m Model) bannerRowCount() int {
	for dayIndex := range 7 {
		day := m.weekMonday.AddDate(0, 0, dayIndex)

		for _, event := range m.source.Events(day, day.AddDate(0, 0, 1)) {
			if event.AllDay {
				return 1
			}
		}
	}

	return 0
}

func (m Model) initialScroll() int {
	windowRows := m.dayEndRow - m.dayStartRow

	surplusRows := max(m.visibleRows()-windowRows, 0)

	offset := m.clampScroll(m.dayStartRow - surplusRows/2)

	return offset - offset%2
}

func eventRowSpan(event calendar.Event, dayStart, nextDayStart time.Time) (int, int) {
	localStart := event.Start.In(dayStart.Location())

	localEnd := event.End.In(dayStart.Location())

	startRow := 0
	if event.Start.After(dayStart) {
		startRow = (localStart.Hour()*60 + localStart.Minute()) / 30
	}

	endRow := halfHourRowCount
	if event.End.Before(nextDayStart) {
		endRow = (localEnd.Hour()*60 + localEnd.Minute() + 29) / 30
	}

	if endRow <= startRow {
		endRow = startRow + 1
	}

	return startRow, endRow
}

func parseHalfHourRow(value string) (int, bool) {
	hoursText, minutesText, found := strings.Cut(value, ":")
	if !found {
		return 0, false
	}

	hours, hoursErr := strconv.Atoi(hoursText)

	minutes, minutesErr := strconv.Atoi(minutesText)

	if hoursErr != nil || minutesErr != nil || hours < 0 || minutes < 0 || minutes > 59 {
		return 0, false
	}

	totalMinutes := hours*60 + minutes
	if totalMinutes > 24*60 {
		return 0, false
	}

	return totalMinutes / 30, true
}

func mondayOf(date time.Time) time.Time {
	monday := date.AddDate(0, 0, -(int(date.Weekday())+6)%7)

	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, date.Location())
}

func startOfDay(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}
