package timezone

import (
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/siliconwitch/caltui/theme"
)

const visibleMatchCount = 6

func Marker(t time.Time, base *time.Location) string {
	abbreviation, offset := t.Zone()

	_, baseOffset := t.In(base).Zone()

	if offset == baseOffset {
		return ""
	}

	return "(" + abbreviation + ")"
}

func Search(query string) []string {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(query), " ", "_"))

	if normalized == "" {
		return zoneNames
	}

	var cityMatches, otherMatches []string
	for _, name := range zoneNames {
		lower := strings.ToLower(name)

		city := lower[strings.LastIndex(lower, "/")+1:]

		switch {
		case strings.HasPrefix(city, normalized):
			cityMatches = append(cityMatches, name)
		case strings.Contains(lower, normalized):
			otherMatches = append(otherMatches, name)
		}
	}

	return append(cityMatches, otherMatches...)
}

type Picker struct {
	query     string
	cursor    int
	reference time.Time
}

func NewPicker() Picker {
	return Picker{}
}

func (p Picker) Opened(reference time.Time, query string) Picker {
	p.query = query
	p.cursor = 0
	p.reference = reference

	return p
}

func (p Picker) Typed(key string) (Picker, *time.Location, bool) {
	switch key {
	case "esc":
		return p, nil, true

	case "enter":
		matches := Search(p.query)

		if len(matches) == 0 {
			return p, nil, false
		}

		location, err := time.LoadLocation(matches[min(p.cursor, len(matches)-1)])

		if err != nil {
			return p, nil, false
		}

		return p, location, true

	case "up":
		p.cursor = max(p.cursor-1, 0)

		return p, nil, false

	case "down":
		limit := min(len(Search(p.query)), visibleMatchCount)

		p.cursor = min(p.cursor+1, max(limit-1, 0))

		return p, nil, false

	case "backspace":
		if p.query != "" {
			p.query = p.query[:len(p.query)-1]
			p.cursor = 0
		}

		return p, nil, false
	}

	switch key {
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

	return p, nil, false
}

func (p Picker) View(width int) []string {
	mutedStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	accentStyle := lipgloss.NewStyle().Foreground(theme.Accent)

	lines := []string{accentStyle.Render("Search: ") + p.query + accentStyle.Render("▏")}

	matches := Search(p.query)

	if len(matches) == 0 {
		return append(lines, mutedStyle.Render("no matches"))
	}

	for index, name := range matches[:min(len(matches), visibleMatchCount)] {
		abbreviation := ""
		if location, err := time.LoadLocation(name); err == nil {
			sample := time.Date(
				p.reference.Year(), p.reference.Month(), p.reference.Day(),
				p.reference.Hour(), p.reference.Minute(), 0, 0, location,
			)

			abbreviation, _ = sample.Zone()
		}

		row := ansi.Truncate(name, max(width-6, 1), "…")

		padding := strings.Repeat(" ", max(width-ansi.StringWidth(row)-ansi.StringWidth(abbreviation), 1))

		line := row + padding + mutedStyle.Render(abbreviation)
		if index == p.cursor {
			line = lipgloss.NewStyle().Reverse(true).Render(row+padding) + mutedStyle.Reverse(true).Render(abbreviation)
		}

		lines = append(lines, line)
	}

	return lines
}
