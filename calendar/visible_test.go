package calendar

import (
	"testing"
	"time"
)

type visibleSourceStub struct{}

func (visibleSourceStub) Events(from, to time.Time) []Event {
	return []Event{
		{ID: "one", Title: "Standup", Calendar: "Work"},
		{ID: "two", Title: "Yoga", Calendar: "Personal"},
	}
}

func (visibleSourceStub) Calendars() []Calendar {
	return []Calendar{{Name: "Work"}, {Name: "Personal"}}
}

func TestVisibleTogglesAndPersists(t *testing.T) {
	t.Setenv("CALTUI_CACHE", t.TempDir())

	visible := NewVisible(visibleSourceStub{})

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	if events := visible.Events(from, to); len(events) != 2 {
		t.Fatalf("want both events before hiding, got %+v", events)
	}

	visible.Toggle("Work")

	events := visible.Events(from, to)

	if len(events) != 1 || events[0].Calendar != "Personal" {
		t.Fatalf("want only Personal events after hiding Work, got %+v", events)
	}

	if calendars := visible.Calendars(); len(calendars) != 1 || calendars[0].Name != "Personal" {
		t.Fatalf("want only Personal in the calendar list, got %+v", calendars)
	}

	all := visible.All()

	if len(all) != 2 || !all[0].Hidden || all[1].Hidden {
		t.Fatalf("want All to flag Work as hidden, got %+v", all)
	}

	restored := NewVisible(visibleSourceStub{})

	if events := restored.Events(from, to); len(events) != 1 {
		t.Fatalf("want the hidden set persisted across restarts, got %+v", events)
	}

	restored.Toggle("Work")

	if events := restored.Events(from, to); len(events) != 2 {
		t.Fatalf("want Work visible again after a second toggle, got %+v", events)
	}
}
