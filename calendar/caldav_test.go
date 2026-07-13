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

const (
	caldavTestICS = "BEGIN:VCALENDAR&#13;\nVERSION:2.0&#13;\nPRODID:-//test//test//EN&#13;\n" +
		"BEGIN:VEVENT&#13;\nUID:offsite-1&#13;\nDTSTART:20260707T100000Z&#13;\nDTEND:20260707T113000Z&#13;\n" +
		"SUMMARY:Offsite&#13;\nEND:VEVENT&#13;\nEND:VCALENDAR&#13;\n"

	caldavHollowICS = "BEGIN:VCALENDAR&#13;\nEND:VCALENDAR&#13;\n"

	caldavRecurringICS = "BEGIN:VCALENDAR&#13;\nVERSION:2.0&#13;\nPRODID:-//test//test//EN&#13;\n" +
		"BEGIN:VEVENT&#13;\nUID:standup-1&#13;\nDTSTART:20260707T090000Z&#13;\nDTEND:20260707T093000Z&#13;\n" +
		"SUMMARY:Standup&#13;\nRRULE:FREQ=DAILY&#13;\nEND:VEVENT&#13;\nEND:VCALENDAR&#13;\n"

	caldavLateICS = "BEGIN:VCALENDAR&#13;\nVERSION:2.0&#13;\nPRODID:-//test//test//EN&#13;\n" +
		"BEGIN:VEVENT&#13;\nUID:late-1&#13;\nDTSTART:20260707T233000Z&#13;\nDTEND:20260708T000000Z&#13;\n" +
		"SUMMARY:Late show&#13;\nRRULE:FREQ=DAILY&#13;\nEND:VEVENT&#13;\nEND:VCALENDAR&#13;\n"

	caldavBydayICS = "BEGIN:VCALENDAR&#13;\nVERSION:2.0&#13;\nPRODID:-//test//test//EN&#13;\n" +
		"BEGIN:VEVENT&#13;\nUID:byday-1&#13;\nDTSTART:20260706T090000Z&#13;\nDTEND:20260706T093000Z&#13;\n" +
		"SUMMARY:Standup&#13;\nRRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR&#13;\nEND:VEVENT&#13;\nEND:VCALENDAR&#13;\n"

	caldavVidaICS = "BEGIN:VCALENDAR&#13;\nVERSION:2.0&#13;\nPRODID:-//test//test//EN&#13;\n" +
		"BEGIN:VEVENT&#13;\nUID:vida-1&#13;\nDTSTART:20260709T180000Z&#13;\nDTEND:20260709T190000Z&#13;\n" +
		"SUMMARY:Vida class&#13;\nEND:VEVENT&#13;\nEND:VCALENDAR&#13;\n"
)

type caldavServerOptions struct {
	wellKnownWorks     bool
	queryMode          string
	putBodies          *[]string
	putHeaders         *[]http.Header
	deleteHeaders      *[]http.Header
	rejectWrites       bool
	rejectDeletes      string
	subscribedCalendar bool
	emptyListing       bool
}

