package search

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
)

type sourceStub struct{ events []calendar.Event }

func (s sourceStub) Events(from, to time.Time) []calendar.Event { return s.events }

func (s sourceStub) Calendars() []calendar.Calendar { return nil }

func TestSearchFiltersAndJumps(t *testing.T) {
	standup := calendar.Event{
		ID:    "one",
		Title: "Standup",
		Start: time.Date(2026, 7, 13, 9, 30, 0, 0, time.UTC),
		End:   time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
	}

	dinner := calendar.Event{
		ID:       "two",
		Title:    "Dinner",
		Location: "Standard Hotel",
		Start:    time.Date(2026, 7, 14, 19, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 7, 14, 21, 0, 0, 0, time.UTC),
	}

	cases := []struct {
		name       string
		query      string
		wantTitles []string
	}{
		{name: "title match", query: "standup", wantTitles: []string{"Standup"}},
		{name: "location match", query: "hotel", wantTitles: []string{"Dinner"}},
		{name: "shared prefix matches both", query: "stand", wantTitles: []string{"Standup", "Dinner"}},
		{name: "no match", query: "retro", wantTitles: nil},
		{name: "blank query lists nothing", query: "", wantTitles: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			model := New(sourceStub{events: []calendar.Event{standup, dinner}}, time.UTC)
			model.queryInput.SetValue(c.query)

			model = model.withResults()

			if len(model.results) != len(c.wantTitles) {
				t.Fatalf("want %d results, got %+v", len(c.wantTitles), model.results)
			}

			for index, title := range c.wantTitles {
				if model.results[index].Title != title {
					t.Errorf("want result %d to be %q, got %q", index, title, model.results[index].Title)
				}
			}
		})
	}
}

func TestSearchEnterJumpsToResult(t *testing.T) {
	event := calendar.Event{
		ID:    "one",
		Title: "Standup",
		Start: time.Date(2026, 7, 13, 9, 30, 0, 0, time.UTC),
		End:   time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
	}

	model := New(sourceStub{events: []calendar.Event{event}}, time.UTC)
	model.queryInput.SetValue("stand")
	model = model.withResults()

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("want a command from enter")
	}

	jump, ok := cmd().(msgs.GotoDateMsg)

	if !ok || !jump.Date.Equal(event.Start) {
		t.Fatalf("want a GotoDateMsg for the event start, got %+v", jump)
	}
}
