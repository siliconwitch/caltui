package calendar

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Event struct {
	ID            string
	Title         string
	Start         time.Time
	End           time.Time
	AllDay        bool
	Location      string
	Description   string
	Attendees     []string
	Calendar      string
	Color         string
	Recurring     bool
	Recurrence    Recurrence
	Alarms        []time.Duration
	Organizer     string
	Participation string
}

type Recurrence struct {
	Frequency string
	Interval  int
	Until     time.Time
}

func (r Recurrence) SameSpec(other Recurrence, zone *time.Location) bool {
	sameUntilDay := r.Until.IsZero() == other.Until.IsZero()
	if !r.Until.IsZero() && !other.Until.IsZero() {
		year, month, day := r.Until.In(zone).Date()

		otherYear, otherMonth, otherDay := other.Until.In(zone).Date()

		sameUntilDay = year == otherYear && month == otherMonth && day == otherDay
	}

	return r.Frequency == other.Frequency &&
		max(r.Interval, 1) == max(other.Interval, 1) &&
		sameUntilDay
}

type Calendar struct {
	Name     string
	Color    string
	ReadOnly bool
}

func sortedByStart(events []Event) []Event {
	sort.Slice(events, func(i, j int) bool {
		if events[i].Start.Equal(events[j].Start) {
			return events[i].Title < events[j].Title
		}
		return events[i].Start.Before(events[j].Start)
	})

	return events
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

const ConfigSection = "calendar"

type Config struct {
	Timezone     string            `toml:"timezone"`
	SyncInterval string            `toml:"sync_interval"`
	Colors       map[string]string `toml:"colors"`
}

func DefaultConfig() Config {
	return Config{Timezone: "", SyncInterval: "15m", Colors: map[string]string{}}
}

func (c Config) Location() (*time.Location, error) {
	if c.Timezone == "" {
		return time.Local, nil
	}

	return time.LoadLocation(c.Timezone)
}

func (c Config) RefreshInterval() (time.Duration, error) {
	if c.SyncInterval == "" {
		return 0, nil
	}

	interval, err := time.ParseDuration(c.SyncInterval)

	if err != nil {
		return 0, err
	}

	if interval != 0 && interval < time.Minute {
		return 0, fmt.Errorf("sync_interval %q is below the 1m minimum (use \"0\" to disable)", c.SyncInterval)
	}

	return interval, nil
}

func cacheDir() (string, error) {
	if path := os.Getenv("CALTUI_CACHE"); path != "" {
		return path, nil
	}

	base, err := os.UserCacheDir()

	if err != nil {
		return "", fmt.Errorf("finding cache directory: %w", err)
	}

	return filepath.Join(base, "caltui"), nil
}
