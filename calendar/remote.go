package calendar

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var palette = []string{
	"#7AA2F7",
	"#F7768E",
	"#9ECE6A",
	"#E0AF68",
	"#BB9AF7",
	"#7DCFFF",
	"#FF9E64",
}

type remoteClient interface {
	fetch(from, to time.Time) ([]Calendar, []Event, error)
}

type writableClient interface {
	create(Event) (Event, error)
	update(Event) error
	remove(id string) error
}

type remoteAccount struct {
	name         string
	opMutex      sync.Mutex
	client       remoteClient
	calendars    []Calendar
	events       []Event
	suspectEmpty bool
}

type Remote struct {
	location       *time.Location
	cacheDir       string
	from, to       time.Time
	clock          func() time.Time
	colorOverrides map[string]string
	stateMutex     sync.RWMutex
	accounts       []*remoteAccount
}

func NewRemote(accounts []Account, colorOverrides map[string]string, location *time.Location, now time.Time) (*Remote, error) {
	cacheDir, err := CacheDir()

	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	remote := &Remote{
		location:       location,
		cacheDir:       cacheDir,
		from:           now.AddDate(-1, 0, 0),
		to:             now.AddDate(1, 0, 0),
		clock:          time.Now,
		colorOverrides: colorOverrides,
	}

	seen := map[string]bool{}
	for _, account := range accounts {
		if account.Type == "caldav" && (account.Username == "" || account.URL == "") {
			entry, err := storedCredentials(account.Name)

			if err != nil {
				return nil, err
			}

			if account.Username == "" {
				account.Username = entry.Username
			}

			if account.URL == "" {
				account.URL = entry.URL
			}
		}

		if err := account.Validate(); err != nil {
			return nil, err
		}

		if seen[account.Name] {
			return nil, fmt.Errorf("two accounts are both named %q", account.Name)
		}
		seen[account.Name] = true

		var client remoteClient

		switch account.Type {
		case "caldav":
			password, err := account.Secret()

			if err != nil {
				return nil, err
			}

			client, err = newCaldavClient(account, password, location)

			if err != nil {
				return nil, err
			}

		case "ics":
			subscriptionURL := account.URL
			if subscriptionURL == "" {
				subscriptionURL, err = account.Secret()

				if err != nil {
					return nil, err
				}
			}

			client = &icsClient{
				accountName: account.Name,
				url:         subscriptionURL,
				httpClient:  &http.Client{Timeout: 30 * time.Second},
				location:    location,
			}
		}

		state := &remoteAccount{name: account.Name, client: client}
		remote.loadCache(state)
		remote.accounts = append(remote.accounts, state)
	}

	return remote, nil
}

func (r *Remote) AccountNames() []string {
	names := make([]string, 0, len(r.accounts))
	for _, account := range r.accounts {
		names = append(names, account.name)
	}

	return names
}

func (r *Remote) Sync(name string) error {
	account := r.accountNamed(name)
	if account == nil {
		return fmt.Errorf("unknown account %q", name)
	}

	account.opMutex.Lock()

	now := r.clock().In(r.location)

	r.stateMutex.Lock()
	r.from = now.AddDate(-1, 0, 0)
	r.to = now.AddDate(1, 0, 0)
	from, to := r.from, r.to
	r.stateMutex.Unlock()

	calendars, events, err := account.client.fetch(from, to)

	if err != nil {
		account.opMutex.Unlock()

		return err
	}

	r.stateMutex.RLock()
	hadEvents := len(account.events) > 0
	r.stateMutex.RUnlock()

	if len(events) == 0 && hadEvents && !account.suspectEmpty {
		account.suspectEmpty = true
		account.opMutex.Unlock()

		return fmt.Errorf(
			"account %q suddenly reports no events, keeping the cached copy: refresh again to accept the empty calendar",
			name,
		)
	}

	account.suspectEmpty = false

	cacheErr := r.saveCache(account.name, calendars, events)
	account.opMutex.Unlock()

	calendars, events = r.decorate(account.name, calendars, events)

	r.stateMutex.Lock()
	account.calendars = calendars
	account.events = events
	r.stateMutex.Unlock()

	return cacheErr
}

