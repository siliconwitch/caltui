package calendar

import "time"

const ConfigSection = "calendar"

type Config struct {
	Timezone string `toml:"timezone"`
}

func DefaultConfig() Config {
	return Config{Timezone: ""}
}

func (c Config) Location() (*time.Location, error) {
	if c.Timezone == "" {
		return time.Local, nil
	}

	return time.LoadLocation(c.Timezone)
}
