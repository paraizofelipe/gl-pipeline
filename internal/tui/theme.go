package tui

import (
	tint "github.com/lrstanley/bubbletint/v2"
)

// DefaultThemeID is the theme used when neither --theme nor GL_PIPELINE_THEME
// is provided.
const DefaultThemeID = "gruvbox_dark"

// AvailableThemes returns every registered bubbletint id, sorted.
func AvailableThemes() []string {
	tint.NewDefaultRegistry()
	return tint.TintIDs()
}

// IsValidTheme reports whether id is a known bubbletint theme id.
func IsValidTheme(id string) bool {
	tint.NewDefaultRegistry()
	for _, t := range tint.TintIDs() {
		if t == id {
			return true
		}
	}
	return false
}
