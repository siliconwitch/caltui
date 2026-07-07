package eventform

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/maskinput"
	"github.com/siliconwitch/caltui/msgs"
	"github.com/siliconwitch/caltui/theme"
	"github.com/siliconwitch/caltui/timezone"
)

const (
	titleSlot = iota
	allDaySlot
	startDateSlot
	startTimeSlot
	startZoneSlot
	endDateSlot
	endTimeSlot
	endZoneSlot
	calendarSlot
)

const (
	maxInnerWidth = 46
	minInnerWidth = 20
	labelWidth    = 10
)

type Model struct {
	calendars     []calendar.Calendar
	location      *time.Location
	titleInput    textinput.Model
	startDate     maskinput.Field
	startTime     maskinput.Field
	endDate       maskinput.Field
	endTime       maskinput.Field
	startZone     *time.Location
	endZone       *time.Location
	picker        timezone.Picker
	pickerOpen    bool
	pickerTarget  int
	allDay        bool
	calendarIndex int
	focusedSlot   int
	original      calendar.Event
	isNew         bool
	errorText     string
	innerWidth    int
}

func New(calendars []calendar.Calendar, location *time.Location) Model {
	titleInput := textinput.New()
	titleInput.Prompt = ""
	titleInput.Width = maxInnerWidth - labelWidth - 2

	return Model{
		calendars:  calendars,
		location:   location,
		titleInput: titleInput,
		startDate:  maskinput.NewDate(false),
		startTime:  maskinput.NewTime(),
		endDate:    maskinput.NewDate(false),
		endTime:    maskinput.NewTime(),
		startZone:  location,
		endZone:    location,
		picker:     timezone.NewPicker(),
		innerWidth: maxInnerWidth,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.innerWidth = min(maxInnerWidth, max(minInnerWidth, msg.Width-4))
		m.titleInput.Width = m.innerWidth - labelWidth - 2

		return m, nil

	case msgs.OpenEventFormMsg:
		m.original = msg.Event
		m.isNew = msg.IsNew
		m.errorText = ""
		m.allDay = msg.Event.AllDay
		m.pickerOpen = false
		m.calendarIndex = 0

		for index, option := range m.calendars {
			if option.Name == msg.Event.Calendar {
				m.calendarIndex = index
			}
		}

		m.startZone = m.location
		m.endZone = m.location
		if !msg.Event.Start.IsZero() {
			m.startZone = msg.Event.Start.Location()
			m.endZone = msg.Event.End.Location()
		}

		endDisplay := msg.Event.End
		if msg.Event.AllDay && msg.Event.End.After(msg.Event.Start) {
			endDisplay = msg.Event.End.AddDate(0, 0, -1)
		}

		m.titleInput.SetValue(msg.Event.Title)
		m.startDate = m.startDate.WithDate(msg.Event.Start)
		m.startTime = m.startTime.WithTime(msg.Event.Start.Hour(), msg.Event.Start.Minute())
		m.endDate = m.endDate.WithDate(endDisplay)
		m.endTime = m.endTime.WithTime(endDisplay.Hour(), endDisplay.Minute())

		m = m.withFocusedSlot(titleSlot)

		return m, textinput.Blink

	case msgs.CalendarsChangedMsg:
		previous := ""
		if m.calendarIndex < len(m.calendars) {
			previous = m.calendars[m.calendarIndex].Name
		}

		m.calendars = msg.Calendars
		m.calendarIndex = 0

		for index, option := range m.calendars {
			if option.Name == previous {
				m.calendarIndex = index
			}
		}

		return m, nil

	case tea.KeyMsg:
		if m.pickerOpen {
			picker, selected, closed := m.picker.Typed(msg.String())
			m.picker = picker

			if selected != nil {
				switch m.pickerTarget {
				case startZoneSlot:
					if m.endZone == m.startZone {
						m.endZone = selected
					}

					m.startZone = selected
				case endZoneSlot:
					m.endZone = selected
				}
			}

			if closed {
				m.pickerOpen = false
			}

			return m, nil
		}

		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return msgs.ClosePopupMsg{} }

		case "tab", "down":
			m = m.withShiftedFocus(1)

			return m, textinput.Blink

		case "shift+tab", "up":
			m = m.withShiftedFocus(-1)

			return m, textinput.Blink

		case "enter":
			if m.focusedSlot == startZoneSlot || m.focusedSlot == endZoneSlot {
				return m.withOpenPicker(""), nil
			}

			if m.isNew && len(m.calendars) == 0 {
				m.errorText = "No writable calendars"

				return m, nil
			}

			calendarName := m.original.Calendar
			if len(m.calendars) > 0 {
				calendarName = m.calendars[m.calendarIndex].Name
			}

			submitted, problem := composedEvent(
				m.original,
				strings.TrimSpace(m.titleInput.Value()),
				m.allDay,
				m.startDate, m.startTime, m.endDate, m.endTime,
				m.startZone, m.endZone, m.location,
				calendarName,
			)

			if problem != "" {
				m.errorText = problem

				return m, nil
			}

			m.errorText = ""
			isNew := m.isNew

			return m, func() tea.Msg {
				return msgs.EventFormSubmittedMsg{Event: submitted, IsNew: isNew}
			}
		}

		switch m.focusedSlot {
		case allDaySlot:
			switch msg.String() {
			case " ", "left", "right":
				m.allDay = !m.allDay
			}

			return m, nil

		case calendarSlot:
			if len(m.calendars) == 0 {
				return m, nil
			}

			switch msg.String() {
			case "left":
				m.calendarIndex = (m.calendarIndex + len(m.calendars) - 1) % len(m.calendars)
			case "right", " ":
				m.calendarIndex = (m.calendarIndex + 1) % len(m.calendars)
			}

			return m, nil

		case startZoneSlot, endZoneSlot:
			key := msg.String()

			switch key {
			case "left", "right", "backspace", "home", "end":
				return m, nil
			case " ":
				return m.withOpenPicker(""), nil
			}

			for _, character := range key {
				if character < ' ' {
					return m, nil
				}
			}

			return m.withOpenPicker(key), nil

		case startDateSlot, startTimeSlot, endDateSlot, endTimeSlot:
			completed := false

			switch m.focusedSlot {
			case startDateSlot:
				m.startDate, completed = m.startDate.Typed(msg.String())
			case startTimeSlot:
				m.startTime, completed = m.startTime.Typed(msg.String())
			case endDateSlot:
				m.endDate, completed = m.endDate.Typed(msg.String())
			case endTimeSlot:
				m.endTime, completed = m.endTime.Typed(msg.String())
			}

			if completed {
				m = m.withTypingAdvance()
			}

			return m, nil

		default:
			input, cmd := m.titleInput.Update(msg)
			m.titleInput = input

			return m, cmd
		}

	default:
		input, cmd := m.titleInput.Update(msg)
		m.titleInput = input

		return m, cmd
	}
}

