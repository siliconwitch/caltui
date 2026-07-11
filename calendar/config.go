package calendar

import "time"

const ConfigSection = "calendar"

type Config struct {
	Timezone string            `toml:"timezone"`
	Colors   map[string]string `toml:"colors"`
}

func DefaultConfig() Config {
	return Config{Timezone: "", Colors: map[string]string{}}
}

func (c Config) Location() (*time.Location, error) {
	if c.Timezone == "" {
		return time.Local, nil
	}

	return time.LoadLocation(c.Timezone)
}
