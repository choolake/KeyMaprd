package mapper

import "fmt"

// Mapper looks up button actions from a loaded Config.
type Mapper struct {
	config *Config
}

// New creates a Mapper from a loaded Config.
func New(cfg *Config) *Mapper {
	return &Mapper{config: cfg}
}

// Lookup takes a button number (from a CGEventTap mouse event) and returns
// the configured action string (e.g. "cmd+space"), or an empty string if
// no mapping exists for that button.
//
// Config keys use the format "btn0", "btn1", "btn3", etc.
// Use --dump to see which button number each physical button reports.
func (m *Mapper) Lookup(button uint8) string {
	key := fmt.Sprintf("btn%d", button)
	return m.config.Buttons[key]
}
