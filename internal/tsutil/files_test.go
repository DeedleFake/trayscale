package tsutil

import (
	"context"
	"slices"
	"testing"
	"time"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/feature/taildrop"
	"tailscale.com/ipn"
	"tailscale.com/types/empty"
)

// runFiles starts a Bus and f against the fake daemon and returns a
// channel that gets each list f pushes.
func (d *fakeDaemon) runFiles(t *testing.T, f *Files) <-chan *FileStatus {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	pushed := make(chan *FileStatus, 16)
	f.Bus = &Bus{New: func(*IPNStatus) {}}
	f.New = func(s *FileStatus) { pushed <- s }
	go f.Bus.Run(ctx)
	go f.Run(ctx)
	return pushed
}

// setState sends a state on the bus.
func (d *fakeDaemon) setState(state ipn.State) {
	d.notes <- ipn.Notify{State: &state}
}

func nextPush(t *testing.T, pushed <-chan *FileStatus) *FileStatus {
	t.Helper()
	select {
	case s := <-pushed:
		return s
	case <-time.After(3 * time.Second):
		t.Fatal("no list pushed")
		return nil
	}
}

func noPush(t *testing.T, pushed <-chan *FileStatus) {
	t.Helper()
	select {
	case s := <-pushed:
		t.Fatalf("pushed %+v", s)
	case <-time.After(200 * time.Millisecond):
	}
}

func checkPush(t *testing.T, s *FileStatus, files []apitype.WaitingFile, arrived []string) {
	t.Helper()
	if !slices.Equal(s.Files, files) || !slices.Equal(s.Arrived, arrived) {
		t.Fatalf("pushed files %v arrived %q, want %v arrived %q", s.Files, s.Arrived, files, arrived)
	}
}

var (
	fileA  = apitype.WaitingFile{Name: "a.txt", Size: 1}
	fileB  = apitype.WaitingFile{Name: "b.txt", Size: 2}
	jsonA  = `[{"Name":"a.txt","Size":1}]`
	jsonAB = `[{"Name":"a.txt","Size":1},{"Name":"b.txt","Size":2}]`
)

// TestFilesOnlyWhileOnline checks that Files doesn't ask tailscaled for
// the list until Tailscale is online.
func TestFilesOnlyWhileOnline(t *testing.T) {
	d := startFakeDaemon(t)
	pushed := d.runFiles(t, &Files{})

	d.setState(ipn.Stopped)
	if f, _ := d.settle(); f != 0 {
		t.Fatalf("%v files fetches while offline", f)
	}
	noPush(t, pushed)

	d.setState(ipn.Running)
	checkPush(t, nextPush(t, pushed), nil, nil)
}

// TestFilesLongPollWhileEmpty checks that while no files are waiting,
// Files waits for one to arrive instead of asking again and again, and
// reports it as arrived once it does.
func TestFilesLongPollWhileEmpty(t *testing.T) {
	d := startFakeDaemon(t)
	pushed := d.runFiles(t, &Files{Interval: 10 * time.Millisecond})

	d.setState(ipn.Running)
	checkPush(t, nextPush(t, pushed), nil, nil)
	before, _ := d.settle()
	if n := d.longPolls.Load(); n != 1 {
		t.Fatalf("%v long polls after the first list, want 1", n)
	}
	time.Sleep(100 * time.Millisecond)
	if f, _ := d.settle(); f != before {
		t.Fatalf("%v fetches while waiting for a file", f-before)
	}

	d.setWaiting(jsonA)
	checkPush(t, nextPush(t, pushed), []apitype.WaitingFile{fileA}, []string{"a.txt"})
}

// TestFilesPollWhileWaiting checks that while files are waiting, Files
// checks every Interval, pushes only lists that changed, and goes back
// to waiting for a file once none are.
func TestFilesPollWhileWaiting(t *testing.T) {
	d := startFakeDaemon(t)
	d.setWaiting(jsonA)
	pushed := d.runFiles(t, &Files{Interval: 20 * time.Millisecond})

	d.setState(ipn.Running)
	// Already waiting, so not reported as arrived.
	checkPush(t, nextPush(t, pushed), []apitype.WaitingFile{fileA}, nil)
	before := d.files.Load()
	time.Sleep(200 * time.Millisecond)
	if n := d.files.Load() - before; n < 3 {
		t.Fatalf("%v checks in 200ms with a 20ms interval", n)
	}
	if n := d.longPolls.Load(); n != 0 {
		t.Fatalf("%v long polls while files were waiting", n)
	}
	noPush(t, pushed)

	d.setWaiting(jsonAB)
	checkPush(t, nextPush(t, pushed), []apitype.WaitingFile{fileA, fileB}, []string{"b.txt"})

	// Deleted outside of Trayscale.
	d.setWaiting(`[]`)
	checkPush(t, nextPush(t, pushed), nil, nil)
	d.settle()
	if n := d.longPolls.Load(); n != 1 {
		t.Fatalf("%v long polls after the last file went, want 1", n)
	}
}

