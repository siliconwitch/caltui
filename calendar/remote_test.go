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

	if err := remote.Sync("home"); err != nil {
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
		time.UTC,
		time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC),
	)

	if err != nil {
		t.Fatal(err)
	}

	if err := remote.Sync("team"); err != nil {
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
