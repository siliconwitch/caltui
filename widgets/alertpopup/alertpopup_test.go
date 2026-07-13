package alertpopup

import (
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

func TestAlertsFireExactlyOnce(t *testing.T) {
	eventStart := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)

	standup := calendar.Event{
		ID:     "one",
		Title:  "Standup",
		Start:  eventStart,
		End:    eventStart.Add(30 * time.Minute),
		Alarms: []time.Duration{-15 * time.Minute},
	}

	model := New(sourceStub{events: []calendar.Event{standup}}, time.UTC)

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80})
	model = updated.(Model)

	steps := []struct {
		tickTime    time.Time
		wantPending int
	}{
		{tickTime: eventStart.Add(-20 * time.Minute), wantPending: 0},
		{tickTime: eventStart.Add(-16 * time.Minute), wantPending: 0},
		{tickTime: eventStart.Add(-14 * time.Minute), wantPending: 1},
		{tickTime: eventStart.Add(-13 * time.Minute), wantPending: 1},
	}

	for _, step := range steps {
		updated, _ = model.Update(msgs.ClockTickMsg{Now: step.tickTime})
		model = updated.(Model)

		if model.Pending() != step.wantPending {
			t.Fatalf("want %d pending alerts at %v, got %d", step.wantPending, step.tickTime, model.Pending())
		}
	}

	if !strings.Contains(model.View(), "Standup") || !strings.Contains(model.View(), "in 14m") {
		t.Fatalf("want the alert to name the event and lead time, got %q", model.View())
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	model = updated.(Model)

	if model.Pending() != 0 {
		t.Fatal("want the alert dismissed by a keypress")
	}

	if cmd == nil {
		t.Fatal("want a close command when the queue drains")
	}

	if _, ok := cmd().(msgs.ClosePopupMsg); !ok {
		t.Fatal("want the drained queue to close the popup")
	}
}
