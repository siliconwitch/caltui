package calendar

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeClient struct {
	calendars []Calendar
	events    []Event
	fetchErr  error
	created   []Event
	updated   []Event
	removed   []string
}

func (f *fakeClient) fetch(from, to time.Time) ([]Calendar, []Event, error) {
	if f.fetchErr != nil {
		return nil, nil, f.fetchErr
	}

	return f.calendars, f.events, nil
}

type fakeWritableClient struct{ fakeClient }

func (f *fakeWritableClient) create(event Event) (Event, error) {
	event.ID = fmt.Sprintf("created-%d", len(f.created)+1)
	f.created = append(f.created, event)
	f.events = append(f.events, event)

	return event, nil
}

func (f *fakeWritableClient) update(event Event) error {
	f.updated = append(f.updated, event)

	return nil
}

func (f *fakeWritableClient) remove(id string) error {
	f.removed = append(f.removed, id)

	return nil
}

func testRemote(t *testing.T, accounts ...*remoteAccount) *Remote {
	t.Helper()

	return &Remote{
		location: time.UTC,
		cacheDir: t.TempDir(),
		from:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		to:       time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		clock:    func() time.Time { return time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC) },
		accounts: accounts,
	}
}

func TestRemoteSyncDecoratesAndCaches(t *testing.T) {
	losAngeles, err := time.LoadLocation("America/Los_Angeles")

	if err != nil {
		t.Fatal(err)
	}

	client := &fakeClient{
		calendars: []Calendar{{Name: "Family"}},
		events: []Event{{
			ID:       "uid-1",
			Title:    "Dinner",
			Calendar: "Family",
			Start:    time.Date(2026, 6, 1, 18, 0, 0, 0, losAngeles),
			End:      time.Date(2026, 6, 1, 19, 0, 0, 0, losAngeles),
		}},
	}

	account := &remoteAccount{name: "home", client: client}

	remote := testRemote(t, account)

	if err := remote.Sync("home", SyncAutomatic); err != nil {
		t.Fatal(err)
	}

	events := remote.Events(remote.from, remote.to)
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}

	if events[0].ID != "home:uid-1" {
		t.Errorf("want account-prefixed id, got %q", events[0].ID)
	}

	if events[0].Color == "" {
		t.Error("want a palette color assigned")
	}

	if events[0].Start.Location() != time.UTC {
		t.Errorf("want event time in the configured location, got %v", events[0].Start.Location())
	}

	info, err := os.Stat(remote.cachePath("home"))

	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Errorf("want cache file mode 0600, got %o", info.Mode().Perm())
	}

	restored := &remoteAccount{name: "home", client: client}
	restoredRemote := testRemote(t, restored)
	restoredRemote.cacheDir = remote.cacheDir
	restoredRemote.loadCache(restored)

	restoredEvents := restoredRemote.Events(remote.from, remote.to)
	if len(restoredEvents) != 1 || restoredEvents[0].ID != "home:uid-1" {
		t.Fatalf("want cached event home:uid-1, got %+v", restoredEvents)
	}
}

func TestRemoteWrites(t *testing.T) {
	readOnly := &remoteAccount{
		name:   "google",
		client: &fakeClient{calendars: []Calendar{{Name: "Google"}}},
	}
	readOnly.calendars = []Calendar{{Name: "Google"}}

	writable := &fakeWritableClient{fakeClient{calendars: []Calendar{{Name: "Work"}}}}

	writableAccount := &remoteAccount{name: "fastmail", client: writable}
	writableAccount.calendars = []Calendar{{Name: "Work"}}

	remote := testRemote(t, readOnly, writableAccount)

	cases := []struct {
		name    string
		action  func() error
		wantErr string
	}{
		{
			name: "add to a read-only subscription is refused",
			action: func() error {
				_, err := remote.Add(Event{Calendar: "Google"})

				return err
			},
			wantErr: "read-only subscription",
		},
		{
			name: "add to an unknown calendar is refused",
			action: func() error {
				_, err := remote.Add(Event{Calendar: "Missing"})

				return err
			},
			wantErr: "no account has a calendar",
		},
		{
			name: "update of an unprefixed event is refused",
			action: func() error {
				return remote.Update(Event{ID: "mock-1", Calendar: "Work"})
			},
			wantErr: "does not belong to any account",
		},
		{
			name: "delete on a read-only subscription is refused",
			action: func() error {
				return remote.Delete("google:uid-9")
			},
			wantErr: "read-only subscription",
		},
		{
			name: "add routes to the owning account",
			action: func() error {
				_, err := remote.Add(Event{Calendar: "Work", Title: "Planning"})

				return err
			},
		},
		{
			name: "delete routes to the owning account",
			action: func() error {
				return remote.Delete("fastmail:uid-3")
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.action()

			if c.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}

				return
			}

			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("want error containing %q, got %v", c.wantErr, err)
			}
		})
	}

	if len(writable.created) != 1 || writable.created[0].Title != "Planning" {
		t.Errorf("want one created event, got %+v", writable.created)
	}

	if len(writable.removed) != 1 || writable.removed[0] != "uid-3" {
		t.Errorf("want stripped id uid-3 removed, got %v", writable.removed)
	}

	writableCalendars := remote.WritableCalendars()
	if len(writableCalendars) != 1 || writableCalendars[0].Name != "Work" {
		t.Errorf("want only Work writable, got %+v", writableCalendars)
	}
}

