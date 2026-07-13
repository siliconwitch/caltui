package search

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
)

type sourceStub struct{ events []calendar.Event }

func (s sourceStub) Events(from, to time.Time) []calendar.Event { return s.events }

func (s sourceStub) Calendars() []calendar.Calendar { return nil }

func typedModel(events []calendar.Event, query string) Model {
	var model tea.Model = New(sourceStub{events: events}, time.UTC)

	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(msgs.OpenSearchMsg{})

	for _, character := range query {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{character}})
	}

	return model.(Model)
}

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

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			view := typedModel([]calendar.Event{standup, dinner}, testCase.query).View()

			for _, title := range []string{"Standup", "Dinner"} {
				wantShown := slices.Contains(testCase.wantTitles, title)

				if strings.Contains(view, title) != wantShown {
					t.Errorf("want %q shown=%v, got view:\n%s", title, wantShown, view)
				}
			}

			if len(testCase.wantTitles) == 2 && strings.Index(view, testCase.wantTitles[0]) > strings.Index(view, testCase.wantTitles[1]) {
				t.Errorf("want %q listed before %q, got view:\n%s", testCase.wantTitles[0], testCase.wantTitles[1], view)
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

	model := typedModel([]calendar.Event{event}, "stand")

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("want a command from enter")
	}

	jump, ok := cmd().(msgs.GotoDateMsg)

	if !ok || !jump.Date.Equal(event.Start) {
		t.Fatalf("want a GotoDateMsg for the event start, got %+v", jump)
	}
}
