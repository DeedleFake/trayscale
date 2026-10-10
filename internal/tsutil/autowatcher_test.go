package tsutil

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Concurrent evaluations must not overlap. Before the evalMu fix, callers
// could interleave inside evaluateCurrent while another was blocked reading
// the poller, letting both pass the dedup check and both act.
//
// A real Poller with nothing feeding it gives a genuinely blocking GetIPN(),
// which is the window the old code raced in.
func TestEvaluateCurrentIsSerialised(t *testing.T) {
	var inFlight, maxInFlight, entered int32

	w := &AutoWatcher{poller: &Poller{}}
	w.IsEnabled = func() bool { return true }
	w.TrustedList = func() []string { return nil }
	w.OnSSIDChange = func(string) {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxInFlight)
			if n <= old || atomic.CompareAndSwapInt32(&maxInFlight, old, n) {
				break
			}
		}
		atomic.AddInt32(&entered, 1)
		time.Sleep(30 * time.Millisecond) // widen the race window
		atomic.AddInt32(&inFlight, -1)
	}

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.evaluateCurrent(ctx)
		}()
	}

	// Let one evaluation take evalMu and park on the poller read, then
	// cancel so every goroutine can unwind.
	time.Sleep(300 * time.Millisecond)
	cancel()
	wg.Wait()

	if got := atomic.LoadInt32(&maxInFlight); got > 1 {
		t.Errorf("evaluations overlapped: max concurrent = %d, want 1", got)
	}
	if atomic.LoadInt32(&entered) == 0 {
		t.Fatal("no evaluation ran; test proved nothing")
	}
	t.Logf("entered=%d maxConcurrent=%d", atomic.LoadInt32(&entered), atomic.LoadInt32(&maxInFlight))
}

// A stopped watcher must not keep firing callbacks into UI code the app has
// already torn down.
func TestNoCallbacksAfterStop(t *testing.T) {
	var after int32

	w := &AutoWatcher{poller: &Poller{}}
	w.IsEnabled = func() bool { return true }
	w.TrustedList = func() []string { return nil }
	w.OnSSIDChange = func(string) { atomic.AddInt32(&after, 1) }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.cancel = cancel

	w.Stop()

	for i := 0; i < 20; i++ {
		w.evaluateCurrent(ctx)
	}

	if got := atomic.LoadInt32(&after); got != 0 {
		t.Errorf("got %d callbacks after Stop(), want 0", got)
	}
	t.Logf("callbacks after Stop = %d", atomic.LoadInt32(&after))
}

// Stop() must not block behind an evaluation that is holding evalMu.
func TestStopDoesNotDeadlock(t *testing.T) {
	w := &AutoWatcher{poller: &Poller{}}
	w.IsEnabled = func() bool { return true }
	w.TrustedList = func() []string { return nil }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.cancel = cancel

	entered := make(chan struct{})
	var once sync.Once
	w.OnSSIDChange = func(string) { once.Do(func() { close(entered) }) }

	go w.evaluateCurrent(ctx) // parks on the poller read holding evalMu

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("evaluation never started")
	}

	done := make(chan struct{})
	go func() { w.Stop(); close(done) }()

	select {
	case <-done:
		t.Log("Stop() returned while an evaluation held evalMu")
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() deadlocked behind a running evaluation")
	}
}
