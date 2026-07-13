package calendars

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
)

type sourceStub struct {
	calendars []calendar.Calendar
}

func (s sourceStub) Events(from, to time.Time) []calendar.Event {
	return nil
}

func (s sourceStub) Calendars() []calendar.Calendar {
	return s.calendars
}

func TestCalendarsPopup(t *testing.T) {
	workAndPersonal := []calendar.Calendar{{Name: "Work"}, {Name: "Personal"}}

	cases := []struct {
		name        string
		calendars   []calendar.Calendar
		messages    []tea.Msg
		wantHidden  []string
		wantLastMsg tea.Msg
		wantInView  []string
	}{
		{
			name:       "opening lists every calendar",
			calendars:  workAndPersonal,
			wantInView: []string{"Calendars", "Work", "Personal"},
		},
		{
			name:       "an empty list shows a placeholder",
			wantInView: []string{"No calendars yet"},
		},
		{
			name:        "space hides the selected calendar",
			calendars:   workAndPersonal,
			messages:    []tea.Msg{tea.KeyMsg{Type: tea.KeySpace}},
			wantHidden:  []string{"Work"},
			wantLastMsg: msgs.EventsChangedMsg{},
		},
		{
			name:      "a second space shows it again",
			calendars: workAndPersonal,
			messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeySpace},
				tea.KeyMsg{Type: tea.KeySpace},
			},
			wantLastMsg: msgs.EventsChangedMsg{},
		},
		{
			name:      "j selects the next calendar",
			calendars: workAndPersonal,
			messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")},
				tea.KeyMsg{Type: tea.KeySpace},
			},
			wantHidden:  []string{"Personal"},
			wantLastMsg: msgs.EventsChangedMsg{},
		},
		{
			name:      "j wraps from the bottom to the top",
			calendars: workAndPersonal,
			messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")},
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")},
				tea.KeyMsg{Type: tea.KeySpace},
			},
			wantHidden:  []string{"Work"},
			wantLastMsg: msgs.EventsChangedMsg{},
		},
		{
			name:      "k wraps from the top to the bottom",
			calendars: workAndPersonal,
			messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")},
				tea.KeyMsg{Type: tea.KeySpace},
			},
			wantHidden:  []string{"Personal"},
			wantLastMsg: msgs.EventsChangedMsg{},
		},
		{
			name:      "reopening resets the selection",
			calendars: workAndPersonal,
			messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")},
				msgs.OpenCalendarsMsg{},
				tea.KeyMsg{Type: tea.KeySpace},
			},
			wantHidden:  []string{"Work"},
			wantLastMsg: msgs.EventsChangedMsg{},
		},
		{
			name: "navigation on an empty list is ignored",
			messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")},
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")},
			},
		},
		{
			name:     "space on an empty list is ignored",
			messages: []tea.Msg{tea.KeyMsg{Type: tea.KeySpace}},
		},
		{
			name:        "esc closes the popup",
			calendars:   workAndPersonal,
			messages:    []tea.Msg{tea.KeyMsg{Type: tea.KeyEsc}},
			wantLastMsg: msgs.ClosePopupMsg{},
		},
		{
			name:        "q closes the popup",
			calendars:   workAndPersonal,
			messages:    []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}},
			wantLastMsg: msgs.ClosePopupMsg{},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("CALTUI_CACHE", t.TempDir())

			visible := calendar.NewVisible(sourceStub{calendars: testCase.calendars})

			var model tea.Model = New(visible)

			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

			model, _ = model.Update(msgs.OpenCalendarsMsg{})

			var lastCmd tea.Cmd
			for _, message := range testCase.messages {
				model, lastCmd = model.Update(message)
			}

			if testCase.wantLastMsg == nil {
				if lastCmd != nil {
					t.Fatalf("expected no command, got %v", lastCmd())
				}
			} else {
				if lastCmd == nil {
					t.Fatal("expected a command, got nil")
				}

				if message := lastCmd(); !reflect.DeepEqual(message, testCase.wantLastMsg) {
					t.Fatalf("expected %v, got %v", testCase.wantLastMsg, message)
				}
			}

			var hidden []string
			for _, entry := range visible.All() {
				if entry.Hidden {
					hidden = append(hidden, entry.Name)
				}
			}

			if !reflect.DeepEqual(hidden, testCase.wantHidden) {
				t.Fatalf("expected hidden %v, got %v", testCase.wantHidden, hidden)
			}

			view := model.View()
			for _, want := range testCase.wantInView {
				if !strings.Contains(view, want) {
					t.Errorf("view does not contain %q:\n%s", want, view)
				}
			}
		})
	}
}
