package services

import (
	"testing"
)

// N-133 / N-135: WindowService methods must be safe to call before
// SetWindow is invoked (window is nil) and after SetWindow(nil).
// These tests verify the nil-guard and the new mutex behavior without
// requiring a real *application.WebviewWindow (which can only be created
// by a running Wails app).
func TestWindowService_NilWindowDoesNotPanic(t *testing.T) {
	t.Parallel() // N-137: pure unit test, no shared state
	w := &WindowService{}
	// All methods should be no-ops when window is nil.
	w.Minimise()
	w.Maximise()
	w.Close()
	w.ToggleFullscreen()
	w.SetTitle("test")
}

func TestWindowService_SetWindowNilThenCall(t *testing.T) {
	t.Parallel() // N-137: pure unit test, no shared state
	w := &WindowService{}
	w.SetWindow(nil)
	// Calling methods after SetWindow(nil) must not panic.
	w.Minimise()
	w.Maximise()
	w.Close()
	w.ToggleFullscreen()
	w.SetTitle("test")
}

func TestWindowService_SetWindowTwice(t *testing.T) {
	t.Parallel() // N-137: pure unit test, no shared state
	w := &WindowService{}
	w.SetWindow(nil)
	w.SetWindow(nil)
	// Verify currentWindow returns nil after nil set.
	if got := w.currentWindow(); got != nil {
		t.Errorf("expected nil window, got %v", got)
	}
}

// N-133: concurrent SetWindow + method calls must not race.
// Run with -race to verify the RWMutex protects the field.
func TestWindowService_ConcurrentAccessNoRace(t *testing.T) {
	t.Parallel() // N-137: pure unit test, no shared state
	w := &WindowService{}
	done := make(chan struct{})

	// Writer goroutine: toggles the window field between nil and nil
	// (we can't construct a real WebviewWindow in a unit test, but the
	// mutex behavior is the same regardless of the value being written).
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			w.SetWindow(nil)
		}
	}()

	// Reader goroutine: calls methods that read the window field.
	for i := 0; i < 200; i++ {
		w.Minimise()
		w.SetTitle("x")
	}

	<-done
}