func (m Model) slotOrder() []int {
	if m.allDay {
		return []int{titleSlot, allDaySlot, startDateSlot, endDateSlot, calendarSlot}
	}

	return []int{
		titleSlot, allDaySlot,
		startDateSlot, startTimeSlot, startZoneSlot,
		endDateSlot, endTimeSlot, endZoneSlot,
		calendarSlot,
	}
}

func (m Model) withShiftedFocus(step int) Model {
	order := m.slotOrder()

	position := 0
	for index, slot := range order {
		if slot == m.focusedSlot {
			position = index
		}
	}

	return m.withFocusedSlot(order[(position+step+len(order))%len(order)])
}

func (m Model) withTypingAdvance() Model {
	switch m.focusedSlot {
	case startDateSlot:
		if m.allDay {
			return m.withFocusedSlot(endDateSlot)
		}

		return m.withFocusedSlot(startTimeSlot)

	case startTimeSlot:
		return m.withFocusedSlot(endDateSlot)

	case endDateSlot:
		if m.allDay {
			return m.withFocusedSlot(calendarSlot)
		}

		return m.withFocusedSlot(endTimeSlot)

	case endTimeSlot:
		return m.withFocusedSlot(calendarSlot)
	}

	return m
}

func (m Model) withFocusedSlot(slot int) Model {
	m.focusedSlot = slot
	m.titleInput.Blur()
	m.startDate = m.startDate.Blur()
	m.startTime = m.startTime.Blur()
	m.endDate = m.endDate.Blur()
	m.endTime = m.endTime.Blur()

	switch slot {
	case titleSlot:
		m.titleInput.Focus()
	case startDateSlot:
		m.startDate = m.startDate.Focus()
	case startTimeSlot:
		m.startTime = m.startTime.Focus()
	case endDateSlot:
		m.endDate = m.endDate.Focus()
	case endTimeSlot:
		m.endTime = m.endTime.Focus()
	}

	return m
}

func (m Model) withOpenPicker(query string) Model {
	dateField, timeField := m.startDate, m.startTime
	if m.focusedSlot == endZoneSlot {
		dateField, timeField = m.endDate, m.endTime
	}

	year, month, day := dateField.Date()

	hour, minute := timeField.Time()

	reference := time.Date(year, month, day, hour, minute, 0, 0, m.location)

	m.picker = m.picker.Opened(reference, query)
	m.pickerOpen = true
	m.pickerTarget = m.focusedSlot

	return m
}

