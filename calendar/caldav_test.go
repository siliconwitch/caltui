package calendar

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

const caldavTestICS = "BEGIN:VCALENDAR&#13;\nVERSION:2.0&#13;\nPRODID:-//test//test//EN&#13;\n" +
	"BEGIN:VEVENT&#13;\nUID:offsite-1&#13;\nDTSTART:20260707T100000Z&#13;\nDTEND:20260707T113000Z&#13;\n" +
	"SUMMARY:Offsite&#13;\nEND:VEVENT&#13;\nEND:VCALENDAR&#13;\n"

const caldavHollowICS = "BEGIN:VCALENDAR&#13;\nEND:VCALENDAR&#13;\n"

const caldavRecurringICS = "BEGIN:VCALENDAR&#13;\nVERSION:2.0&#13;\nPRODID:-//test//test//EN&#13;\n" +
	"BEGIN:VEVENT&#13;\nUID:standup-1&#13;\nDTSTART:20260707T090000Z&#13;\nDTEND:20260707T093000Z&#13;\n" +
	"SUMMARY:Standup&#13;\nRRULE:FREQ=DAILY&#13;\nEND:VEVENT&#13;\nEND:VCALENDAR&#13;\n"

type caldavServerOptions struct {
	wellKnownWorks     bool
	queryMode          string
	putBodies          *[]string
	putHeaders         *[]http.Header
	deleteHeaders      *[]http.Header
	rejectWrites       bool
	rejectDeletes      string
	subscribedCalendar bool
}

const caldavVidaICS = "BEGIN:VCALENDAR&#13;\nVERSION:2.0&#13;\nPRODID:-//test//test//EN&#13;\n" +
	"BEGIN:VEVENT&#13;\nUID:vida-1&#13;\nDTSTART:20260709T180000Z&#13;\nDTEND:20260709T190000Z&#13;\n" +
	"SUMMARY:Vida class&#13;\nEND:VEVENT&#13;\nEND:VCALENDAR&#13;\n"

