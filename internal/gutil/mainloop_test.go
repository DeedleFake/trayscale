package gutil_test

import (
	"slices"
	"sync"
	"testing"

	"deedles.dev/trayscale/internal/gutil"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

func TestRunOnMain(t *testing.T) {
	var got []int
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := range 5 {
			gutil.RunOnMain(func() {
				got = append(got, i)
				if i == 2 {
					// Queued while draining: runs after the rest.
					gutil.RunOnMain(func() { got = append(got, 100) })
				}
			})
		}
	})
	wg.Wait()

	ctx := glib.MainContextDefault()
	for ctx.Iteration(false) {
	}

	want := []int{0, 1, 2, 3, 4, 100}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
