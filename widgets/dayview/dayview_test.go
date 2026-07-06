package dayview

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
	testCases := []struct {
		value string
		row   int
		ok    bool
	}{
		{"00:00", 0, true},
		{"24:00", 48, true},
		{"09:30", 19, true},
		{"09:29", 18, true},
		{"23:59", 47, true},
		{"24:30", 0, false},
		{"25:00", 0, false},
		{"-1:00", 0, false},
		{"12", 0, false},
		{"aa:bb", 0, false},
		{"", 0, false},
	}

	for _, testCase := range testCases {
		row, ok := parseHalfHourRow(testCase.value)

		if row != testCase.row || ok != testCase.ok {
			t.Errorf("parseHalfHourRow(%q) = %d, %v, want %d, %v", testCase.value, row, ok, testCase.row, testCase.ok)
		}
	}
}

func TestEventRows(t *testing.T) {
	day := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	model := Model{selectedDate: day}

	testCases := []struct {
		name     string
		start    time.Time
		end      time.Time
		startRow int
		endRow   int
	}{
		{"morning quarter hour", day.Add(9*time.Hour + 30*time.Minute), day.Add(9*time.Hour + 45*time.Minute), 19, 20},
		{"afternoon block", day.Add(14 * time.Hour), day.Add(16 * time.Hour), 28, 32},
		{"spills from previous day", day.Add(-2 * time.Hour), day.Add(time.Hour), 0, 2},
		{"spills into next day", day.Add(23 * time.Hour), day.AddDate(0, 0, 1).Add(time.Hour), 46, 48},
		{"ends exactly at midnight", day.Add(23*time.Hour + 30*time.Minute), day.AddDate(0, 0, 1), 47, 48},
		{"zero duration", day.Add(10 * time.Hour), day.Add(10 * time.Hour), 20, 21},
	}

	for _, testCase := range testCases {
		startRow, endRow := model.eventRows(calendar.Event{Start: testCase.start, End: testCase.end})

		if startRow != testCase.startRow || endRow != testCase.endRow {
			t.Errorf("%s: got rows %d-%d, want %d-%d", testCase.name, startRow, endRow, testCase.startRow, testCase.endRow)
		}
	}
}

func TestViewDimensions(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	testCases := []struct {
		width  int
		height int
	}{
		{80, 24},
		{120, 40},
		{20, 5},
		{5, 3},
		{40, 60},
		{100, 2},
	}

	for _, testCase := range testCases {
		model := New(calendar.NewMock(now), DefaultConfig(), time.Local)

		focused, _ := model.Update(msgs.FocusDateMsg{Date: now})

		sized, _ := focused.Update(tea.WindowSizeMsg{Width: testCase.width, Height: testCase.height})

		view := sized.View()

		viewLines := strings.Split(view, "\n")
		if len(viewLines) != testCase.height {
			t.Errorf("%dx%d: got %d lines, want %d", testCase.width, testCase.height, len(viewLines), testCase.height)
		}

		for lineNumber, line := range viewLines {
			if ansi.StringWidth(line) > testCase.width {
				t.Errorf("%dx%d: line %d is %d cells wide, want at most %d", testCase.width, testCase.height, lineNumber, ansi.StringWidth(line), testCase.width)
			}
		}
	}
}

func TestTabSelectionCycle(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	model := New(calendar.NewMock(now), DefaultConfig(), time.Local)

	focused, _ := model.Update(msgs.FocusDateMsg{Date: now})
	model = focused.(Model)

	steps := []struct {
		key           tea.KeyMsg
		expectedTitle string
	}{
		{tea.KeyMsg{Type: tea.KeyTab}, "Standup"},
		{tea.KeyMsg{Type: tea.KeyTab}, "Quarterly planning session with the entire hardware team"},
		{tea.KeyMsg{Type: tea.KeyTab}, "Pick up parcel from the post office"},
		{tea.KeyMsg{Type: tea.KeyTab}, "Standup"},
		{tea.KeyMsg{Type: tea.KeyShiftTab}, "Pick up parcel from the post office"},
		{tea.KeyMsg{Type: tea.KeyEsc}, ""},
	}

	for stepNumber, step := range steps {
		updated, cmd := model.Update(step.key)
		model = updated.(Model)

		if cmd == nil {
			t.Fatalf("step %d: expected a command", stepNumber)
		}

		emitted, ok := cmd().(msgs.EventSelectedMsg)
		if !ok {
			t.Fatalf("step %d: expected an EventSelectedMsg", stepNumber)
		}

		selected := model.SelectedEvent()

		switch {
		case step.expectedTitle == "":
			if emitted.Event != nil || selected != nil {
				t.Errorf("step %d: expected no selection", stepNumber)
			}
		case emitted.Event == nil || emitted.Event.Title != step.expectedTitle:
			t.Errorf("step %d: emitted %v, want %q", stepNumber, emitted.Event, step.expectedTitle)
		case selected == nil || selected.Title != step.expectedTitle:
			t.Errorf("step %d: selected %v, want %q", stepNumber, selected, step.expectedTitle)
		}
	}
}

func TestTabOnDayWithoutEvents(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	model := New(calendar.NewMock(now), DefaultConfig(), time.Local)

	focused, _ := model.Update(msgs.FocusDateMsg{Date: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)})
	model = focused.(Model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)

	if cmd != nil {
		t.Errorf("expected no command")
	}

	if model.SelectedEvent() != nil {
		t.Errorf("expected no selection")
	}
}