func caldavTestServer(t *testing.T, opts caldavServerOptions) *httptest.Server {
	t.Helper()

	multistatus := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`+
			`<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">`+body+`</d:multistatus>`)
	}

	calendarResponse := func(href, name string) string {
		return `<d:response><d:href>` + href + `</d:href><d:propstat><d:prop>` +
			`<d:resourcetype><d:collection/><c:calendar/></d:resourcetype>` +
			`<d:displayname>` + name + `</d:displayname>` +
			`<c:supported-calendar-component-set><c:comp name="VEVENT"/></c:supported-calendar-component-set>` +
			`</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`
	}

	eventReport := func(href, data string) string {
		return `<d:response><d:href>` + href + `</d:href><d:propstat><d:prop>` +
			`<d:getetag>"etag-1"</d:getetag>` +
			`<c:calendar-data>` + data + `</c:calendar-data>` +
			`</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}

		requestPath := r.URL.Path
		if !strings.HasSuffix(requestPath, "/") {
			requestPath += "/"
		}

		switch r.Method + " " + requestPath {
		case "PROPFIND /.well-known/caldav/":
			if !opts.wellKnownWorks {
				w.WriteHeader(http.StatusNotImplemented)

				return
			}

			multistatus(w, `<d:response><d:href>/.well-known/caldav</d:href><d:propstat><d:prop>`+
				`<d:current-user-principal><d:href>/principals/raj/</d:href></d:current-user-principal>`+
				`</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)

		case "PROPFIND /principals/raj/":
			multistatus(w, `<d:response><d:href>/principals/raj/</d:href><d:propstat><d:prop>`+
				`<c:calendar-home-set><d:href>/cal/raj/</d:href></c:calendar-home-set>`+
				`</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)

		case "PROPFIND /cal/raj/":
			body := calendarResponse("/cal/raj/work/", "Work")

			if opts.subscribedCalendar {
				body += `<d:response><d:href>/cal/raj/vida/</d:href><d:propstat><d:prop>` +
					`<d:resourcetype><d:collection/><cs:subscribed xmlns:cs="http://calendarserver.org/ns/"/></d:resourcetype>` +
					`<d:displayname>Vida classes</d:displayname>` +
					`</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`
			}

			multistatus(w, body)

		case "REPORT /cal/raj/vida/":
			multistatus(w, eventReport("/cal/raj/vida/vida-1.ics", caldavVidaICS))

		case "PROPFIND /onlycal/":
			multistatus(w, calendarResponse("/onlycal/", "Direct"))

		case "PROPFIND /cal/raj/work/":
			multistatus(w, `<d:response><d:href>/cal/raj/work/</d:href><d:propstat><d:prop>`+
				`<d:resourcetype><d:collection/></d:resourcetype>`+
				`</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`+
				`<d:response><d:href>/cal/raj/work/offsite-1.ics</d:href><d:propstat><d:prop>`+
				`<d:resourcetype/>`+
				`<d:getcontenttype>text/calendar; charset=utf-8</d:getcontenttype>`+
				`<d:getetag>"etag-1"</d:getetag>`+
				`</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)

		case "REPORT /cal/raj/work/", "REPORT /onlycal/":
			requestBody, _ := io.ReadAll(r.Body)

			if strings.Contains(string(requestBody), "calendar-multiget") {
				if opts.queryMode == "multiget" {
					multistatus(w, eventReport(requestPath+"offsite-1.ics", caldavTestICS))
				} else {
					w.WriteHeader(http.StatusNotImplemented)
				}

				return
			}

			switch {
			case opts.queryMode == "icloud" && strings.Contains(string(requestBody), "allcomp"):
				multistatus(w, eventReport(requestPath+"offsite-1.ics", caldavHollowICS))

			case opts.queryMode == "icloud", opts.queryMode == "inline":
				multistatus(w, eventReport(requestPath+"offsite-1.ics", caldavTestICS))

			case opts.queryMode == "recurring":
				multistatus(w, eventReport(requestPath+"standup-1.ics", caldavRecurringICS))

			case opts.queryMode == "hollow":
				multistatus(w, eventReport(requestPath+"offsite-1.ics", caldavHollowICS))

			default:
				multistatus(w, `<d:response><d:href>`+requestPath+`offsite-1.ics</d:href>`+
					`<d:propstat><d:prop><d:getetag>"etag-1"</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>`+
					`<d:propstat><d:prop><c:calendar-data/></d:prop><d:status>HTTP/1.1 404 Not Found</d:status></d:propstat>`+
					`</d:response>`)
			}

		case "GET /cal/raj/work/offsite-1.ics/":
			w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
			w.Header().Set("ETag", `W/"weak-etag-like-zoho"`)
			fmt.Fprint(w, strings.ReplaceAll(caldavTestICS, "&#13;\n", "\r\n"))

		default:
			if r.Method == http.MethodPut && strings.HasPrefix(requestPath, "/cal/raj/work/") {
				putBody, _ := io.ReadAll(r.Body)
				if opts.putBodies != nil {
					*opts.putBodies = append(*opts.putBodies, string(putBody))
				}

				if opts.putHeaders != nil {
					*opts.putHeaders = append(*opts.putHeaders, r.Header.Clone())
				}

				if opts.rejectWrites {
					w.WriteHeader(http.StatusPreconditionFailed)

					return
				}

				w.Header().Set("ETag", `W/"weak-etag-like-zoho"`)
				w.WriteHeader(http.StatusCreated)

				return
			}

			if r.Method == http.MethodDelete && strings.HasPrefix(requestPath, "/cal/raj/work/") {
				if opts.deleteHeaders != nil {
					*opts.deleteHeaders = append(*opts.deleteHeaders, r.Header.Clone())
				}

				rejected := opts.rejectWrites ||
					opts.rejectDeletes == "all" ||
					(opts.rejectDeletes == "original" && strings.Contains(requestPath, "offsite-1"))

				if rejected {
					w.WriteHeader(http.StatusPreconditionFailed)

					return
				}

				w.WriteHeader(http.StatusNoContent)

				return
			}

			w.WriteHeader(http.StatusNotImplemented)
		}
	}))
}