func TestNewRemoteCaldavIdentityFromCredentialsFile(t *testing.T) {
	cases := []struct {
		name            string
		credentialsFile string
		wantErr         string
	}{
		{
			name: "username and url come from the credentials file",
			credentialsFile: "[work]\n" +
				"secret = \"app-password\"\n" +
				"username = \"raj@example.com\"\n" +
				"url = \"https://dav.example.com\"\n",
		},
		{
			name:            "missing username everywhere is reported",
			credentialsFile: "[work]\nsecret = \"app-password\"\nurl = \"https://dav.example.com\"\n",
			wantErr:         "need a username",
		},
		{
			name:            "missing url everywhere is reported",
			credentialsFile: "[work]\nsecret = \"app-password\"\nusername = \"raj@example.com\"\n",
			wantErr:         "need a url",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credentials.toml")
			t.Setenv("CALTUI_CREDENTIALS", path)
			t.Setenv("CALTUI_CACHE", t.TempDir())

			if err := os.WriteFile(path, []byte(c.credentialsFile), 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := NewRemote(
				[]Account{{Name: "work", Type: "caldav"}},
				nil,
				time.UTC,
				time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC),
			)

			if c.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}

				return
			}

			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("want error containing %q, got %v", c.wantErr, err)
			}
		})
	}
}

func TestNewRemoteWithICSSubscription(t *testing.T) {
	body := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//test//test//EN",
		"X-WR-CALNAME:Team calendar",
		"BEGIN:VEVENT",
		"UID:one",
		"DTSTART:20260707T100000Z",
		"DTEND:20260707T110000Z",
		"SUMMARY:Offsite",
		"END:VEVENT",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer server.Close()

	t.Setenv("CALTUI_CACHE", t.TempDir())

	remote, err := NewRemote(
		[]Account{{Name: "team", Type: "ics", URL: server.URL}},
		nil,
		time.UTC,
		time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC),
	)

	if err != nil {
		t.Fatal(err)
	}

	if err := remote.Sync("team", SyncAutomatic); err != nil {
		t.Fatal(err)
	}

	events := remote.Events(remote.from, remote.to)
	if len(events) != 1 || events[0].Title != "Offsite" {
		t.Fatalf("want the Offsite event, got %+v", events)
	}

	calendars := remote.Calendars()
	if len(calendars) != 1 || calendars[0].Name != "Team calendar" {
		t.Fatalf("want calendar named from X-WR-CALNAME, got %+v", calendars)
	}

	if len(remote.WritableCalendars()) != 0 {
		t.Fatal("want no writable calendars for an ics subscription")
	}

	if _, err := remote.Add(Event{Calendar: "Team calendar"}); err == nil {
		t.Fatal("want add to an ics subscription to fail")
	}
}

func TestDecorateColorOverrides(t *testing.T) {
	cases := []struct {
		name      string
		overrides map[string]string
		wantColor string
	}{
		{
			name:      "no override falls back to the palette",
			overrides: nil,
			wantColor: paletteColor("icloud/General"),
		},
		{
			name:      "bare calendar name override",
			overrides: map[string]string{"General": "#112233"},
			wantColor: "#112233",
		},
		{
			name:      "account qualified override",
			overrides: map[string]string{"icloud/General": "#445566"},
			wantColor: "#445566",
		},
		{
			name:      "account qualified override beats bare name",
			overrides: map[string]string{"General": "#112233", "icloud/General": "#445566"},
			wantColor: "#445566",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			remote := &Remote{location: time.UTC, colorOverrides: c.overrides}

			calendars, events := remote.decorate(
				"icloud",
				[]Calendar{{Name: "General"}},
				[]Event{{ID: "one", Calendar: "General"}},
			)

			if calendars[0].Color != c.wantColor {
				t.Errorf("want calendar color %q, got %q", c.wantColor, calendars[0].Color)
			}

			if events[0].Color != c.wantColor {
				t.Errorf("want event color %q, got %q", c.wantColor, events[0].Color)
			}
		})
	}
}

