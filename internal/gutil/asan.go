//go:build asan && cgo

package gutil

// static void trayscale_asan_noop(void) {}
import "C"

// The Go runtime only runs LeakSanitizer at exit if the program has
// made a cgo call, and calls through purego don't count. Make one so
// that -asan builds always check for leaked C memory, such as
// unreleased GObjects and strings, at exit.
func init() {
	C.trayscale_asan_noop()
}