func TestCaldavDiscoveryLadder(t *testing.T) {
	cases := []struct {
		name         string
		urlPath      string
		options      caldavServerOptions
		wantCalendar string
	}{
		{
			name:         "well-known fallback when the root rejects propfind",
			urlPath:      "",
			options:      caldavServerOptions{wellKnownWorks: true, queryMode: "inline"},
			wantCalendar: "Work",
		},
		{
			name:         "url pointing at a single calendar collection",
			urlPath:      "/onlycal/",
			options:      caldavServerOptions{queryMode: "inline"},
			wantCalendar: "Direct",
		},
		{
			name:         "query without inline data falls back to multiget",
			urlPath:      "",
			options:      caldavServerOptions{wellKnownWorks: true, queryMode: "multiget"},
			wantCalendar: "Work",
		},
		{
			name:         "query and multiget both unsupported falls back to downloads",
			urlPath:      "",
			options:      caldavServerOptions{wellKnownWorks: true, queryMode: "get"},
			wantCalendar: "Work",
		},
		{
			name:         "icloud serving hollow data for component-filtered requests",
			urlPath:      "",
			options:      caldavServerOptions{wellKnownWorks: true, queryMode: "icloud"},
			wantCalendar: "Work",
		},
		{
			name:         "hollow inline data falls back to downloads",
			urlPath:      "",
			options:      caldavServerOptions{wellKnownWorks: true, queryMode: "hollow"},
			wantCalendar: "Work",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := caldavTestServer(t, c.options)
			defer server.Close()

			account := Account{
				Name:     "zoho",
				Type:     "caldav",
				URL:      server.URL + c.urlPath,
				Username: "raj@example.com",
			}

			client, err := newCaldavClient(account, "app-password", time.UTC)

			if err != nil {
				t.Fatal(err)
			}

			from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

			to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

			calendars, events, err := client.fetch(from, to)

			if err != nil {
				t.Fatal(err)
			}

			if len(calendars) != 1 || calendars[0].Name != c.wantCalendar {
				t.Fatalf("want calendar %q, got %+v", c.wantCalendar, calendars)
			}

			if len(events) != 1 || events[0].Title != "Offsite" {
				t.Fatalf("want the Offsite event, got %+v", events)
			}
		})
	}
}

func TestCaldavGuards(t *testing.T) {
	client := &caldavClient{
		calendarPaths: map[string]string{"Work": "/calendars/raj/work/"},
		objects: map[string]caldavObject{
			"recurring-uid": {path: "/calendars/raj/work/recurring.ics", uid: "recurring-uid", calendarName: "Work", recurring: true},
		},
	}

	cases := []struct {
		name    string
		action  func() error
		wantErr string
	}{
		{
			name: "create into an unknown calendar",
			action: func() error {
				_, err := client.create(Event{Calendar: "Missing"})

				return err
			},
			wantErr: "unknown calendar",
		},
		{
			name:    "update of an unknown event",
			action:  func() error { return client.update(Event{ID: "missing-uid"}) },
			wantErr: "refresh and try again",
		},
		{
			name:    "update of a recurring event",
			action:  func() error { return client.update(Event{ID: "recurring-uid", Calendar: "Work"}) },
			wantErr: "read-only for now",
		},
		{
			name:    "delete of a recurring event",
			action:  func() error { return client.remove("recurring-uid") },
			wantErr: "read-only for now",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.action()

			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("want error containing %q, got %v", c.wantErr, err)
			}
		})
	}
}

