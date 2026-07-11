package calendar

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
)

type caldavObject struct {
	path         string
	uid          string
	calendarName string
	recurring    bool
	etag         string
	data         *ical.Calendar
}

type caldavClient struct {
	client        *caldav.Client
	httpClient    webdav.HTTPClient
	endpoint      *url.URL
	wellKnownURL  string
	location      *time.Location
	calendarPaths map[string]string
	objects       map[string]caldavObject
}

func newCaldavClient(account Account, password string, location *time.Location) (*caldavClient, error) {
	httpClient := webdav.HTTPClientWithBasicAuth(
		&http.Client{Timeout: 30 * time.Second},
		account.Username,
		password,
	)

	client, err := caldav.NewClient(httpClient, account.URL)

	if err != nil {
		return nil, fmt.Errorf("account %q: %w", account.Name, err)
	}

	endpoint, err := url.Parse(account.URL)

	if err != nil {
		return nil, fmt.Errorf("account %q: %w", account.Name, err)
	}

	return &caldavClient{
		client:        client,
		httpClient:    httpClient,
		endpoint:      endpoint,
		wellKnownURL:  endpoint.Scheme + "://" + endpoint.Host + "/.well-known/caldav",
		location:      location,
		calendarPaths: map[string]string{},
		objects:       map[string]caldavObject{},
	}, nil
}

func (c *caldavClient) discoverCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	discoverFrom := func(client *caldav.Client) ([]caldav.Calendar, error) {
		principal, err := client.FindCurrentUserPrincipal(ctx)

		if err != nil {
			return nil, fmt.Errorf("finding principal: %w", err)
		}

		homeSet, err := client.FindCalendarHomeSet(ctx, principal)

		if err != nil {
			return nil, fmt.Errorf("finding calendar home: %w", err)
		}

		calendars, err := client.FindCalendars(ctx, homeSet)

		if err != nil {
			return nil, fmt.Errorf("listing calendars: %w", err)
		}

		return calendars, nil
	}

	calendars, configuredErr := discoverFrom(c.client)
	if configuredErr == nil {
		return calendars, nil
	}

	if wellKnownClient, err := caldav.NewClient(c.httpClient, c.wellKnownURL); err == nil {
		if calendars, err := discoverFrom(wellKnownClient); err == nil {
			return calendars, nil
		}
	}

	calendars, err := c.client.FindCalendars(ctx, "")
	if err == nil && len(calendars) > 0 {
		return calendars, nil
	}

	return nil, fmt.Errorf(
		"%w (also tried %s and treating the url as a calendar itself; try the exact CalDAV server address from your provider's calendar settings)",
		configuredErr, c.wellKnownURL,
	)
}

func caldavContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

func (c *caldavClient) fetch(from, to time.Time) ([]Calendar, []Event, error) {
	ctx, cancel := caldavContext()
	defer cancel()

	serverCalendars, err := c.discoverCalendars(ctx)

	if err != nil {
		return nil, nil, err
	}

	calendarPaths := map[string]string{}
	objects := map[string]caldavObject{}

	var calendars []Calendar
	var events []Event

	for _, serverCalendar := range serverCalendars {
		supported := serverCalendar.SupportedComponentSet
		if len(supported) > 0 && !slices.Contains(supported, "VEVENT") {
			continue
		}

		name := serverCalendar.Name
		if name == "" {
			name = path.Base(serverCalendar.Path)
		}

		base := name
		for suffix := 2; calendarPaths[name] != ""; suffix++ {
			name = fmt.Sprintf("%s (%d)", base, suffix)
		}

		calendarPaths[name] = serverCalendar.Path
		calendars = append(calendars, Calendar{Name: name})

		calendarObjects, err := c.calendarObjects(ctx, serverCalendar.Path, from, to)

		if err != nil {
			return nil, nil, fmt.Errorf("calendar %q: %w", name, err)
		}

		for _, object := range calendarObjects {
			if object.Data == nil {
				continue
			}

			etag := object.ETag
			if etag != "" && !strings.HasPrefix(etag, `"`) && !strings.HasPrefix(etag, "W/") {
				etag = `"` + etag + `"`
			}

			for _, parsed := range eventsFromICal(object.Data, name, from, to, c.location) {
				objects[parsed.Event.ID] = caldavObject{
					path:         object.Path,
					uid:          parsed.UID,
					calendarName: name,
					recurring:    parsed.Event.Recurring,
					etag:         etag,
					data:         object.Data,
				}
				events = append(events, parsed.Event)
			}
		}
	}

	c.calendarPaths = calendarPaths
	c.objects = objects

	return calendars, events, nil
}

