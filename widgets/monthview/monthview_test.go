package monthview

import (
	"slices"
	"testing"
	"time"
)

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
