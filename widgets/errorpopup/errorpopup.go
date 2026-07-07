package errorpopup

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/siliconwitch/caltui/msgs"
	"github.com/siliconwitch/caltui/theme"
)

const maxInnerWidth = 76

type Model struct {
	errors []string
	copied bool
	width  int
	height int
}

func New() Model {
	return Model{}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Pending() int {
	return len(m.errors)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		return m, nil

	case msgs.SyncedMsg:
		if msg.Err != nil {
			m.errors = append(m.errors, "Syncing "+msg.Account+" failed:\n\n"+msg.Err.Error())
		}

		return m, nil

	case msgs.StoreErrorMsg:
		m.errors = append(m.errors, msg.Err.Error())

		return m, nil

	case tea.KeyMsg:
		if msg.String() == "y" && len(m.errors) > 0 {
			text := m.errors[0]
			m.copied = true

			return m, func() tea.Msg {
				termenv.Copy(text)

				return nil
			}
		}

		m.copied = false
		if len(m.errors) > 0 {
			m.errors = m.errors[1:]
		}

		if len(m.errors) == 0 {
			return m, func() tea.Msg { return msgs.ClosePopupMsg{} }
		}

		return m, nil
	}

	return m, nil
}

func (m Model) View() string {
	if len(m.errors) == 0 {
		return ""
	}

	innerWidth := min(maxInnerWidth, max(20, m.width-8))

	heading := "Error"
	if len(m.errors) > 1 {
		heading = fmt.Sprintf("Error (1 of %d)", len(m.errors))
	}

	body := lipgloss.NewStyle().Width(innerWidth).Render(m.errors[0])

	lines := strings.Split(body, "\n")

	maxBodyLines := max(m.height-8, 3)
	if len(lines) > maxBodyLines {
		lines = append(lines[:maxBodyLines], "…")
	}

	hint := "y yank  ·  any other key dismiss"
	if m.copied {
		hint = "copied to clipboard"
	}

	content := lipgloss.NewStyle().Bold(true).Foreground(theme.Danger).Render(heading) +
		"\n\n" + strings.Join(lines, "\n") + "\n\n" +
		lipgloss.NewStyle().Foreground(theme.Muted).Render(hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Danger).
		Padding(0, 1).
		Width(innerWidth + 2)

	return box.Render(content)
}

func (m Model) KeyHints() []msgs.KeyHint {
	return []msgs.KeyHint{
		{Key: "y", Action: "yank"},
		{Key: "any other key", Action: "dismiss"},
	}
}
