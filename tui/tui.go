package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
)

type Model struct {
	store     calendar.Store
	month     tea.Model
	week      tea.Model
	day       tea.Model
	form      tea.Model
	confirm   tea.Model
	gotoDate  tea.Model
	detail    tea.Model
	active    string
	popup     string
	clipboard *calendar.Event
	width     int
	height    int
}

func New(store calendar.Store, month, week, day, form, confirm, gotoDate, detail tea.Model) Model {
	return Model{
		store:    store,
		month:    month,
		week:     week,
		day:      day,
		form:     form,
		confirm:  confirm,
		gotoDate: gotoDate,
		detail:   detail,
		active:   "month",
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.month.Init(),
		m.week.Init(),
		m.day.Init(),
		m.form.Init(),
		m.confirm.Init(),
		m.gotoDate.Init(),
		m.detail.Init(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		return m.broadcast(tea.WindowSizeMsg{Width: msg.Width, Height: msg.Height - 1})

	case tea.KeyMsg:
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

		case "d":
			if m.selectedEvent() != nil {
				return m.updateActive(msg)
			}

			return m.switchView("day")

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
		m.popup = "form"
		form, cmd := m.form.Update(msg)
		m.form = form

		return m, cmd

	case msgs.RequestDeleteMsg:
		m.popup = "confirm"
		confirm, cmd := m.confirm.Update(msg)
		m.confirm = confirm

		return m, cmd

	case msgs.ClosePopupMsg:
		m.popup = ""

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
		if msg.IsNew {
			m.store.Add(msg.Event)
		} else {
			m.store.Update(msg.Event)
		}

		return m.broadcast(msgs.EventsChangedMsg{})

	case msgs.DeleteConfirmedMsg:
		m.popup = ""
		m.store.Delete(msg.Event.ID)

		return m.broadcast(msgs.EventsChangedMsg{})

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
		pasted.Start = start
		pasted.End = time.Date(
			endDate.Year(), endDate.Month(), endDate.Day(),
			m.clipboard.End.Hour(), m.clipboard.End.Minute(), 0, 0,
			msg.Date.Location(),
		)
		m.store.Add(pasted)

		return m.broadcast(msgs.EventsChangedMsg{})
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

func (m Model) broadcast(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	return m, tea.Batch(cmds...)
}
