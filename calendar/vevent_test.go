package calendar

import (
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

func decodeICS(t *testing.T, body ...string) *ical.Calendar {
	t.Helper()

	lines := append([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//test//test//EN",
	}, body...)
	lines = append(lines, "END:VCALENDAR", "")

	data, err := ical.NewDecoder(strings.NewReader(strings.Join(lines, "\r\n"))).Decode()

	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestEventsFromICal(t *testing.T) {
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	type expected struct {
		title     string
		start     string
		end       string
		allDay    bool
		recurring bool
	}

	cases := []struct {
		name string
		body []string
		want []expected
	}{
		{
			name: "timed event",
			body: []string{
				"BEGIN:VEVENT",
				"UID:one",
				"DTSTART:20260601T100000Z",
				"DTEND:20260601T110000Z",
				"SUMMARY:Standup",
				"LOCATION:Zoho Meeting",
				"END:VEVENT",
			},
			want: []expected{{title: "Standup", start: "2026-06-01 10:00", end: "2026-06-01 11:00"}},
		},
		{
			name: "all day event without an end",
			body: []string{
				"BEGIN:VEVENT",
				"UID:one",
				"DTSTART;VALUE=DATE:20260601",
				"SUMMARY:Holiday",
				"END:VEVENT",
			},
			want: []expected{{title: "Holiday", start: "2026-06-01 00:00", end: "2026-06-02 00:00", allDay: true}},
		},
		{
			name: "event outside the window is dropped",
			body: []string{
				"BEGIN:VEVENT",
				"UID:one",
				"DTSTART:20270601T100000Z",
				"DTEND:20270601T110000Z",
				"SUMMARY:Too far out",
				"END:VEVENT",
			},
			want: nil,
		},
		{
			name: "cancelled event is dropped",
			body: []string{
				"BEGIN:VEVENT",
				"UID:one",
				"DTSTART:20260601T100000Z",
				"DTEND:20260601T110000Z",
				"SUMMARY:Cancelled",
				"STATUS:CANCELLED",
				"END:VEVENT",
			},
			want: nil,
		},
		{
			name: "daily recurrence",
			body: []string{
				"BEGIN:VEVENT",
				"UID:one",
				"DTSTART:20260601T100000Z",
				"DTEND:20260601T103000Z",
				"RRULE:FREQ=DAILY;COUNT=3",
				"SUMMARY:Standup",
				"END:VEVENT",
			},
			want: []expected{
				{title: "Standup", start: "2026-06-01 10:00", end: "2026-06-01 10:30", recurring: true},
				{title: "Standup", start: "2026-06-02 10:00", end: "2026-06-02 10:30", recurring: true},
				{title: "Standup", start: "2026-06-03 10:00", end: "2026-06-03 10:30", recurring: true},
			},
		},
		{
			name: "exdate removes an occurrence",
			body: []string{
				"BEGIN:VEVENT",
				"UID:one",
				"DTSTART:20260601T100000Z",
				"DTEND:20260601T103000Z",
				"RRULE:FREQ=DAILY;COUNT=3",
				"EXDATE:20260602T100000Z",
				"SUMMARY:Standup",
				"END:VEVENT",
			},
			want: []expected{
				{title: "Standup", start: "2026-06-01 10:00", end: "2026-06-01 10:30", recurring: true},
				{title: "Standup", start: "2026-06-03 10:00", end: "2026-06-03 10:30", recurring: true},
			},
		},
		{
			name: "override moves and renames an occurrence",
			body: []string{
				"BEGIN:VEVENT",
				"UID:one",
				"DTSTART:20260601T100000Z",
				"DTEND:20260601T103000Z",
				"RRULE:FREQ=DAILY;COUNT=2",
				"SUMMARY:Standup",
				"END:VEVENT",
				"BEGIN:VEVENT",
				"UID:one",
				"RECURRENCE-ID:20260602T100000Z",
				"DTSTART:20260602T150000Z",
				"DTEND:20260602T153000Z",
				"SUMMARY:Moved standup",
				"END:VEVENT",
			},
			want: []expected{
				{title: "Standup", start: "2026-06-01 10:00", end: "2026-06-01 10:30", recurring: true},
				{title: "Moved standup", start: "2026-06-02 15:00", end: "2026-06-02 15:30", recurring: true},
			},
		},
		{
			name: "cancelled override drops one occurrence",
			body: []string{
				"BEGIN:VEVENT",
				"UID:one",
				"DTSTART:20260601T100000Z",
				"DTEND:20260601T103000Z",
				"RRULE:FREQ=DAILY;COUNT=2",
				"SUMMARY:Standup",
				"END:VEVENT",
				"BEGIN:VEVENT",
				"UID:one",
				"RECURRENCE-ID:20260602T100000Z",
				"DTSTART:20260602T100000Z",
				"DTEND:20260602T103000Z",
				"SUMMARY:Standup",
				"STATUS:CANCELLED",
				"END:VEVENT",
			},
			want: []expected{
				{title: "Standup", start: "2026-06-01 10:00", end: "2026-06-01 10:30", recurring: true},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			parsed := eventsFromICal(decodeICS(t, c.body...), "Test", from, to, time.UTC)

			if len(parsed) != len(c.want) {
				t.Fatalf("want %d events, got %d: %+v", len(c.want), len(parsed), parsed)
			}

			seenIDs := map[string]bool{}
			for index, want := range c.want {
				got := parsed[index].Event

				if seenIDs[got.ID] {
					t.Errorf("event %d: duplicate id %q", index, got.ID)
				}
				seenIDs[got.ID] = true

				if got.Calendar != "Test" {
					t.Errorf("event %d: want calendar Test, got %q", index, got.Calendar)
				}

				if got.Title != want.title {
					t.Errorf("event %d: want title %q, got %q", index, want.title, got.Title)
				}

				if got.Start.UTC().Format("2006-01-02 15:04") != want.start {
					t.Errorf("event %d: want start %s, got %s", index, want.start, got.Start.UTC())
				}

				if got.End.UTC().Format("2006-01-02 15:04") != want.end {
					t.Errorf("event %d: want end %s, got %s", index, want.end, got.End.UTC())
				}

				if got.AllDay != want.allDay {
					t.Errorf("event %d: want allDay %v, got %v", index, want.allDay, got.AllDay)
				}

				if got.Recurring != want.recurring {
					t.Errorf("event %d: want recurring %v, got %v", index, want.recurring, got.Recurring)
				}
			}
		})
	}
}

func TestEventsFromICalAttendees(t *testing.T) {
	data := decodeICS(t,
		"BEGIN:VEVENT",
		"UID:one",
		"DTSTART:20260601T100000Z",
		"DTEND:20260601T110000Z",
		"SUMMARY:Design review",
		"ATTENDEE;CN=Priya:mailto:priya@example.com",
		"ATTENDEE:mailto:marcus@example.com",
		"END:VEVENT",
	)

	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	to := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	parsed := eventsFromICal(data, "Test", from, to, time.UTC)

	if len(parsed) != 1 {
		t.Fatalf("want 1 event, got %d", len(parsed))
	}

	attendees := parsed[0].Attendees
	if len(attendees) != 2 || attendees[0] != "Priya" || attendees[1] != "marcus@example.com" {
		t.Fatalf("want [Priya marcus@example.com], got %v", attendees)
	}
}

func TestDescriptionRoundTrip(t *testing.T) {
	data := decodeICS(t,
		"BEGIN:VEVENT",
		"UID:one",
		"DTSTART:20260601T100000Z",
		"DTEND:20260601T110000Z",
		"SUMMARY:Standup",
		"DESCRIPTION:Agenda:\\n1. Retro\\n2. Plans",
		"END:VEVENT",
	)

	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	to := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	events := eventsFromICal(data, "Work", from, to, time.UTC)

	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}

	if events[0].Description != "Agenda:\n1. Retro\n2. Plans" {
		t.Fatalf("want unescaped multiline description, got %q", events[0].Description)
	}
}
