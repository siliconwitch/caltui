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
	path           string
	uid            string
	calendarName   string
	recurring      bool
	occurrenceTime time.Time
	etag           string
	data           *ical.Calendar
}

type caldavClient struct {
	client            *caldav.Client
	httpClient        webdav.HTTPClient
	configuredURL     *url.URL
	endpoint          *url.URL
	wellKnownURL      string
	location          *time.Location
	selfEmail         string
	calendarPaths     map[string]string
	readOnlyCalendars map[string]bool
	objects           map[string]caldavObject
}

type discoveredCalendar struct {
	path       string
	name       string
	components []string
	readOnly   bool
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

	configuredURL, err := url.Parse(account.URL)

	if err != nil {
		return nil, fmt.Errorf("account %q: %w", account.Name, err)
	}

	selfEmail := account.Email
	if selfEmail == "" && strings.Contains(account.Username, "@") {
		selfEmail = account.Username
	}

	return &caldavClient{
		client:            client,
		httpClient:        httpClient,
		configuredURL:     configuredURL,
		selfEmail:         selfEmail,
		endpoint:          configuredURL,
		wellKnownURL:      configuredURL.Scheme + "://" + configuredURL.Host + "/.well-known/caldav",
		location:          location,
		calendarPaths:     map[string]string{},
		readOnlyCalendars: map[string]bool{},
		objects:           map[string]caldavObject{},
	}, nil
}

func (c *caldavClient) propfind(ctx context.Context, requestURL *url.URL, depth, requestBody string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, "PROPFIND", requestURL.String(), strings.NewReader(requestBody))

	if err != nil {
		return nil, err
	}

	request.Header.Set("Depth", depth)
	request.Header.Set("Content-Type", "text/xml; charset=utf-8")

	response, err := c.httpClient.Do(request)

	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("server said %s", response.Status)
	}

	return io.ReadAll(response.Body)
}

func (c *caldavClient) findPrincipalURL(ctx context.Context, baseURL *url.URL) (*url.URL, error) {
	requestBody := `<?xml version="1.0" encoding="utf-8"?>` +
		`<d:propfind xmlns:d="DAV:"><d:prop><d:current-user-principal/></d:prop></d:propfind>`

	data, err := c.propfind(ctx, baseURL, "0", requestBody)

	if err != nil {
		return nil, fmt.Errorf("finding principal: %w", err)
	}

	var report struct {
		Responses []struct {
			Propstat []struct {
				Prop struct {
					Principal struct {
						Href string `xml:"href"`
					} `xml:"current-user-principal"`
				} `xml:"prop"`
			} `xml:"propstat"`
		} `xml:"response"`
	}

	if err := xml.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("finding principal: %w", err)
	}

	for _, entry := range report.Responses {
		for _, propstat := range entry.Propstat {
			href := strings.TrimSpace(propstat.Prop.Principal.Href)
			if href == "" {
				continue
			}

			principalURL, err := url.Parse(href)

			if err != nil {
				continue
			}

			return baseURL.ResolveReference(principalURL), nil
		}
	}

	return nil, fmt.Errorf("finding principal: the server returned none")
}

func (c *caldavClient) findCalendarHomeURL(ctx context.Context, principalURL *url.URL) (*url.URL, error) {
	requestBody := `<?xml version="1.0" encoding="utf-8"?>` +
		`<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">` +
		`<d:prop><c:calendar-home-set/></d:prop></d:propfind>`

	data, err := c.propfind(ctx, principalURL, "0", requestBody)

	if err != nil {
		return nil, fmt.Errorf("finding calendar home: %w", err)
	}

	var report struct {
		Responses []struct {
			Propstat []struct {
				Prop struct {
					HomeSet struct {
						Href string `xml:"href"`
					} `xml:"calendar-home-set"`
				} `xml:"prop"`
			} `xml:"propstat"`
		} `xml:"response"`
	}

	if err := xml.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("finding calendar home: %w", err)
	}

	for _, entry := range report.Responses {
		for _, propstat := range entry.Propstat {
			href := strings.TrimSpace(propstat.Prop.HomeSet.Href)
			if href == "" {
				continue
			}

			homeURL, err := url.Parse(href)

			if err != nil {
				continue
			}

			return principalURL.ResolveReference(homeURL), nil
		}
	}

	return nil, fmt.Errorf("finding calendar home: the server returned none")
}

