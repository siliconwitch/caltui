package main

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/config"
	"github.com/siliconwitch/caltui/tui"
	"github.com/siliconwitch/caltui/widgets/confirm"
	"github.com/siliconwitch/caltui/widgets/dayview"
	"github.com/siliconwitch/caltui/widgets/detail"
	"github.com/siliconwitch/caltui/widgets/eventform"
	"github.com/siliconwitch/caltui/widgets/gotodate"
	"github.com/siliconwitch/caltui/widgets/monthview"
	"github.com/siliconwitch/caltui/widgets/weekview"
)

func main() {
	monthConfig := monthview.DefaultConfig()
	weekConfig := weekview.DefaultConfig()
	dayConfig := dayview.DefaultConfig()
	calendarConfig := calendar.DefaultConfig()

	err := config.Load(map[string]any{
		monthview.ConfigSection: &monthConfig,
		weekview.ConfigSection:  &weekConfig,
		dayview.ConfigSection:   &dayConfig,
		calendar.ConfigSection:  &calendarConfig,
	})

	if err != nil {
		fmt.Fprintln(os.Stderr, "caltui: reading config:", err)
		os.Exit(1)
	}

	location, err := calendarConfig.Location()

	if err != nil {
		fmt.Fprintln(os.Stderr, "caltui: unknown timezone in config:", err)
		os.Exit(1)
	}

	store := calendar.NewMock(time.Now().In(location))

	root := tui.New(store,
		monthview.New(store, monthConfig, location),
		weekview.New(store, weekConfig, location),
		dayview.New(store, dayConfig, location),
		eventform.New(store.Calendars(), location),
		confirm.New(),
		gotodate.New(),
		detail.New(location),
	)

	program := tea.NewProgram(root, tea.WithAltScreen())

	_, err = program.Run()

	if err != nil {
		fmt.Fprintln(os.Stderr, "caltui:", err)
		os.Exit(1)
	}
}
