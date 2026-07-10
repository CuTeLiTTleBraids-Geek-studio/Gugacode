package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// WindowService exposes native window controls to the frontend.
// It manages both the main editor window and the independent AI companion
// window (prompt-4 Task 1).
//
// N-133: window fields are guarded with a sync.RWMutex so concurrent
// SetWindow / OpenAIWindow / method calls cannot race.
type WindowService struct {
	mu            sync.RWMutex
	window        *application.WebviewWindow // main editor window
	aiWindow      *application.WebviewWindow // independent AI companion window
	app           *application.App           // needed to (re)create the AI window
	aiAlwaysOnTop bool                       // default true (prompt-4 Task 6)
}

// SetApp injects the application handle so OpenAIWindow can create windows.
func (w *WindowService) SetApp(app *application.App) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.app = app
	w.aiAlwaysOnTop = true // prompt-4: 置顶默认开启
}

// SetWindow injects the main window. Called from main.go after creation.
func (w *WindowService) SetWindow(window *application.WebviewWindow) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.window = window
}

// SetAIWindow injects the AI companion window (or nil on close).
func (w *WindowService) SetAIWindow(win *application.WebviewWindow) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.aiWindow = win
}

// AIWindow returns the current AI window handle (may be nil).
func (w *WindowService) AIWindow() *application.WebviewWindow {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.aiWindow
}

// currentWindow returns the main window under a read lock.
func (w *WindowService) currentWindow() *application.WebviewWindow {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.window
}

func (w *WindowService) currentAIWindow() *application.WebviewWindow {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.aiWindow
}

// Minimise minimises the main window.
func (w *WindowService) Minimise() {
	if win := w.currentWindow(); win != nil {
		win.Minimise()
	}
}

// Maximise toggles the maximised state of the main window.
func (w *WindowService) Maximise() {
	if win := w.currentWindow(); win != nil {
		win.Maximise()
	}
}

// ToggleMaximise toggles between maximised and restored state.
func (w *WindowService) ToggleMaximise() {
	if win := w.currentWindow(); win != nil {
		win.ToggleMaximise()
	}
}

// IsMaximised returns whether the main window is currently maximised.
func (w *WindowService) IsMaximised() bool {
	if win := w.currentWindow(); win != nil {
		return win.IsMaximised()
	}
	return false
}

// Close closes the main window (which should also tear down the AI window).
func (w *WindowService) Close() {
	// Close AI first so it does not outlive the main process lifecycle.
	w.CloseAIWindow()
	if win := w.currentWindow(); win != nil {
		win.Close()
	}
}

// ToggleFullscreen toggles fullscreen mode on the main window.
func (w *WindowService) ToggleFullscreen() {
	if win := w.currentWindow(); win != nil {
		win.ToggleFullscreen()
	}
}

// SetTitle updates the main window title bar text.
func (w *WindowService) SetTitle(title string) {
	if win := w.currentWindow(); win != nil {
		win.SetTitle(title)
	}
}

// ---- AI companion window (prompt-4 Task 1 / 5 / 6) ----

// createAIWindowLocked creates a new AI WebviewWindow. Caller must hold w.mu write lock
// OR call this only when no concurrent access is possible. We take no lock here —
// OpenAIWindow holds the write lock around the full create+assign path.
func (w *WindowService) createAIWindowLocked() *application.WebviewWindow {
	if w.app == nil {
		return nil
	}
	alwaysOnTop := w.aiAlwaysOnTop
	aiWin := w.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:        "ai",
		Title:       "gugacode AI",
		Width:       1200,
		Height:      780,
		MinWidth:    900,
		MinHeight:   560,
		Frameless:   false, // keep native title bar for independent drag/resize
		AlwaysOnTop: alwaysOnTop,
		// Hash-router SPA: load with the /ai-window hash so the companion
		// view mounts without reusing the main-window layout.
		URL:              "/#/ai-window",
		BackgroundColour: application.NewRGB(6, 7, 15),
	})
	// When the user closes the AI window, clear our handle so OpenAIWindow
	// can recreate it later.
	aiWin.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		w.mu.Lock()
		if w.aiWindow == aiWin {
			w.aiWindow = nil
		}
		w.mu.Unlock()
	})
	return aiWin
}