func (c *caldavClient) listCalendars(ctx context.Context, homeURL *url.URL) ([]discoveredCalendar, error) {
	requestBody := `<?xml version="1.0" encoding="utf-8"?>` +
		`<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">` +
		`<d:prop><d:resourcetype/><d:displayname/><c:supported-calendar-component-set/></d:prop></d:propfind>`

	data, err := c.propfind(ctx, homeURL, "1", requestBody)

	if err != nil {
		return nil, fmt.Errorf("listing calendars: %w", err)
	}

	// The namespaces distinguish real caldav calendars from calendarserver
	// subscriptions, which iCloud serves read-only.
	var report struct {
		Responses []struct {
			Href     string `xml:"href"`
			Propstat []struct {
				Prop struct {
					ResourceType struct {
						Calendar   *struct{} `xml:"urn:ietf:params:xml:ns:caldav calendar"`
						Subscribed *struct{} `xml:"http://calendarserver.org/ns/ subscribed"`
					} `xml:"resourcetype"`
					DisplayName  string `xml:"displayname"`
					ComponentSet struct {
						Components []struct {
							Name string `xml:"name,attr"`
						} `xml:"comp"`
					} `xml:"supported-calendar-component-set"`
				} `xml:"prop"`
			} `xml:"propstat"`
		} `xml:"response"`
	}

	if err := xml.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("listing calendars: %w", err)
	}

	var discovered []discoveredCalendar
	for _, entry := range report.Responses {
		parsedHref, err := url.Parse(strings.TrimSpace(entry.Href))

		if err != nil {
			continue
		}

		isCalendar := false
		isSubscribed := false
		name := ""

		var components []string

		for _, propstat := range entry.Propstat {
			if propstat.Prop.ResourceType.Calendar != nil {
				isCalendar = true
			}

			if propstat.Prop.ResourceType.Subscribed != nil {
				isSubscribed = true
			}

			if propstat.Prop.DisplayName != "" {
				name = propstat.Prop.DisplayName
			}

			for _, component := range propstat.Prop.ComponentSet.Components {
				components = append(components, component.Name)
			}
		}

		if !isCalendar && !isSubscribed {
			continue
		}

		discovered = append(discovered, discoveredCalendar{
			path:       parsedHref.Path,
			name:       name,
			components: components,
			readOnly:   isSubscribed && !isCalendar,
		})
	}

	return discovered, nil
}

func (c *caldavClient) rerooted(targetURL *url.URL) error {
	if targetURL.Scheme == c.endpoint.Scheme && targetURL.Host == c.endpoint.Host {
		return nil
	}

	rebasedClient, err := caldav.NewClient(c.httpClient, targetURL.Scheme+"://"+targetURL.Host)

	if err != nil {
		return err
	}

	c.client = rebasedClient
	c.endpoint = &url.URL{Scheme: targetURL.Scheme, Host: targetURL.Host, User: targetURL.User}

	return nil
}

