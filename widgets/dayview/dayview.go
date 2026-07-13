package dayview

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

const ConfigSection = "dayview"

const (
	totalHalfHourRows      = 48
	gutterWidth            = 6
	maxOverlapLanes        = 4
	eventMarker            = "▎"
	hourRuleCharacter      = "─"
	nowLineCharacter       = "╌"
	selectedEventTextColor = "#1A1B26"
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
	selectedDate   time.Time
	selectionIndex int
	yankedEventID  string
	width          int
	height         int
	scrollOffset   int
	dayStartRow    int
	dayEndRow      int
}

func New(source calendar.Source, config Config, location *time.Location) Model {
	dayStartRow, ok := parseHalfHourRow(config.DayStart)

	if !ok {
		dayStartRow = 0
	}

	dayEndRow, ok := parseHalfHourRow(config.DayEnd)

	if !ok {
		dayEndRow = totalHalfHourRows
	}

	if dayEndRow <= dayStartRow {
		dayStartRow = 0
		dayEndRow = totalHalfHourRows
	}

	now := time.Now().In(location)

	return Model{
		source:         source,
		location:       location,
		selectedDate:   time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()),
		selectionIndex: -1,
		scrollOffset:   dayStartRow,
		dayStartRow:    dayStartRow,
		dayEndRow:      dayEndRow,
	}
}