func composedEvent(
	original calendar.Event,
	title string,
	allDay bool,
	startDate, startTime, endDate, endTime maskinput.Field,
	startZone, endZone, defaultLocation *time.Location,
	calendarName string,
) (calendar.Event, string) {
	startYear, startMonth, startDay := startDate.Date()

	endYear, endMonth, endDay := endDate.Date()

	event := original
	event.Title = title
	event.AllDay = allDay
	event.Calendar = calendarName
	event.Color = ""

	if allDay {
		event.Start = time.Date(startYear, startMonth, startDay, 0, 0, 0, 0, defaultLocation)
		event.End = time.Date(endYear, endMonth, endDay, 0, 0, 0, 0, defaultLocation).AddDate(0, 0, 1)

		if event.End.Before(event.Start.AddDate(0, 0, 1)) {
			return event, "End must not be before start"
		}

		return event, ""
	}

	startHour, startMinute := startTime.Time()

	endHour, endMinute := endTime.Time()

	event.Start = time.Date(startYear, startMonth, startDay, startHour, startMinute, 0, 0, startZone)
	event.End = time.Date(endYear, endMonth, endDay, endHour, endMinute, 0, 0, endZone)

	if !event.End.After(event.Start) {
		return event, "End must be after start"
	}

	return event, ""
}

func (m Model) zoneLabel(slot int) string {
	dateField, timeField, zone := m.startDate, m.startTime, m.startZone
	if slot == endZoneSlot {
		dateField, timeField, zone = m.endDate, m.endTime, m.endZone
	}

	year, month, day := dateField.Date()

	hour, minute := timeField.Time()

	abbreviation, _ := time.Date(year, month, day, hour, minute, 0, 0, zone).Zone()

	style := lipgloss.NewStyle().Foreground(theme.Muted)
	if m.focusedSlot == slot && !m.pickerOpen {
		style = lipgloss.NewStyle().Reverse(true)
	}

	return style.Render(abbreviation)
}

func (m Model) View() string {
	heading := "Edit event"
	if m.isNew {
		heading = "New event"
	}

	lines := []string{lipgloss.NewStyle().Bold(true).Render(heading), ""}

	chevronStyle := lipgloss.NewStyle().Foreground(theme.Accent)

	allDayValue := "[ ]"
	if m.allDay {
		allDayValue = "[x]"
	}
	if m.focusedSlot == allDaySlot {
		allDayValue = chevronStyle.Render("‹ ") + allDayValue + chevronStyle.Render(" ›")
	}

	startValue := m.startDate.View()
	endValue := m.endDate.View()
	if !m.allDay {
		startValue += "  " + m.startTime.View() + "  " + m.zoneLabel(startZoneSlot)
		endValue += "  " + m.endTime.View() + "  " + m.zoneLabel(endZoneSlot)
	}

	calendarValue := m.original.Calendar
	bulletColor := lipgloss.Color(m.original.Color)

	if len(m.calendars) > 0 {
		selected := m.calendars[m.calendarIndex]
		calendarValue = selected.Name
		bulletColor = lipgloss.Color(selected.Color)
	}

	if m.isNew && len(m.calendars) == 0 {
		calendarValue = "none writable"
		bulletColor = lipgloss.Color("")
	}

	calendarValue = lipgloss.NewStyle().Foreground(bulletColor).Render("●") + " " + calendarValue
	if m.focusedSlot == calendarSlot {
		calendarValue = chevronStyle.Render("‹ ") + calendarValue + chevronStyle.Render(" ›")
	}

	rows := []struct {
		label string
		slots []int
		value string
	}{
		{"Title", []int{titleSlot}, m.titleInput.View()},
		{"All day", []int{allDaySlot}, allDayValue},
		{"Start", []int{startDateSlot, startTimeSlot, startZoneSlot}, startValue},
		{"End", []int{endDateSlot, endTimeSlot, endZoneSlot}, endValue},
		{"Calendar", []int{calendarSlot}, calendarValue},
	}

	for _, row := range rows {
		labelStyle := lipgloss.NewStyle().Foreground(theme.Muted)
		if slices.Contains(row.slots, m.focusedSlot) {
			labelStyle = labelStyle.Foreground(theme.Accent)
		}

		label := labelStyle.Render(fmt.Sprintf("%-*s", labelWidth, row.label))

		lines = append(lines, ansi.Truncate(label+row.value, m.innerWidth, ""))
	}

	if m.pickerOpen {
		lines = append(lines, "")
		lines = append(lines, m.picker.View(m.innerWidth)...)
	}

	if m.errorText != "" {
		errorLine := lipgloss.NewStyle().Foreground(theme.Danger).Render(m.errorText)

		lines = append(lines, "", ansi.Truncate(errorLine, m.innerWidth, ""))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Padding(0, 1).
		Width(m.innerWidth + 2)

	return box.Render(strings.Join(lines, "\n"))
}

func (m Model) KeyHints() []msgs.KeyHint {
	if m.pickerOpen {
		return []msgs.KeyHint{
			{Key: "type", Action: "search"},
			{Key: "↑/↓", Action: "choose"},
			{Key: "enter", Action: "select"},
			{Key: "esc", Action: "back"},
		}
	}

	return []msgs.KeyHint{
		{Key: "tab", Action: "next field"},
		{Key: "←/→", Action: "adjust"},
		{Key: "enter", Action: "save"},
		{Key: "esc", Action: "cancel"},
	}
}
