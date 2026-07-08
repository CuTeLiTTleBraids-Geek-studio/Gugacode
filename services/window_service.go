package services

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// WindowService exposes native window controls to the frontend.
// The window reference is injected after app creation in main.go.
//
// N-133: the window field is read by every method and written once at
// startup via SetWindow. Although the write happens before any frontend
// call is possible, a plain unsynchronized read/write pair is technically
// a data race if SetWindow runs concurrently with a method call (which
// can happen during shutdown or hot-reload). We guard the field with a
// sync.RWMutex: setters take a write lock, readers take a read lock.
type WindowService struct {
	mu     sync.RWMutex
	window *application.WebviewWindow
}

// SetWindow injects the active window. Called from main.go after the window is created.
func (w *WindowService) SetWindow(window *application.WebviewWindow) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.window = window
}

// currentWindow returns the window under a read lock. Callers must NOT
// hold the returned pointer beyond the immediate call site — the lock is
// released before the window method is invoked, which is safe because the
// WebviewWindow is reference-counted by the application and only nilled
// out by SetWindow(nil) during shutdown.
func (w *WindowService) currentWindow() *application.WebviewWindow {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.window
}

// Minimise minimises the window.
func (w *WindowService) Minimise() {
	if win := w.currentWindow(); win != nil {
		win.Minimise()
	}
}

// Maximise toggles the maximised state of the window.
func (w *WindowService) Maximise() {
	if win := w.currentWindow(); win != nil {
		win.Maximise()
	}
}

// ToggleMaximise toggles between maximised and restored state.
// 适合前端"最大化/还原"按钮共用一个控件时调用。
func (w *WindowService) ToggleMaximise() {
	if win := w.currentWindow(); win != nil {
		win.ToggleMaximise()
	}
}

// IsMaximised returns whether the window is currently maximised.
// 前端标题栏据此切换图标（放大 ↔ 还原）。
func (w *WindowService) IsMaximised() bool {
	if win := w.currentWindow(); win != nil {
		return win.IsMaximised()
	}
	return false
}

// Close closes the window.
func (w *WindowService) Close() {
	if win := w.currentWindow(); win != nil {
		win.Close()
	}
}

// ToggleFullscreen toggles fullscreen mode.
func (w *WindowService) ToggleFullscreen() {
	if win := w.currentWindow(); win != nil {
		win.ToggleFullscreen()
	}
}

// SetTitle updates the window title bar text.
func (w *WindowService) SetTitle(title string) {
	if win := w.currentWindow(); win != nil {
		win.SetTitle(title)
	}
}
