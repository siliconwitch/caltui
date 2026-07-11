package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
)

type syncer interface {
	AccountNames() []string
	Sync(account string, trigger calendar.SyncTrigger) error
}

type clockTickMsg time.Time

type Model struct {
	store        calendar.Store
	month        tea.Model
	week         tea.Model
	day          tea.Model
	agenda       tea.Model
	form         tea.Model
	confirm      tea.Model
	gotoDate     tea.Model
	detail       tea.Model
	errorPopup   tea.Model
	scopePicker  tea.Model
	search       tea.Model
	calendars    tea.Model
	alertPopup   tea.Model
	active       string
	popup        string
	clipboard    *calendar.Event
	width        int
	height       int
	pendingSyncs int
	notice       string
	syncInterval time.Duration
	lastSync     time.Time
}

func New(store calendar.Store, syncInterval time.Duration, month, week, day, agenda, form, confirm, gotoDate, detail, errorPopup, scopePicker, search, calendars, alertPopup tea.Model) Model {
	model := Model{
		store:        store,
		month:        month,
		week:         week,
		day:          day,
		agenda:       agenda,
		form:         form,
		confirm:      confirm,
		gotoDate:     gotoDate,
		detail:       detail,
		errorPopup:   errorPopup,
		scopePicker:  scopePicker,
		search:       search,
		calendars:    calendars,
		alertPopup:   alertPopup,
		active:       "month",
		syncInterval: syncInterval,
		lastSync:     time.Now(),
	}

	if source, ok := store.(syncer); ok {
		model.pendingSyncs = len(source.AccountNames())
	}

	return model
}

func (m Model) Init() tea.Cmd {
	commands := []tea.Cmd{
		m.month.Init(),
		m.week.Init(),
		m.day.Init(),
		m.agenda.Init(),
		m.form.Init(),
		m.confirm.Init(),
		m.gotoDate.Init(),
		m.detail.Init(),
		m.errorPopup.Init(),
		m.scopePicker.Init(),
		m.search.Init(),
		m.calendars.Init(),
		m.alertPopup.Init(),
	}

	commands = append(commands, m.syncCommands(calendar.SyncAutomatic)...)

	return tea.Batch(append(commands, clockTick())...)
}

func clockTick() tea.Cmd {
	return tea.Tick(time.Minute, func(t time.Time) tea.Msg {
		return clockTickMsg(t)
	})
}

