package main

import "deedles.dev/state"

// CancelGroup provides a simple abstraction to make it simpler to
// cancel multiple listeners simultaneously.
type CancelGroup struct {
	c []state.CancelFunc
}

// Add adds a CancelFunc to the cancel group.
func (g *CancelGroup) Add(f state.CancelFunc) {
	g.c = append(g.c, f)
}

// Cancel cancels all of the added CancelFuncs and resets the list to
// allow it to be reused.
func (g *CancelGroup) Cancel() {
	for _, f := range g.c {
		f()
	}
	g.c = g.c[:0]
}