func TestCaldavCreateAgainstQuirkyServer(t *testing.T) {
	var putBodies []string

	server := caldavTestServer(t, caldavServerOptions{
		wellKnownWorks: true,
		queryMode:      "get",
		putBodies:      &putBodies,
	})
	defer server.Close()

	account := Account{
		Name:     "zoho",
		Type:     "caldav",
		URL:      server.URL,
		Username: "raj@example.com",
	}

	client, err := newCaldavClient(account, "app-password", time.UTC)

	if err != nil {
		t.Fatal(err)
	}

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	if _, _, err := client.fetch(from, to); err != nil {
		t.Fatal(err)
	}

	created, err := client.create(Event{
		Title:    "Planning",
		Calendar: "Work",
		Start:    time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatal(err)
	}

	if created.ID == "" {
		t.Fatal("want a generated event id")
	}

	if len(putBodies) != 1 || !strings.Contains(putBodies[0], "SUMMARY:Planning") {
		t.Fatalf("want one uploaded event with SUMMARY:Planning, got %q", putBodies)
	}

	if err := client.update(created); err != nil {
		t.Fatal(err)
	}

	if len(putBodies) != 2 {
		t.Fatalf("want a second upload after update, got %d", len(putBodies))
	}
}

func TestApplyEventProps(t *testing.T) {
	cases := []struct {
		name          string
		event         Event
		wantValueType ical.ValueType
		wantStart     string
		wantEnd       string
	}{
		{
			name: "timed event is written in utc",
			event: Event{
				Title: "Standup",
				Start: time.Date(2026, 6, 1, 12, 0, 0, 0, time.FixedZone("CEST", 2*3600)),
				End:   time.Date(2026, 6, 1, 12, 30, 0, 0, time.FixedZone("CEST", 2*3600)),
			},
			wantValueType: ical.ValueDateTime,
			wantStart:     "20260601T100000Z",
			wantEnd:       "20260601T103000Z",
		},
		{
			name: "all day event is written as dates",
			event: Event{
				Title:  "Holiday",
				AllDay: true,
				Start:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
				End:    time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC),
			},
			wantValueType: ical.ValueDate,
			wantStart:     "20260601",
			wantEnd:       "20260603",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			icalEvent := ical.NewEvent()

			applyEventProps(icalEvent, c.event)

			startProp := icalEvent.Props.Get(ical.PropDateTimeStart)

			endProp := icalEvent.Props.Get(ical.PropDateTimeEnd)

			if startProp.ValueType() != c.wantValueType {
				t.Errorf("want start value type %v, got %v", c.wantValueType, startProp.ValueType())
			}

			if startProp.Value != c.wantStart {
				t.Errorf("want start %q, got %q", c.wantStart, startProp.Value)
			}

			if endProp.Value != c.wantEnd {
				t.Errorf("want end %q, got %q", c.wantEnd, endProp.Value)
			}

			title, err := icalEvent.Props.Text(ical.PropSummary)

			if err != nil || title != c.event.Title {
				t.Errorf("want summary %q, got %q (%v)", c.event.Title, title, err)
			}
		})
	}
}

