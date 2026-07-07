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

type caldavServerOptions struct {
	wellKnownWorks bool
	queryMode      string
	putBodies      *[]string
}

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

	eventReport := func(href string) string {
		return `<d:response><d:href>` + href + `</d:href><d:propstat><d:prop>` +
			`<d:getetag>"etag-1"</d:getetag>` +
			`<c:calendar-data>` + caldavTestICS + `</c:calendar-data>` +
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
			multistatus(w, calendarResponse("/cal/raj/work/", "Work"))

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
					multistatus(w, eventReport(requestPath+"offsite-1.ics"))
				} else {
					w.WriteHeader(http.StatusNotImplemented)
				}

				return
			}

			if opts.queryMode == "inline" {
				multistatus(w, eventReport(requestPath+"offsite-1.ics"))

				return
			}

			multistatus(w, `<d:response><d:href>`+requestPath+`offsite-1.ics</d:href>`+
				`<d:propstat><d:prop><d:getetag>"etag-1"</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>`+
				`<d:propstat><d:prop><c:calendar-data/></d:prop><d:status>HTTP/1.1 404 Not Found</d:status></d:propstat>`+
				`</d:response>`)

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

				w.Header().Set("ETag", `W/"weak-etag-like-zoho"`)
				w.WriteHeader(http.StatusCreated)

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