func caldavTestServer(t *testing.T, options caldavServerOptions) *httptest.Server {
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
			if !options.wellKnownWorks {
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

			if options.subscribedCalendar {
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
			if options.emptyListing {
				multistatus(w, `<d:response><d:href>/cal/raj/work/</d:href><d:propstat><d:prop>`+
					`<d:resourcetype><d:collection/></d:resourcetype>`+
					`</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)

				return
			}

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
				if options.queryMode == "multiget" {
					multistatus(w, eventReport(requestPath+"offsite-1.ics", caldavTestICS))
				} else {
					w.WriteHeader(http.StatusNotImplemented)
				}

				return
			}

			switch {
			case options.queryMode == "icloud" && strings.Contains(string(requestBody), "allcomp"):
				multistatus(w, eventReport(requestPath+"offsite-1.ics", caldavHollowICS))

			case options.queryMode == "icloud", options.queryMode == "inline":
				multistatus(w, eventReport(requestPath+"offsite-1.ics", caldavTestICS))

			case options.queryMode == "recurring":
				multistatus(w, eventReport(requestPath+"standup-1.ics", caldavRecurringICS))

			case options.queryMode == "late":
				multistatus(w, eventReport(requestPath+"late-1.ics", caldavLateICS))

			case options.queryMode == "byday":
				multistatus(w, eventReport(requestPath+"byday-1.ics", caldavBydayICS))

			case options.queryMode == "hollow":
				multistatus(w, eventReport(requestPath+"offsite-1.ics", caldavHollowICS))

			case options.queryMode == "truncated":
				w.Header().Set("Content-Type", "text/xml; charset=utf-8")
				w.WriteHeader(http.StatusMultiStatus)
				fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`+
					`<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">`+
					eventReport(requestPath+"offsite-1.ics", caldavTestICS)+
					`<d:response><d:href>`)

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
				if options.putBodies != nil {
					*options.putBodies = append(*options.putBodies, string(putBody))
				}

				if options.putHeaders != nil {
					*options.putHeaders = append(*options.putHeaders, r.Header.Clone())
				}

				if options.rejectWrites {
					w.WriteHeader(http.StatusPreconditionFailed)

					return
				}

				w.Header().Set("ETag", `W/"weak-etag-like-zoho"`)
				w.WriteHeader(http.StatusCreated)

				return
			}

			if r.Method == http.MethodDelete && strings.HasPrefix(requestPath, "/cal/raj/work/") {
				if options.deleteHeaders != nil {
					*options.deleteHeaders = append(*options.deleteHeaders, r.Header.Clone())
				}

				rejected := options.rejectWrites ||
					options.rejectDeletes == "all" ||
					(options.rejectDeletes == "original" && strings.Contains(requestPath, "offsite-1"))

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

func fetchedCaldavClientAt(t *testing.T, serverURL string, location *time.Location, from, to time.Time) (*caldavClient, []Calendar, []Event) {
	t.Helper()

	account := Account{
		Name:     "work",
		Type:     "caldav",
		URL:      serverURL,
		Username: "raj@example.com",
	}

	client, err := newCaldavClient(account, "app-password", location)

	if err != nil {
		t.Fatal(err)
	}

	calendars, events, err := client.fetch(from, to)

	if err != nil {
		t.Fatal(err)
	}

	return client, calendars, events
}

func fetchedCaldavClient(t *testing.T, options caldavServerOptions, urlPath string, location *time.Location, from, to time.Time) (*caldavClient, []Calendar, []Event) {
	t.Helper()

	server := caldavTestServer(t, options)
	t.Cleanup(server.Close)

	return fetchedCaldavClientAt(t, server.URL+urlPath, location, from, to)
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

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, calendars, events := fetchedCaldavClient(t, testCase.options, testCase.urlPath, time.UTC,
				time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))

			if len(calendars) != 1 || calendars[0].Name != testCase.wantCalendar {
				t.Fatalf("want calendar %q, got %+v", testCase.wantCalendar, calendars)
			}

			if len(events) != 1 || events[0].Title != "Offsite" {
				t.Fatalf("want the Offsite event, got %+v", events)
			}
		})
	}
}

func TestCaldavPartialQueryResultsAreDiscarded(t *testing.T) {
	_, calendars, events := fetchedCaldavClient(t, caldavServerOptions{
		wellKnownWorks: true,
		queryMode:      "truncated",
		emptyListing:   true,
	}, "", time.UTC, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))

	if len(calendars) != 1 || calendars[0].Name != "Work" {
		t.Fatalf("want the Work calendar, got %+v", calendars)
	}

	if len(events) != 0 {
		t.Fatalf("want the failed query's partial results discarded, got %+v", events)
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

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.action()

			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("want error containing %q, got %v", testCase.wantErr, err)
			}
		})
	}
}

