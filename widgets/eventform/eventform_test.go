package eventform

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
)

func TestSubmittedEvent(t *testing.T) {
	losAngeles, err := time.LoadLocation("America/Los_Angeles")

	if err != nil {
		t.Fatal(err)
	}

	stockholm, err := time.LoadLocation("Europe/Stockholm")

	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		allDay    bool
		start     time.Time
		end       time.Time
		wantStart time.Time
		wantEnd   time.Time
		wantError string
	}{
		{
			name:      "cross zone flight",
			start:     time.Date(2026, time.July, 6, 12, 41, 0, 0, losAngeles),
			end:       time.Date(2026, time.July, 7, 9, 30, 0, 0, stockholm),
			wantStart: time.Date(2026, time.July, 6, 12, 41, 0, 0, losAngeles),
			wantEnd:   time.Date(2026, time.July, 7, 9, 30, 0, 0, stockholm),
		},
		{
			name:      "cross zone end before start in absolute time",
			start:     time.Date(2026, time.July, 6, 12, 41, 0, 0, losAngeles),
			end:       time.Date(2026, time.July, 6, 16, 30, 0, 0, stockholm),
			wantError: "End must be after start",
		},
		{
			name:      "timed same day",
			start:     time.Date(2026, time.July, 6, 9, 0, 0, 0, time.Local),
			end:       time.Date(2026, time.July, 6, 10, 30, 0, 0, time.Local),
			wantStart: time.Date(2026, time.July, 6, 9, 0, 0, 0, time.Local),
			wantEnd:   time.Date(2026, time.July, 6, 10, 30, 0, 0, time.Local),
		},
		{
			name:      "timed multi day",
			start:     time.Date(2026, time.July, 6, 16, 0, 0, 0, time.Local),
			end:       time.Date(2026, time.July, 7, 9, 30, 0, 0, time.Local),
			wantStart: time.Date(2026, time.July, 6, 16, 0, 0, 0, time.Local),
			wantEnd:   time.Date(2026, time.July, 7, 9, 30, 0, 0, time.Local),
		},
		{
			name:      "timed end before start",
			start:     time.Date(2026, time.July, 6, 10, 0, 0, 0, time.Local),
			end:       time.Date(2026, time.July, 6, 9, 0, 0, 0, time.Local),
			wantError: "End must be after start",
		},
		{
			name:      "timed end equal to start",
			start:     time.Date(2026, time.July, 6, 10, 0, 0, 0, time.Local),
			end:       time.Date(2026, time.July, 6, 10, 0, 0, 0, time.Local),
			wantError: "End must be after start",
		},
		{
			name:      "all day single day",
			allDay:    true,
			start:     time.Date(2026, time.July, 6, 0, 0, 0, 0, time.Local),
			end:       time.Date(2026, time.July, 7, 0, 0, 0, 0, time.Local),
			wantStart: time.Date(2026, time.July, 6, 0, 0, 0, 0, time.Local),
			wantEnd:   time.Date(2026, time.July, 7, 0, 0, 0, 0, time.Local),
		},
		{
			name:      "all day multi day",
			allDay:    true,
			start:     time.Date(2026, time.July, 6, 0, 0, 0, 0, time.Local),
			end:       time.Date(2026, time.July, 9, 0, 0, 0, 0, time.Local),
			wantStart: time.Date(2026, time.July, 6, 0, 0, 0, 0, time.Local),
			wantEnd:   time.Date(2026, time.July, 9, 0, 0, 0, 0, time.Local),
		},
		{
			name:      "all day end before start",
			allDay:    true,
			start:     time.Date(2026, time.July, 6, 0, 0, 0, 0, time.Local),
			end:       time.Date(2026, time.July, 5, 0, 0, 0, 0, time.Local),
			wantError: "End must not be before start",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			form := New([]calendar.Calendar{{Name: "Personal"}}, time.Local)

			opened, _ := form.Update(msgs.OpenEventFormMsg{
				Event: calendar.Event{
					Title:       "Test event",
					Location:    "Test location",
					Description: "Test notes",
					AllDay:      testCase.allDay,
					Start:       testCase.start,
					End:         testCase.end,
				},
				IsNew: true,
			})

			entered, cmd := opened.Update(tea.KeyMsg{Type: tea.KeyEnter})

			if errorText := entered.(Model).errorText; errorText != testCase.wantError {
				t.Fatalf("got error %q, want %q", errorText, testCase.wantError)
			}

			if testCase.wantError != "" {
				return
			}

			if cmd == nil {
				t.Fatal("got no command, want a submit command")
			}

			submitted, ok := cmd().(msgs.EventFormSubmittedMsg)

			if !ok {
				t.Fatalf("got %T, want msgs.EventFormSubmittedMsg", cmd())
			}

			event := submitted.Event

			if !event.Start.Equal(testCase.wantStart) || !event.End.Equal(testCase.wantEnd) {
				t.Fatalf("got %v - %v, want %v - %v", event.Start, event.End, testCase.wantStart, testCase.wantEnd)
			}

			if event.AllDay != testCase.allDay {
				t.Fatalf("got AllDay %v, want %v", event.AllDay, testCase.allDay)
			}

			if event.Location != "Test location" || event.Description != "Test notes" {
				t.Fatalf("got location %q description %q, want the composed values", event.Location, event.Description)
			}
		})
	}
}
