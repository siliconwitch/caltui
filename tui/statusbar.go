package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/siliconwitch/caltui/msgs"
	"github.com/siliconwitch/caltui/theme"
)

func (m Model) statusBar() string {
	source := m.activeView()
	switch m.popup {
	case "form":
		source = m.form
	case "confirm":
		source = m.confirm
	case "goto":
		source = m.gotoDate
	}

	var hints []msgs.KeyHint
	if hinter, ok := source.(interface{ KeyHints() []msgs.KeyHint }); ok {
		hints = hinter.KeyHints()
	}

	if m.popup == "" {
		viewKeys := "m/w/d"
		if m.selectedEvent() != nil {
			viewKeys = "m/w"
		}

		hints = append(hints,
			msgs.KeyHint{Key: "g", Action: "go to"},
			msgs.KeyHint{Key: viewKeys, Action: "view"},
			msgs.KeyHint{Key: "q", Action: "quit"},
		)
	}

	keyStyle := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	actionStyle := lipgloss.NewStyle().Foreground(theme.Muted)

	render := func(hints []msgs.KeyHint, separator string) string {
		parts := make([]string, 0, len(hints))
		for _, hint := range hints {
			parts = append(parts, keyStyle.Render(hint.Key)+" "+actionStyle.Render(hint.Action))
		}

		return " " + strings.Join(parts, actionStyle.Render(separator))
	}

	bar := render(hints, "  ·  ")

	if ansi.StringWidth(bar) <= m.width {
		return bar
	}

	bar = render(hints, " · ")

	for ansi.StringWidth(bar) > m.width {
		dropIndex := -1
		dropDistance := len(hints)

		for index, hint := range hints {
			if hint.Key == "q" || hint.Key == "esc" {
				continue
			}

			distance := index - len(hints)/2
			if distance < 0 {
				distance = -distance
			}

			if distance < dropDistance {
				dropIndex = index
				dropDistance = distance
			}
		}

		if dropIndex < 0 {
			break
		}

		hints = append(hints[:dropIndex], hints[dropIndex+1:]...)

		bar = render(hints, " · ")
	}

	return ansi.Truncate(bar, m.width, "…")
}
