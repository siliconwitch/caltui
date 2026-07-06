package weekview

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/msgs"
)

func TestParseHalfHourRow(t *testing.T) {
	cases := []struct {
		value   string
		wantRow int
		wantOk  bool
	}{
		{"00:00", 0, true},
		{"09:00", 18, true},
		{"09:30", 19, true},
		{"9:15", 18, true},
		{"23:30", 47, true},
		{"24:00", 48, true},
		{"24:30", 0, false},
		{"25:00", 0, false},
		{"12:60", 0, false},
		{"-1:00", 0, false},
		{"noon", 0, false},
		{"", 0, false},
	}

	for _, testCase := range cases {
		row, ok := parseHalfHourRow(testCase.value)

		if row != testCase.wantRow || ok != testCase.wantOk {
			t.Errorf("parseHalfHourRow(%q) = %d, %v; want %d, %v", testCase.value, row, ok, testCase.wantRow, testCase.wantOk)
		}
	}
}

func TestEventRowSpan(t *testing.T) {
	day := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)

	nextDay := day.AddDate(0, 0, 1)

	cases := []struct {
		name         string
		start        time.Time
		end          time.Time
		wantStartRow int
		wantEndRow   int
	}{
		{"aligned hour", day.Add(9 * time.Hour), day.Add(10 * time.Hour), 18, 20},
		{"quarter past ceilings up", day.Add(9*time.Hour + 15*time.Minute), day.Add(10*time.Hour + 45*time.Minute), 18, 22},
		{"short event fills one row", day.Add(9*time.Hour + 30*time.Minute), day.Add(9*time.Hour + 45*time.Minute), 19, 20},
		{"zero length keeps one row", day.Add(9 * time.Hour), day.Add(9 * time.Hour), 18, 19},
		{"clamps start before day", day.Add(-2 * time.Hour), day.Add(1 * time.Hour), 0, 2},
		{"clamps end past midnight", day.Add(23 * time.Hour), nextDay.Add(1 * time.Hour), 46, 48},
	}

	for _, testCase := range cases {
		event := calendar.Event{Start: testCase.start, End: testCase.end}

		startRow, endRow := eventRowSpan(event, day, nextDay)

		if startRow != testCase.wantStartRow || endRow != testCase.wantEndRow {
			t.Errorf("%s: eventRowSpan = %d, %d; want %d, %d", testCase.name, startRow, endRow, testCase.wantStartRow, testCase.wantEndRow)
		}
	}
}

func TestMondayOf(t *testing.T) {
	cases := []struct {
		name string
		date time.Time
		want time.Time
	}{
		{"monday maps to itself", time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)},
		{"midweek with time of day", time.Date(2026, 7, 8, 15, 30, 0, 0, time.UTC), time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)},
		{"sunday maps back", time.Date(2026, 7, 12, 23, 59, 0, 0, time.UTC), time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)},
		{"crosses month edge", time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC), time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)},
	}

	for _, testCase := range cases {
		got := mondayOf(testCase.date)

		if !got.Equal(testCase.want) {
			t.Errorf("%s: mondayOf(%v) = %v; want %v", testCase.name, testCase.date, got, testCase.want)
		}
	}
}

func TestViewDimensions(t *testing.T) {
	cases := []struct {
		width  int
		height int
	}{
		{120, 40},
		{80, 24},
		{45, 10},
		{20, 5},
		{200, 60},
	}

	for _, testCase := range cases {
		model := New(calendar.NewMock(time.Now()), DefaultConfig(), time.Local)

		resized, _ := model.Update(tea.WindowSizeMsg{Width: testCase.width, Height: testCase.height})

		view := resized.(Model).View()

		lines := strings.Split(view, "\n")
		if len(lines) != testCase.height {
			t.Errorf("%dx%d: got %d lines; want %d", testCase.width, testCase.height, len(lines), testCase.height)
		}

		for lineIndex, line := range lines {
			if lineWidth := ansi.StringWidth(line); lineWidth > testCase.width {
				t.Errorf("%dx%d: line %d is %d cells wide; want at most %d", testCase.width, testCase.height, lineIndex, lineWidth, testCase.width)
			}
		}
	}
}

func TestSelectionKeys(t *testing.T) {
	keyFor := func(key string) tea.KeyMsg {
		switch key {
		case "tab":
			return tea.KeyMsg{Type: tea.KeyTab}
		case "shift+tab":
			return tea.KeyMsg{Type: tea.KeyShiftTab}
		case "esc":
			return tea.KeyMsg{Type: tea.KeyEsc}
		default:
			return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		}
	}

	base := New(calendar.NewMock(time.Now()), DefaultConfig(), time.Local)

	resized, _ := base.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	base = resized.(Model)

	eventCount := len(base.selectedDayEvents())
	if eventCount == 0 {
		t.Fatal("mock source seeded no events for today")
	}

	wrappingTabs := make([]string, eventCount+1)
	for index := range wrappingTabs {
		wrappingTabs[index] = "tab"
	}

	cases := []struct {
		name      string
		keys      []string
		wantIndex int
	}{
		{"tab selects first", []string{"tab"}, 0},
		{"tab twice advances", []string{"tab", "tab"}, 1 % eventCount},
		{"tab wraps around", wrappingTabs, 0},
		{"shift+tab wraps to last", []string{"shift+tab"}, eventCount - 1},
		{"esc deselects", []string{"tab", "esc"}, -1},
		{"cursor move deselects", []string{"tab", "l"}, -1},
	}

	for _, testCase := range cases {
		model := base

		var lastCmd tea.Cmd
		for _, key := range testCase.keys {
			updated, cmd := model.Update(keyFor(key))
			model = updated.(Model)
			lastCmd = cmd
		}

		if model.selectionIndex != testCase.wantIndex {
			t.Errorf("%s: selection index = %d; want %d", testCase.name, model.selectionIndex, testCase.wantIndex)
			continue
		}

		if lastCmd == nil {
			t.Errorf("%s: expected a command from the final key", testCase.name)
			continue
		}

		selectedMsg, ok := lastCmd().(msgs.EventSelectedMsg)
		if !ok {
			t.Errorf("%s: final command did not produce an EventSelectedMsg", testCase.name)
			continue
		}

		if wantEvent := testCase.wantIndex >= 0; (selectedMsg.Event != nil) != wantEvent {
			t.Errorf("%s: EventSelectedMsg event presence = %v; want %v", testCase.name, selectedMsg.Event != nil, wantEvent)
		}
	}
}