func TestSyncRollsTheWindowForward(t *testing.T) {
	client := &fakeClient{calendars: []Calendar{{Name: "Work"}}}

	account := &remoteAccount{name: "work", client: client}

	remote := testRemote(t, account)
	remote.clock = func() time.Time { return time.Date(2027, 3, 1, 12, 0, 0, 0, time.UTC) }

	if err := remote.Sync("work", SyncAutomatic); err != nil {
		t.Fatal(err)
	}

	wantFrom := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	wantTo := time.Date(2028, 3, 1, 12, 0, 0, 0, time.UTC)

	if !remote.from.Equal(wantFrom) || !remote.to.Equal(wantTo) {
		t.Errorf("want window %v to %v, got %v to %v", wantFrom, wantTo, remote.from, remote.to)
	}
}

func TestSyncEmptyResultGuard(t *testing.T) {
	offsite := Event{
		ID:       "uid-1",
		Title:    "Offsite",
		Calendar: "Work",
		Start:    time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC),
	}

	client := &fakeClient{
		calendars: []Calendar{{Name: "Work"}},
		events:    []Event{offsite},
	}

	account := &remoteAccount{name: "work", client: client}

	remote := testRemote(t, account)

	if err := remote.Sync("work", SyncAutomatic); err != nil {
		t.Fatal(err)
	}

	client.events = nil

	err := remote.Sync("work", SyncAutomatic)

	if err == nil || !strings.Contains(err.Error(), "keeping the cached copy") {
		t.Fatalf("want a kept-cache error on the first empty sync, got %v", err)
	}

	if err := remote.Sync("work", SyncAutomatic); err == nil {
		t.Fatal("want automatic refreshes to never accept the empty result")
	}

	if events := remote.Events(remote.from, remote.to); len(events) != 1 {
		t.Fatalf("want the cached event kept after suspect empty syncs, got %+v", events)
	}

	cached, readErr := os.ReadFile(remote.cachePath("work"))

	if readErr != nil || !strings.Contains(string(cached), "Offsite") {
		t.Fatalf("want the on-disk cache untouched, got %q (%v)", cached, readErr)
	}

	if err := remote.Sync("work", SyncManual); err != nil {
		t.Fatal(err)
	}

	if events := remote.Events(remote.from, remote.to); len(events) != 0 {
		t.Fatalf("want the manually confirmed empty result accepted, got %+v", events)
	}

	client.events = []Event{offsite}

	if err := remote.Sync("work", SyncAutomatic); err != nil {
		t.Fatal(err)
	}

	client.events = nil

	if err := remote.Sync("work", SyncManual); err == nil {
		t.Fatal("want the guard re-armed after a non-empty sync")
	}
}

func TestSyncEmptyResultGuardExceptions(t *testing.T) {
	offsite := Event{
		ID:       "uid-1",
		Title:    "Offsite",
		Calendar: "Work",
		Start:    time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC),
	}

	client := &fakeClient{
		calendars: []Calendar{{Name: "Work"}},
		events:    []Event{offsite},
	}

	account := &remoteAccount{name: "work", client: client}

	remote := testRemote(t, account)

	if err := remote.Sync("work", SyncAutomatic); err != nil {
		t.Fatal(err)
	}

	client.events = nil

	if err := remote.Sync("work", syncPostWrite); err != nil {
		t.Fatalf("want a post-write sync to accept the empty result at once, got %v", err)
	}

	agedOut := offsite
	agedOut.Start = time.Date(2020, 6, 1, 10, 0, 0, 0, time.UTC)
	agedOut.End = time.Date(2020, 6, 1, 11, 0, 0, 0, time.UTC)

	client.events = []Event{agedOut}

	if err := remote.Sync("work", SyncAutomatic); err != nil {
		t.Fatal(err)
	}

	client.events = nil

	if err := remote.Sync("work", SyncAutomatic); err != nil {
		t.Fatalf("want an empty fetch to pass when cached events sit outside the window, got %v", err)
	}
}
