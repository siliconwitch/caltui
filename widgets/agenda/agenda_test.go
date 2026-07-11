package agenda

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
)

type sourceStub struct{ events []calendar.Event }

func (s sourceStub) Events(from, to time.Time) []calendar.Event {
	var events []calendar.Event
	for _, event := range s.events {
		if event.End.After(from) && event.Start.Before(to) {
			events = append(events, event)
		}
	}

	return events
}

func (s sourceStub) Calendars() []calendar.Calendar {
	return []calendar.Calendar{{Name: "Work", Color: "#7AA2F7"}}
}

func testEvents() []calendar.Event {
	return []calendar.Event{
		{
			ID:    "one",
			Title: "Standup",
			Start: time.Date(2026, 7, 13, 9, 30, 0, 0, time.UTC),
			End:   time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:     "two",
			Title:  "Conference",
			AllDay: true,
			Start:  time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
			End:    time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
		},
	}
}

func testModel(events []calendar.Event) Model {
	model := New(sourceStub{events: events}, DefaultConfig(), time.UTC)
	model.anchorDate = time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	model.width = 80
	model.height = 20

	return model
}

func TestAgendaGroupsByDay(t *testing.T) {
	view := testModel(testEvents()).View()

	cases := []struct {
		name string
		want string
	}{
		{name: "first day header", want: "Monday 13 July"},
		{name: "second day header", want: "Tuesday 14 July"},
		{name: "timed row", want: "09:30–10:00  Standup"},
		{name: "all day row", want: "all day  Conference"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(view, c.want) {
				t.Fatalf("want view to contain %q, got:\n%s", c.want, view)
			}
		})
	}
}

func TestAgendaSelectionAndFocus(t *testing.T) {
	model := testModel(testEvents())

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	model = updated.(Model)

	if cmd == nil {
		t.Fatal("want a selection command")
	}

	selected, ok := cmd().(msgs.EventSelectedMsg)

	if !ok || selected.Event == nil || selected.Event.Title != "Standup" {
		t.Fatalf("want the first event selected, got %+v", selected)
	}

	wantDay := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)

	if !model.FocusedDate().Equal(wantDay) {
		t.Fatalf("want focused date %v, got %v", wantDay, model.FocusedDate())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	model = updated.(Model)

	if selected := model.SelectedEvent(); selected == nil || selected.Title != "Conference" {
		t.Fatalf("want the second event selected, got %+v", selected)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	model = updated.(Model)

	if selected := model.SelectedEvent(); selected == nil || selected.Title != "Conference" {
		t.Fatalf("want selection clamped at the last event, got %+v", selected)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)

	if model.SelectedEvent() != nil {
		t.Fatal("want escape to clear the selection")
	}

	if !model.FocusedDate().Equal(model.anchorDate) {
		t.Fatal("want focus back on the anchor after deselecting")
	}
}
