package gutil_test

import (
	"os"
	"runtime"
	"testing"
)

// TestMain collects garbage after the tests so that stale Go pointers
// to C objects don't hide leaks from go test -asan's check at exit.
func TestMain(m *testing.M) {
	code := m.Run()
	runtime.GC()
	os.Exit(code)
}