func TestCaldavConditionalWrites(t *testing.T) {
	fetchedEvent := Event{
		ID:       "offsite-1",
		Title:    "Offsite",
		Calendar: "Work",
		Start:    time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 7, 7, 11, 30, 0, 0, time.UTC),
	}

	fetchAgainst := func(t *testing.T, opts caldavServerOptions) (*caldavClient, func()) {
		t.Helper()

		server := caldavTestServer(t, opts)

		account := Account{
			Name:     "work",
			Type:     "caldav",
			URL:      server.URL,
			Username: "raj@example.com",
		}

		client, err := newCaldavClient(account, "app-password", time.UTC)

		if err != nil {
			t.Fatal(err)
		}

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

		if _, _, err := client.fetch(from, to); err != nil {
			t.Fatal(err)
		}

		return client, server.Close
	}

	t.Run("update sends the strong etag as if-match", func(t *testing.T) {
		var putHeaders []http.Header

		client, close := fetchAgainst(t, caldavServerOptions{
			wellKnownWorks: true,
			queryMode:      "inline",
			putHeaders:     &putHeaders,
		})
		defer close()

		if err := client.update(fetchedEvent); err != nil {
			t.Fatal(err)
		}

		if len(putHeaders) != 1 || putHeaders[0].Get("If-Match") != `"etag-1"` {
			t.Fatalf("want If-Match %q, got %+v", `"etag-1"`, putHeaders)
		}

		if putHeaders[0].Get("If-None-Match") != "" {
			t.Fatal("want no If-None-Match on update")
		}
	})

	t.Run("weak etag from the download fallback stays unconditional", func(t *testing.T) {
		var putHeaders []http.Header

		client, close := fetchAgainst(t, caldavServerOptions{
			wellKnownWorks: true,
			queryMode:      "get",
			putHeaders:     &putHeaders,
		})
		defer close()

		if err := client.update(fetchedEvent); err != nil {
			t.Fatal(err)
		}

		if len(putHeaders) != 1 || putHeaders[0].Get("If-Match") != "" {
			t.Fatalf("want no If-Match for a weak etag, got %+v", putHeaders)
		}
	})

	t.Run("create refuses to overwrite an existing object", func(t *testing.T) {
		var putHeaders []http.Header

		client, close := fetchAgainst(t, caldavServerOptions{
			wellKnownWorks: true,
			queryMode:      "inline",
			putHeaders:     &putHeaders,
		})
		defer close()

		if _, err := client.create(Event{Title: "Planning", Calendar: "Work", Start: fetchedEvent.Start, End: fetchedEvent.End}); err != nil {
			t.Fatal(err)
		}

		if len(putHeaders) != 1 || putHeaders[0].Get("If-None-Match") != "*" {
			t.Fatalf("want If-None-Match *, got %+v", putHeaders)
		}

		if putHeaders[0].Get("If-Match") != "" {
			t.Fatal("want no If-Match on create")
		}
	})

	t.Run("delete sends the strong etag as if-match", func(t *testing.T) {
		var deleteHeaders []http.Header

		client, close := fetchAgainst(t, caldavServerOptions{
			wellKnownWorks: true,
			queryMode:      "inline",
			deleteHeaders:  &deleteHeaders,
		})
		defer close()

		if err := client.remove("offsite-1"); err != nil {
			t.Fatal(err)
		}

		if len(deleteHeaders) != 1 || deleteHeaders[0].Get("If-Match") != `"etag-1"` {
			t.Fatalf("want If-Match %q, got %+v", `"etag-1"`, deleteHeaders)
		}
	})

	t.Run("precondition failures ask the user to refresh", func(t *testing.T) {
		client, close := fetchAgainst(t, caldavServerOptions{
			wellKnownWorks: true,
			queryMode:      "inline",
			rejectWrites:   true,
		})
		defer close()

		err := client.update(fetchedEvent)

		if err == nil || !strings.Contains(err.Error(), "refresh and try again") {
			t.Fatalf("want a refresh-and-retry error on 412, got %v", err)
		}

		if err := client.remove("offsite-1"); err == nil || !strings.Contains(err.Error(), "refresh and try again") {
			t.Fatalf("want a refresh-and-retry error on delete 412, got %v", err)
		}
	})
}

