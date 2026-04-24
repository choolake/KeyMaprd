package mapper

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the full contents of config.json.
// Example config.json:
//
//	{
//	  "buttons": {
//	    "btn3": "cmd+space",
//	    "btn4": "cmd+tab",
//	    "btn8": "mission_control"
//	  }
//	}
//
// Keys are button numbers discovered via --dump (e.g. "btn3").
// Values are the action to perform (a key combo or named action).
type Config struct {
	// Buttons maps a button number key (e.g. "btn3") to an action string.
	Buttons map[string]string `json:"buttons"`
}

// DefaultConfigPath returns the default location for config.json.
// Uses ~/.config/keymaprd/config.json — standard XDG-style user config.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(home, ".config", "keymaprd", "config.json")
}

// LoadConfig reads and parses the config file at the given path.
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %q: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	if cfg.Buttons == nil {
		cfg.Buttons = make(map[string]string)
	}
	return &cfg, nil
}