func (m Model) syncCommands(trigger calendar.SyncTrigger) []tea.Cmd {
	source, ok := m.store.(syncer)
	if !ok {
		return nil
	}

	var commands []tea.Cmd
	for _, name := range source.AccountNames() {
		commands = append(commands, func() tea.Msg {
			return msgs.SyncedMsg{Account: name, Err: source.Sync(name, trigger)}
		})
	}

	return commands
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		return m.broadcast(tea.WindowSizeMsg{Width: msg.Width, Height: msg.Height - 1})

	case tea.KeyMsg:
		m.notice = ""

		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		if m.popup != "" {
			switch m.popup {
			case "form":
				form, cmd := m.form.Update(msg)
				m.form = form

				return m, cmd

			case "confirm":
				confirm, cmd := m.confirm.Update(msg)
				m.confirm = confirm

				return m, cmd

			case "goto":
				gotoDate, cmd := m.gotoDate.Update(msg)
				m.gotoDate = gotoDate

				return m, cmd

			case "scope":
				scopePicker, cmd := m.scopePicker.Update(msg)
				m.scopePicker = scopePicker

				return m, cmd

			case "search":
				search, cmd := m.search.Update(msg)
				m.search = search

				return m, cmd

			case "calendars":
				calendars, cmd := m.calendars.Update(msg)
				m.calendars = calendars

				return m, cmd

			case "alert":
				alertPopup, cmd := m.alertPopup.Update(msg)
				m.alertPopup = alertPopup

				return m, cmd

			case "error":
				errorPopup, cmd := m.errorPopup.Update(msg)
				m.errorPopup = errorPopup

				return m, cmd
			}

			return m, nil
		}

		switch msg.String() {
		case "q":
			return m, tea.Quit

		case "m":
			return m.switchView("month")

		case "w":
			return m.switchView("week")

		case "a":
			return m.switchView("agenda")

		case "d":
			if m.selectedEvent() != nil {
				return m.updateActive(msg)
			}

			return m.switchView("day")

		case "r":
			if m.pendingSyncs > 0 {
				return m, nil
			}

			commands := m.syncCommands(calendar.SyncManual)
			m.pendingSyncs += len(commands)
			m.lastSync = time.Now()

			return m, tea.Batch(commands...)

		case "c":
			m.popup = "calendars"

			calendars, cmd := m.calendars.Update(msgs.OpenCalendarsMsg{})
			m.calendars = calendars

			return m, cmd

		case "/":
			m.popup = "search"

			search, cmd := m.search.Update(msgs.OpenSearchMsg{})
			m.search = search

			return m, cmd

		case "g":
			date := time.Now()
			if focuser, ok := m.activeView().(interface{ FocusedDate() time.Time }); ok {
				date = focuser.FocusedDate()
			}

			m.popup = "goto"

			gotoDate, cmd := m.gotoDate.Update(msgs.OpenGotoMsg{Date: date})
			m.gotoDate = gotoDate

			return m, cmd

		default:
			return m.updateActive(msg)
		}

	case msgs.EventSelectedMsg:
		detail, cmd := m.detail.Update(msg)
		m.detail = detail

		return m, cmd

	case msgs.OpenEventFormMsg:
		if !msg.IsNew {
			if reason := m.readOnlyReason(msg.Event); reason != "" {
				m.notice = reason

				return m, nil
			}
		}

		if !msg.IsNew && msg.Event.Recurring && msg.Scope == msgs.ScopeUnset {
			m.popup = "scope"
			scopePicker, cmd := m.scopePicker.Update(msg)
			m.scopePicker = scopePicker

			return m, cmd
		}

		m.popup = "form"
		form, cmd := m.form.Update(msg)
		m.form = form

		return m, cmd

	case msgs.RequestDeleteMsg:
		if reason := m.readOnlyReason(msg.Event); reason != "" {
			m.notice = reason

			return m, nil
		}

		m.popup = "confirm"
		confirm, cmd := m.confirm.Update(msg)
		m.confirm = confirm

		return m, cmd

	case msgs.ClosePopupMsg:
		m.popup = ""

		if pending, ok := m.errorPopup.(interface{ Pending() int }); ok && pending.Pending() > 0 {
			m.popup = "error"
		}

		if pending, ok := m.alertPopup.(interface{ Pending() int }); ok && m.popup == "" && pending.Pending() > 0 {
			m.popup = "alert"
		}

		return m, nil

	case msgs.GotoDateMsg:
		m.popup = ""

		view, cmd := m.activeView().Update(msgs.FocusDateMsg{Date: msg.Date})
		m = m.withActiveView(view)

		detail, detailCmd := m.detail.Update(msgs.EventSelectedMsg{})
		m.detail = detail

		return m, tea.Batch(cmd, detailCmd)

	case msgs.EventFormSubmittedMsg:
		m.popup = ""
		store := m.store

		return m, func() tea.Msg {
			var err error

			switch {
			case msg.IsNew:
				_, err = store.Add(msg.Event)

			case msg.Scope == msgs.ScopeOccurrence:
				occurrenceStore, ok := store.(interface{ UpdateOccurrence(calendar.Event) error })
				if !ok {
					err = fmt.Errorf("this account cannot edit repeating events")
				} else {
					err = occurrenceStore.UpdateOccurrence(msg.Event)
				}

			case msg.Scope == msgs.ScopeSeries:
				seriesStore, ok := store.(interface{ UpdateSeries(calendar.Event) error })
				if !ok {
					err = fmt.Errorf("this account cannot edit repeating events")
				} else {
					err = seriesStore.UpdateSeries(msg.Event)
				}

			default:
				err = store.Update(msg.Event)
			}

			if err != nil {
				return msgs.StoreErrorMsg{Err: err}
			}

			return msgs.EventsChangedMsg{}
		}

	case msgs.DeleteConfirmedMsg:
		m.popup = ""
		store := m.store

		return m, func() tea.Msg {
			var err error

			switch {
			case msg.Scope == msgs.ScopeOccurrence:
				occurrenceStore, ok := store.(interface{ DeleteOccurrence(string) error })
				if !ok {
					err = fmt.Errorf("this account cannot edit repeating events")
				} else {
					err = occurrenceStore.DeleteOccurrence(msg.Event.ID)
				}

			case msg.Scope == msgs.ScopeSeries:
				seriesStore, ok := store.(interface{ DeleteSeries(string) error })
				if !ok {
					err = fmt.Errorf("this account cannot edit repeating events")
				} else {
					err = seriesStore.DeleteSeries(msg.Event.ID)
				}

			default:
				err = store.Delete(msg.Event.ID)
			}

			if err != nil {
				return msgs.StoreErrorMsg{Err: err}
			}

			return msgs.EventsChangedMsg{}
		}

	case clockTickMsg:
		commands := []tea.Cmd{clockTick()}

		refreshDue := m.syncInterval > 0 &&
			m.popup == "" &&
			m.selectedEvent() == nil &&
			m.pendingSyncs == 0 &&
			time.Since(m.lastSync) >= m.syncInterval

		if refreshDue {
			syncs := m.syncCommands(calendar.SyncAutomatic)
			m.pendingSyncs += len(syncs)
			m.lastSync = time.Now()
			commands = append(commands, syncs...)
		}

		updated, broadcastCmd := m.broadcast(msgs.ClockTickMsg{Now: time.Time(msg)})
		commands = append(commands, broadcastCmd)

		if updated.popup == "" {
			if pending, ok := updated.alertPopup.(interface{ Pending() int }); ok && pending.Pending() > 0 {
				updated.popup = "alert"
			}
		}

		return updated, tea.Batch(commands...)

	case msgs.SyncedMsg:
		if m.pendingSyncs > 0 {
			m.pendingSyncs--
		}

		updated, syncedCmd := m.broadcast(msg)

		if msg.Err != nil && (updated.popup == "" || updated.popup == "error") {
			updated.popup = "error"
		}

		updated, eventsCmd := updated.broadcast(msgs.EventsChangedMsg{})

		updated, calendarsCmd := updated.broadcast(msgs.CalendarsChangedMsg{
			Calendars: calendar.WritableCalendars(updated.store),
		})

		return updated, tea.Batch(syncedCmd, eventsCmd, calendarsCmd)

	case msgs.StoreErrorMsg:
		updated, errorCmd := m.broadcast(msg)

		if updated.popup == "" || updated.popup == "error" {
			updated.popup = "error"
		}

		updated, eventsCmd := updated.broadcast(msgs.EventsChangedMsg{})

		return updated, tea.Batch(errorCmd, eventsCmd)

	case msgs.YankMsg:
		yanked := msg.Event
		m.clipboard = &yanked

		return m.broadcast(msgs.YankedMsg{EventID: yanked.ID})

	case msgs.PasteMsg:
		if m.clipboard == nil {
			return m, nil
		}

		startDay := time.Date(
			m.clipboard.Start.Year(), m.clipboard.Start.Month(), m.clipboard.Start.Day(),
			0, 0, 0, 0, time.UTC,
		)
		endDay := time.Date(
			m.clipboard.End.Year(), m.clipboard.End.Month(), m.clipboard.End.Day(),
			0, 0, 0, 0, time.UTC,
		)

		spanDays := int(endDay.Sub(startDay).Hours() / 24)

		start := time.Date(
			msg.Date.Year(), msg.Date.Month(), msg.Date.Day(),
			m.clipboard.Start.Hour(), m.clipboard.Start.Minute(), 0, 0,
			msg.Date.Location(),
		)

		endDate := start.AddDate(0, 0, spanDays)

		pasted := *m.clipboard
		pasted.ID = ""
		pasted.Recurring = false
		pasted.Recurrence = calendar.Recurrence{}
		pasted.Start = start
		pasted.End = time.Date(
			endDate.Year(), endDate.Month(), endDate.Day(),
			m.clipboard.End.Hour(), m.clipboard.End.Minute(), 0, 0,
			msg.Date.Location(),
		)

		store := m.store

		return m, func() tea.Msg {
			if _, err := store.Add(pasted); err != nil {
				return msgs.StoreErrorMsg{Err: err}
			}

			return msgs.EventsChangedMsg{}
		}
	}

	return m.broadcast(msg)
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	screen := m.activeView().View()

	lines := strings.Split(screen, "\n")
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	screen = strings.Join(lines[:m.height-1], "\n")

	if detail := m.detail.View(); detail != "" && m.popup == "" {
		x := (m.width - lipgloss.Width(detail)) / 2
		y := m.height - 2 - lipgloss.Height(detail)
		screen = Compose(screen, detail, x, y, m.width)
	}

	if m.popup != "" {
		var popup string
		switch m.popup {
		case "form":
			popup = m.form.View()
		case "confirm":
			popup = m.confirm.View()
		case "goto":
			popup = m.gotoDate.View()
		case "scope":
			popup = m.scopePicker.View()
		case "search":
			popup = m.search.View()
		case "calendars":
			popup = m.calendars.View()
		case "alert":
			popup = m.alertPopup.View()
		case "error":
			popup = m.errorPopup.View()
		}

		x := (m.width - lipgloss.Width(popup)) / 2
		y := (m.height - 1 - lipgloss.Height(popup)) / 2
		screen = Compose(screen, popup, x, y, m.width)
	}

	return screen + "\n" + m.statusBar()
}

