package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const ansiReset = "\x1b[0m"

func Compose(background, overlay string, x, y, width int) string {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	backgroundLines := strings.Split(background, "\n")
	overlayLines := strings.Split(overlay, "\n")

	overlayWidth := 0
	for _, line := range overlayLines {
		if lineWidth := ansi.StringWidth(line); lineWidth > overlayWidth {
			overlayWidth = lineWidth
		}
	}

	for i, overlayLine := range overlayLines {
		row := y + i
		if row >= len(backgroundLines) {
			break
		}

		backgroundLine := backgroundLines[row]

		var wordCells []bool
		for _, character := range ansi.Strip(backgroundLine) {
			isWord := !strings.ContainsRune(" │─╌", character)
			for range ansi.StringWidth(string(character)) {
				wordCells = append(wordCells, isWord)
			}
		}

		wordAt := func(cell int) bool {
			return cell >= 0 && cell < len(wordCells) && wordCells[cell]
		}

		leftEdge := x
		if wordAt(leftEdge-1) && wordAt(leftEdge) {
			for wordAt(leftEdge - 1) {
				leftEdge--
			}
		}

		rightEdge := x + overlayWidth
		if wordAt(rightEdge-1) && wordAt(rightEdge) {
			for wordAt(rightEdge) {
				rightEdge++
			}
		}

		left := ansi.Truncate(backgroundLine, leftEdge, "")
		if leftWidth := ansi.StringWidth(left); leftWidth < x {
			left += strings.Repeat(" ", x-leftWidth)
		}
		left += ansiReset

		padded := overlayLine
		if lineWidth := ansi.StringWidth(overlayLine); lineWidth < overlayWidth {
			padded += strings.Repeat(" ", overlayWidth-lineWidth)
		}
		padded += ansiReset

		right := strings.Repeat(" ", rightEdge-x-overlayWidth) + ansi.TruncateLeft(backgroundLine, rightEdge, "")

		backgroundLines[row] = ansi.Truncate(left+padded+right, width, "") + ansiReset
	}

	return strings.Join(backgroundLines, "\n")
}
