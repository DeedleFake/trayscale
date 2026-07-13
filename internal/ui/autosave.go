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

// UniqueSaveName returns a base file name derived from name that is not
// already taken according to taken. If the original base name is free it
// is returned unchanged; otherwise names of the form "stem (n).ext" are
// tried for n = 1, 2, … so a series of same-named files can coexist.
func UniqueSaveName(name string, taken func(string) bool) string {
	base := filepath.Base(name)
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "file"
	}
	if !taken(base) {
		return base
	}

	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem = "file"
	}

	for n := 1; ; n++ {
		candidate := fmt.Sprintf("%s (%d)%s", stem, n, ext)
		if !taken(candidate) {
			return candidate
		}
	}
}

// UniqueSavePath is like AutoSavePath but never returns a path that
// already exists under dir, using UniqueSaveName against the local
// filesystem.
func UniqueSavePath(dir, name string) string {
	return filepath.Join(dir, UniqueSaveName(name, func(base string) bool {
		_, err := os.Stat(filepath.Join(dir, base))
		return err == nil
	}))
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