func parseHalfHourRow(value string) (int, bool) {
	hourText, minuteText, separated := strings.Cut(value, ":")
	if !separated {
		return 0, false
	}

	hour, err := strconv.Atoi(hourText)

	if err != nil || hour < 0 || hour > 24 {
		return 0, false
	}

	minute, err := strconv.Atoi(minuteText)

	if err != nil || minute < 0 || minute > 59 {
		return 0, false
	}

	if hour == 24 && minute != 0 {
		return 0, false
	}

	return hour*2 + minute/30, true
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

		m.scrollOffset = m.clampedScroll(m.scrollOffset)

		return m, nil

	case msgs.FocusDateMsg:
		m.selectedDate = time.Date(msg.Date.Year(), msg.Date.Month(), msg.Date.Day(), 0, 0, 0, 0, msg.Date.Location())
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
			return m.withShiftedDay(-1)

		case "l", "right":
			return m.withShiftedDay(1)

		case "j", "down":
			m.scrollOffset = m.clampedScroll(m.scrollOffset + 1)

			return m, nil

		case "k", "up":
			m.scrollOffset = m.clampedScroll(m.scrollOffset - 1)

			return m, nil

		case "ctrl+d":
			m.scrollOffset = m.clampedScroll(m.scrollOffset + m.visibleRows()/2)

			return m, nil

		case "ctrl+u":
			m.scrollOffset = m.clampedScroll(m.scrollOffset - m.visibleRows()/2)

			return m, nil

		case "t":
			now := time.Now().In(m.location)

			m.selectedDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			m.scrollOffset = m.initialScroll()

			if m.selectionIndex < 0 {
				return m, nil
			}

			m.selectionIndex = -1

			return m, func() tea.Msg { return msgs.EventSelectedMsg{} }

		case "tab", "shift+tab":
			events := m.dayEvents()

			if len(events) == 0 {
				return m, nil
			}

			step := 1
			if msg.String() == "shift+tab" {
				step = -1
			}

			switch {
			case m.selectionIndex < 0 && step > 0:
				m.selectionIndex = 0
			case m.selectionIndex < 0:
				m.selectionIndex = len(events) - 1
			default:
				m.selectionIndex = (m.selectionIndex + step + len(events)) % len(events)
			}

			selected := events[m.selectionIndex]

			if !selected.AllDay {
				startRow, endRow := m.eventRows(selected)
				switch {
				case startRow < m.scrollOffset:
					m.scrollOffset = m.clampedScroll(startRow)
				case endRow > m.scrollOffset+m.visibleRows():
					m.scrollOffset = m.clampedScroll(min(startRow, endRow-m.visibleRows()))
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

			calendars := m.source.Calendars()
			if len(calendars) > 0 {
				template.Calendar = calendars[0].Name
				template.Color = calendars[0].Color
			}

			return m, func() tea.Msg { return msgs.OpenEventFormMsg{Event: template, IsNew: true} }

		case "e", "d", "y":
			selected := m.SelectedEvent()

			if selected == nil {
				return m, nil
			}

			event := *selected

			switch msg.String() {
			case "e":
				return m, func() tea.Msg { return msgs.OpenEventFormMsg{Event: event, IsNew: false} }
			case "d":
				return m, func() tea.Msg { return msgs.RequestDeleteMsg{Event: event} }
			case "y":
				return m, func() tea.Msg { return msgs.YankMsg{Event: event} }
			}

			return m, nil

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

	now := time.Now().In(m.location)

	viewingToday := now.Year() == m.selectedDate.Year() && now.YearDay() == m.selectedDate.YearDay()

	gridStyle := lipgloss.NewStyle().Foreground(theme.Grid)
	mutedStyle := lipgloss.NewStyle().Foreground(theme.Muted)

	// header line
	header := lipgloss.NewStyle().Bold(true).Render(m.selectedDate.Format("Monday 2 January 2006"))
	if viewingToday {
		header += "  " + lipgloss.NewStyle().Foreground(theme.Accent).Render("Today")
	}

	lines := make([]string, 0, m.height)
	lines = append(lines, ansi.Truncate(header, m.width, ""))
	lines = append(lines, gridStyle.Render(strings.Repeat(hourRuleCharacter, m.width)))

	columnWidth := max(0, m.width-gutterWidth)

	// partition all-day events from timed blocks
	type eventBlock struct {
		event     calendar.Event
		index     int
		startRow  int
		endRow    int
		lane      int
		laneX     int
		laneWidth int
	}

	events := m.dayEvents()

	var allDayEvents []calendar.Event
	var allDayIndexes []int
	blocks := make([]eventBlock, 0, len(events))
	for i, event := range events {
		if event.AllDay {
			allDayEvents = append(allDayEvents, event)
			allDayIndexes = append(allDayIndexes, i)

			continue
		}

		startRow, endRow := m.eventRows(event)

		blocks = append(blocks, eventBlock{event: event, index: i, startRow: startRow, endRow: endRow})
	}

	// all-day banner
	if len(allDayEvents) > 0 {
		var banner strings.Builder

		banner.WriteString(strings.Repeat(" ", gutterWidth))
		for i, event := range allDayEvents {
			if i > 0 {
				banner.WriteString(" ")
			}

			chipStyle := lipgloss.NewStyle().
				Background(lipgloss.Color(event.Color)).
				Foreground(lipgloss.Color(selectedEventTextColor))

			switch {
			case event.ID == m.yankedEventID:
				chipStyle = lipgloss.NewStyle().Foreground(theme.Yank).Italic(true)
			case allDayIndexes[i] == m.selectionIndex:
				chipStyle = chipStyle.Reverse(true)
			}

			banner.WriteString(chipStyle.Render(" " + event.Title + " "))
		}

		lines = append(lines, ansi.Truncate(banner.String(), m.width, "…"))
	}

	// cluster overlapping blocks and lay them out into lanes
	type clusterRange struct{ from, to int }

	var clusters []clusterRange

	sweepEnd := 0
	for i, block := range blocks {
		if len(clusters) == 0 || block.startRow >= sweepEnd {
			clusters = append(clusters, clusterRange{from: i, to: i + 1})
			sweepEnd = block.endRow

			continue
		}

		clusters[len(clusters)-1].to = i + 1
		sweepEnd = max(sweepEnd, block.endRow)
	}

	for _, cluster := range clusters {
		var laneEnds []int
		for i := cluster.from; i < cluster.to; i++ {
			lane := len(laneEnds)
			for candidate, occupiedUntil := range laneEnds {
				if blocks[i].startRow >= occupiedUntil {
					lane = candidate

					break
				}
			}

			if lane == len(laneEnds) {
				laneEnds = append(laneEnds, 0)
			}

			laneEnds[lane] = blocks[i].endRow
			blocks[i].lane = lane
		}

		laneCount := min(len(laneEnds), maxOverlapLanes)
		laneWidth := columnWidth / laneCount

		for i := cluster.from; i < cluster.to; i++ {
			if blocks[i].lane >= laneCount {
				continue
			}

			blocks[i].laneX = blocks[i].lane * laneWidth
			blocks[i].laneWidth = laneWidth

			if blocks[i].lane == laneCount-1 {
				blocks[i].laneWidth = columnWidth - laneWidth*(laneCount-1)
			}
		}
	}

	// render the visible half-hour rows
	nowRow := now.Hour()*2 + now.Minute()/30

	for rowOffset := 0; rowOffset < m.visibleRows(); rowOffset++ {
		row := m.scrollOffset + rowOffset
		if row >= totalHalfHourRows {
			lines = append(lines, "")

			continue
		}

		gutter := strings.Repeat(" ", gutterWidth)
		if row%2 == 0 {
			gutter = mutedStyle.Render(fmt.Sprintf("%02d:00 ", row/2))
		}

		fillerCharacter := " "
		fillerStyle := lipgloss.NewStyle()

		switch {
		case viewingToday && row == nowRow:
			fillerCharacter = nowLineCharacter
			fillerStyle = lipgloss.NewStyle().Foreground(theme.Danger)
		case row%2 == 0:
			fillerCharacter = hourRuleCharacter
			fillerStyle = gridStyle
		}

		var rowBlocks []eventBlock
		for _, block := range blocks {
			if block.laneWidth > 0 && row >= block.startRow && row < block.endRow {
				rowBlocks = append(rowBlocks, block)
			}
		}

		sort.Slice(rowBlocks, func(i, j int) bool { return rowBlocks[i].laneX < rowBlocks[j].laneX })

		var column strings.Builder

		cursor := 0
		for _, block := range rowBlocks {
			if block.laneX > cursor {
				column.WriteString(fillerStyle.Render(strings.Repeat(fillerCharacter, block.laneX-cursor)))
			}

			plainBody := ""
			styledBody := ""
			if row == block.startRow {
				startLabel := block.event.Start.Format("15:04")
				if marker := timezone.Marker(block.event.Start, m.location); marker != "" {
					startLabel += " " + marker
				}

				endLabel := block.event.End.Format("15:04")
				if marker := timezone.Marker(block.event.End, m.location); marker != "" {
					endLabel += " " + marker
				}

				times := startLabel + "-" + endLabel
				plainBody = times + " " + block.event.Title
				styledBody = mutedStyle.Render(times) + " " + block.event.Title

				if block.event.Location != "" {
					plainBody += " · " + block.event.Location
					styledBody += mutedStyle.Render(" · " + block.event.Location)
				}
			}

			isSelected := block.index == m.selectionIndex
			isYanked := block.event.ID == m.yankedEventID

			switch {
			case isSelected:
				style := lipgloss.NewStyle().
					Background(lipgloss.Color(block.event.Color)).
					Foreground(lipgloss.Color(selectedEventTextColor)).
					Italic(isYanked)

				span := ansi.Truncate(eventMarker+plainBody, block.laneWidth, "…")

				column.WriteString(style.Render(span + strings.Repeat(" ", max(0, block.laneWidth-ansi.StringWidth(span)))))

			case isYanked:
				style := lipgloss.NewStyle().Foreground(theme.Yank).Italic(true)

				span := ansi.Truncate(eventMarker+plainBody, block.laneWidth, "…")

				column.WriteString(style.Render(span) + strings.Repeat(" ", max(0, block.laneWidth-ansi.StringWidth(span))))

			default:
				marker := lipgloss.NewStyle().Foreground(lipgloss.Color(block.event.Color)).Render(eventMarker)

				text := ansi.Truncate(styledBody, block.laneWidth-1, "…")

				column.WriteString(marker + text + strings.Repeat(" ", max(0, block.laneWidth-1-ansi.StringWidth(text))))
			}

			cursor = block.laneX + block.laneWidth
		}

		if cursor < columnWidth {
			column.WriteString(fillerStyle.Render(strings.Repeat(fillerCharacter, columnWidth-cursor)))
		}

		lines = append(lines, ansi.Truncate(gutter+column.String(), m.width, ""))
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
	if m.selectionIndex < 0 {
		return nil
	}

	events := m.dayEvents()

	if m.selectionIndex >= len(events) {
		return nil
	}

	event := events[m.selectionIndex]

	return &event
}

func (m Model) KeyHints() []msgs.KeyHint {
	if m.SelectedEvent() != nil {
		return []msgs.KeyHint{
			{Key: "tab", Action: "next"},
			{Key: "S-tab", Action: "prev"},
			{Key: "e", Action: "edit"},
			{Key: "d", Action: "delete"},
			{Key: "y", Action: "yank"},
			{Key: "esc", Action: "deselect"},
		}
	}

	return []msgs.KeyHint{
		{Key: "h/l", Action: "day"},
		{Key: "j/k", Action: "scroll"},
		{Key: "tab", Action: "select"},
		{Key: "n", Action: "new"},
		{Key: "p", Action: "paste"},
		{Key: "t", Action: "today"},
	}
}

func (m Model) withShiftedDay(days int) (tea.Model, tea.Cmd) {
	m.selectedDate = m.selectedDate.AddDate(0, 0, days)

	if m.selectionIndex < 0 {
		return m, nil
	}

	m.selectionIndex = -1

	return m, func() tea.Msg { return msgs.EventSelectedMsg{} }
}

func (m Model) dayEvents() []calendar.Event {
	events := m.source.Events(m.selectedDate, m.selectedDate.AddDate(0, 0, 1))

	sort.SliceStable(events, func(i, j int) bool { return events[i].AllDay && !events[j].AllDay })

	return events
}

func (m Model) eventRows(event calendar.Event) (int, int) {
	dayStart := m.selectedDate
	nextDayStart := dayStart.AddDate(0, 0, 1)

	localStart := event.Start.In(dayStart.Location())
	localEnd := event.End.In(dayStart.Location())

	startRow := 0
	if !event.Start.Before(dayStart) {
		startRow = localStart.Hour()*2 + localStart.Minute()/30
	}

	endRow := totalHalfHourRows
	if event.End.Before(nextDayStart) {
		endMinutes := localEnd.Hour()*60 + localEnd.Minute()

		endRow = (endMinutes + 29) / 30
	}

	if endRow <= startRow {
		endRow = startRow + 1
	}

	return startRow, endRow
}

func (m Model) visibleRows() int {
	rows := m.height - 2

	for _, event := range m.dayEvents() {
		if event.AllDay {
			rows--

			break
		}
	}

	return max(0, rows)
}

func (m Model) clampedScroll(offset int) int {
	maxOffset := max(0, totalHalfHourRows-m.visibleRows())

	return max(0, min(offset, maxOffset))
}

func (m Model) initialScroll() int {
	windowRows := m.dayEndRow - m.dayStartRow

	surplusRows := max(m.visibleRows()-windowRows, 0)

	offset := m.clampedScroll(m.dayStartRow - surplusRows/2)

	return offset - offset%2
}