func (m Model) activeView() tea.Model {
	switch m.active {
	case "week":
		return m.week
	case "day":
		return m.day
	case "agenda":
		return m.agenda
	default:
		return m.month
	}
}

func (m Model) withActiveView(view tea.Model) Model {
	switch m.active {
	case "week":
		m.week = view
	case "day":
		m.day = view
	case "agenda":
		m.agenda = view
	default:
		m.month = view
	}

	return m
}

func (m Model) updateActive(msg tea.Msg) (tea.Model, tea.Cmd) {
	view, cmd := m.activeView().Update(msg)

	return m.withActiveView(view), cmd
}

func (m Model) switchView(target string) (tea.Model, tea.Cmd) {
	if target == m.active {
		return m, nil
	}

	date := time.Now()
	if focuser, ok := m.activeView().(interface{ FocusedDate() time.Time }); ok {
		date = focuser.FocusedDate()
	}

	m.active = target

	view, cmd := m.activeView().Update(msgs.FocusDateMsg{Date: date})
	m = m.withActiveView(view)

	detail, detailCmd := m.detail.Update(msgs.EventSelectedMsg{})
	m.detail = detail

	return m, tea.Batch(cmd, detailCmd)
}

func (m Model) selectedEvent() *calendar.Event {
	selector, ok := m.activeView().(interface{ SelectedEvent() *calendar.Event })
	if !ok {
		return nil
	}

	return selector.SelectedEvent()
}

func (m Model) readOnlyReason(event calendar.Event) string {
	for _, writable := range calendar.WritableCalendars(m.store) {
		if writable.Name == event.Calendar {
			return ""
		}
	}

	return "calendar " + event.Calendar + " is read-only"
}

func (m Model) broadcast(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	update := func(widget tea.Model) tea.Model {
		updated, cmd := widget.Update(msg)
		cmds = append(cmds, cmd)

		return updated
	}

	m.month = update(m.month)
	m.week = update(m.week)
	m.day = update(m.day)
	m.form = update(m.form)
	m.confirm = update(m.confirm)
	m.gotoDate = update(m.gotoDate)
	m.detail = update(m.detail)
	m.errorPopup = update(m.errorPopup)
	m.scopePicker = update(m.scopePicker)
	m.agenda = update(m.agenda)
	m.search = update(m.search)
	m.calendars = update(m.calendars)
	m.alertPopup = update(m.alertPopup)

	return m, tea.Batch(cmds...)
}