func (c *caldavClient) calendarObjects(ctx context.Context, calendarPath string, from, to time.Time) ([]caldav.CalendarObject, error) {
	objects, queryErr := c.queryObjects(ctx, calendarPath, from, to)
	if queryErr == nil {
		return objects, nil
	}

	objectPaths, err := c.listObjectPaths(ctx, calendarPath)

	if err != nil {
		return nil, fmt.Errorf("querying: %w", queryErr)
	}

	if len(objectPaths) == 0 {
		return nil, nil
	}

	objects, err = c.client.MultiGetCalendar(ctx, calendarPath, &caldav.CalendarMultiGet{
		Paths:       objectPaths,
		CompRequest: caldav.CalendarCompRequest{Name: "VCALENDAR", AllProps: true, AllComps: true},
	})

	if err == nil {
		return objects, nil
	}

	objects = nil
	for _, objectPath := range objectPaths {
		data, etag, err := c.downloadObject(ctx, objectPath)

		if err != nil {
			return nil, fmt.Errorf("downloading %s: %w", path.Base(objectPath), err)
		}

		objects = append(objects, caldav.CalendarObject{Path: objectPath, ETag: etag, Data: data})
	}

	return objects, nil
}

// iCloud answers a calendar-data request that names components explicitly
// (<comp name="VCALENDAR"><allcomp/>…) with hollow BEGIN:VCALENDAR/END:VCALENDAR
// shells, so the query must request calendar-data bare.
func (c *caldavClient) queryObjects(ctx context.Context, calendarPath string, from, to time.Time) ([]caldav.CalendarObject, error) {
	timeRange := `<c:time-range start="` + from.UTC().Format("20060102T150405Z") +
		`" end="` + to.UTC().Format("20060102T150405Z") + `"/>`

	requestBody := `<?xml version="1.0" encoding="utf-8"?>` +
		`<c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">` +
		`<d:prop><d:getetag/><c:calendar-data/></d:prop>` +
		`<c:filter><c:comp-filter name="VCALENDAR"><c:comp-filter name="VEVENT">` +
		timeRange +
		`</c:comp-filter></c:comp-filter></c:filter></c:calendar-query>`

	request, err := http.NewRequestWithContext(ctx, "REPORT", c.objectURL(calendarPath), strings.NewReader(requestBody))

	if err != nil {
		return nil, err
	}

	request.Header.Set("Depth", "1")
	request.Header.Set("Content-Type", "text/xml; charset=utf-8")

	response, err := c.httpClient.Do(request)

	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("querying events: server said %s", response.Status)
	}

	var report struct {
		Responses []struct {
			Href     string `xml:"href"`
			Propstat []struct {
				Prop struct {
					CalendarData string `xml:"calendar-data"`
					ETag         string `xml:"getetag"`
				} `xml:"prop"`
			} `xml:"propstat"`
		} `xml:"response"`
	}

	data, err := io.ReadAll(response.Body)

	if err != nil {
		return nil, err
	}

	if err := xml.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}

	var objects []caldav.CalendarObject
	for _, entry := range report.Responses {
		parsedHref, err := url.Parse(strings.TrimSpace(entry.Href))

		if err != nil {
			continue
		}

		calendarData := ""
		etag := ""
		for _, propstat := range entry.Propstat {
			if propstat.Prop.CalendarData != "" {
				calendarData = propstat.Prop.CalendarData
			}

			if propstat.Prop.ETag != "" {
				etag = strings.TrimSpace(propstat.Prop.ETag)
			}
		}

		if calendarData == "" {
			continue
		}

		parsed, err := ical.NewDecoder(strings.NewReader(calendarData)).Decode()

		if err != nil || len(parsed.Children) == 0 {
			continue
		}

		objects = append(objects, caldav.CalendarObject{Path: parsedHref.Path, ETag: etag, Data: parsed})
	}

	if len(objects) == 0 && len(report.Responses) > 0 {
		return nil, fmt.Errorf("querying events: server returned no inline calendar data")
	}

	return objects, nil
}

func (c *caldavClient) objectURL(objectPath string) string {
	requestURL := *c.endpoint
	requestURL.Path = objectPath
	requestURL.RawQuery = ""

	return requestURL.String()
}

