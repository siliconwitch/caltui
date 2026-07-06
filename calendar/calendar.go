package calendar

import "time"

type Event struct {
	ID        string
	Title     string
	Start     time.Time
	End       time.Time
	AllDay    bool
	Location  string
	Attendees []string
	Calendar  string
	Color     string
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
	Add(Event) Event
	Update(Event)
	Delete(id string)
}
