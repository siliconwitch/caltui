package maskinput

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/siliconwitch/caltui/theme"
)

type Field struct {
	isDate          bool
	resetDownstream bool
	segmentCount    int
	values          [3]int
	digitCounts     [3]int
	focusedSegment  int
	pendingDigits   int
	focused         bool
}

func NewDate(resetDownstream bool) Field {
	return Field{
		isDate:          true,
		resetDownstream: resetDownstream,
		segmentCount:    3,
		values:          [3]int{2000, 1, 1},
		digitCounts:     [3]int{4, 2, 2},
	}
}

func NewTime() Field {
	return Field{
		segmentCount: 2,
		values:       [3]int{0, 0, 0},
		digitCounts:  [3]int{2, 2, 0},
	}
}

func (f Field) WithDate(date time.Time) Field {
	f.values = [3]int{date.Year(), int(date.Month()), date.Day()}
	f.pendingDigits = 0

	return f
}

func (f Field) WithTime(hour, minute int) Field {
	f.values = [3]int{hour, minute, 0}
	f.pendingDigits = 0

	return f
}

func (f Field) Date() (int, time.Month, int) {
	committed := f.withCommittedSegment()

	return committed.values[0], time.Month(committed.values[1]), committed.values[2]
}

func (f Field) Time() (int, int) {
	committed := f.withCommittedSegment()

	return committed.values[0], committed.values[1]
}

func (f Field) Focus() Field {
	f.focused = true
	f.focusedSegment = 0
	f.pendingDigits = 0

	return f
}

func (f Field) Blur() Field {
	f = f.withCommittedSegment()
	f.focused = false

	return f
}

func (f Field) Typed(key string) (Field, bool) {
	switch key {
	case "left":
		f = f.withCommittedSegment()
		f.focusedSegment = max(f.focusedSegment-1, 0)

		return f, false

	case "right":
		f = f.withCommittedSegment()
		f.focusedSegment = min(f.focusedSegment+1, f.segmentCount-1)

		return f, false

	case "backspace":
		if f.pendingDigits == 0 {
			f.focusedSegment = max(f.focusedSegment-1, 0)

			return f, false
		}

		f.values[f.focusedSegment] /= 10
		f.pendingDigits--

		return f, false
	}

	completed := false

	for _, character := range key {
		if character < '0' || character > '9' {
			return f, completed
		}

		digit := int(character - '0')

		if f.pendingDigits == 0 {
			if f.resetDownstream {
				for segment := f.focusedSegment + 1; segment < f.segmentCount; segment++ {
					f.values[segment] = f.segmentMin(segment)
				}
			}

			f.values[f.focusedSegment] = digit
			f.pendingDigits = 1
		} else {
			f.values[f.focusedSegment] = min(f.values[f.focusedSegment]*10+digit, f.segmentMax(f.focusedSegment))
			f.pendingDigits++
		}

		segmentFull := f.pendingDigits == f.digitCounts[f.focusedSegment] ||
			(f.digitCounts[f.focusedSegment] == 2 && f.values[f.focusedSegment]*10 > f.segmentMax(f.focusedSegment))

		if !segmentFull {
			continue
		}

		f = f.withCommittedSegment()

		if f.focusedSegment == f.segmentCount-1 {
			completed = true

			continue
		}

		f.focusedSegment++
	}

	return f, completed
}

func (f Field) View() string {
	separator := ":"
	if f.isDate {
		separator = "-"
	}

	separatorStyle := lipgloss.NewStyle().Foreground(theme.Muted)

	pieces := make([]string, 0, f.segmentCount)
	for segment := range f.segmentCount {
		text := fmt.Sprintf("%0*d", f.digitCounts[segment], f.values[segment])

		style := lipgloss.NewStyle()

		if f.focused && segment == f.focusedSegment {
			style = style.Reverse(true)

			if f.pendingDigits > 0 {
				typed := fmt.Sprintf("%0*d", f.pendingDigits, f.values[segment])

				text = typed + strings.Repeat("_", f.digitCounts[segment]-f.pendingDigits)
			}
		}

		pieces = append(pieces, style.Render(text))
	}

	return strings.Join(pieces, separatorStyle.Render(separator))
}

func (f Field) withCommittedSegment() Field {
	if f.pendingDigits == 0 {
		return f
	}

	value := f.values[f.focusedSegment]

	f.values[f.focusedSegment] = min(max(value, f.segmentMin(f.focusedSegment)), f.segmentMax(f.focusedSegment))
	f.pendingDigits = 0

	if f.isDate && f.focusedSegment < 2 {
		f.values[2] = min(max(f.values[2], 1), f.segmentMax(2))
	}

	return f
}

func (f Field) segmentMin(segment int) int {
	switch {
	case !f.isDate:
		return 0
	case segment == 0:
		return 1900
	default:
		return 1
	}
}

func (f Field) segmentMax(segment int) int {
	switch {
	case !f.isDate && segment == 0:
		return 23
	case !f.isDate:
		return 59
	case segment == 0:
		return 2999
	case segment == 1:
		return 12
	default:
		return time.Date(f.values[0], time.Month(f.values[1])+1, 0, 0, 0, 0, 0, time.UTC).Day()
	}
}
