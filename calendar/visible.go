package calendar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Visible struct {
	source     Source
	mutex      sync.RWMutex
	hiddenPath string
	hidden     map[string]bool
}

type CalendarVisibility struct {
	Calendar
	Hidden bool
}

func NewVisible(source Source) *Visible {
	visible := &Visible{source: source, hidden: map[string]bool{}}

	cacheDir, err := CacheDir()

	if err != nil {
		return visible
	}

	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return visible
	}

	// The leading dot keeps the file out of the <account>.json namespace,
	// since account names cannot contain dots.
	visible.hiddenPath = filepath.Join(cacheDir, ".hidden.json")

	data, err := os.ReadFile(visible.hiddenPath)

	if err != nil {
		return visible
	}

	var names []string

	if err := json.Unmarshal(data, &names); err != nil {
		return visible
	}

	for _, name := range names {
		visible.hidden[name] = true
	}

	return visible
}

func (v *Visible) Events(from, to time.Time) []Event {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	var events []Event
	for _, event := range v.source.Events(from, to) {
		if v.hidden[event.Calendar] {
			continue
		}

		events = append(events, event)
	}

	return events
}

func (v *Visible) Calendars() []Calendar {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	var calendars []Calendar
	for _, visibleCalendar := range v.source.Calendars() {
		if v.hidden[visibleCalendar.Name] {
			continue
		}

		calendars = append(calendars, visibleCalendar)
	}

	return calendars
}

func (v *Visible) All() []CalendarVisibility {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	var all []CalendarVisibility
	for _, sourceCalendar := range v.source.Calendars() {
		all = append(all, CalendarVisibility{
			Calendar: sourceCalendar,
			Hidden:   v.hidden[sourceCalendar.Name],
		})
	}

	return all
}

func (v *Visible) Toggle(name string) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if v.hidden[name] {
		delete(v.hidden, name)
	} else {
		v.hidden[name] = true
	}

	if v.hiddenPath == "" {
		return
	}

	names := make([]string, 0, len(v.hidden))
	for hiddenName := range v.hidden {
		names = append(names, hiddenName)
	}

	sort.Strings(names)

	data, err := json.Marshal(names)

	if err != nil {
		return
	}

	os.WriteFile(v.hiddenPath, data, 0o600)
}
