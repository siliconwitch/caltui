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
	case "scope":
		source = m.scopePicker
	case "search":
		source = m.search
	case "calendars":
		source = m.calendars
	case "alert":
		source = m.alertPopup
	case "error":
		source = m.errorPopup
	}

	var hints []msgs.KeyHint
	if hinter, ok := source.(interface{ KeyHints() []msgs.KeyHint }); ok {
		hints = hinter.KeyHints()
	}

	if m.popup == "" {
		viewKeys := "m/w/d/a"
		if m.selectedEvent() != nil {
			viewKeys = "m/w/a"
		}

		hints = append(hints,
			msgs.KeyHint{Key: "/", Action: "search"},
			msgs.KeyHint{Key: "g", Action: "go to"},
			msgs.KeyHint{Key: viewKeys, Action: "view"},
		)

		if _, ok := m.store.(syncer); ok {
			hints = append(hints, msgs.KeyHint{Key: "r", Action: "refresh"})
		}

		hints = append(hints, msgs.KeyHint{Key: "q", Action: "quit"})
	}

	notice := ""
	noticeStyle := lipgloss.NewStyle().Foreground(theme.Muted)

	switch {
	case m.notice != "":
		notice = m.notice
		noticeStyle = lipgloss.NewStyle().Foreground(theme.Danger)

	case m.pendingSyncs > 0:
		notice = "syncing…"
	}

	hintWidth := m.width
	if notice != "" {
		notice = ansi.Truncate(notice, max(m.width/2, 10), "…")
		hintWidth = m.width - ansi.StringWidth(notice) - 2
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

	if ansi.StringWidth(bar) > hintWidth {
		bar = render(hints, " · ")

		for ansi.StringWidth(bar) > hintWidth {
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

		bar = ansi.Truncate(bar, hintWidth, "…")
	}

	if notice == "" {
		return bar
	}

	padding := max(m.width-ansi.StringWidth(bar)-ansi.StringWidth(notice)-1, 1)

	return bar + strings.Repeat(" ", padding) + noticeStyle.Render(notice) + " "
}
