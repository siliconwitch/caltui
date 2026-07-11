package calendar

import (
	"fmt"
	"time"
)

const ConfigSection = "calendar"

type Config struct {
	Timezone     string            `toml:"timezone"`
	SyncInterval string            `toml:"sync_interval"`
	Colors       map[string]string `toml:"colors"`
}

func DefaultConfig() Config {
	return Config{Timezone: "", SyncInterval: "15m", Colors: map[string]string{}}
}

func (c Config) Location() (*time.Location, error) {
	if c.Timezone == "" {
		return time.Local, nil
	}

	return time.LoadLocation(c.Timezone)
}

func (c Config) RefreshInterval() (time.Duration, error) {
	if c.SyncInterval == "" {
		return 0, nil
	}

	interval, err := time.ParseDuration(c.SyncInterval)

	if err != nil {
		return 0, err
	}

	if interval != 0 && interval < time.Minute {
		return 0, fmt.Errorf("sync_interval %q is below the 1m minimum (use \"0\" to disable)", c.SyncInterval)
	}

	return interval, nil
}
