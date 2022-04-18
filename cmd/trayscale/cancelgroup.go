package main

import "deedles.dev/state"

type CancelGroup struct {
	c []state.CancelFunc
}

func (g *CancelGroup) Add(f state.CancelFunc) {
	g.c = append(g.c, f)
}

func (g *CancelGroup) Cancel() {
	for _, f := range g.c {
		f()
	}
	g.c = g.c[:0]
}
