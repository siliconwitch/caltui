package calendar

import "time"

type Event struct {
	ID          string
	Title       string
	Start       time.Time
	End         time.Time
	AllDay      bool
	Location    string
	Description string
	Attendees   []string
	Calendar    string
	Color       string
	Recurring   bool
	Recurrence  Recurrence
}

type Recurrence struct {
	Frequency string
	Interval  int
	Until     time.Time
}

type Calendar struct {
	Name  string
	Color string
}

type Source interface {
	Events(from, to time.Time) []Event
	Calendars() []Calendar
}

type Store interface {
	Source
	Add(Event) (Event, error)
	Update(Event) error
	Delete(id string) error
}

func WritableCalendars(store Store) []Calendar {
	if writable, ok := store.(interface{ WritableCalendars() []Calendar }); ok {
		return writable.WritableCalendars()
	}

	return store.Calendars()
}
