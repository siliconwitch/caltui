package detail

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
)

var start = time.Date(2026, 7, 9, 13, 30, 0, 0, time.UTC)

func TestVisibility(t *testing.T) {
	event := calendar.Event{
		Title:    "Call with John",
		Start:    start,
		End:      start.Add(45 * time.Minute),
		Calendar: "Personal",
	}

	cases := []struct {
		name     string
		messages []tea.Msg
		visible  bool
	}{
		{"hidden before any selection", nil, false},
		{"shown after selection", []tea.Msg{msgs.EventSelectedMsg{Event: &event}}, true},
		{"hidden after nil selection", []tea.Msg{msgs.EventSelectedMsg{Event: &event}, msgs.EventSelectedMsg{}}, false},
		{"hidden after events change", []tea.Msg{msgs.EventSelectedMsg{Event: &event}, msgs.EventsChangedMsg{}}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var model tea.Model = New(time.Local)

			model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
			for _, message := range c.messages {
				model, _ = model.Update(message)
			}

			view := model.View()

			if visible := view != ""; visible != c.visible {
				t.Fatalf("expected visible=%v, got view %q", c.visible, view)
			}
		})
	}
}

func TestDurationSummary(t *testing.T) {
	cases := []struct {
		minutes  int
		expected string
	}{
		{45, "45m"},
		{120, "2h"},
		{90, "1h 30m"},
	}

	for _, c := range cases {
		event := calendar.Event{
			Title:    "Call with John",
			Start:    start,
			End:      start.Add(time.Duration(c.minutes) * time.Minute),
			Calendar: "Personal",
		}

		var model tea.Model = New(time.Local)

		model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
		model, _ = model.Update(msgs.EventSelectedMsg{Event: &event})

		view := model.View()

		if !strings.Contains(view, c.expected) {
			t.Errorf("duration %dm: expected view to contain %q, got %q", c.minutes, c.expected, view)
		}
	}
}

func TestLineWidths(t *testing.T) {
	event := calendar.Event{
		Title:     "Quarterly planning session with the entire hardware team",
		Start:     start,
		End:       start.Add(150 * time.Minute),
		Calendar:  "Work",
		Location:  "Off-site",
		Attendees: []string{"Raj", "Priya", "Marcus", "Elena", "Tobias", "Ines", "Naomi"},
	}

	cases := []struct {
		terminalWidth   int
		maximumBoxWidth int
	}{
		{120, 80},
		{100, 80},
		{60, 56},
		{30, 26},
	}

	for _, c := range cases {
		var model tea.Model = New(time.Local)

		model, _ = model.Update(tea.WindowSizeMsg{Width: c.terminalWidth, Height: 40})
		model, _ = model.Update(msgs.EventSelectedMsg{Event: &event})

		for _, line := range strings.Split(model.View(), "\n") {
			if width := ansi.StringWidth(line); width > c.maximumBoxWidth {
				t.Errorf("terminal width %d: line width %d exceeds %d: %q", c.terminalWidth, width, c.maximumBoxWidth, line)
			}
		}
	}
}
