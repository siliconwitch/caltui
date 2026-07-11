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
	locationSlot
	notesSlot
	allDaySlot
	startDateSlot
	startTimeSlot
	startZoneSlot
	endDateSlot
	endTimeSlot
	endZoneSlot
	repeatSlot
	intervalSlot
	endsSlot
	untilDateSlot
	calendarSlot
)

var frequencies = []string{"", "daily", "weekly", "monthly", "yearly"}

var frequencyLabels = []string{"never", "daily", "weekly", "monthly", "yearly"}

var intervalUnits = []string{"", "day", "week", "month", "year"}

const (
	maxInnerWidth = 46
	minInnerWidth = 20
	labelWidth    = 10
)

type Model struct {
	calendars      []calendar.Calendar
	location       *time.Location
	titleInput     textinput.Model
	locationInput  textinput.Model
	notesInput     textinput.Model
	startDate      maskinput.Field
	startTime      maskinput.Field
	endDate        maskinput.Field
	endTime        maskinput.Field
	startZone      *time.Location
	endZone        *time.Location
	picker         timezone.Picker
	pickerOpen     bool
	pickerTarget   int
	allDay         bool
	calendarIndex  int
	focusedSlot    int
	frequencyIndex int
	interval       int
	endsOnDate     bool
	untilDate      maskinput.Field
	scope          msgs.EditScope
	original       calendar.Event
	isNew          bool
	errorText      string
	innerWidth     int
}

func New(calendars []calendar.Calendar, location *time.Location) Model {
	return Model{
		calendars:     calendars,
		location:      location,
		titleInput:    newTextInput(),
		locationInput: newTextInput(),
		notesInput:    newTextInput(),
		startDate:     maskinput.NewDate(false),
		startTime:     maskinput.NewTime(),
		endDate:       maskinput.NewDate(false),
		endTime:       maskinput.NewTime(),
		startZone:     location,
		endZone:       location,
		untilDate:     maskinput.NewDate(false),
		picker:        timezone.NewPicker(),
		innerWidth:    maxInnerWidth,
	}
}

func newTextInput() textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Width = maxInnerWidth - labelWidth - 2

	return input
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.innerWidth = min(maxInnerWidth, max(minInnerWidth, msg.Width-4))
		m.titleInput.Width = m.innerWidth - labelWidth - 2
		m.locationInput.Width = m.innerWidth - labelWidth - 2
		m.notesInput.Width = m.innerWidth - labelWidth - 2

		return m, nil

	case msgs.OpenEventFormMsg:
		m.original = msg.Event
		m.isNew = msg.IsNew
		m.scope = msg.Scope
		m.errorText = ""
		m.allDay = msg.Event.AllDay
		m.pickerOpen = false
		m.calendarIndex = 0

		m.frequencyIndex = max(0, slices.Index(frequencies, msg.Event.Recurrence.Frequency))
		m.interval = max(1, msg.Event.Recurrence.Interval)
		m.endsOnDate = !msg.Event.Recurrence.Until.IsZero()

		untilSeed := msg.Event.Start
		if m.endsOnDate {
			untilSeed = msg.Event.Recurrence.Until.In(msg.Event.Start.Location())
		}
		m.untilDate = m.untilDate.WithDate(untilSeed)

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
		m.locationInput.SetValue(msg.Event.Location)
		m.notesInput.SetValue(strings.ReplaceAll(msg.Event.Description, "\n", " "))
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

			description := m.original.Description
			if typed := strings.TrimSpace(m.notesInput.Value()); typed != strings.TrimSpace(strings.ReplaceAll(description, "\n", " ")) {
				description = typed
			}

			submitted, problem := composedEvent(
				m.original,
				strings.TrimSpace(m.titleInput.Value()),
				strings.TrimSpace(m.locationInput.Value()),
				description,
				m.composedRecurrence(),
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
			scope := m.scope

			return m, func() tea.Msg {
				return msgs.EventFormSubmittedMsg{Event: submitted, IsNew: isNew, Scope: scope}
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

		case repeatSlot:
			switch msg.String() {
			case "left":
				m.frequencyIndex = (m.frequencyIndex + len(frequencies) - 1) % len(frequencies)
			case "right", " ":
				m.frequencyIndex = (m.frequencyIndex + 1) % len(frequencies)
			}

			return m, nil

		case intervalSlot:
			switch msg.String() {
			case "left":
				m.interval = max(1, m.interval-1)
			case "right", " ":
				m.interval = min(99, m.interval+1)
			}

			return m, nil

		case endsSlot:
			switch msg.String() {
			case "left", "right", " ":
				m.endsOnDate = !m.endsOnDate
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

		case startDateSlot, startTimeSlot, endDateSlot, endTimeSlot, untilDateSlot:
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
			case untilDateSlot:
				m.untilDate, _ = m.untilDate.Typed(msg.String())
			}

			if completed {
				m = m.withTypingAdvance()
			}

			return m, nil

		case locationSlot:
			input, cmd := m.locationInput.Update(msg)
			m.locationInput = input

			return m, cmd

		case notesSlot:
			input, cmd := m.notesInput.Update(msg)
			m.notesInput = input

			return m, cmd

		default:
			input, cmd := m.titleInput.Update(msg)
			m.titleInput = input

			return m, cmd
		}

	default:
		title, titleCmd := m.titleInput.Update(msg)
		m.titleInput = title

		location, locationCmd := m.locationInput.Update(msg)
		m.locationInput = location

		notes, notesCmd := m.notesInput.Update(msg)
		m.notesInput = notes

		return m, tea.Batch(titleCmd, locationCmd, notesCmd)
	}
}