// The endpoint re-root is committed only after a discovery step succeeds, so
// a failed sync never leaves object paths pointing at the wrong host.
func (c *caldavClient) discoverCalendars(ctx context.Context) ([]discoveredCalendar, error) {
	discoverFrom := func(baseURL *url.URL) ([]discoveredCalendar, *url.URL, error) {
		principalURL, err := c.findPrincipalURL(ctx, baseURL)

		if err != nil {
			return nil, nil, err
		}

		homeURL, err := c.findCalendarHomeURL(ctx, principalURL)

		if err != nil {
			return nil, nil, err
		}

		calendars, err := c.listCalendars(ctx, homeURL)

		if err != nil {
			return nil, nil, err
		}

		return calendars, homeURL, nil
	}

	calendars, homeURL, configuredErr := discoverFrom(c.configuredURL)
	if configuredErr == nil {
		return calendars, c.rerooted(homeURL)
	}

	if wellKnownURL, err := url.Parse(c.wellKnownURL); err == nil {
		if calendars, homeURL, err := discoverFrom(wellKnownURL); err == nil {
			return calendars, c.rerooted(homeURL)
		}
	}

	if calendars, err := c.listCalendars(ctx, c.configuredURL); err == nil && len(calendars) > 0 {
		return calendars, c.rerooted(c.configuredURL)
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
	readOnlyCalendars := map[string]bool{}
	objects := map[string]caldavObject{}

	var calendars []Calendar
	var events []Event

	for _, serverCalendar := range serverCalendars {
		if len(serverCalendar.components) > 0 && !slices.Contains(serverCalendar.components, "VEVENT") {
			continue
		}

		name := serverCalendar.name
		if name == "" {
			name = path.Base(serverCalendar.path)
		}

		base := name
		for suffix := 2; calendarPaths[name] != ""; suffix++ {
			name = fmt.Sprintf("%s (%d)", base, suffix)
		}

		calendarPaths[name] = serverCalendar.path
		readOnlyCalendars[name] = serverCalendar.readOnly
		calendars = append(calendars, Calendar{Name: name, ReadOnly: serverCalendar.readOnly})

		calendarObjects, err := c.calendarObjects(ctx, serverCalendar.path, from, to)

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

			for _, parsed := range eventsFromICal(object.Data, name, c.selfEmail, from, to, c.location) {
				objects[parsed.Event.ID] = caldavObject{
					path:           object.Path,
					uid:            parsed.UID,
					calendarName:   name,
					recurring:      parsed.Event.Recurring,
					occurrenceTime: parsed.OccurrenceTime,
					etag:           etag,
					data:           object.Data,
				}
				events = append(events, parsed.Event)
			}
		}
	}

	c.calendarPaths = calendarPaths
	c.readOnlyCalendars = readOnlyCalendars
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
	// The encoder requires DTSTAMP on every VEVENT, but some servers omit it
	// on events other clients created.
	for _, child := range data.Children {
		if child.Name == ical.CompEvent && child.Props.Get(ical.PropDateTimeStamp) == nil {
			child.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
		}
	}

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

	if err := c.refuseReadOnly(event.Calendar); err != nil {
		return Event{}, err
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

	if err := c.refuseReadOnly(object.calendarName); err != nil {
		return err
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

	target := masterChild(object.data, object.uid)
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

func (c *caldavClient) refuseReadOnly(calendarName string) error {
	if c.readOnlyCalendars[calendarName] {
		return fmt.Errorf("calendar %q is a read-only subscription", calendarName)
	}

	return nil
}

func ruleBeyondCaltui(rule string) bool {
	for _, part := range []string{
		"BYDAY=", "COUNT=", "BYMONTHDAY=", "BYSETPOS=", "BYWEEKNO=",
		"BYYEARDAY=", "BYMONTH=", "BYHOUR=", "BYMINUTE=", "WKST=",
		"FREQ=SECONDLY", "FREQ=MINUTELY", "FREQ=HOURLY",
	} {
		if strings.Contains(rule, part) {
			return true
		}
	}

	return false
}

func masterChild(data *ical.Calendar, uid string) *ical.Component {
	for _, child := range data.Children {
		if child.Name != ical.CompEvent {
			continue
		}

		childUID, _ := child.Props.Text(ical.PropUID)

		if childUID == uid && child.Props.Get(ical.PropRecurrenceID) == nil {
			return child
		}
	}

	return nil
}

func occurrenceProp(name string, master *ical.Component, occurrenceTime time.Time) *ical.Prop {
	prop := ical.NewProp(name)

	startProp := master.Props.Get(ical.PropDateTimeStart)

	if startProp != nil && startProp.ValueType() == ical.ValueDate {
		prop.SetDate(occurrenceTime)
	} else {
		prop.SetDateTime(occurrenceTime.UTC())
	}

	return prop
}

func (c *caldavClient) updateOccurrence(event Event) error {
	object, ok := c.objects[event.ID]
	if !ok {
		return fmt.Errorf("event not found on the server: refresh and try again")
	}

	if err := c.refuseReadOnly(object.calendarName); err != nil {
		return err
	}

	if event.Calendar != object.calendarName {
		return fmt.Errorf("moving a single occurrence to another calendar is not supported yet")
	}

	master := masterChild(object.data, object.uid)
	if master == nil {
		return fmt.Errorf("event not found on the server: refresh and try again")
	}

	var override *ical.Component
	for _, child := range object.data.Children {
		if child.Name != ical.CompEvent || child.Props.Get(ical.PropRecurrenceID) == nil {
			continue
		}

		childUID, _ := child.Props.Text(ical.PropUID)

		recurrenceTime, err := child.Props.DateTime(ical.PropRecurrenceID, c.location)

		if childUID == object.uid && err == nil && recurrenceTime.Unix() == object.occurrenceTime.Unix() {
			override = child

			break
		}
	}

	if override == nil {
		overrideEvent := ical.NewEvent()

		for name, props := range master.Props {
			switch name {
			case ical.PropRecurrenceRule, ical.PropExceptionDates, ical.PropRecurrenceDates, ical.PropRecurrenceID:
				continue
			}

			overrideEvent.Props[name] = append([]ical.Prop(nil), props...)
		}

		for _, child := range master.Children {
			if child.Name == ical.CompAlarm {
				overrideEvent.Children = append(overrideEvent.Children, child)
			}
		}

		overrideEvent.Props.Set(occurrenceProp(ical.PropRecurrenceID, master, object.occurrenceTime))

		object.data.Children = append(object.data.Children, overrideEvent.Component)
		override = overrideEvent.Component
	}

	applyEventProps(&ical.Event{Component: override}, event)

	ctx, cancel := caldavContext()
	defer cancel()

	finalPath, etag, err := c.uploadObject(ctx, object.path, object.data, object.etag, false)

	if err != nil {
		return fmt.Errorf("updating occurrence: %w", err)
	}

	object.path = finalPath
	object.etag = etag
	c.objects[event.ID] = object

	return nil
}

func (c *caldavClient) removeOccurrence(id string) error {
	object, ok := c.objects[id]
	if !ok {
		return fmt.Errorf("event not found on the server: refresh and try again")
	}

	if err := c.refuseReadOnly(object.calendarName); err != nil {
		return err
	}

	master := masterChild(object.data, object.uid)
	if master == nil {
		return fmt.Errorf("event not found on the server: refresh and try again")
	}

	master.Props.Add(occurrenceProp(ical.PropExceptionDates, master, object.occurrenceTime))

	var children []*ical.Component
	for _, child := range object.data.Children {
		if child.Name == ical.CompEvent && child.Props.Get(ical.PropRecurrenceID) != nil {
			childUID, _ := child.Props.Text(ical.PropUID)

			recurrenceTime, err := child.Props.DateTime(ical.PropRecurrenceID, c.location)

			if childUID == object.uid && err == nil && recurrenceTime.Unix() == object.occurrenceTime.Unix() {
				continue
			}
		}

		children = append(children, child)
	}
	object.data.Children = children

	ctx, cancel := caldavContext()
	defer cancel()

	_, _, err := c.uploadObject(ctx, object.path, object.data, object.etag, false)

	if err != nil {
		return fmt.Errorf("deleting occurrence: %w", err)
	}

	delete(c.objects, id)

	return nil
}

func (c *caldavClient) updateSeries(event Event) error {
	object, ok := c.objects[event.ID]
	if !ok {
		return fmt.Errorf("event not found on the server: refresh and try again")
	}

	if err := c.refuseReadOnly(object.calendarName); err != nil {
		return err
	}

	if event.Calendar != object.calendarName {
		return fmt.Errorf("moving a repeating series to another calendar is not supported yet")
	}

	master := masterChild(object.data, object.uid)
	if master == nil {
		return fmt.Errorf("event not found on the server: refresh and try again")
	}

	masterEvent := ical.Event{Component: master}

	masterStart, err := masterEvent.DateTimeStart(c.location)

	if err != nil {
		return fmt.Errorf("updating series: %w", err)
	}

	masterStartProp := master.Props.Get(ical.PropDateTimeStart)

	masterAllDay := masterStartProp != nil && masterStartProp.ValueType() == ical.ValueDate

	ruleProp := master.Props.Get(ical.PropRecurrenceRule)

	specTouched := false
	if ruleProp != nil {
		existing := recurrenceSpec(master.Props)

		sameUntilDay := existing.Until.IsZero() == event.Recurrence.Until.IsZero()
		if !existing.Until.IsZero() && !event.Recurrence.Until.IsZero() {
			zone := event.Start.Location()

			existingYear, existingMonth, existingDay := existing.Until.In(zone).Date()

			untilYear, untilMonth, untilDay := event.Recurrence.Until.In(zone).Date()

			sameUntilDay = existingYear == untilYear && existingMonth == untilMonth && existingDay == untilDay
		}

		specTouched = existing.Frequency != event.Recurrence.Frequency ||
			max(existing.Interval, 1) != max(event.Recurrence.Interval, 1) ||
			!sameUntilDay
	}

	if ruleProp != nil && (specTouched || masterAllDay != event.AllDay) && ruleBeyondCaltui(ruleProp.Value) {
		return fmt.Errorf("this series repeats with rules caltui cannot edit; change it in the app that created it")
	}

	// The user's date fields are ignored for a whole series: only the time of
	// day moves, anchored on the master's date as seen in the user's zone.
	masterLocal := masterStart.In(event.Start.Location())

	seriesEvent := event

	if event.AllDay {
		startDay := time.Date(event.Start.Year(), event.Start.Month(), event.Start.Day(), 0, 0, 0, 0, time.UTC)

		endDay := time.Date(event.End.Year(), event.End.Month(), event.End.Day(), 0, 0, 0, 0, time.UTC)

		daySpan := max(1, int(endDay.Sub(startDay).Hours()/24))

		seriesEvent.Start = time.Date(masterLocal.Year(), masterLocal.Month(), masterLocal.Day(), 0, 0, 0, 0, event.Start.Location())
		seriesEvent.End = seriesEvent.Start.AddDate(0, 0, daySpan)
	} else {
		seriesEvent.Start = time.Date(
			masterLocal.Year(), masterLocal.Month(), masterLocal.Day(),
			event.Start.Hour(), event.Start.Minute(), masterLocal.Second(), 0, event.Start.Location(),
		)
		seriesEvent.End = seriesEvent.Start.Add(event.End.Sub(event.Start))
	}

	repeatTurnedOff := ruleProp != nil && event.Recurrence.Frequency == ""

	structureChanged := !seriesEvent.Start.Equal(masterStart) ||
		masterAllDay != event.AllDay ||
		repeatTurnedOff

	if structureChanged {
		master.Props.Del(ical.PropExceptionDates)

		var children []*ical.Component
		for _, child := range object.data.Children {
			if child.Name == ical.CompEvent && child.Props.Get(ical.PropRecurrenceID) != nil {
				childUID, _ := child.Props.Text(ical.PropUID)

				if childUID == object.uid {
					continue
				}
			}

			children = append(children, child)
		}
		object.data.Children = children
	}

	if masterAllDay != event.AllDay {
		master.Props.Del(ical.PropRecurrenceRule)
	}

	applyEventProps(&ical.Event{Component: master}, seriesEvent)

	ctx, cancel := caldavContext()
	defer cancel()

	finalPath, etag, err := c.uploadObject(ctx, object.path, object.data, object.etag, false)

	if err != nil {
		return fmt.Errorf("updating series: %w", err)
	}

	object.path = finalPath
	object.etag = etag
	c.objects[event.ID] = object

	return nil
}

func (c *caldavClient) removeSeries(id string) error {
	object, ok := c.objects[id]
	if !ok {
		return fmt.Errorf("event not found on the server: refresh and try again")
	}

	if err := c.refuseReadOnly(object.calendarName); err != nil {
		return err
	}

	return c.removeObject(object, id)
}

func (c *caldavClient) remove(id string) error {
	object, ok := c.objects[id]
	if !ok {
		return fmt.Errorf("event not found on the server: refresh and try again")
	}

	if err := c.refuseReadOnly(object.calendarName); err != nil {
		return err
	}

	if object.recurring {
		return fmt.Errorf("recurring events are read-only for now")
	}

	return c.removeObject(object, id)
}

func (c *caldavClient) removeObject(object caldavObject, id string) error {
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

	recurs := event.Recurrence.Frequency != "" || icalEvent.Props.Get(ical.PropRecurrenceRule) != nil

	zoneName := event.Start.Location().String()

	switch {
	case event.AllDay:
		startProp := ical.NewProp(ical.PropDateTimeStart)
		startProp.SetDate(event.Start)
		icalEvent.Props.Set(startProp)

		endProp := ical.NewProp(ical.PropDateTimeEnd)
		endProp.SetDate(event.End)
		icalEvent.Props.Set(endProp)

	// A repeating time must stay wall-clock across DST changes, which needs a
	// TZID anchor; a fixed UTC instant would drift. Zones without an IANA name
	// (time.Local) cannot be written as a TZID, so they keep the UTC form.
	case recurs && strings.Contains(zoneName, "/"):
		icalEvent.Props.SetDateTime(ical.PropDateTimeStart, event.Start)
		icalEvent.Props.SetDateTime(ical.PropDateTimeEnd, event.End)

	default:
		icalEvent.Props.SetDateTime(ical.PropDateTimeStart, event.Start.UTC())
		icalEvent.Props.SetDateTime(ical.PropDateTimeEnd, event.End.UTC())
	}

	if icalEvent.Props.Get(ical.PropRecurrenceID) != nil {
		return
	}

	existing := recurrenceSpec(icalEvent.Props)

	sameUntil := existing.Until.IsZero() == event.Recurrence.Until.IsZero()
	if !existing.Until.IsZero() && !event.Recurrence.Until.IsZero() {
		zone := event.Start.Location()

		existingYear, existingMonth, existingDay := existing.Until.In(zone).Date()

		untilYear, untilMonth, untilDay := event.Recurrence.Until.In(zone).Date()

		sameUntil = existingYear == untilYear && existingMonth == untilMonth && existingDay == untilDay
	}

	sameFrequency := existing.Frequency == event.Recurrence.Frequency
	sameInterval := max(existing.Interval, 1) == max(event.Recurrence.Interval, 1)

	if sameFrequency && sameInterval && sameUntil {
		return
	}

	if event.Recurrence.Frequency == "" {
		icalEvent.Props.Del(ical.PropRecurrenceRule)

		return
	}

	parts := []string{"FREQ=" + strings.ToUpper(event.Recurrence.Frequency)}

	if event.Recurrence.Interval > 1 {
		parts = append(parts, fmt.Sprintf("INTERVAL=%d", event.Recurrence.Interval))
	}

	if !event.Recurrence.Until.IsZero() {
		// RFC 5545 requires a DATE-typed UNTIL when DTSTART is a DATE, which
		// rrule-go cannot emit, hence the hand-built rule value.
		until := event.Recurrence.Until.UTC().Format("20060102T150405Z")
		if event.AllDay {
			until = event.Recurrence.Until.Format("20060102")
		}

		parts = append(parts, "UNTIL="+until)
	}

	rule := ical.NewProp(ical.PropRecurrenceRule)
	rule.Value = strings.Join(parts, ";")
	icalEvent.Props.Set(rule)
}
