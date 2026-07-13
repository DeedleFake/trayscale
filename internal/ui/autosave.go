package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tailscale.com/client/tailscale/apitype"
)

// AutoSaveEnabled reports whether automatic Taildrop saving should run
// given the enable flag and destination directory from settings.
func AutoSaveEnabled(enabled bool, dir string) bool {
	return enabled && strings.TrimSpace(dir) != ""
}

// AutoSaveDirOK reports whether dir exists and is a directory suitable
// as an auto-save destination. A missing or non-directory path is an
// error so callers can skip retries instead of spamming failed writes.
func AutoSaveDirOK(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("not a directory")
	}
	return nil
}

// AutoSavePath returns the destination filesystem path for a waiting
// file name under dir. The base name is used so path components in the
// waiting-file name cannot escape dir.
func AutoSavePath(dir, name string) string {
	return filepath.Join(dir, filepath.Base(name))
}

// FilesToAutoSave returns the names of waiting files that should be
// auto-saved. Names present in skip (in-flight or previously failed for
// this wait cycle) are omitted to avoid concurrent or repeated saves.
func FilesToAutoSave(enabled bool, dir string, files []apitype.WaitingFile, skip map[string]bool) []string {
	if !AutoSaveEnabled(enabled, dir) {
		return nil
	}

	var names []string
	for _, f := range files {
		if skip[f.Name] {
			continue
		}
		names = append(names, f.Name)
	}
	return names
}
