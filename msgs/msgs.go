package msgs

import (
	"time"

	"github.com/siliconwitch/caltui/calendar"
)

type FocusDateMsg struct{ Date time.Time }

type EventSelectedMsg struct{ Event *calendar.Event }

type EditScope int

const (
	ScopeUnset EditScope = iota
	ScopeOccurrence
	ScopeSeries
)

type OpenEventFormMsg struct {
	Event calendar.Event
	IsNew bool
	Scope EditScope
}

type EventFormSubmittedMsg struct {
	Event calendar.Event
	IsNew bool
	Scope EditScope
}

type RequestDeleteMsg struct{ Event calendar.Event }

type DeleteConfirmedMsg struct {
	Event calendar.Event
	Scope EditScope
}

type ClosePopupMsg struct{}

type YankMsg struct{ Event calendar.Event }

type YankedMsg struct{ EventID string }

type PasteMsg struct{ Date time.Time }

type OpenGotoMsg struct{ Date time.Time }

type OpenSearchMsg struct{}

type OpenCalendarsMsg struct{}

type GotoDateMsg struct{ Date time.Time }

type EventsChangedMsg struct{}

type ClockTickMsg struct{ Now time.Time }

type SyncedMsg struct {
	Account string
	Err     error
}

type StoreErrorMsg struct{ Err error }

type CalendarsChangedMsg struct{ Calendars []calendar.Calendar }

type KeyHint struct{ Key, Action string }