func (c *caldavClient) downloadObject(ctx context.Context, objectPath string) (*ical.Calendar, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.objectURL(objectPath), nil)

	if err != nil {
		return nil, "", err
	}

	request.Header.Set("Accept", ical.MIMEType)

	response, err := c.httpClient.Do(request)

	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("server said %s", response.Status)
	}

	data, err := ical.NewDecoder(response.Body).Decode()

	if err != nil {
		return nil, "", err
	}

	return data, response.Header.Get("ETag"), nil
}

// If-Match requires strong comparison, so weak W/ etags (Zoho) are sent
// unconditionally rather than always failing the precondition.
func (c *caldavClient) uploadObject(ctx context.Context, objectPath string, data *ical.Calendar, ifMatch string, refuseExisting bool) (string, string, error) {
	var body bytes.Buffer

	if err := ical.NewEncoder(&body).Encode(data); err != nil {
		return "", "", err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPut, c.objectURL(objectPath), bytes.NewReader(body.Bytes()))

	if err != nil {
		return "", "", err
	}

	request.Header.Set("Content-Type", "text/calendar; charset=utf-8")

	if ifMatch != "" && !strings.HasPrefix(ifMatch, "W/") {
		request.Header.Set("If-Match", ifMatch)
	}

	if refuseExisting {
		request.Header.Set("If-None-Match", "*")
	}

	response, err := c.httpClient.Do(request)

	if err != nil {
		return "", "", err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusPreconditionFailed {
		return "", "", fmt.Errorf("the event changed on the server: refresh and try again")
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return "", "", fmt.Errorf("server said %s", response.Status)
	}

	finalPath := objectPath
	if location := response.Header.Get("Location"); location != "" {
		parsed, err := url.Parse(location)

		if err == nil && parsed.Path != "" {
			finalPath = parsed.Path
		}
	}

	return finalPath, response.Header.Get("ETag"), nil
}

func (c *caldavClient) listObjectPaths(ctx context.Context, calendarPath string) ([]string, error) {
	requestURL := *c.endpoint
	requestURL.Path = calendarPath
	requestURL.RawQuery = ""

	requestBody := `<?xml version="1.0" encoding="utf-8"?>` +
		`<d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/><d:getcontenttype/></d:prop></d:propfind>`

	request, err := http.NewRequestWithContext(ctx, "PROPFIND", requestURL.String(), strings.NewReader(requestBody))

	if err != nil {
		return nil, err
	}

	request.Header.Set("Depth", "1")
	request.Header.Set("Content-Type", "text/xml; charset=utf-8")

	response, err := c.httpClient.Do(request)

	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("listing events: server said %s", response.Status)
	}

	var listing struct {
		Responses []struct {
			Href     string `xml:"href"`
			Propstat []struct {
				Prop struct {
					ResourceType struct {
						Collection *struct{} `xml:"collection"`
					} `xml:"resourcetype"`
					ContentType string `xml:"getcontenttype"`
				} `xml:"prop"`
			} `xml:"propstat"`
		} `xml:"response"`
	}

	data, err := io.ReadAll(response.Body)

	if err != nil {
		return nil, err
	}

	if err := xml.Unmarshal(data, &listing); err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}

	var objectPaths []string
	for _, entry := range listing.Responses {
		parsedHref, err := url.Parse(strings.TrimSpace(entry.Href))

		if err != nil || strings.TrimSuffix(parsedHref.Path, "/") == strings.TrimSuffix(calendarPath, "/") {
			continue
		}

		isCollection := false
		contentType := ""
		for _, propstat := range entry.Propstat {
			if propstat.Prop.ResourceType.Collection != nil {
				isCollection = true
			}

			if propstat.Prop.ContentType != "" {
				contentType = propstat.Prop.ContentType
			}
		}

		if isCollection || (contentType != "" && !strings.HasPrefix(contentType, ical.MIMEType)) {
			continue
		}

		objectPaths = append(objectPaths, parsedHref.Path)
	}

	return objectPaths, nil
}

