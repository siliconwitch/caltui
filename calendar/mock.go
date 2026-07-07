package calendar

import (
	"fmt"
	"sort"
	"time"
)

const (
	workColor     = "#7AA2F7"
	personalColor = "#F7768E"
)

type Mock struct {
	events map[string]Event
	nextID int
}

func NewMock(now time.Time) *Mock {
	mock := &Mock{events: map[string]Event{}, nextID: 1}

	monday := now.AddDate(0, 0, -(int(now.Weekday())+6)%7)
	monday = time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, now.Location())

	losAngeles, err := time.LoadLocation("America/Los_Angeles")

	if err != nil {
		losAngeles = now.Location()
	}

	seed := func(day time.Time, hour, minute, durationMinutes int, event Event) {
		event.Start = time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, day.Location())
		event.End = event.Start.Add(time.Duration(durationMinutes) * time.Minute)
		mock.Add(event)
	}

	for weekOffset := -8; weekOffset <= 8; weekOffset++ {
		week := monday.AddDate(0, 0, weekOffset*7)

		for weekday := range 5 {
			seed(week.AddDate(0, 0, weekday), 9, 30, 15, Event{
				Title:     "Standup",
				Calendar:  "Work",
				Location:  "Zoho Meeting",
				Attendees: []string{"Raj", "Priya", "Marcus", "Elena"},
			})
		}

		seed(week.AddDate(0, 0, 1), 11, 0, 45, Event{
			Title:     "1:1 with Priya",
			Calendar:  "Work",
			Location:  "Zoho Meeting",
			Attendees: []string{"Raj", "Priya"},
		})

		seed(week.AddDate(0, 0, 2), 14, 0, 120, Event{
			Title:     "Design review",
			Calendar:  "Work",
			Location:  "Conference room",
			Attendees: []string{"Raj", "Priya", "Marcus", "Elena", "Tobias", "Ines"},
		})

		seed(week.AddDate(0, 0, 2), 15, 0, 30, Event{
			Title:     "Investor sync",
			Calendar:  "Work",
			Location:  "Zoho Meeting",
			Attendees: []string{"Raj", "Naomi"},
		})

		seed(week.AddDate(0, 0, 3), 13, 30, 45, Event{
			Title:     "Call with John",
			Calendar:  "Personal",
			Attendees: []string{"Raj", "John"},
		})

		sanFranciscoDay := week.AddDate(0, 0, 3)

		sanFranciscoStart := time.Date(
			sanFranciscoDay.Year(), sanFranciscoDay.Month(), sanFranciscoDay.Day(),
			9, 0, 0, 0, losAngeles,
		)

		mock.Add(Event{
			Title:     "Call with SF office",
			Calendar:  "Work",
			Location:  "Zoho Meeting",
			Attendees: []string{"Raj", "Dana", "Miguel"},
			Start:     sanFranciscoStart,
			End:       sanFranciscoStart.Add(45 * time.Minute),
		})

		seed(week.AddDate(0, 0, 4), 16, 0, 30, Event{
			Title:    "Weekly wrap-up",
			Calendar: "Work",
			Location: "Zoho Meeting",
		})

		seed(week.AddDate(0, 0, 5), 10, 0, 180, Event{
			Title:    "Climbing",
			Calendar: "Personal",
			Location: "Klättercentret",
		})

		seed(week.AddDate(0, 0, 6), 18, 30, 120, Event{
			Title:     "Dinner with Sara",
			Calendar:  "Personal",
			Attendees: []string{"Raj", "Sara"},
		})

		if weekOffset%4 == 0 {
			seed(week, 10, 0, 150, Event{
				Title:     "Quarterly planning session with the entire hardware team",
				Calendar:  "Work",
				Location:  "Off-site",
				Attendees: []string{"Raj", "Priya", "Marcus", "Elena", "Tobias", "Ines", "Naomi"},
			})
		}

		if weekOffset%4 == 2 {
			sprintStart := week.AddDate(0, 0, 1)

			mock.Add(Event{
				Title:    "Hardware design sprint",
				Calendar: "Work",
				Location: "Studio",
				AllDay:   true,
				Start:    sprintStart,
				End:      sprintStart.AddDate(0, 0, 3),
			})
		}
	}

	birthday := monday.AddDate(0, 0, 12)

	mock.Add(Event{
		Title:    "Anna's birthday",
		Calendar: "Personal",
		AllDay:   true,
		Start:    birthday,
		End:      birthday.AddDate(0, 0, 1),
	})

	seed(monday.AddDate(0, 0, 11), 16, 0, 1050, Event{
		Title:    "Flight to Berlin",
		Calendar: "Personal",
		Location: "ARN → BER",
	})

	seed(now, 17, 30, 15, Event{
		Title:    "Pick up parcel from the post office",
		Calendar: "Personal",
	})

	return mock
}

func (m *Mock) Calendars() []Calendar {
	return []Calendar{
		{Name: "Personal", Color: personalColor},
		{Name: "Work", Color: workColor},
	}
}

func (m *Mock) Events(from, to time.Time) []Event {
	var events []Event
	for _, event := range m.events {
		if event.End.After(from) && event.Start.Before(to) {
			events = append(events, event)
		}
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].Start.Equal(events[j].Start) {
			return events[i].Title < events[j].Title
		}
		return events[i].Start.Before(events[j].Start)
	})

	return events
}

func (m *Mock) Add(event Event) (Event, error) {
	if event.ID == "" {
		event.ID = fmt.Sprintf("mock-%d", m.nextID)
		m.nextID++
	}

	event.Color = m.colorFor(event.Calendar)
	m.events[event.ID] = event

	return event, nil
}

func (m *Mock) Update(event Event) error {
	event.Color = m.colorFor(event.Calendar)
	m.events[event.ID] = event

	return nil
}

func (m *Mock) Delete(id string) error {
	delete(m.events, id)

	return nil
}

func (m *Mock) colorFor(name string) string {
	for _, c := range m.Calendars() {
		if c.Name == name {
			return c.Color
		}
	}

	return workColor
}