func TestCaldavMoveRollsBackOnFailedDelete(t *testing.T) {
	movedEvent := Event{
		ID:       "offsite-1",
		Title:    "Offsite",
		Calendar: "Personal",
		Start:    time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 7, 7, 11, 30, 0, 0, time.UTC),
	}

	cases := []struct {
		name        string
		rejectMode  string
		wantErr     string
		wantDeletes int
	}{
		{
			name:        "clean move deletes the original once",
			rejectMode:  "",
			wantErr:     "",
			wantDeletes: 1,
		},
		{
			name:        "failed delete rolls the copy back",
			rejectMode:  "original",
			wantErr:     "refresh and try again",
			wantDeletes: 2,
		},
		{
			name:        "failed rollback names the surviving copy",
			rejectMode:  "all",
			wantErr:     "a copy now exists",
			wantDeletes: 2,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var deleteHeaders []http.Header

			server := caldavTestServer(t, caldavServerOptions{
				wellKnownWorks: true,
				queryMode:      "inline",
				deleteHeaders:  &deleteHeaders,
				rejectDeletes:  c.rejectMode,
			})
			defer server.Close()

			account := Account{
				Name:     "work",
				Type:     "caldav",
				URL:      server.URL,
				Username: "raj@example.com",
			}

			client, err := newCaldavClient(account, "app-password", time.UTC)

			if err != nil {
				t.Fatal(err)
			}

			from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

			to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

			if _, _, err := client.fetch(from, to); err != nil {
				t.Fatal(err)
			}

			client.calendarPaths["Personal"] = "/cal/raj/work/"

			err = client.update(movedEvent)

			if c.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("want error containing %q, got %v", c.wantErr, err)
			}

			if len(deleteHeaders) != c.wantDeletes {
				t.Errorf("want %d delete requests, got %d", c.wantDeletes, len(deleteHeaders))
			}
		})
	}
}

func TestCaldavRecurringWrites(t *testing.T) {
	julyEighth := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)

	instanceID := fmt.Sprintf("standup-1@%d", julyEighth.Unix())

	occurrenceEvent := Event{
		ID:         instanceID,
		Title:      "Standup",
		Calendar:   "Work",
		Start:      julyEighth,
		End:        julyEighth.Add(30 * time.Minute),
		Recurring:  true,
		Recurrence: Recurrence{Frequency: "daily", Interval: 1},
	}

	recurringClientFor := func(t *testing.T, putBodies *[]string, deleteHeaders *[]http.Header) (*caldavClient, func()) {
		t.Helper()

		server := caldavTestServer(t, caldavServerOptions{
			wellKnownWorks: true,
			queryMode:      "recurring",
			putBodies:      putBodies,
			deleteHeaders:  deleteHeaders,
		})

		account := Account{
			Name:     "work",
			Type:     "caldav",
			URL:      server.URL,
			Username: "raj@example.com",
		}

		client, err := newCaldavClient(account, "app-password", time.UTC)

		if err != nil {
			t.Fatal(err)
		}

		from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

		to := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

		_, events, err := client.fetch(from, to)

		if err != nil {
			t.Fatal(err)
		}

		if len(events) < 10 {
			t.Fatalf("want expanded daily instances, got %d", len(events))
		}

		return client, server.Close
	}

	t.Run("delete one occurrence writes an exdate", func(t *testing.T) {
		var putBodies []string

		client, close := recurringClientFor(t, &putBodies, nil)
		defer close()

		if err := client.removeOccurrence(instanceID); err != nil {
			t.Fatal(err)
		}

		if len(putBodies) != 1 || !strings.Contains(putBodies[0], "EXDATE:20260708T090000Z") {
			t.Fatalf("want an EXDATE for the occurrence, got %q", putBodies)
		}

		if _, ok := client.objects[instanceID]; ok {
			t.Fatal("want the occurrence dropped from the object map")
		}
	})

	t.Run("edit one occurrence appends an override", func(t *testing.T) {
		var putBodies []string

		client, close := recurringClientFor(t, &putBodies, nil)
		defer close()

		moved := occurrenceEvent
		moved.Title = "Moved standup"
		moved.Start = julyEighth.Add(2 * time.Hour)
		moved.End = moved.Start.Add(30 * time.Minute)

		if err := client.updateOccurrence(moved); err != nil {
			t.Fatal(err)
		}

		if len(putBodies) != 1 {
			t.Fatalf("want one upload, got %d", len(putBodies))
		}

		for _, wanted := range []string{"RECURRENCE-ID:20260708T090000Z", "SUMMARY:Moved standup", "DTSTART:20260708T110000Z", "RRULE:FREQ=DAILY"} {
			if !strings.Contains(putBodies[0], wanted) {
				t.Errorf("want uploaded object to contain %q, got %q", wanted, putBodies[0])
			}
		}

		if strings.Count(putBodies[0], "RRULE") != 1 {
			t.Errorf("want the override to carry no RRULE of its own, got %q", putBodies[0])
		}
	})

	t.Run("series edit keeps an untouched rule and master date", func(t *testing.T) {
		var putBodies []string

		client, close := recurringClientFor(t, &putBodies, nil)
		defer close()

		renamed := occurrenceEvent
		renamed.Title = "Renamed standup"

		if err := client.updateSeries(renamed); err != nil {
			t.Fatal(err)
		}

		for _, wanted := range []string{"SUMMARY:Renamed standup", "RRULE:FREQ=DAILY", "DTSTART:20260707T090000Z"} {
			if !strings.Contains(putBodies[0], wanted) {
				t.Errorf("want uploaded master to contain %q, got %q", wanted, putBodies[0])
			}
		}
	})

	t.Run("series time edit shifts the master but not its date", func(t *testing.T) {
		var putBodies []string

		client, close := recurringClientFor(t, &putBodies, nil)
		defer close()

		shifted := occurrenceEvent
		shifted.Start = julyEighth.Add(5 * time.Hour)
		shifted.End = shifted.Start.Add(time.Hour)

		if err := client.updateSeries(shifted); err != nil {
			t.Fatal(err)
		}

		for _, wanted := range []string{"DTSTART:20260707T140000Z", "DTEND:20260707T150000Z"} {
			if !strings.Contains(putBodies[0], wanted) {
				t.Errorf("want master shifted to the new time on its own date, got %q", putBodies[0])
			}
		}
	})

	t.Run("series delete removes the object", func(t *testing.T) {
		var deleteHeaders []http.Header

		client, close := recurringClientFor(t, nil, &deleteHeaders)
		defer close()

		if err := client.removeSeries(instanceID); err != nil {
			t.Fatal(err)
		}

		if len(deleteHeaders) != 1 {
			t.Fatalf("want one delete request, got %d", len(deleteHeaders))
		}
	})

	t.Run("plain update and remove still refuse recurring events", func(t *testing.T) {
		client, close := recurringClientFor(t, nil, nil)
		defer close()

		if err := client.update(occurrenceEvent); err == nil || !strings.Contains(err.Error(), "read-only") {
			t.Fatalf("want the plain update guard, got %v", err)
		}

		if err := client.remove(instanceID); err == nil || !strings.Contains(err.Error(), "read-only") {
			t.Fatalf("want the plain remove guard, got %v", err)
		}
	})
}

