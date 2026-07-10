package services

import (
	"path/filepath"
	"testing"
)

// N-133 / N-135: WindowService methods must be safe to call before
// SetWindow is invoked (window is nil) and after SetWindow(nil).
// These tests verify the nil-guard and the new mutex behavior without
// requiring a real *application.WebviewWindow (which can only be created
// by a running Wails app).
func TestWindowService_NilWindowDoesNotPanic(t *testing.T) {
	t.Parallel()
	w := &WindowService{}
	// All methods should be no-ops when window is nil.
	w.Minimise()
	w.Maximise()
	w.Close()
	w.ToggleFullscreen()
	w.SetTitle("test")
	w.OpenAIWindow()
	w.CloseAIWindow()
	w.ToggleAIWindow()
	w.SetAIAlwaysOnTop(true)
	w.SendSelectionToAI("code", "go", "main.go")
	w.FocusMainWindow()
	if w.IsAIWindowOpen() {
		t.Error("expected AI window closed when no app")
	}
	if !w.IsAIAlwaysOnTop() {
		// Default is true after SetApp; without SetApp, zero-value is false.
		// Ensure SetAIAlwaysOnTop works without a window.
		w.SetAIAlwaysOnTop(true)
		if !w.IsAIAlwaysOnTop() {
			t.Error("expected always-on-top true after SetAIAlwaysOnTop")
		}
	}
}

func TestWindowService_SetWindowNilThenCall(t *testing.T) {
	t.Parallel()
	w := &WindowService{}
	w.SetWindow(nil)
	w.SetAIWindow(nil)
	w.Minimise()
	w.Maximise()
	w.Close()
	w.ToggleFullscreen()
	w.SetTitle("test")
	w.CloseAIWindow()
}

func TestWindowService_SetWindowTwice(t *testing.T) {
	t.Parallel()
	w := &WindowService{}
	w.SetWindow(nil)
	w.SetWindow(nil)
	if got := w.currentWindow(); got != nil {
		t.Errorf("expected nil window, got %v", got)
	}
	if got := w.AIWindow(); got != nil {
		t.Errorf("expected nil AI window, got %v", got)
	}
}

// N-133: concurrent SetWindow + method calls must not race.
// Run with -race to verify the RWMutex protects the field.
func TestWindowService_ConcurrentAccessNoRace(t *testing.T) {
	t.Parallel()
	w := &WindowService{}
	done := make(chan struct{})

	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			w.SetWindow(nil)
			w.SetAIWindow(nil)
			w.SetAIAlwaysOnTop(i%2 == 0)
		}
	}()

	for i := 0; i < 200; i++ {
		w.Minimise()
		w.SetTitle("x")
		_ = w.IsAIWindowOpen()
		_ = w.IsAIAlwaysOnTop()
		w.SendSelectionToAI("x", "go", "a.go")
	}

	<-done
}

func TestWindowService_SendSelectionToAI_EmptyCodeNoOp(t *testing.T) {
	t.Parallel()
	w := &WindowService{}
	// Must not panic with empty code even without app.
	w.SendSelectionToAI("", "go", "main.go")
}

func TestWindowService_OpenPathValidation(t *testing.T) {
	t.Parallel()
	w := &WindowService{}
	if err := w.OpenPathInExplorer(""); err == nil {
		t.Error("expected error for empty explorer path")
	}
	if err := w.OpenPathInVSCode(""); err == nil {
		t.Error("expected error for empty vscode path")
	}
	// prompt-5 Task E: relative paths rejected.
	if err := w.OpenPathInExplorer("relative/path"); err == nil {
		t.Error("expected error for relative explorer path")
	}
	if err := w.OpenPathInVSCode("relative/path"); err == nil {
		t.Error("expected error for relative vscode path")
	}
	// Non-existent absolute path rejected (OS-specific abs form).
	missing := filepath.Join(t.TempDir(), "does-not-exist-xyz")
	if err := w.OpenPathInExplorer(missing); err == nil {
		t.Error("expected error for non-existent explorer path")
	}
}

func TestWindowService_DefaultAlwaysOnTopAfterSetApp(t *testing.T) {
	t.Parallel()
	w := &WindowService{}
	w.SetApp(nil) // still sets the default preference
	if !w.IsAIAlwaysOnTop() {
		t.Error("expected default always-on-top true after SetApp")
	}
}
