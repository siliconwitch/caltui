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

	cases := []struct {
		name     string
		key      tea.KeyMsg
		expected tea.Msg
	}{
		{"y confirms", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}, msgs.DeleteConfirmedMsg{Event: event}},
		{"enter confirms", tea.KeyMsg{Type: tea.KeyEnter}, msgs.DeleteConfirmedMsg{Event: event}},
		{"n cancels", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}, msgs.ClosePopupMsg{}},
		{"esc cancels", tea.KeyMsg{Type: tea.KeyEsc}, msgs.ClosePopupMsg{}},
		{"other keys are ignored", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			model, _ := New().Update(msgs.RequestDeleteMsg{Event: event})

			_, cmd := model.Update(c.key)

			if c.expected == nil {
				if cmd != nil {
					t.Fatalf("expected no command, got %v", cmd())
				}

				return
			}

			if cmd == nil {
				t.Fatal("expected a command, got nil")
			}

			message := cmd()

			if !reflect.DeepEqual(message, c.expected) {
				t.Fatalf("expected %v, got %v", c.expected, message)
			}
		})
	}
}
