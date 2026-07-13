package confirm

import (
	"reflect"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
)

func TestKeyHandling(t *testing.T) {
	start := time.Date(2026, 7, 9, 13, 30, 0, 0, time.UTC)

	event := calendar.Event{
		ID:       "mock-1",
		Title:    "Call with John",
		Start:    start,
		End:      start.Add(45 * time.Minute),
		Calendar: "Personal",
	}

	recurringEvent := event
	recurringEvent.Recurring = true

	cases := []struct {
		name     string
		event    calendar.Event
		key      tea.KeyMsg
		expected tea.Msg
	}{
		{"y confirms", event, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}, msgs.DeleteConfirmedMsg{Event: event}},
		{"enter confirms", event, tea.KeyMsg{Type: tea.KeyEnter}, msgs.DeleteConfirmedMsg{Event: event}},
		{"n cancels", event, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}, msgs.ClosePopupMsg{}},
		{"esc cancels", event, tea.KeyMsg{Type: tea.KeyEsc}, msgs.ClosePopupMsg{}},
		{"other keys are ignored", event, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}, nil},
		{"o deletes one occurrence when recurring", recurringEvent, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")}, msgs.DeleteConfirmedMsg{Event: recurringEvent, Scope: msgs.ScopeOccurrence}},
		{"s deletes the series when recurring", recurringEvent, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")}, msgs.DeleteConfirmedMsg{Event: recurringEvent, Scope: msgs.ScopeSeries}},
		{"n cancels when recurring", recurringEvent, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}, msgs.ClosePopupMsg{}},
		{"y is ignored when recurring", recurringEvent, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}, nil},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model, _ := Model{}.Update(msgs.RequestDeleteMsg{Event: testCase.event})

			_, cmd := model.Update(testCase.key)

			if testCase.expected == nil {
				if cmd != nil {
					t.Fatalf("expected no command, got %v", cmd())
				}

				return
			}

			if cmd == nil {
				t.Fatal("expected a command, got nil")
			}

			message := cmd()

			if !reflect.DeepEqual(message, testCase.expected) {
				t.Fatalf("expected %v, got %v", testCase.expected, message)
			}
		})
	}
}