func TestCaldavCreateRecurringEvent(t *testing.T) {
	cases := []struct {
		name     string
		event    Event
		wantRule string
	}{
		{
			name: "timed weekly with until",
			event: Event{
				Title:    "Climbing",
				Calendar: "Work",
				Start:    time.Date(2026, 7, 8, 18, 0, 0, 0, time.UTC),
				End:      time.Date(2026, 7, 8, 20, 0, 0, 0, time.UTC),
				Recurrence: Recurrence{
					Frequency: "weekly",
					Interval:  2,
					Until:     time.Date(2026, 9, 1, 23, 59, 59, 0, time.UTC),
				},
			},
			wantRule: "RRULE:FREQ=WEEKLY;INTERVAL=2;UNTIL=20260901T235959Z",
		},
		{
			name: "all day yearly with date typed until",
			event: Event{
				Title:    "Anniversary",
				Calendar: "Work",
				AllDay:   true,
				Start:    time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
				End:      time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC),
				Recurrence: Recurrence{
					Frequency: "yearly",
					Until:     time.Date(2030, 7, 8, 0, 0, 0, 0, time.UTC),
				},
			},
			wantRule: "RRULE:FREQ=YEARLY;UNTIL=20300708",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var putBodies []string

			server := caldavTestServer(t, caldavServerOptions{
				wellKnownWorks: true,
				queryMode:      "inline",
				putBodies:      &putBodies,
			})
			defer server.Close()

			account := Account{
				Name:     "work",
				Type:     "caldav",
				URL:      server.URL,
				Username: "raj@example.com",
			}

			client, err := newCaldavClient(account, "app-password", time.UTC)

			if err != nil {
				t.Fatal(err)
			}

			from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

			to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

			if _, _, err := client.fetch(from, to); err != nil {
				t.Fatal(err)
			}

			if _, err := client.create(c.event); err != nil {
				t.Fatal(err)
			}

			if len(putBodies) != 1 || !strings.Contains(putBodies[0], c.wantRule) {
				t.Fatalf("want uploaded event to contain %q, got %q", c.wantRule, putBodies)
			}
		})
	}
}