func (m Model) slotOrder() []int {
	order := []int{titleSlot, locationSlot, notesSlot, allDaySlot, startDateSlot, endDateSlot}
	if !m.allDay {
		order = []int{
			titleSlot, locationSlot, notesSlot, allDaySlot,
			startDateSlot, startTimeSlot, startZoneSlot,
			endDateSlot, endTimeSlot, endZoneSlot,
		}
	}

	if m.scope != msgs.ScopeOccurrence {
		order = append(order, repeatSlot)

		if m.frequencyIndex > 0 {
			order = append(order, intervalSlot, endsSlot)

			if m.endsOnDate {
				order = append(order, untilDateSlot)
			}
		}
	}

	return append(order, calendarSlot)
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
	m.locationInput.Blur()
	m.notesInput.Blur()
	m.startDate = m.startDate.Blur()
	m.startTime = m.startTime.Blur()
	m.endDate = m.endDate.Blur()
	m.endTime = m.endTime.Blur()
	m.untilDate = m.untilDate.Blur()

	switch slot {
	case titleSlot:
		m.titleInput.Focus()
	case locationSlot:
		m.locationInput.Focus()
	case notesSlot:
		m.notesInput.Focus()
	case startDateSlot:
		m.startDate = m.startDate.Focus()
	case startTimeSlot:
		m.startTime = m.startTime.Focus()
	case endDateSlot:
		m.endDate = m.endDate.Focus()
	case endTimeSlot:
		m.endTime = m.endTime.Focus()
	case untilDateSlot:
		m.untilDate = m.untilDate.Focus()
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

func (m Model) composedRecurrence() calendar.Recurrence {
	if m.scope == msgs.ScopeOccurrence {
		return m.original.Recurrence
	}

	if m.frequencyIndex == 0 {
		return calendar.Recurrence{}
	}

	recurrence := calendar.Recurrence{
		Frequency: frequencies[m.frequencyIndex],
		Interval:  m.interval,
	}

	if m.endsOnDate {
		year, month, day := m.untilDate.Date()

		recurrence.Until = time.Date(year, month, day, 23, 59, 59, 0, m.startZone)
		if m.allDay {
			recurrence.Until = time.Date(year, month, day, 0, 0, 0, 0, m.location)
		}
	}

	original := m.original.Recurrence

	comparisonZone := m.startZone
	if m.allDay {
		comparisonZone = m.location
	}

	sameEnding := original.Until.IsZero() == recurrence.Until.IsZero()
	if !recurrence.Until.IsZero() && !original.Until.IsZero() {
		originalYear, originalMonth, originalDay := original.Until.In(comparisonZone).Date()

		untilYear, untilMonth, untilDay := recurrence.Until.Date()

		sameEnding = originalYear == untilYear && originalMonth == untilMonth && originalDay == untilDay
	}

	untouched := recurrence.Frequency == original.Frequency &&
		max(recurrence.Interval, 1) == max(original.Interval, 1) &&
		sameEnding

	if untouched {
		return original
	}

	return recurrence
}

func composedEvent(
	original calendar.Event,
	title, location, description string,
	recurrence calendar.Recurrence,
	allDay bool,
	startDate, startTime, endDate, endTime maskinput.Field,
	startZone, endZone, defaultLocation *time.Location,
	calendarName string,
) (calendar.Event, string) {
	startYear, startMonth, startDay := startDate.Date()

	endYear, endMonth, endDay := endDate.Date()

	event := original
	event.Title = title
	event.Location = location
	event.Description = description
	event.Recurrence = recurrence
	event.AllDay = allDay
	event.Calendar = calendarName
	event.Color = ""

	if !recurrence.Until.IsZero() && recurrence.Until.Before(time.Date(startYear, startMonth, startDay, 0, 0, 0, 0, defaultLocation)) {
		return event, "Repeat until must not be before start"
	}

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

	cycler := func(slot int, value string) string {
		if m.focusedSlot == slot {
			return chevronStyle.Render("‹ ") + value + chevronStyle.Render(" ›")
		}

		return value
	}

	type formRow struct {
		label string
		slots []int
		value string
	}

	rows := []formRow{
		{"Title", []int{titleSlot}, m.titleInput.View()},
		{"Location", []int{locationSlot}, m.locationInput.View()},
		{"Notes", []int{notesSlot}, m.notesInput.View()},
		{"All day", []int{allDaySlot}, allDayValue},
		{"Start", []int{startDateSlot, startTimeSlot, startZoneSlot}, startValue},
		{"End", []int{endDateSlot, endTimeSlot, endZoneSlot}, endValue},
	}

	if m.scope != msgs.ScopeOccurrence {
		rows = append(rows, formRow{"Repeat", []int{repeatSlot}, cycler(repeatSlot, frequencyLabels[m.frequencyIndex])})

		if m.frequencyIndex > 0 {
			everyValue := "every " + intervalUnits[m.frequencyIndex]
			if m.interval > 1 {
				everyValue = fmt.Sprintf("every %d %ss", m.interval, intervalUnits[m.frequencyIndex])
			}

			untilValue := cycler(endsSlot, "never")
			if m.endsOnDate {
				untilValue = cycler(endsSlot, "on date") + "  " + m.untilDate.View()
			}

			rows = append(rows,
				formRow{"Every", []int{intervalSlot}, cycler(intervalSlot, everyValue)},
				formRow{"Until", []int{endsSlot, untilDateSlot}, untilValue},
			)
		}
	}

	rows = append(rows, formRow{"Calendar", []int{calendarSlot}, calendarValue})

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
