package ui

import (
	"encoding/json/v2"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn"
)

// pendingDelete is a waiting file that was saved while another profile
// was current. LocalAPI can only delete the current profile's files,
// so it stays waiting until its profile is current again.
type pendingDelete struct {
	Profile ipn.ProfileID
	File    apitype.WaitingFile
}

// pendingDeletes is the list of files to delete once their profile is
// current again. It's kept in a state file, so that it lasts until
// Trayscale next runs on that profile. If the file can't be read or
// written, the list is kept in memory only. The zero value keeps it in
// memory only. Only the main thread uses it.
type pendingDeletes struct {
	path       string // the state file, or "" to keep the list in memory only
	list       []pendingDelete
	saveFailed bool // a save failed and was logged, so a repeat isn't logged
}

// pendingDeletesPath returns where to keep the pending deletes, under
// $XDG_STATE_HOME, or "" if neither it nor the home directory is known.
func pendingDeletesPath() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(dir) {
		// The XDG spec says to ignore a relative path.
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "trayscale", "pending-deletes.json")
}

// loadPendingDeletes reads the pending deletes kept at path. A missing
// or unreadable file gives an empty list.
func loadPendingDeletes(path string) pendingDeletes {
	p := pendingDeletes{path: path}
	if path == "" {
		return p
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Error("read pending deletes", "path", path, "err", err)
		}
		return p
	}
	if err := json.Unmarshal(data, &p.list); err != nil {
		slog.Error("read pending deletes", "path", path, "err", err)
		p.list = nil
	}
	return p
}

// add remembers to delete profile's file once profile is current.
func (p *pendingDeletes) add(profile ipn.ProfileID, file apitype.WaitingFile) {
	d := pendingDelete{Profile: profile, File: file}
	if slices.Contains(p.list, d) {
		return
	}
	p.list = append(p.list, d)
	p.save()
}

// take forgets every file to delete for profile and returns the ones
// that are in waiting, profile's waiting files, with the same name and
// size. Matching the size as well keeps a file sent again under that
// name from being deleted unsaved. One that isn't waiting was taken
// some other way.
func (p *pendingDeletes) take(profile ipn.ProfileID, waiting []apitype.WaitingFile) []apitype.WaitingFile {
	var files []apitype.WaitingFile
	n := len(p.list)
	p.list = slices.DeleteFunc(p.list, func(d pendingDelete) bool {
		if d.Profile != profile {
			return false
		}
		if slices.Contains(waiting, d.File) {
			files = append(files, d.File)
		}
		return true
	})
	if len(p.list) != n {
		p.save()
	}
	return files
}

// prune forgets the files to delete for profiles that aren't in
// profiles, as their files are gone with them.
func (p *pendingDeletes) prune(profiles []ipn.LoginProfile) {
	n := len(p.list)
	p.list = slices.DeleteFunc(p.list, func(d pendingDelete) bool {
		return !slices.ContainsFunc(profiles, func(l ipn.LoginProfile) bool { return l.ID == d.Profile })
	})
	if len(p.list) != n {
		p.save()
	}
}

// save writes the list to the state file. A failure is logged once
// until a save works, and the list stays in memory.
func (p *pendingDeletes) save() {
	if p.path == "" {
		return
	}
	data, err := json.Marshal(p.list)
	if err == nil {
		err = writeFileAtomic(p.path, data)
	}
	if err != nil {
		if !p.saveFailed {
			slog.Error("save pending deletes", "path", p.path, "err", err)
		}
		p.saveFailed = true
		return
	}
	p.saveFailed = false
}

// writeFileAtomic replaces the file at path with data, so that a crash
// leaves either the old file or the new one.
func writeFileAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
