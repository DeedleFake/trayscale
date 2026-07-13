package ui

import (
	"path/filepath"
	"strings"

	"tailscale.com/client/tailscale/apitype"
)

// AutoSaveEnabled reports whether automatic Taildrop saving should run
// given the enable flag and destination directory from settings.
func AutoSaveEnabled(enabled bool, dir string) bool {
	return enabled && strings.TrimSpace(dir) != ""
}

// AutoSavePath returns the destination filesystem path for a waiting
// file name under dir. The base name is used so path components in the
// waiting-file name cannot escape dir.
func AutoSavePath(dir, name string) string {
	return filepath.Join(dir, filepath.Base(name))
}

// FilesToAutoSave returns the names of waiting files that should be
// auto-saved. Files whose names are already in inFlight are skipped to
// avoid concurrent double-saves of the same waiting file.
func FilesToAutoSave(enabled bool, dir string, files []apitype.WaitingFile, inFlight map[string]bool) []string {
	if !AutoSaveEnabled(enabled, dir) {
		return nil
	}

	var names []string
	for _, f := range files {
		if inFlight[f.Name] {
			continue
		}
		names = append(names, f.Name)
	}
	return names
}
