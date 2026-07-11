package calendar

import "time"

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

	return time.ParseDuration(c.SyncInterval)
}
