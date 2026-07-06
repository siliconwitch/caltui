package maskinput

import (
	"testing"
	"time"
)

func TestDateTyping(t *testing.T) {
	prefill := time.Date(2026, time.July, 6, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name            string
		start           time.Time
		resetDownstream bool
		keys            []string
		wantYear        int
		wantMonth       time.Month
		wantDay         int
	}{
		{
			name:      "full date types through separators",
			start:     prefill,
			keys:      []string{"2", "0", "2", "6", "0", "4", "1", "2"},
			wantYear:  2026,
			wantMonth: time.April,
			wantDay:   12,
		},
		{
			name:      "month fifteen clamps to december",
			start:     time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC),
			keys:      []string{"right", "1", "5"},
			wantYear:  2026,
			wantMonth: time.December,
			wantDay:   10,
		},
		{
			name:      "day beyond month length clamps",
			start:     time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC),
			keys:      []string{"right", "right", "3", "9"},
			wantYear:  2026,
			wantMonth: time.January,
			wantDay:   31,
		},
		{
			name:      "year out of range clamps",
			start:     prefill,
			keys:      []string{"3", "0", "2", "6"},
			wantYear:  2999,
			wantMonth: time.July,
			wantDay:   6,
		},
		{
			name:      "changing month reclamps the day",
			start:     time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC),
			keys:      []string{"right", "0", "2"},
			wantYear:  2026,
			wantMonth: time.February,
			wantDay:   28,
		},
		{
			name:            "reset downstream on year entry",
			start:           prefill,
			resetDownstream: true,
			keys:            []string{"2", "0", "2", "6"},
			wantYear:        2026,
			wantMonth:       time.January,
			wantDay:         1,
		},
		{
			name:            "reset downstream partial month",
			start:           prefill,
			resetDownstream: true,
			keys:            []string{"2", "0", "2", "6", "0", "5"},
			wantYear:        2026,
			wantMonth:       time.May,
			wantDay:         1,
		},
		{
			name:      "pending month commits on read",
			start:     prefill,
			keys:      []string{"right", "1"},
			wantYear:  2026,
			wantMonth: time.January,
			wantDay:   6,
		},
		{
			name:      "zero month commits to january",
			start:     prefill,
			keys:      []string{"right", "0", "0"},
			wantYear:  2026,
			wantMonth: time.January,
			wantDay:   6,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			field := NewDate(testCase.resetDownstream).WithDate(testCase.start).Focus()

			for _, key := range testCase.keys {
				field, _ = field.Typed(key)
			}

			year, month, day := field.Date()

			if year != testCase.wantYear || month != testCase.wantMonth || day != testCase.wantDay {
				t.Fatalf("got %04d-%02d-%02d, want %04d-%02d-%02d",
					year, month, day, testCase.wantYear, testCase.wantMonth, testCase.wantDay)
			}
		})
	}
}

func TestTimeTyping(t *testing.T) {
	cases := []struct {
		name          string
		keys          []string
		wantHour      int
		wantMinute    int
		wantCompleted bool
	}{
		{name: "regular time", keys: []string{"0", "9", "3", "0"}, wantHour: 9, wantMinute: 30, wantCompleted: true},
		{name: "high first digits auto advance", keys: []string{"9", "7"}, wantHour: 9, wantMinute: 7, wantCompleted: true},
		{name: "last valid minute", keys: []string{"2", "3", "5", "9"}, wantHour: 23, wantMinute: 59, wantCompleted: true},
		{name: "hour clamps to 23", keys: []string{"2", "8"}, wantHour: 23, wantMinute: 0, wantCompleted: false},
		{name: "partial entry", keys: []string{"1"}, wantHour: 1, wantMinute: 0, wantCompleted: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			field := NewTime().WithTime(0, 0).Focus()

			completed := false
			for _, key := range testCase.keys {
				var keyCompleted bool

				field, keyCompleted = field.Typed(key)
				completed = completed || keyCompleted
			}

			hour, minute := field.Time()

			if hour != testCase.wantHour || minute != testCase.wantMinute || completed != testCase.wantCompleted {
				t.Fatalf("got %02d:%02d completed=%v, want %02d:%02d completed=%v",
					hour, minute, completed, testCase.wantHour, testCase.wantMinute, testCase.wantCompleted)
			}
		})
	}
}
