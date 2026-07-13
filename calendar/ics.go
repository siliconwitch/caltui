package calendar

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/emersion/go-ical"
)

type icsClient struct {
	accountName string
	selfEmail   string
	url         string
	httpClient  *http.Client
	location    *time.Location
}

func (c *icsClient) fetch(from, to time.Time) ([]Calendar, []Event, error) {
	response, err := c.httpClient.Get(c.url)

	if err != nil {
		// The subscription URL is a credential, and url.Error embeds it, so
		// the wrapper is stripped before the error can reach the screen.
		var urlErr *url.Error

		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}

		return nil, nil, fmt.Errorf("fetching calendar: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("fetching calendar: server said %s", response.Status)
	}

	data, err := ical.NewDecoder(response.Body).Decode()

	if err != nil {
		return nil, nil, fmt.Errorf("parsing calendar: %w", err)
	}

	name, _ := data.Props.Text("X-WR-CALNAME")
	if name == "" {
		name = c.accountName
	}

	parsed := eventsFromICal(data, name, c.selfEmail, from, to, c.location)

	events := make([]Event, 0, len(parsed))
	for _, event := range parsed {
		events = append(events, event.Event)
	}

	return []Calendar{{Name: name}}, events, nil
}
