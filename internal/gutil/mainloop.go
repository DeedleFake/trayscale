package gutil

import (
	"sync"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

var (
	mainQueueM sync.Mutex
	mainQueue  []func()
)

// RunOnMain arranges for f to run on the GLib main loop, after any
// functions queued before it. It is safe to call from any goroutine.
//
// All of the app's main-loop hand-offs go through this one function
// so that a single idle source drains a Go-side queue, rather than
// each call registering its own C callback.
func RunOnMain(f func()) {
	mainQueueM.Lock()
	first := len(mainQueue) == 0
	mainQueue = append(mainQueue, f)
	mainQueueM.Unlock()

	if first {
		glib.IdleAdd(runMainQueue)
	}
}

func runMainQueue() {
	mainQueueM.Lock()
	q := mainQueue
	mainQueue = nil
	mainQueueM.Unlock()

	for _, f := range q {
		f()
	}
}