func (c *caldavClient) create(event Event) (Event, error) {
	collectionPath, ok := c.calendarPaths[event.Calendar]
	if !ok {
		return Event{}, fmt.Errorf("unknown calendar %q", event.Calendar)
	}

	uidBytes := make([]byte, 16)

	_, err := rand.Read(uidBytes)

	if err != nil {
		return Event{}, fmt.Errorf("generating event id: %w", err)
	}

	uid := hex.EncodeToString(uidBytes)

	data := ical.NewCalendar()
	data.Props.SetText(ical.PropVersion, "2.0")
	data.Props.SetText(ical.PropProductID, "-//siliconwitchery//caltui//EN")

	icalEvent := ical.NewEvent()
	icalEvent.Props.SetText(ical.PropUID, uid)
	applyEventProps(icalEvent, event)
	data.Children = append(data.Children, icalEvent.Component)

	objectPath := strings.TrimSuffix(collectionPath, "/") + "/" + uid + ".ics"

	ctx, cancel := caldavContext()
	defer cancel()

	finalPath, etag, err := c.uploadObject(ctx, objectPath, data, "", true)

	if err != nil {
		return Event{}, fmt.Errorf("creating event: %w", err)
	}

	created := event
	created.ID = uid
	c.objects[uid] = caldavObject{
		path:         finalPath,
		uid:          uid,
		calendarName: event.Calendar,
		etag:         etag,
		data:         data,
	}

	return created, nil
}

func (c *caldavClient) update(event Event) error {
	object, ok := c.objects[event.ID]
	if !ok {
		return fmt.Errorf("event not found on the server: refresh and try again")
	}

	if object.recurring {
		return fmt.Errorf("recurring events are read-only for now")
	}

	if event.Calendar != object.calendarName {
		created, err := c.create(event)

		if err != nil {
			return err
		}

		if removeErr := c.remove(event.ID); removeErr != nil {
			if rollbackErr := c.remove(created.ID); rollbackErr != nil {
				return fmt.Errorf(
					"moving event: a copy now exists in %q but the original could not be removed: %w",
					event.Calendar, removeErr,
				)
			}

			return fmt.Errorf("moving event: %w", removeErr)
		}

		return nil
	}

	var target *ical.Component
	for _, child := range object.data.Children {
		if child.Name != ical.CompEvent {
			continue
		}

		uid, _ := child.Props.Text(ical.PropUID)
		if uid == object.uid {
			target = child

			break
		}
	}

	if target == nil {
		return fmt.Errorf("event not found on the server: refresh and try again")
	}

	applyEventProps(&ical.Event{Component: target}, event)

	ctx, cancel := caldavContext()
	defer cancel()

	finalPath, etag, err := c.uploadObject(ctx, object.path, object.data, object.etag, false)

	if err != nil {
		return fmt.Errorf("updating event: %w", err)
	}

	object.path = finalPath
	object.etag = etag
	c.objects[event.ID] = object

	return nil
}

func (c *caldavClient) remove(id string) error {
	object, ok := c.objects[id]
	if !ok {
		return fmt.Errorf("event not found on the server: refresh and try again")
	}

	if object.recurring {
		return fmt.Errorf("recurring events are read-only for now")
	}

	ctx, cancel := caldavContext()
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.objectURL(object.path), nil)

	if err != nil {
		return fmt.Errorf("deleting event: %w", err)
	}

	if object.etag != "" && !strings.HasPrefix(object.etag, "W/") {
		request.Header.Set("If-Match", object.etag)
	}

	response, err := c.httpClient.Do(request)

	if err != nil {
		return fmt.Errorf("deleting event: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusPreconditionFailed {
		return fmt.Errorf("the event changed on the server: refresh and try again")
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("deleting event: server said %s", response.Status)
	}

	delete(c.objects, id)

	return nil
}

func applyEventProps(icalEvent *ical.Event, event Event) {
	icalEvent.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	icalEvent.Props.SetText(ical.PropSummary, event.Title)

	if event.Location == "" {
		icalEvent.Props.Del(ical.PropLocation)
	} else {
		icalEvent.Props.SetText(ical.PropLocation, event.Location)
	}

	if event.Description == "" {
		icalEvent.Props.Del(ical.PropDescription)
	} else {
		icalEvent.Props.SetText(ical.PropDescription, event.Description)
	}

	icalEvent.Props.Del(ical.PropDuration)

	if event.AllDay {
		startProp := ical.NewProp(ical.PropDateTimeStart)
		startProp.SetDate(event.Start)
		icalEvent.Props.Set(startProp)

		endProp := ical.NewProp(ical.PropDateTimeEnd)
		endProp.SetDate(event.End)
		icalEvent.Props.Set(endProp)

		return
	}

	icalEvent.Props.SetDateTime(ical.PropDateTimeStart, event.Start.UTC())
	icalEvent.Props.SetDateTime(ical.PropDateTimeEnd, event.End.UTC())
}
