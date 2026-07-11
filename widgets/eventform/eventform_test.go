package eventform

import (
	"testing"
	"time"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/maskinput"
)

func TestComposedEvent(t *testing.T) {
	date := func(year int, month time.Month, day int) maskinput.Field {
		return maskinput.NewDate(false).WithDate(time.Date(year, month, day, 0, 0, 0, 0, time.Local))
	}

	clock := func(hour, minute int) maskinput.Field {
		return maskinput.NewTime().WithTime(hour, minute)
	}

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
		startDate maskinput.Field
		startTime maskinput.Field
		endDate   maskinput.Field
		endTime   maskinput.Field
		startZone *time.Location
		endZone   *time.Location
		wantStart time.Time
		wantEnd   time.Time
		wantError string
	}{
		{
			name:      "cross zone flight",
			startDate: date(2026, time.July, 6),
			startTime: clock(12, 41),
			endDate:   date(2026, time.July, 7),
			endTime:   clock(9, 30),
			startZone: losAngeles,
			endZone:   stockholm,
			wantStart: time.Date(2026, time.July, 6, 12, 41, 0, 0, losAngeles),
			wantEnd:   time.Date(2026, time.July, 7, 9, 30, 0, 0, stockholm),
		},
		{
			name:      "cross zone end before start in absolute time",
			startDate: date(2026, time.July, 6),
			startTime: clock(12, 41),
			endDate:   date(2026, time.July, 6),
			endTime:   clock(16, 30),
			startZone: losAngeles,
			endZone:   stockholm,
			wantError: "End must be after start",
		},
		{
			name:      "timed same day",
			startDate: date(2026, time.July, 6),
			startTime: clock(9, 0),
			endDate:   date(2026, time.July, 6),
			endTime:   clock(10, 30),
			wantStart: time.Date(2026, time.July, 6, 9, 0, 0, 0, time.Local),
			wantEnd:   time.Date(2026, time.July, 6, 10, 30, 0, 0, time.Local),
		},
		{
			name:      "timed multi day",
			startDate: date(2026, time.July, 6),
			startTime: clock(16, 0),
			endDate:   date(2026, time.July, 7),
			endTime:   clock(9, 30),
			wantStart: time.Date(2026, time.July, 6, 16, 0, 0, 0, time.Local),
			wantEnd:   time.Date(2026, time.July, 7, 9, 30, 0, 0, time.Local),
		},
		{
			name:      "timed end before start",
			startDate: date(2026, time.July, 6),
			startTime: clock(10, 0),
			endDate:   date(2026, time.July, 6),
			endTime:   clock(9, 0),
			wantError: "End must be after start",
		},
		{
			name:      "timed end equal to start",
			startDate: date(2026, time.July, 6),
			startTime: clock(10, 0),
			endDate:   date(2026, time.July, 6),
			endTime:   clock(10, 0),
			wantError: "End must be after start",
		},
		{
			name:      "all day single day",
			allDay:    true,
			startDate: date(2026, time.July, 6),
			startTime: clock(9, 0),
			endDate:   date(2026, time.July, 6),
			endTime:   clock(10, 0),
			wantStart: time.Date(2026, time.July, 6, 0, 0, 0, 0, time.Local),
			wantEnd:   time.Date(2026, time.July, 7, 0, 0, 0, 0, time.Local),
		},
		{
			name:      "all day multi day",
			allDay:    true,
			startDate: date(2026, time.July, 6),
			startTime: clock(0, 0),
			endDate:   date(2026, time.July, 8),
			endTime:   clock(0, 0),
			wantStart: time.Date(2026, time.July, 6, 0, 0, 0, 0, time.Local),
			wantEnd:   time.Date(2026, time.July, 9, 0, 0, 0, 0, time.Local),
		},
		{
			name:      "all day end before start",
			allDay:    true,
			startDate: date(2026, time.July, 6),
			startTime: clock(0, 0),
			endDate:   date(2026, time.July, 5),
			endTime:   clock(0, 0),
			wantError: "End must not be before start",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			startZone := testCase.startZone
			if startZone == nil {
				startZone = time.Local
			}

			endZone := testCase.endZone
			if endZone == nil {
				endZone = time.Local
			}

			event, problem := composedEvent(
				calendar.Event{},
				"Test event", "Test location", "Test notes",
				calendar.Recurrence{},
				testCase.allDay,
				testCase.startDate, testCase.startTime, testCase.endDate, testCase.endTime,
				startZone, endZone, time.Local,
				"Personal",
			)

			if problem != testCase.wantError {
				t.Fatalf("got error %q, want %q", problem, testCase.wantError)
			}

			if testCase.wantError != "" {
				return
			}

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