func TestCaldavCreateAgainstQuirkyServer(t *testing.T) {
	var putBodies []string

	client, _, _ := fetchedCaldavClient(t, caldavServerOptions{
		wellKnownWorks: true,
		queryMode:      "get",
		putBodies:      &putBodies,
	}, "", time.UTC, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))

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

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			icalEvent := ical.NewEvent()

			applyEventProps(icalEvent, testCase.event)

			startProp := icalEvent.Props.Get(ical.PropDateTimeStart)

			endProp := icalEvent.Props.Get(ical.PropDateTimeEnd)

			if startProp.ValueType() != testCase.wantValueType {
				t.Errorf("want start value type %v, got %v", testCase.wantValueType, startProp.ValueType())
			}

			if startProp.Value != testCase.wantStart {
				t.Errorf("want start %q, got %q", testCase.wantStart, startProp.Value)
			}

			if endProp.Value != testCase.wantEnd {
				t.Errorf("want end %q, got %q", testCase.wantEnd, endProp.Value)
			}

			title, err := icalEvent.Props.Text(ical.PropSummary)

			if err != nil || title != testCase.event.Title {
				t.Errorf("want summary %q, got %q (%v)", testCase.event.Title, title, err)
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

	cases := []struct {
		name            string
		queryMode       string
		rejectWrites    bool
		operation       func(client *caldavClient) error
		wantDelete      bool
		wantIfMatch     string
		wantIfNoneMatch string
		wantErr         string
	}{
		{
			name:        "update sends the strong etag as if-match",
			queryMode:   "inline",
			operation:   func(client *caldavClient) error { return client.update(fetchedEvent) },
			wantIfMatch: `"etag-1"`,
		},
		{
			name:      "weak etag from the download fallback stays unconditional",
			queryMode: "get",
			operation: func(client *caldavClient) error { return client.update(fetchedEvent) },
		},
		{
			name:      "create refuses to overwrite an existing object",
			queryMode: "inline",
			operation: func(client *caldavClient) error {
				_, err := client.create(Event{Title: "Planning", Calendar: "Work", Start: fetchedEvent.Start, End: fetchedEvent.End})

				return err
			},
			wantIfNoneMatch: "*",
		},
		{
			name:        "delete sends the strong etag as if-match",
			queryMode:   "inline",
			operation:   func(client *caldavClient) error { return client.remove("offsite-1") },
			wantDelete:  true,
			wantIfMatch: `"etag-1"`,
		},
		{
			name:         "precondition failure on update asks the user to refresh",
			queryMode:    "inline",
			rejectWrites: true,
			operation:    func(client *caldavClient) error { return client.update(fetchedEvent) },
			wantErr:      "refresh and try again",
		},
		{
			name:         "precondition failure on delete asks the user to refresh",
			queryMode:    "inline",
			rejectWrites: true,
			operation:    func(client *caldavClient) error { return client.remove("offsite-1") },
			wantErr:      "refresh and try again",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var putHeaders, deleteHeaders []http.Header

			client, _, _ := fetchedCaldavClient(t, caldavServerOptions{
				wellKnownWorks: true,
				queryMode:      testCase.queryMode,
				rejectWrites:   testCase.rejectWrites,
				putHeaders:     &putHeaders,
				deleteHeaders:  &deleteHeaders,
			}, "", time.UTC, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))

			err := testCase.operation(client)

			if testCase.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					t.Fatalf("want error containing %q, got %v", testCase.wantErr, err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			writeHeaders, otherHeaders := putHeaders, deleteHeaders

			if testCase.wantDelete {
				writeHeaders, otherHeaders = deleteHeaders, putHeaders
			}

			if len(otherHeaders) != 0 {
				t.Fatalf("want the write to use the expected method only, got %d puts and %d deletes", len(putHeaders), len(deleteHeaders))
			}

			if len(writeHeaders) != 1 {
				t.Fatalf("want one write request, got %d puts and %d deletes", len(putHeaders), len(deleteHeaders))
			}

			if writeHeaders[0].Get("If-Match") != testCase.wantIfMatch {
				t.Errorf("want If-Match %q, got %q", testCase.wantIfMatch, writeHeaders[0].Get("If-Match"))
			}

			if writeHeaders[0].Get("If-None-Match") != testCase.wantIfNoneMatch {
				t.Errorf("want If-None-Match %q, got %q", testCase.wantIfNoneMatch, writeHeaders[0].Get("If-None-Match"))
			}
		})
	}
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

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var deleteHeaders []http.Header

			client, _, _ := fetchedCaldavClient(t, caldavServerOptions{
				wellKnownWorks: true,
				queryMode:      "inline",
				deleteHeaders:  &deleteHeaders,
				rejectDeletes:  testCase.rejectMode,
			}, "", time.UTC, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))

			client.calendarPaths["Personal"] = "/cal/raj/work/"

			err := client.update(movedEvent)

			if testCase.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("want error containing %q, got %v", testCase.wantErr, err)
			}

			if len(deleteHeaders) != testCase.wantDeletes {
				t.Errorf("want %d delete requests, got %d", testCase.wantDeletes, len(deleteHeaders))
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

	recurringClientFor := func(t *testing.T, putBodies *[]string, deleteHeaders *[]http.Header) *caldavClient {
		t.Helper()

		client, _, events := fetchedCaldavClient(t, caldavServerOptions{
			wellKnownWorks: true,
			queryMode:      "recurring",
			putBodies:      putBodies,
			deleteHeaders:  deleteHeaders,
		}, "", time.UTC, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

		if len(events) < 10 {
			t.Fatalf("want expanded daily instances, got %d", len(events))
		}

		return client
	}

	t.Run("delete one occurrence writes an exdate", func(t *testing.T) {
		var putBodies []string

		client := recurringClientFor(t, &putBodies, nil)

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

		client := recurringClientFor(t, &putBodies, nil)

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

		client := recurringClientFor(t, &putBodies, nil)

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

		client := recurringClientFor(t, &putBodies, nil)

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

		client := recurringClientFor(t, nil, &deleteHeaders)

		if err := client.removeSeries(instanceID); err != nil {
			t.Fatal(err)
		}

		if len(deleteHeaders) != 1 {
			t.Fatalf("want one delete request, got %d", len(deleteHeaders))
		}
	})

	t.Run("plain update and remove still refuse recurring events", func(t *testing.T) {
		client := recurringClientFor(t, nil, nil)

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

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var putBodies []string

			client, _, _ := fetchedCaldavClient(t, caldavServerOptions{
				wellKnownWorks: true,
				queryMode:      "inline",
				putBodies:      &putBodies,
			}, "", time.UTC, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))

			if _, err := client.create(testCase.event); err != nil {
				t.Fatal(err)
			}

			if len(putBodies) != 1 || !strings.Contains(putBodies[0], testCase.wantRule) {
				t.Fatalf("want uploaded event to contain %q, got %q", testCase.wantRule, putBodies)
			}
		})
	}
}