// TestFilesResetOnSwitch checks that after a profile switch, files that
// were already waiting in the new profile aren't reported as arrived,
// and that the switch stops a wait for a file, which tailscaled
// wouldn't end for a file in the new profile.
func TestFilesResetOnSwitch(t *testing.T) {
	d := startFakeDaemon(t)
	pushed := d.runFiles(t, &Files{})

	d.setState(ipn.Running)
	checkPush(t, nextPush(t, pushed), nil, nil)
	d.settle()

	d.setState(ipn.NoState)
	for d.canceled.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	d.setWaiting(jsonAB)
	d.setState(ipn.Starting)
	if f, _ := d.settle(); f != 2 {
		t.Fatalf("%v files fetches during the switch, want none", f-2)
	}
	d.setState(ipn.Running)
	checkPush(t, nextPush(t, pushed), []apitype.WaitingFile{fileA, fileB}, nil)
}

// TestFilesDiscardsCheckAcrossReset checks that a list fetched while
// the bus reported a reset isn't compared to the list from before it,
// as it could belong to the new profile.
func TestFilesDiscardsCheckAcrossReset(t *testing.T) {
	d := startFakeDaemon(t)
	d.setWaiting(jsonA)
	files := &Files{Interval: time.Hour}
	pushed := d.runFiles(t, files)

	d.setState(ipn.Running)
	checkPush(t, nextPush(t, pushed), []apitype.WaitingFile{fileA}, nil)
	before, _ := d.settle()

	d.holdFiles.Store(before + 1)
	files.Refresh()
	for d.files.Load() == before {
		time.Sleep(5 * time.Millisecond)
	}
	// Another client switches profiles while the check is in flight
	// and the new profile has another file waiting.
	d.setWaiting(jsonAB)
	head := files.Bus.signals()
	d.notes <- ipn.Notify{LoginFinished: &empty.Message{}}
	for files.Bus.signals() == head {
		time.Sleep(5 * time.Millisecond)
	}
	close(d.release)

	checkPush(t, nextPush(t, pushed), []apitype.WaitingFile{fileA, fileB}, nil)
}

// TestFilesRefreshCoalesces checks that refreshes during a check cause
// exactly one more, and that bus signals that don't reset the list
// cause none.
func TestFilesRefreshCoalesces(t *testing.T) {
	d := startFakeDaemon(t)
	d.setWaiting(jsonA)
	files := &Files{Interval: time.Hour}
	pushed := d.runFiles(t, files)

	d.setState(ipn.Running)
	nextPush(t, pushed)
	before, _ := d.settle()

	d.holdFiles.Store(before + 1)
	files.Refresh()
	for d.files.Load() == before {
		time.Sleep(5 * time.Millisecond)
	}
	for range 5 {
		files.Refresh()
		d.notes <- ipn.Notify{Engine: &ipn.EngineStatus{}}
	}
	close(d.release)

	if f, _ := d.settle(); f != before+2 {
		t.Fatalf("%v checks after the held one, want 1", f-before-1)
	}
	noPush(t, pushed)
}

// TestFilesNoTaildrop checks that Files doesn't keep asking tailscaled
// when it has nowhere to keep files, as it would answer at once each
// time.
func TestFilesNoTaildrop(t *testing.T) {
	d := startFakeDaemon(t)
	msg := taildrop.ErrNoTaildrop.Error()
	d.filesErr.Store(&msg)
	pushed := d.runFiles(t, &Files{Interval: 10 * time.Millisecond})

	d.setState(ipn.Running)
	checkPush(t, nextPush(t, pushed), nil, nil)
	time.Sleep(200 * time.Millisecond)
	if f, _ := d.settle(); f != 1 {
		t.Fatalf("%v checks without Taildrop, want 1", f)
	}
}

// TestFilesRetry checks that a failed check is retried with backoff
// until it works.
func TestFilesRetry(t *testing.T) {
	fastRetries(t)
	d := startFakeDaemon(t)
	msg := "busy"
	d.filesErr.Store(&msg)
	pushed := d.runFiles(t, &Files{})

	d.setState(ipn.Running)
	for d.files.Load() < 3 {
		time.Sleep(5 * time.Millisecond)
	}
	noPush(t, pushed)
	d.setWaiting(jsonA)
	d.filesErr.Store(nil)
	checkPush(t, nextPush(t, pushed), []apitype.WaitingFile{fileA}, nil)
}

func TestArrivedFiles(t *testing.T) {
	t.Parallel()

	resized := apitype.WaitingFile{Name: "a.txt", Size: 5}
	for _, tt := range []struct {
		name      string
		prev, cur []apitype.WaitingFile
		want      []string
	}{
		{name: "none"},
		{name: "new", cur: []apitype.WaitingFile{fileA}, want: []string{"a.txt"}},
		{name: "same", prev: []apitype.WaitingFile{fileA}, cur: []apitype.WaitingFile{fileA}},
		{name: "removed", prev: []apitype.WaitingFile{fileA, fileB}, cur: []apitype.WaitingFile{fileB}},
		{name: "added", prev: []apitype.WaitingFile{fileA}, cur: []apitype.WaitingFile{fileA, fileB}, want: []string{"b.txt"}},
		{name: "resized", prev: []apitype.WaitingFile{fileA}, cur: []apitype.WaitingFile{resized}, want: []string{"a.txt"}},
	} {
		if got := arrivedFiles(tt.prev, tt.cur); !slices.Equal(got, tt.want) {
			t.Errorf("%v: arrivedFiles = %q, want %q", tt.name, got, tt.want)
		}
	}
}
