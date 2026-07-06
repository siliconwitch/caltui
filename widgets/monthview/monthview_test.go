package monthview

import (
	"slices"
	"testing"
	"time"

	"github.com/siliconwitch/caltui/calendar"
)

type fakeSource struct {
	events []calendar.Event
}

func (f fakeSource) Events(from, to time.Time) []calendar.Event {
	var events []calendar.Event
	for _, event := range f.events {
		if event.End.After(from) && event.Start.Before(to) {
			events = append(events, event)
		}
	}

	return events
}

func (f fakeSource) Calendars() []calendar.Calendar {
	return nil
}

func TestWeekLayout(t *testing.T) {
	week := time.Date(2026, time.July, 6, 0, 0, 0, 0, time.UTC)

	day := func(offset int) time.Time { return week.AddDate(0, 0, offset) }

	at := func(offset, hour, minute int) time.Time {
		return time.Date(2026, time.July, 6+offset, hour, minute, 0, 0, time.UTC)
	}

	type expectation struct {
		title     string
		firstCol  int
		lastCol   int
		multiDay  bool
		continued bool
		lane      int
	}

	cases := []struct {
		name   string
		events []calendar.Event
		want   []expectation
	}{
		{
			name: "bar keeps its lane and singles fill below",
			events: []calendar.Event{
				{ID: "1", Title: "Sprint", AllDay: true, Start: day(1), End: day(4)},
				{ID: "2", Title: "Standup", Start: at(2, 9, 30), End: at(2, 9, 45)},
			},
			want: []expectation{
				{title: "Sprint", firstCol: 1, lastCol: 3, multiDay: true, lane: 0},
				{title: "Standup", firstCol: 2, lastCol: 2, lane: 1},
			},
		},
		{
			name: "event from previous week is clipped and continued",
			events: []calendar.Event{
				{ID: "1", Title: "Trip", AllDay: true, Start: day(-2), End: day(2)},
			},
			want: []expectation{
				{title: "Trip", firstCol: 0, lastCol: 1, multiDay: true, continued: true, lane: 0},
			},
		},
		{
			name: "event past the week end is clipped at sunday",
			events: []calendar.Event{
				{ID: "1", Title: "Holiday", AllDay: true, Start: day(5), End: day(9)},
			},
			want: []expectation{
				{title: "Holiday", firstCol: 5, lastCol: 6, multiDay: true, lane: 0},
			},
		},
		{
			name: "timed event crossing midnight spans both days",
			events: []calendar.Event{
				{ID: "1", Title: "Flight", Start: at(2, 16, 0), End: at(3, 9, 30)},
			},
			want: []expectation{
				{title: "Flight", firstCol: 2, lastCol: 3, multiDay: true, lane: 0},
			},
		},
		{
			name: "midnight end stays on the start day",
			events: []calendar.Event{
				{ID: "1", Title: "Late show", Start: at(1, 22, 0), End: at(2, 0, 0)},
			},
			want: []expectation{
				{title: "Late show", firstCol: 1, lastCol: 1, lane: 0},
			},
		},
		{
			name: "disjoint events share a lane",
			events: []calendar.Event{
				{ID: "1", Title: "Sprint", AllDay: true, Start: day(0), End: day(2)},
				{ID: "2", Title: "Fair", AllDay: true, Start: day(3), End: day(5)},
				{ID: "3", Title: "Standup", Start: at(2, 9, 30), End: at(2, 9, 45)},
			},
			want: []expectation{
				{title: "Sprint", firstCol: 0, lastCol: 1, multiDay: true, lane: 0},
				{title: "Fair", firstCol: 3, lastCol: 4, multiDay: true, lane: 0},
				{title: "Standup", firstCol: 2, lastCol: 2, lane: 0},
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := New(fakeSource{events: testCase.events}, DefaultConfig(), time.UTC)

			weekEvents, lanes := model.weekLayout(week)

			if len(weekEvents) != len(testCase.want) {
				t.Fatalf("laid out %d events, expected %d", len(weekEvents), len(testCase.want))
			}

			laneOf := map[int]int{}
			for laneIndex, lane := range lanes {
				for _, eventIndex := range lane {
					if eventIndex >= 0 {
						laneOf[eventIndex] = laneIndex
					}
				}
			}

			for _, expected := range testCase.want {
				found := false
				for index, entry := range weekEvents {
					if entry.event.Title != expected.title {
						continue
					}

					found = true

					actual := expectation{
						title:     entry.event.Title,
						firstCol:  entry.firstCol,
						lastCol:   entry.lastCol,
						multiDay:  entry.multiDay,
						continued: entry.continued,
						lane:      laneOf[index],
					}

					if actual != expected {
						t.Fatalf("layout for %q: got %+v, expected %+v", expected.title, actual, expected)
					}
				}

				if !found {
					t.Fatalf("event %q missing from layout", expected.title)
				}
			}
		})
	}
}