func TestCaldavSubscribedCalendars(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	client, calendars, events := fetchedCaldavClient(t, caldavServerOptions{
		wellKnownWorks:     true,
		queryMode:          "inline",
		subscribedCalendar: true,
	}, "", time.UTC, from, to)

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

	_, err := client.create(Event{Title: "Blocked", Calendar: "Vida classes", Start: from, End: from.Add(time.Hour)})

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

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	client, calendars, events := fetchedCaldavClientAt(t, gateway.URL, time.UTC, from, to)

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

func TestCaldavSeriesEditAcrossZones(t *testing.T) {
	stockholm, err := time.LoadLocation("Europe/Stockholm")

	if err != nil {
		t.Fatal(err)
	}

	lateOccurrence := time.Date(2026, 7, 8, 23, 30, 0, 0, time.UTC)

	bydayOccurrence := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)

	standupOccurrence := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		queryMode    string
		event        Event
		wantContains []string
		wantMissing  []string
		wantErr      string
	}{
		{
			name:      "title edit keeps the series anchored when utc and local dates differ",
			queryMode: "late",
			event: Event{
				ID:         fmt.Sprintf("late-1@%d", lateOccurrence.Unix()),
				Title:      "Renamed late show",
				Calendar:   "Work",
				Start:      lateOccurrence.In(stockholm),
				End:        lateOccurrence.Add(30 * time.Minute).In(stockholm),
				Recurring:  true,
				Recurrence: Recurrence{Frequency: "daily", Interval: 1},
			},
			wantContains: []string{"DTSTART;TZID=Europe/Stockholm:20260708T013000", "RRULE:FREQ=DAILY"},
		},
		{
			name:      "rename keeps a byday rule the form cannot edit",
			queryMode: "byday",
			event: Event{
				ID:         fmt.Sprintf("byday-1@%d", bydayOccurrence.Unix()),
				Title:      "Renamed standup",
				Calendar:   "Work",
				Start:      bydayOccurrence.In(stockholm),
				End:        bydayOccurrence.Add(30 * time.Minute).In(stockholm),
				Recurring:  true,
				Recurrence: Recurrence{Frequency: "weekly", Interval: 1},
			},
			wantContains: []string{"BYDAY=MO,WE,FR"},
		},
		{
			name:      "rule edit on a byday rule is refused",
			queryMode: "byday",
			event: Event{
				ID:        fmt.Sprintf("byday-1@%d", bydayOccurrence.Unix()),
				Title:     "Renamed standup",
				Calendar:  "Work",
				Start:     bydayOccurrence.In(stockholm),
				End:       bydayOccurrence.Add(30 * time.Minute).In(stockholm),
				Recurring: true,
				Recurrence: Recurrence{
					Frequency: "weekly",
					Interval:  1,
					Until:     time.Date(2026, 9, 1, 0, 0, 0, 0, stockholm),
				},
			},
			wantErr: "cannot edit",
		},
		{
			name:      "turning repeat off deletes the rule",
			queryMode: "recurring",
			event: Event{
				ID:        fmt.Sprintf("standup-1@%d", standupOccurrence.Unix()),
				Title:     "Standup",
				Calendar:  "Work",
				Start:     standupOccurrence.In(stockholm),
				End:       standupOccurrence.Add(30 * time.Minute).In(stockholm),
				Recurring: true,
			},
			wantMissing: []string{"RRULE"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var putBodies []string

			client, _, _ := fetchedCaldavClient(t, caldavServerOptions{
				wellKnownWorks: true,
				queryMode:      testCase.queryMode,
				putBodies:      &putBodies,
			}, "", stockholm, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

			err := client.updateSeries(testCase.event)

			if testCase.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					t.Fatalf("want error containing %q, got %v", testCase.wantErr, err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if len(putBodies) != 1 {
				t.Fatalf("want one upload, got %d", len(putBodies))
			}

			for _, wanted := range testCase.wantContains {
				if !strings.Contains(putBodies[0], wanted) {
					t.Errorf("want uploaded master to contain %q, got %q", wanted, putBodies[0])
				}
			}

			for _, unwanted := range testCase.wantMissing {
				if strings.Contains(putBodies[0], unwanted) {
					t.Errorf("want uploaded master without %q, got %q", unwanted, putBodies[0])
				}
			}
		})
	}

	t.Run("rule edit on a byday rule is still refused after a rename on the same client", func(t *testing.T) {
		var putBodies []string

		client, _, _ := fetchedCaldavClient(t, caldavServerOptions{
			wellKnownWorks: true,
			queryMode:      "byday",
			putBodies:      &putBodies,
		}, "", stockholm, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

		renamed := Event{
			ID:         fmt.Sprintf("byday-1@%d", bydayOccurrence.Unix()),
			Title:      "Renamed standup",
			Calendar:   "Work",
			Start:      bydayOccurrence.In(stockholm),
			End:        bydayOccurrence.Add(30 * time.Minute).In(stockholm),
			Recurring:  true,
			Recurrence: Recurrence{Frequency: "weekly", Interval: 1},
		}

		if err := client.updateSeries(renamed); err != nil {
			t.Fatal(err)
		}

		if len(putBodies) != 1 || !strings.Contains(putBodies[0], "BYDAY=MO,WE,FR") {
			t.Fatalf("want the rename to keep the byday rule, got %q", putBodies)
		}

		bounded := renamed
		bounded.Recurrence.Until = time.Date(2026, 9, 1, 0, 0, 0, 0, stockholm)

		err := client.updateSeries(bounded)

		if err == nil || !strings.Contains(err.Error(), "cannot edit") {
			t.Fatalf("want the rule edit refused after the rename, got %v", err)
		}

		if len(putBodies) != 1 {
			t.Fatalf("want no upload from the refused edit, got %d", len(putBodies))
		}
	})
}

func TestCaldavCreateRecurringWithZone(t *testing.T) {
	stockholm, err := time.LoadLocation("Europe/Stockholm")

	if err != nil {
		t.Fatal(err)
	}

	var putBodies []string

	client, _, _ := fetchedCaldavClient(t, caldavServerOptions{
		wellKnownWorks: true,
		queryMode:      "inline",
		putBodies:      &putBodies,
	}, "", stockholm, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))

	weekly := Event{
		Title:      "Climbing",
		Calendar:   "Work",
		Start:      time.Date(2026, 7, 8, 18, 0, 0, 0, stockholm),
		End:        time.Date(2026, 7, 8, 20, 0, 0, 0, stockholm),
		Recurrence: Recurrence{Frequency: "weekly"},
	}

	if _, err := client.create(weekly); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(putBodies[0], "DTSTART;TZID=Europe/Stockholm:20260708T180000") {
		t.Fatalf("want a TZID anchored start for a repeating event, got %q", putBodies[0])
	}
}
