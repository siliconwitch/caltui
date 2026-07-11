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
	model.lastCheck = eventStart.Add(-20 * time.Minute)
	model.width = 80

	beforeAlarm := eventStart.Add(-16 * time.Minute)

	updated, _ := model.Update(msgs.ClockTickMsg{Now: beforeAlarm})
	model = updated.(Model)

	if model.Pending() != 0 {
		t.Fatalf("want no alert before the trigger time, got %d", model.Pending())
	}

	atAlarm := eventStart.Add(-14 * time.Minute)

	updated, _ = model.Update(msgs.ClockTickMsg{Now: atAlarm})
	model = updated.(Model)

	if model.Pending() != 1 {
		t.Fatalf("want exactly one alert at the trigger time, got %d", model.Pending())
	}

	if !strings.Contains(model.View(), "Standup") || !strings.Contains(model.View(), "in 14m") {
		t.Fatalf("want the alert to name the event and lead time, got %q", model.View())
	}

	updated, _ = model.Update(msgs.ClockTickMsg{Now: atAlarm.Add(time.Minute)})
	model = updated.(Model)

	if model.Pending() != 1 {
		t.Fatalf("want no duplicate alert on later ticks, got %d", model.Pending())
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