func TestDayEventsOrder(t *testing.T) {
	week := time.Date(2026, time.July, 6, 0, 0, 0, 0, time.UTC)

	wednesday := week.AddDate(0, 0, 2)

	source := fakeSource{events: []calendar.Event{
		{ID: "1", Title: "Standup", Start: wednesday.Add(9 * time.Hour), End: wednesday.Add(10 * time.Hour)},
		{ID: "2", Title: "Holiday", AllDay: true, Start: wednesday, End: wednesday.AddDate(0, 0, 1)},
		{ID: "3", Title: "Sprint", AllDay: true, Start: week.AddDate(0, 0, 1), End: week.AddDate(0, 0, 4)},
	}}

	model := New(source, DefaultConfig(), time.UTC)

	var titles []string
	for _, event := range model.dayEvents(wednesday) {
		titles = append(titles, event.Title)
	}

	if !slices.Equal(titles, []string{"Sprint", "Holiday", "Standup"}) {
		t.Fatalf("day order = %v, expected bar first then all-day then timed", titles)
	}
}

func TestMondayOf(t *testing.T) {
	cases := []struct {
		name     string
		date     time.Time
		expected time.Time
	}{
		{"monday", time.Date(2026, time.July, 6, 15, 30, 0, 0, time.UTC), time.Date(2026, time.July, 6, 0, 0, 0, 0, time.UTC)},
		{"tuesday", time.Date(2026, time.July, 7, 0, 0, 0, 0, time.UTC), time.Date(2026, time.July, 6, 0, 0, 0, 0, time.UTC)},
		{"wednesday", time.Date(2026, time.July, 8, 23, 59, 59, 0, time.UTC), time.Date(2026, time.July, 6, 0, 0, 0, 0, time.UTC)},
		{"thursday", time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC), time.Date(2026, time.July, 6, 0, 0, 0, 0, time.UTC)},
		{"friday", time.Date(2026, time.July, 10, 8, 15, 0, 0, time.UTC), time.Date(2026, time.July, 6, 0, 0, 0, 0, time.UTC)},
		{"saturday", time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC), time.Date(2026, time.July, 6, 0, 0, 0, 0, time.UTC)},
		{"sunday", time.Date(2026, time.July, 12, 18, 45, 0, 0, time.UTC), time.Date(2026, time.July, 6, 0, 0, 0, 0, time.UTC)},
		{"sunday across month boundary", time.Date(2026, time.March, 1, 10, 0, 0, 0, time.UTC), time.Date(2026, time.February, 23, 0, 0, 0, 0, time.UTC)},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := mondayOf(testCase.date)

			if !actual.Equal(testCase.expected) {
				t.Fatalf("mondayOf(%v) = %v, expected %v", testCase.date, actual, testCase.expected)
			}
		})
	}
}

func TestRowHeights(t *testing.T) {
	cases := []struct {
		name     string
		height   int
		expected []int
	}{
		{"minimum grid", 10, []int{4, 4}},
		{"even split", 20, []int{6, 6, 6}},
		{"remainder to first row", 24, []int{8, 7, 7}},
		{"six rows", 40, []int{7, 7, 6, 6, 6, 6}},
		{"capped at ten rows", 80, []int{8, 8, 8, 8, 8, 8, 8, 8, 7, 7}},
		{"tiny terminal", 3, []int{1, 0}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := rowHeights(testCase.height)

			if !slices.Equal(actual, testCase.expected) {
				t.Fatalf("rowHeights(%d) = %v, expected %v", testCase.height, actual, testCase.expected)
			}

			total := 0
			for _, rowHeight := range actual {
				total += rowHeight
			}

			if total != testCase.height-2 {
				t.Fatalf("rowHeights(%d) sums to %d, expected %d", testCase.height, total, testCase.height-2)
			}
		})
	}
}

func TestColumnWidths(t *testing.T) {
	cases := []struct {
		name            string
		width           int
		showWeekNumbers bool
		expectedGutter  int
		expectedCells   []int
	}{
		{"80 columns", 80, false, 0, []int{11, 11, 11, 11, 10, 10, 10}},
		{"80 columns with gutter", 80, true, 3, []int{11, 10, 10, 10, 10, 10, 10}},
		{"120 columns", 120, false, 0, []int{17, 17, 16, 16, 16, 16, 16}},
		{"121 columns with gutter", 121, true, 3, []int{16, 16, 16, 16, 16, 16, 16}},
		{"narrow with gutter", 33, true, 3, []int{4, 4, 4, 3, 3, 3, 3}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			gutterWidth, cellWidths := columnWidths(testCase.width, testCase.showWeekNumbers)

			if gutterWidth != testCase.expectedGutter {
				t.Fatalf("columnWidths(%d, %v) gutter = %d, expected %d", testCase.width, testCase.showWeekNumbers, gutterWidth, testCase.expectedGutter)
			}

			if !slices.Equal(cellWidths, testCase.expectedCells) {
				t.Fatalf("columnWidths(%d, %v) cells = %v, expected %v", testCase.width, testCase.showWeekNumbers, cellWidths, testCase.expectedCells)
			}

			total := gutterWidth + 6
			for _, cellWidth := range cellWidths {
				total += cellWidth
			}

			if total != testCase.width {
				t.Fatalf("columnWidths(%d, %v) spans %d columns, expected %d", testCase.width, testCase.showWeekNumbers, total, testCase.width)
			}
		})
	}
}
