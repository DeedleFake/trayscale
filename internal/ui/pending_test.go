package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn"
)

var (
	pendingA = apitype.WaitingFile{Name: "a.txt", Size: 1}
	pendingB = apitype.WaitingFile{Name: "b.txt", Size: 2}
)

func TestPendingDeletesPath(t *testing.T) {
	t.Setenv("HOME", "/home/u")

	t.Setenv("XDG_STATE_HOME", "/state")
	require.Equal(t, "/state/trayscale/pending-deletes.json", pendingDeletesPath())

	t.Setenv("XDG_STATE_HOME", "")
	require.Equal(t, "/home/u/.local/state/trayscale/pending-deletes.json", pendingDeletesPath())

	t.Setenv("XDG_STATE_HOME", "relative")
	require.Equal(t, "/home/u/.local/state/trayscale/pending-deletes.json", pendingDeletesPath(), "a relative XDG_STATE_HOME wasn't ignored")
}

// TestPendingDeletesTake checks that take returns only the profile's
// files that are still waiting with the same name and size, and forgets
// all of that profile's files either way.
func TestPendingDeletesTake(t *testing.T) {
	var p pendingDeletes
	p.add("pA", pendingA)
	p.add("pA", pendingA)
	p.add("pA", pendingB)
	p.add("pB", pendingA)

	resent := apitype.WaitingFile{Name: "b.txt", Size: 3}
	require.Equal(t, []apitype.WaitingFile{pendingA}, p.take("pA", []apitype.WaitingFile{pendingA, resent}))
	require.Empty(t, p.take("pA", []apitype.WaitingFile{pendingA}), "take kept a profile's files after taking them")
	require.Equal(t, []apitype.WaitingFile{pendingA}, p.take("pB", []apitype.WaitingFile{pendingA}))
}

// TestPendingDeletesPrune checks that prune forgets the files of
// profiles that are gone.
func TestPendingDeletesPrune(t *testing.T) {
	var p pendingDeletes
	p.add("pA", pendingA)
	p.add("pB", pendingB)

	p.prune([]ipn.LoginProfile{{ID: "pB"}})
	require.Empty(t, p.take("pA", []apitype.WaitingFile{pendingA}))
	require.Equal(t, []apitype.WaitingFile{pendingB}, p.take("pB", []apitype.WaitingFile{pendingB}))
}

// TestPendingDeletesPersist checks that the list lasts across loads and
// that each change is saved.
func TestPendingDeletesPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "trayscale", "pending-deletes.json")

	p := loadPendingDeletes(path)
	p.add("pA", pendingA)
	p.add("pB", pendingB)

	p = loadPendingDeletes(path)
	require.Equal(t, []apitype.WaitingFile{pendingA}, p.take("pA", []apitype.WaitingFile{pendingA}))

	p = loadPendingDeletes(path)
	require.Empty(t, p.take("pA", []apitype.WaitingFile{pendingA}), "a take wasn't saved")
	p.prune(nil)

	p = loadPendingDeletes(path)
	require.Empty(t, p.take("pB", []apitype.WaitingFile{pendingB}), "a prune wasn't saved")

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1, "a temporary file was left behind")
}

// TestPendingDeletesFallback checks that the list works in memory when
// the state file can't be read or written.
func TestPendingDeletesFallback(t *testing.T) {
	dir := t.TempDir()

	corrupt := filepath.Join(dir, "corrupt.json")
	require.NoError(t, os.WriteFile(corrupt, []byte("{"), 0o600))
	p := loadPendingDeletes(corrupt)
	require.Empty(t, p.list)
	p.add("pA", pendingA)
	p = loadPendingDeletes(corrupt)
	require.Equal(t, []apitype.WaitingFile{pendingA}, p.take("pA", []apitype.WaitingFile{pendingA}), "a corrupt file wasn't replaced")

	// A regular file where the directory should be.
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	p = loadPendingDeletes(filepath.Join(blocker, "pending-deletes.json"))
	p.add("pA", pendingA)
	require.True(t, p.saveFailed)
	require.Equal(t, []apitype.WaitingFile{pendingA}, p.take("pA", []apitype.WaitingFile{pendingA}), "the list wasn't kept in memory")
}
