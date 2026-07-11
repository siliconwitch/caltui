package main

import (
	"fmt"
	"os"
	"time"
	_ "time/tzdata"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/config"
	"github.com/siliconwitch/caltui/tui"
	"github.com/siliconwitch/caltui/widgets/agenda"
	"github.com/siliconwitch/caltui/widgets/confirm"
	"github.com/siliconwitch/caltui/widgets/dayview"
	"github.com/siliconwitch/caltui/widgets/detail"
	"github.com/siliconwitch/caltui/widgets/errorpopup"
	"github.com/siliconwitch/caltui/widgets/eventform"
	"github.com/siliconwitch/caltui/widgets/gotodate"
	"github.com/siliconwitch/caltui/widgets/monthview"
	"github.com/siliconwitch/caltui/widgets/scopepicker"
	"github.com/siliconwitch/caltui/widgets/search"
	"github.com/siliconwitch/caltui/widgets/weekview"
)

func main() {
	monthConfig := monthview.DefaultConfig()
	weekConfig := weekview.DefaultConfig()
	dayConfig := dayview.DefaultConfig()
	agendaConfig := agenda.DefaultConfig()
	calendarConfig := calendar.DefaultConfig()

	var accounts []calendar.Account

	err := config.Load(map[string]any{
		monthview.ConfigSection:  &monthConfig,
		weekview.ConfigSection:   &weekConfig,
		dayview.ConfigSection:    &dayConfig,
		agenda.ConfigSection:     &agendaConfig,
		calendar.ConfigSection:   &calendarConfig,
		calendar.AccountsSection: &accounts,
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

	syncInterval, err := calendarConfig.RefreshInterval()

	if err != nil {
		fmt.Fprintln(os.Stderr, "caltui: invalid sync_interval in config:", err)
		os.Exit(1)
	}

	var store calendar.Store

	if len(accounts) == 0 {
		store = calendar.NewMock(time.Now().In(location))
	} else {
		store, err = calendar.NewRemote(accounts, calendarConfig.Colors, location, time.Now().In(location))

		if err != nil {
			fmt.Fprintln(os.Stderr, "caltui:", err)
			os.Exit(1)
		}
	}

	root := tui.New(store, syncInterval,
		monthview.New(store, monthConfig, location),
		weekview.New(store, weekConfig, location),
		dayview.New(store, dayConfig, location),
		agenda.New(store, agendaConfig, location),
		eventform.New(calendar.WritableCalendars(store), location),
		confirm.New(),
		gotodate.New(),
		detail.New(location),
		errorpopup.New(),
		scopepicker.New(),
		search.New(store, location),
	)

	program := tea.NewProgram(root, tea.WithAltScreen())

	_, err = program.Run()

	if err != nil {
		fmt.Fprintln(os.Stderr, "caltui:", err)
		os.Exit(1)
	}
}
