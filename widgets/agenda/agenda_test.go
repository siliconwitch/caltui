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
	var model tea.Model = New(sourceStub{events: events}, DefaultConfig(), time.UTC)

	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	model, _ = model.Update(msgs.FocusDateMsg{Date: time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)})

	return model.(Model)
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

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if !strings.Contains(view, testCase.want) {
				t.Fatalf("want view to contain %q, got:\n%s", testCase.want, view)
			}
		})
	}
}

func TestAgendaSelectionAndFocus(t *testing.T) {
	model := testModel(testEvents())

	steps := []struct {
		key               string
		wantSelectedTitle string
		wantFocusedDate   time.Time
	}{
		{key: "j", wantSelectedTitle: "Standup", wantFocusedDate: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)},
		{key: "j", wantSelectedTitle: "Conference", wantFocusedDate: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)},
		{key: "j", wantSelectedTitle: "Conference", wantFocusedDate: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)},
	}

	for _, step := range steps {
		updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(step.key)})
		model = updated.(Model)

		if cmd == nil {
			t.Fatalf("want a selection command after pressing %q", step.key)
		}

		selected, ok := cmd().(msgs.EventSelectedMsg)

		if !ok || selected.Event == nil || selected.Event.Title != step.wantSelectedTitle {
			t.Fatalf("want %q selected, got %+v", step.wantSelectedTitle, selected)
		}

		if current := model.SelectedEvent(); current == nil || current.Title != step.wantSelectedTitle {
			t.Fatalf("want SelectedEvent %q after pressing %q, got %+v", step.wantSelectedTitle, step.key, current)
		}

		if !model.FocusedDate().Equal(step.wantFocusedDate) {
			t.Fatalf("want focused date %v after pressing %q, got %v", step.wantFocusedDate, step.key, model.FocusedDate())
		}
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)

	if model.SelectedEvent() != nil {
		t.Fatal("want escape to clear the selection")
	}

	if !model.FocusedDate().Equal(model.anchorDate) {
		t.Fatal("want focus back on the anchor after deselecting")
	}
}