// OpenAIWindow creates the AI companion window if missing/closed, otherwise focuses it.
func (w *WindowService) OpenAIWindow() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.aiWindow != nil {
		// Already open — show + focus.
		w.aiWindow.Show()
		w.aiWindow.UnMinimise()
		w.aiWindow.Focus()
		return
	}
	aiWin := w.createAIWindowLocked()
	if aiWin == nil {
		return
	}
	w.aiWindow = aiWin
	aiWin.Show()
	aiWin.Focus()
}

// CloseAIWindow closes the AI companion window if it exists.
func (w *WindowService) CloseAIWindow() {
	w.mu.Lock()
	aiWin := w.aiWindow
	w.aiWindow = nil
	w.mu.Unlock()
	if aiWin != nil {
		aiWin.Close()
	}
}

// ToggleAIWindow shows the AI window if hidden/closed, or hides it if visible.
func (w *WindowService) ToggleAIWindow() {
	w.mu.Lock()
	aiWin := w.aiWindow
	app := w.app
	_ = app
	w.mu.Unlock()

	if aiWin == nil {
		w.OpenAIWindow()
		return
	}
	if aiWin.IsVisible() {
		aiWin.Hide()
		return
	}
	aiWin.Show()
	aiWin.UnMinimise()
	aiWin.Focus()
}

// IsAIWindowOpen reports whether the AI companion window currently exists.
func (w *WindowService) IsAIWindowOpen() bool {
	return w.currentAIWindow() != nil
}

// IsAIWindowVisible reports whether the AI companion window currently exists
// and is visible. A hidden window remains open but must not keep the editor's
// activity-bar AI item highlighted.
func (w *WindowService) IsAIWindowVisible() bool {
	w.mu.RLock()
	aiWin := w.aiWindow
	w.mu.RUnlock()
	return aiWin != nil && aiWin.IsVisible()
}

// SetAIAlwaysOnTop enables/disables always-on-top for the AI window (prompt-4 Task 6).
// Default is true. Persists the preference for future recreations.
func (w *WindowService) SetAIAlwaysOnTop(onTop bool) {
	w.mu.Lock()
	w.aiAlwaysOnTop = onTop
	aiWin := w.aiWindow
	w.mu.Unlock()
	if aiWin != nil {
		aiWin.SetAlwaysOnTop(onTop)
	}
}

// IsAIAlwaysOnTop returns the current always-on-top preference.
func (w *WindowService) IsAIAlwaysOnTop() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.aiAlwaysOnTop
}

// SelectionPayload is the payload for SendSelectionToAI / ai:selection events.
type SelectionPayload struct {
	Code     string `json:"code"`
	Language string `json:"language"`
	FilePath string `json:"filePath"`
}

// SendSelectionToAI opens (or focuses) the AI window and emits an "ai:selection"
// event so the AI window can inject a code-context user message (prompt-4 Task 5).
func (w *WindowService) SendSelectionToAI(code string, language string, filePath string) {
	if code == "" {
		return
	}
	// Ensure the AI window is open and focused first.
	w.OpenAIWindow()

	w.mu.RLock()
	app := w.app
	w.mu.RUnlock()
	if app == nil {
		return
	}
	app.Event.Emit("ai:selection", map[string]string{
		"code":     code,
		"language": language,
		"filePath": filePath,
	})
}

// validateOpenPath rejects empty, relative, or non-existent paths before
// handing them to OS launchers (prompt-5 Task E / BUG-M6).
func validateOpenPath(path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute: %s", path)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("path does not exist: %w", err)
	}
	return nil
}

// OpenPathInExplorer opens the given path in the OS file manager
// (Windows: explorer, macOS: open, Linux: xdg-open).
func (w *WindowService) OpenPathInExplorer(path string) error {
	if err := validateOpenPath(path); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open explorer: %w", err)
	}
	// Reap to avoid zombies (N-105 pattern).
	go func() { _ = cmd.Wait() }()
	return nil
}

// OpenPathInVSCode opens the given path in VS Code via the `code` CLI.
func (w *WindowService) OpenPathInVSCode(path string) error {
	if err := validateOpenPath(path); err != nil {
		return err
	}
	cmd := exec.Command("code", path)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open vscode: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// FocusMainWindow brings the main editor window to the front.
func (w *WindowService) FocusMainWindow() {
	if win := w.currentWindow(); win != nil {
		win.Show()
		win.UnMinimise()
		win.Focus()
	}
}
