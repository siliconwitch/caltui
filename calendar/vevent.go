package calendar

import (
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

type parsedEvent struct {
	Event
	UID            string
	OccurrenceTime time.Time
}

func recurrenceSpec(props ical.Props) Recurrence {
	roption, err := props.RecurrenceRule()

	if err != nil || roption == nil {
		return Recurrence{}
	}

	spec := Recurrence{Interval: roption.Interval, Until: roption.Until}

	switch roption.Freq {
	case rrule.DAILY:
		spec.Frequency = "daily"
	case rrule.WEEKLY:
		spec.Frequency = "weekly"
	case rrule.MONTHLY:
		spec.Frequency = "monthly"
	case rrule.YEARLY:
		spec.Frequency = "yearly"
	default:
		return Recurrence{}
	}

	return spec
}

func eventsFromICal(data *ical.Calendar, calendarName string, from, to time.Time, location *time.Location) []parsedEvent {
	type series struct {
		master    *ical.Event
		overrides []*ical.Event
	}

	var order []string

	byUID := map[string]*series{}
	for _, icalEvent := range data.Events() {
		uid, err := icalEvent.Props.Text(ical.PropUID)
		if err != nil || uid == "" {
			continue
		}

		group, ok := byUID[uid]
		if !ok {
			group = &series{}
			byUID[uid] = group
			order = append(order, uid)
		}

		event := icalEvent
		if event.Props.Get(ical.PropRecurrenceID) != nil {
			group.overrides = append(group.overrides, &event)
		} else {
			group.master = &event
		}
	}

	var events []parsedEvent

	emitOverride := func(uid string, override *ical.Event, spec Recurrence) {
		event, ok := singleEvent(*override, calendarName, location)
		if !ok || !event.End.After(from) || !event.Start.Before(to) {
			return
		}

		recurrenceTime, err := override.Props.DateTime(ical.PropRecurrenceID, location)

		if err != nil {
			return
		}

		event.ID = fmt.Sprintf("%s@%d", uid, recurrenceTime.Unix())
		event.Recurring = true
		event.Recurrence = spec
		events = append(events, parsedEvent{Event: event, UID: uid, OccurrenceTime: recurrenceTime})
	}

	for _, uid := range order {
		group := byUID[uid]

		if group.master == nil {
			for _, override := range group.overrides {
				emitOverride(uid, override, Recurrence{})
			}

			continue
		}

		masterEvent, ok := singleEvent(*group.master, calendarName, location)
		if !ok {
			continue
		}

		set, err := group.master.RecurrenceSet(location)

		if err != nil {
			continue
		}

		if set == nil {
			if masterEvent.End.After(from) && masterEvent.Start.Before(to) {
				masterEvent.ID = uid
				events = append(events, parsedEvent{Event: masterEvent, UID: uid})
			}

			continue
		}

		spec := recurrenceSpec(group.master.Props)

		overridesByTime := map[int64]*ical.Event{}
		for _, override := range group.overrides {
			recurrenceTime, err := override.Props.DateTime(ical.PropRecurrenceID, location)

			if err != nil {
				continue
			}

			overridesByTime[recurrenceTime.Unix()] = override
		}

		duration := masterEvent.End.Sub(masterEvent.Start)

		consumed := map[int64]bool{}
		for _, occurrence := range set.Between(from.Add(-duration), to, true) {
			if override, ok := overridesByTime[occurrence.Unix()]; ok {
				consumed[occurrence.Unix()] = true
				emitOverride(uid, override, spec)

				continue
			}

			occurrenceEvent := masterEvent
			occurrenceEvent.ID = fmt.Sprintf("%s@%d", uid, occurrence.Unix())
			occurrenceEvent.Start = occurrence
			occurrenceEvent.End = occurrence.Add(duration)
			occurrenceEvent.Recurring = true
			occurrenceEvent.Recurrence = spec
			events = append(events, parsedEvent{Event: occurrenceEvent, UID: uid, OccurrenceTime: occurrence})
		}

		for _, override := range group.overrides {
			recurrenceTime, err := override.Props.DateTime(ical.PropRecurrenceID, location)

			if err != nil || consumed[recurrenceTime.Unix()] {
				continue
			}

			emitOverride(uid, override, spec)
		}
	}

	return events
}

func singleEvent(icalEvent ical.Event, calendarName string, location *time.Location) (Event, bool) {
	status, err := icalEvent.Status()

	if err == nil && status == ical.EventCancelled {
		return Event{}, false
	}

	start, err := icalEvent.DateTimeStart(location)

	if err != nil || start.IsZero() {
		return Event{}, false
	}

	end, err := icalEvent.DateTimeEnd(location)

	if err != nil {
		return Event{}, false
	}

	if !end.After(start) {
		end = start
	}

	title, _ := icalEvent.Props.Text(ical.PropSummary)
	eventLocation, _ := icalEvent.Props.Text(ical.PropLocation)
	description, _ := icalEvent.Props.Text(ical.PropDescription)

	var attendees []string
	for _, attendeeProp := range icalEvent.Props.Values(ical.PropAttendee) {
		name := attendeeProp.Params.Get(ical.ParamCommonName)
		if name == "" {
			name = strings.TrimPrefix(attendeeProp.Value, "mailto:")
		}

		attendees = append(attendees, name)
	}

	startProp := icalEvent.Props.Get(ical.PropDateTimeStart)

	return Event{
		Title:       title,
		Start:       start,
		End:         end,
		AllDay:      startProp.ValueType() == ical.ValueDate,
		Location:    eventLocation,
		Description: strings.ReplaceAll(description, "\r\n", "\n"),
		Attendees:   attendees,
		Calendar:    calendarName,
	}, true
}
