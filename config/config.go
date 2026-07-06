package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

func Load(sections map[string]any) error {
	path := os.Getenv("CALTUI_CONFIG")
	if path == "" {
		configDir, err := os.UserConfigDir()

		if err != nil {
			return err
		}

		path = filepath.Join(configDir, "caltui", "config.toml")
	}

	var raw map[string]toml.Primitive

	meta, err := toml.DecodeFile(path, &raw)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	}

	for name, target := range sections {
		primitive, ok := raw[name]
		if !ok {
			continue
		}

		if err := meta.PrimitiveDecode(primitive, target); err != nil {
			return err
		}
	}

	return nil
}
