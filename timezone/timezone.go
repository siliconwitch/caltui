package timezone

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/siliconwitch/caltui/theme"
)

const visibleMatchCount = 6

func Marker(instant time.Time, base *time.Location) string {
	abbreviation, offset := instant.Zone()

	_, baseOffset := instant.In(base).Zone()

	if offset == baseOffset {
		return ""
	}

	return "(" + abbreviation + ")"
}

func search(query string) []zone {
	normalized := strings.ToLower(strings.TrimSpace(query))
	normalized = strings.ReplaceAll(normalized, "_", " ")

	if normalized == "" {
		return zones
	}

	var cityMatches, countryMatches, substringMatches, fuzzyMatches []zone

	for _, candidate := range zones {
		name := strings.ToLower(strings.ReplaceAll(candidate.name, "_", " "))

		country := strings.ToLower(candidate.country)

		city := name[strings.LastIndex(name, "/")+1:]

		switch {
		case strings.HasPrefix(city, normalized):
			cityMatches = append(cityMatches, candidate)

		case strings.HasPrefix(country, normalized):
			countryMatches = append(countryMatches, candidate)

		case strings.Contains(name, normalized) || strings.Contains(country, normalized):
			substringMatches = append(substringMatches, candidate)

		default:
			for _, haystack := range []string{name, country} {
				remaining := normalized
				for index := 0; index < len(haystack) && remaining != ""; index++ {
					if haystack[index] == remaining[0] {
						remaining = remaining[1:]
					}
				}

				if remaining == "" {
					fuzzyMatches = append(fuzzyMatches, candidate)

					break
				}
			}
		}
	}

	matches := append(cityMatches, countryMatches...)
	matches = append(matches, substringMatches...)

	return append(matches, fuzzyMatches...)
}

type Picker struct {
	query     string
	cursor    int
	scroll    int
	reference time.Time
}

func NewPicker(reference time.Time, query string) Picker {
	return Picker{query: query, reference: reference}
}

func (p Picker) Typed(key string) (Picker, *time.Location, bool) {
	switch key {
	case "esc":
		return p, nil, true

	case "enter":
		matches := search(p.query)

		if len(matches) == 0 {
			return p, nil, false
		}

		location, err := time.LoadLocation(matches[min(p.cursor, len(matches)-1)].name)

		if err != nil {
			return p, nil, false
		}

		return p, location, true

	case "up", "down", "pgup", "pgdown":
		lastIndex := max(len(search(p.query))-1, 0)

		switch key {
		case "up":
			p.cursor = max(p.cursor-1, 0)
		case "down":
			p.cursor = min(p.cursor+1, lastIndex)
		case "pgup":
			p.cursor = max(p.cursor-visibleMatchCount, 0)
		case "pgdown":
			p.cursor = min(p.cursor+visibleMatchCount, lastIndex)
		}

		switch {
		case p.cursor < p.scroll:
			p.scroll = p.cursor
		case p.cursor >= p.scroll+visibleMatchCount:
			p.scroll = p.cursor - visibleMatchCount + 1
		}

		return p, nil, false

	case "backspace":
		if p.query != "" {
			p.query = p.query[:len(p.query)-1]
			p.cursor = 0
			p.scroll = 0
		}

		return p, nil, false

	case "", "left", "right", "tab", "shift+tab", "home", "end":
		return p, nil, false
	}

	for _, character := range key {
		if character < ' ' {
			return p, nil, false
		}
	}

	p.query += key
	p.cursor = 0
	p.scroll = 0

	return p, nil, false
}

func (p Picker) View(width int) []string {
	mutedStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	accentStyle := lipgloss.NewStyle().Foreground(theme.Accent)

	searchLine := accentStyle.Render("Search: ") + p.query + accentStyle.Render("▏")

	matches := search(p.query)

	if len(matches) == 0 {
		return []string{searchLine, mutedStyle.Render("no matches")}
	}

	cursor := min(p.cursor, len(matches)-1)

	scroll := min(p.scroll, max(len(matches)-visibleMatchCount, 0))

	position := fmt.Sprintf("%d/%d", cursor+1, len(matches))

	searchWidth := ansi.StringWidth("Search: "+p.query+"▏") + ansi.StringWidth(position)

	lines := []string{searchLine + strings.Repeat(" ", max(width-searchWidth, 1)) + mutedStyle.Render(position)}

	for index := scroll; index < min(len(matches), scroll+visibleMatchCount); index++ {
		match := matches[index]

		abbreviation := ""
		if location, err := time.LoadLocation(match.name); err == nil {
			sample := time.Date(
				p.reference.Year(), p.reference.Month(), p.reference.Day(),
				p.reference.Hour(), p.reference.Minute(), 0, 0, location,
			)

			abbreviation, _ = sample.Zone()
		}

		detail := strings.TrimSpace(match.country + " " + abbreviation)
		detail = ansi.Truncate(detail, max(width/2, 1), "…")

		row := ansi.Truncate(match.name, max(width-ansi.StringWidth(detail)-2, 1), "…")

		padding := strings.Repeat(" ", max(width-ansi.StringWidth(row)-ansi.StringWidth(detail), 1))

		line := row + padding + mutedStyle.Render(detail)
		if index == cursor {
			line = lipgloss.NewStyle().Reverse(true).Render(row+padding) + mutedStyle.Reverse(true).Render(detail)
		}

		lines = append(lines, line)
	}

	return lines
}