func (r *Remote) Events(from, to time.Time) []Event {
	r.stateMutex.RLock()
	defer r.stateMutex.RUnlock()

	var events []Event
	for _, account := range r.accounts {
		for _, event := range account.events {
			if event.End.After(from) && event.Start.Before(to) {
				events = append(events, event)
			}
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

func (r *Remote) Calendars() []Calendar {
	r.stateMutex.RLock()
	defer r.stateMutex.RUnlock()

	var calendars []Calendar
	for _, account := range r.accounts {
		calendars = append(calendars, account.calendars...)
	}

	return calendars
}

func (r *Remote) WritableCalendars() []Calendar {
	r.stateMutex.RLock()
	defer r.stateMutex.RUnlock()

	var calendars []Calendar
	for _, account := range r.accounts {
		if _, ok := account.client.(writableClient); !ok {
			continue
		}

		calendars = append(calendars, account.calendars...)
	}

	return calendars
}

func (r *Remote) Add(event Event) (Event, error) {
	account := r.accountForCalendar(event.Calendar)
	if account == nil {
		return Event{}, fmt.Errorf("no account has a calendar named %q", event.Calendar)
	}

	writer, ok := account.client.(writableClient)
	if !ok {
		return Event{}, fmt.Errorf("%s is a read-only subscription", account.name)
	}

	account.opMutex.Lock()
	created, err := writer.create(event)
	account.opMutex.Unlock()

	if err != nil {
		return Event{}, err
	}

	if err := r.Sync(account.name); err != nil {
		return Event{}, fmt.Errorf("event saved, but refreshing failed: %w", err)
	}

	created.ID = account.name + ":" + created.ID

	return created, nil
}

func (r *Remote) Update(event Event) error {
	account, id, err := r.accountForEvent(event.ID)

	if err != nil {
		return err
	}

	writer, ok := account.client.(writableClient)
	if !ok {
		return fmt.Errorf("%s is a read-only subscription", account.name)
	}

	target := r.accountForCalendar(event.Calendar)
	if target != nil && target != account {
		if _, err := r.Add(event); err != nil {
			return err
		}

		return r.Delete(event.ID)
	}

	event.ID = id

	account.opMutex.Lock()
	err = writer.update(event)
	account.opMutex.Unlock()

	if err != nil {
		return err
	}

	if err := r.Sync(account.name); err != nil {
		return fmt.Errorf("event saved, but refreshing failed: %w", err)
	}

	return nil
}

func (r *Remote) Delete(id string) error {
	account, strippedID, err := r.accountForEvent(id)

	if err != nil {
		return err
	}

	writer, ok := account.client.(writableClient)
	if !ok {
		return fmt.Errorf("%s is a read-only subscription", account.name)
	}

	account.opMutex.Lock()
	err = writer.remove(strippedID)
	account.opMutex.Unlock()

	if err != nil {
		return err
	}

	if err := r.Sync(account.name); err != nil {
		return fmt.Errorf("event deleted, but refreshing failed: %w", err)
	}

	return nil
}

func (r *Remote) accountNamed(name string) *remoteAccount {
	for _, account := range r.accounts {
		if account.name == name {
			return account
		}
	}

	return nil
}

func (r *Remote) accountForEvent(id string) (*remoteAccount, string, error) {
	accountName, strippedID, found := strings.Cut(id, ":")
	if !found {
		return nil, "", fmt.Errorf("event %q does not belong to any account", id)
	}

	account := r.accountNamed(accountName)
	if account == nil {
		return nil, "", fmt.Errorf("unknown account %q", accountName)
	}

	return account, strippedID, nil
}

func (r *Remote) accountForCalendar(name string) *remoteAccount {
	r.stateMutex.RLock()
	defer r.stateMutex.RUnlock()

	for _, account := range r.accounts {
		for _, accountCalendar := range account.calendars {
			if accountCalendar.Name == name {
				return account
			}
		}
	}

	return nil
}

func (r *Remote) decorate(accountName string, calendars []Calendar, events []Event) ([]Calendar, []Event) {
	colors := map[string]string{}
	for index := range calendars {
		override, ok := r.colorOverrides[accountName+"/"+calendars[index].Name]
		if !ok {
			override, ok = r.colorOverrides[calendars[index].Name]
		}

		switch {
		case ok:
			calendars[index].Color = override

		case calendars[index].Color == "":
			calendars[index].Color = paletteColor(accountName + "/" + calendars[index].Name)
		}

		colors[calendars[index].Name] = calendars[index].Color
	}

	for index := range events {
		events[index].ID = accountName + ":" + events[index].ID
		events[index].Start = events[index].Start.In(r.location)
		events[index].End = events[index].End.In(r.location)

		if events[index].Color == "" {
			events[index].Color = colors[events[index].Calendar]
		}
	}

	return calendars, events
}

func paletteColor(key string) string {
	hash := fnv.New32a()
	hash.Write([]byte(key))

	return palette[hash.Sum32()%uint32(len(palette))]
}

type cacheFile struct {
	Calendars []Calendar
	Events    []Event
}

func (r *Remote) cachePath(accountName string) string {
	return filepath.Join(r.cacheDir, accountName+".json")
}

func (r *Remote) loadCache(account *remoteAccount) {
	data, err := os.ReadFile(r.cachePath(account.name))

	if err != nil {
		return
	}

	var cached cacheFile

	if err := json.Unmarshal(data, &cached); err != nil {
		return
	}

	account.calendars, account.events = r.decorate(account.name, cached.Calendars, cached.Events)
}

func (r *Remote) saveCache(accountName string, calendars []Calendar, events []Event) error {
	data, err := json.Marshal(cacheFile{Calendars: calendars, Events: events})

	if err != nil {
		return fmt.Errorf("caching events: %w", err)
	}

	path := r.cachePath(accountName)
	temporary := path + ".tmp"

	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("caching events: %w", err)
	}

	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("caching events: %w", err)
	}

	return nil
}