func TestCaldavSubscribedCalendars(t *testing.T) {
	server := caldavTestServer(t, caldavServerOptions{
		wellKnownWorks:     true,
		queryMode:          "inline",
		subscribedCalendar: true,
	})
	defer server.Close()

	account := Account{
		Name:     "icloud",
		Type:     "caldav",
		URL:      server.URL,
		Username: "raj@example.com",
	}

	client, err := newCaldavClient(account, "app-password", time.UTC)

	if err != nil {
		t.Fatal(err)
	}

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	calendars, events, err := client.fetch(from, to)

	if err != nil {
		t.Fatal(err)
	}

	if len(calendars) != 2 {
		t.Fatalf("want Work and Vida classes, got %+v", calendars)
	}

	if calendars[1].Name != "Vida classes" || !calendars[1].ReadOnly {
		t.Fatalf("want Vida classes flagged read-only, got %+v", calendars[1])
	}

	foundVida := false
	for _, event := range events {
		if event.Title == "Vida class" {
			foundVida = true
		}
	}

	if !foundVida {
		t.Fatalf("want the subscribed calendar's events fetched, got %+v", events)
	}

	_, err = client.create(Event{Title: "Blocked", Calendar: "Vida classes", Start: from, End: from.Add(time.Hour)})

	if err == nil || !strings.Contains(err.Error(), "read-only subscription") {
		t.Fatalf("want creates into the subscription refused, got %v", err)
	}
}

func TestCaldavCrossHostDiscovery(t *testing.T) {
	var putBodies []string

	partition := caldavTestServer(t, caldavServerOptions{
		queryMode: "inline",
		putBodies: &putBodies,
	})
	defer partition.Close()

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			w.WriteHeader(http.StatusNotImplemented)

			return
		}

		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)

		switch r.URL.Path {
		case "/", "":
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<d:multistatus xmlns:d="DAV:"><d:response><d:href>/</d:href><d:propstat><d:prop>`+
				`<d:current-user-principal><d:href>/principals/raj/</d:href></d:current-user-principal>`+
				`</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`)

		default:
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">`+
				`<d:response><d:href>/principals/raj/</d:href><d:propstat><d:prop>`+
				`<c:calendar-home-set><d:href>`+partition.URL+`/cal/raj/</d:href></c:calendar-home-set>`+
				`</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`)
		}
	}))
	defer gateway.Close()

	account := Account{
		Name:     "icloud",
		Type:     "caldav",
		URL:      gateway.URL,
		Username: "raj@example.com",
	}

	client, err := newCaldavClient(account, "app-password", time.UTC)

	if err != nil {
		t.Fatal(err)
	}

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	calendars, events, err := client.fetch(from, to)

	if err != nil {
		t.Fatal(err)
	}

	if len(calendars) != 1 || calendars[0].Name != "Work" {
		t.Fatalf("want the partition host's Work calendar, got %+v", calendars)
	}

	if len(events) != 1 || events[0].Title != "Offsite" {
		t.Fatalf("want the partition host's events, got %+v", events)
	}

	if _, err := client.create(Event{Title: "Planning", Calendar: "Work", Start: from, End: from.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	if len(putBodies) != 1 || !strings.Contains(putBodies[0], "SUMMARY:Planning") {
		t.Fatalf("want the create PUT to land on the partition host, got %q", putBodies)
	}
}
