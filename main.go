package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
	_ "time/tzdata"

	"github.com/BurntSushi/toml"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/siliconwitch/caltui/calendar"
	"github.com/siliconwitch/caltui/tui"
	"github.com/siliconwitch/caltui/widgets/agenda"
	"github.com/siliconwitch/caltui/widgets/alertpopup"
	"github.com/siliconwitch/caltui/widgets/calendars"
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

	sections := map[string]any{
		monthview.ConfigSection:  &monthConfig,
		weekview.ConfigSection:   &weekConfig,
		dayview.ConfigSection:    &dayConfig,
		agenda.ConfigSection:     &agendaConfig,
		calendar.ConfigSection:   &calendarConfig,
		calendar.AccountsSection: &accounts,
	}

	configPath := os.Getenv("CALTUI_CONFIG")
	if configPath == "" {
		configDir, err := os.UserConfigDir()

		if err != nil {
			fmt.Fprintln(os.Stderr, "caltui: reading config:", err)
			os.Exit(1)
		}

		configPath = filepath.Join(configDir, "caltui", "config.toml")
	}

	var raw map[string]toml.Primitive

	metadata, err := toml.DecodeFile(configPath, &raw)

	switch {
	case errors.Is(err, fs.ErrNotExist):

	case err != nil:
		fmt.Fprintln(os.Stderr, "caltui: reading config:", err)
		os.Exit(1)
	}

	for name, target := range sections {
		primitive, ok := raw[name]
		if !ok {
			continue
		}

		if err := metadata.PrimitiveDecode(primitive, target); err != nil {
			fmt.Fprintln(os.Stderr, "caltui: reading config:", err)
			os.Exit(1)
		}
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
		store, err = calendar.NewRemote(accounts, calendarConfig.Colors, location)

		if err != nil {
			fmt.Fprintln(os.Stderr, "caltui:", err)
			os.Exit(1)
		}
	}

	visible := calendar.NewVisible(store)

	root := tui.New(store, syncInterval,
		monthview.New(visible, monthConfig, location),
		weekview.New(visible, weekConfig, location),
		dayview.New(visible, dayConfig, location),
		agenda.New(visible, agendaConfig, location),
		eventform.New(calendar.WritableCalendars(store), location),
		confirm.Model{},
		gotodate.Model{},
		detail.New(location),
		errorpopup.Model{},
		scopepicker.Model{},
		search.New(visible, location),
		calendars.New(visible),
		alertpopup.New(visible, location),
	)

	program := tea.NewProgram(root, tea.WithAltScreen())

	_, err = program.Run()

	if err != nil {
		fmt.Fprintln(os.Stderr, "caltui:", err)
		os.Exit(1)
	}
}
