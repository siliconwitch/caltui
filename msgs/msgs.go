package msgs

import (
	"time"

	"github.com/siliconwitch/caltui/calendar"
)

type FocusDateMsg struct{ Date time.Time }

type EventSelectedMsg struct{ Event *calendar.Event }

type OpenEventFormMsg struct {
	Event calendar.Event
	IsNew bool
}

type EventFormSubmittedMsg struct {
	Event calendar.Event
	IsNew bool
}

type RequestDeleteMsg struct{ Event calendar.Event }

type DeleteConfirmedMsg struct{ Event calendar.Event }

type ClosePopupMsg struct{}

type YankMsg struct{ Event calendar.Event }

type YankedMsg struct{ EventID string }

type PasteMsg struct{ Date time.Time }

type OpenGotoMsg struct{ Date time.Time }

type GotoDateMsg struct{ Date time.Time }

type EventsChangedMsg struct{}

type KeyHint struct{ Key, Action string }
