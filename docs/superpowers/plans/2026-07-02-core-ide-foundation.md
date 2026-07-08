# Core IDE Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform the gugacode UI shells into a working desktop IDE that can open folders, browse files, edit code with Monaco, manage tabs, persist settings, and control the native window.

**Architecture:** Wails v3 (Go backend) + Vue 3 (TypeScript frontend). Go services expose file-system, project, settings, and window operations to the frontend via auto-generated bindings. The frontend wires existing UI shells to these services and integrates the Monaco editor. State is managed via Vue reactive stores.

**Tech Stack:** Go 1.25, Wails v3 (alpha2.111), Vue 3, TypeScript, Vite, Element Plus, Tailwind CSS v4, Monaco Editor, Vitest, `github.com/adrg/xdg`

**Project root:** `e:\gugacode\gugacode\gugacode\` (the directory containing `go.mod`, `main.go`, `frontend/`). All relative paths in this plan are from this root.

**Module name note:** `go.mod` declares `module changeme`. Generated bindings land in `frontend/bindings/changeme/`. This plan uses that path as-is. Renaming the module is out of scope but recommended later.

---

## Scope Check

This project's full scope spans **four independent subsystems**. Per the writing-plans skill, each should be its own plan. This document covers **Plan 1 only**:

| Plan | Subsystem | Status |
|------|-----------|--------|
| **Plan 1 (this doc)** | Core IDE Foundation — file system, projects, settings, window, editor, tabs, explorer, status bar | **Detailed below** |
| Plan 2 | Terminal & AI Chat — PTY terminal (xterm.js), AI chat messaging | Outlined at end |
| Plan 3 | Git & Search — source control panel, cross-file search | Outlined at end |
| Plan 4 | Plugins & Extensions — marketplace, extension loading | Outlined at end (recommend deferring) |

**Plan 1 produces working, testable software on its own:** a desktop app that opens a folder, displays a file tree, edits files in Monaco with multi-tab support, saves settings, and controls the native window.

---

## File Structure

```
services/                          # NEW — Go backend services
├── file_service.go                # File system operations (list, read, write, create, delete, rename, pick dir)
├── file_service_test.go           # Tests for file operations
├── project_service.go             # Recent projects persistence (JSON in XDG config dir)
├── project_service_test.go        # Tests for project CRUD
├── settings_service.go            # App settings persistence (JSON)
├── settings_service_test.go       # Tests for settings load/save
└── window_service.go              # Native window controls (minimise, maximise, close, fullscreen)

main.go                            # MODIFY — register new services, wire WindowService

frontend/
├── package.json                   # MODIFY — add monaco-editor, @guolao/vue-monaco-editor, vitest
├── vitest.config.ts               # NEW — vitest configuration
├── src/
│   ├── main.ts                    # MODIFY — register Monaco editor plugin
│   ├── types/
│   │   └── index.ts               # NEW — shared TypeScript types (DirEntry, Project, Settings)
│   ├── api/
│   │   └── services.ts            # NEW — typed re-exports of Wails bindings
│   ├── stores/
│   │   ├── app.ts                 # MODIFY — add settings load/save, persistence
│   │   └── editor.ts              # NEW — open files, active tab, dirty state, language detection
│   ├── lib/
│   │   └── language.ts            # NEW — file-extension-to-language mapping (pure, testable)
│   ├── components/
│   │   ├── editor/
│   │   │   ├── CodeEditor.vue     # NEW — Monaco wrapper component
│   │   │   └── TabBar.vue         # NEW — open-file tabs with close + dirty indicator
│   │   └── explorer/
│   │       └── FileTree.vue       # NEW — recursive file/folder tree
│   ├── views/
│   │   ├── WelcomeView.vue        # MODIFY — wire Open Project to folder picker
│   │   ├── ProjectsView.vue       # MODIFY — list/add/remove/open recent projects
│   │   └── EditorView.vue         # MODIFY — integrate TabBar + CodeEditor
│   └── components/
│       └── layout/
│           ├── TitleBar.vue       # MODIFY — wire window controls + menu navigation
│           ├── SidePanel.vue      # MODIFY — show FileTree in explorer tab
│           └── StatusBar.vue      # MODIFY — real cursor position + language mode
```

---

## Task 1: Go Backend — FileService

**Files:**
- Create: `services/file_service.go`
- Create: `services/file_service_test.go`

- [ ] **Step 1: Write the failing tests**

Create `services/file_service_test.go`:

```go
package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileService_ListDirectory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	svc := &FileService{}
	entries, err := svc.ListDirectory(dir)
	if err != nil {
		t.Fatalf("ListDirectory failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if !entries[0].IsDir {
		t.Error("expected directory to sort first")
	}
	if entries[0].Name != "subdir" {
		t.Errorf("expected subdir first, got %s", entries[0].Name)
	}
}

func TestFileService_ReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	svc := &FileService{}
	content, err := svc.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if content != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", content)
	}
}

func TestFileService_WriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	svc := &FileService{}
	err := svc.WriteFile(path, "written content")
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "written content" {
		t.Errorf("expected 'written content', got '%s'", string(data))
	}
}

func TestFileService_CreateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	svc := &FileService{}
	err := svc.CreateFile(path)
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	info, _ := os.Stat(path)
	if info.Size() != 0 {
		t.Errorf("expected empty file, got size %d", info.Size())
	}
}

func TestFileService_CreateDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c")

	svc := &FileService{}
	err := svc.CreateDirectory(path)
	if err != nil {
		t.Fatalf("CreateDirectory failed: %v", err)
	}
	info, _ := os.Stat(path)
	if !info.IsDir() {
		t.Error("expected directory to exist")
	}
}

func TestFileService_DeletePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gone.txt")
	os.WriteFile(path, []byte("x"), 0644)

	svc := &FileService{}
	err := svc.DeletePath(path)
	if err != nil {
		t.Fatalf("DeletePath failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}
}

func TestFileService_RenamePath(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	os.WriteFile(oldPath, []byte("x"), 0644)

	svc := &FileService{}
	err := svc.RenamePath(oldPath, newPath)
	if err != nil {
		t.Fatalf("RenamePath failed: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Error("expected new file to exist")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./services/ -v`
Expected: FAIL — package not found / `FileService` undefined (compilation error, no `file_service.go` yet)

- [ ] **Step 3: Write minimal implementation**

Create `services/file_service.go`:

```go
package services

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// DirEntry represents a single file or folder returned by ListDirectory.
type DirEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	IsDir    bool   `json:"isDir"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
}

// FileService exposes file-system operations to the frontend.
type FileService struct{}

// ListDirectory returns the immediate children of path, directories first.
func (f *FileService) ListDirectory(path string) ([]DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	result := make([]DirEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		result = append(result, DirEntry{
			Name:     entry.Name(),
			Path:     filepath.Join(path, entry.Name()),
			IsDir:    entry.IsDir(),
			Size:     info.Size(),
			Modified: info.ModTime().UnixMilli(),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// ReadFile reads and returns the full text content of a file.
func (f *FileService) ReadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteFile writes text content to a file, creating or truncating it.
func (f *FileService) WriteFile(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// CreateFile creates an empty file.
func (f *FileService) CreateFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	return file.Close()
}

// CreateDirectory creates a directory and any necessary parents.
func (f *FileService) CreateDirectory(path string) error {
	return os.MkdirAll(path, 0755)
}

// DeletePath removes a file or directory recursively.
func (f *FileService) DeletePath(path string) error {
	return os.RemoveAll(path)
}

// RenamePath moves or renames a file or directory.
func (f *FileService) RenamePath(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// PickDirectory opens a native directory-selection dialog and returns the chosen path.
// Returns an empty string if the user cancels.
func (f *FileService) PickDirectory() (string, error) {
	dialog := application.OpenDirectoryDialog()
	dialog.SetTitle("Open Folder")
	return dialog.PromptForSingle()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/ -v`
Expected: PASS — all 7 tests pass (PickDirectory is not unit-tested; it requires a running GUI and is verified manually later)

- [ ] **Step 5: Commit**

```bash
git add services/file_service.go services/file_service_test.go
git commit -m "feat: add FileService backend with file-system operations"
```

---

## Task 2: Go Backend — ProjectService

**Files:**
- Create: `services/project_service.go`
- Create: `services/project_service_test.go`

- [ ] **Step 1: Write the failing tests**

Create `services/project_service_test.go`:

```go
package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectService_GetRecentProjects_empty(t *testing.T) {
	svc := &ProjectService{configPath: filepath.Join(t.TempDir(), "projects.json")}
	projects, err := svc.GetRecentProjects()
	if err != nil {
		t.Fatalf("GetRecentProjects failed: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(projects))
	}
}

func TestProjectService_AddProject(t *testing.T) {
	svc := &ProjectService{configPath: filepath.Join(t.TempDir(), "projects.json")}

	proj, err := svc.AddProject("/some/path/my-project")
	if err != nil {
		t.Fatalf("AddProject failed: %v", err)
	}
	if proj.Name != "my-project" {
		t.Errorf("expected name 'my-project', got '%s'", proj.Name)
	}
	if proj.ID == "" {
		t.Error("expected non-empty ID")
	}

	projects, _ := svc.GetRecentProjects()
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
}

func TestProjectService_AddProject_duplicateUpdatesLastOpened(t *testing.T) {
	svc := &ProjectService{configPath: filepath.Join(t.TempDir(), "projects.json")}

	first, _ := svc.AddProject("/path/proj")
	second, _ := svc.AddProject("/path/proj")

	if first.ID != second.ID {
		t.Error("expected same ID for duplicate path")
	}

	projects, _ := svc.GetRecentProjects()
	if len(projects) != 1 {
		t.Fatalf("expected 1 project (no duplicate), got %d", len(projects))
	}
}

func TestProjectService_RemoveProject(t *testing.T) {
	svc := &ProjectService{configPath: filepath.Join(t.TempDir(), "projects.json")}

	proj, _ := svc.AddProject("/path/to-remove")
	err := svc.RemoveProject(proj.ID)
	if err != nil {
		t.Fatalf("RemoveProject failed: %v", err)
	}

	projects, _ := svc.GetRecentProjects()
	if len(projects) != 0 {
		t.Fatalf("expected 0 projects after removal, got %d", len(projects))
	}
}

func TestProjectService_persistsAcrossInstances(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "projects.json")
	svc1 := &ProjectService{configPath: configPath}
	svc1.AddProject("/path/persisted")

	// Simulate app restart with same config path
	svc2 := &ProjectService{configPath: configPath}
	projects, _ := svc2.GetRecentProjects()
	if len(projects) != 1 {
		t.Fatalf("expected 1 persisted project, got %d", len(projects))
	}
	if projects[0].Path != "/path/persisted" {
		t.Errorf("expected path '/path/persisted', got '%s'", projects[0].Path)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./services/ -run ProjectService -v`
Expected: FAIL — `ProjectService` undefined

- [ ] **Step 3: Write minimal implementation**

Create `services/project_service.go`:

```go
package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adrg/xdg"
)

// Project represents a recently-opened project folder.
type Project struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	CreatedAt  int64  `json:"createdAt"`
	LastOpened int64  `json:"lastOpened"`
}

// ProjectService manages the list of recent projects, persisted as JSON.
type ProjectService struct {
	configPath string
}

// NewProjectService creates a ProjectService that stores data in the
// OS-specific config directory (via XDG). Used in production.
func NewProjectService() *ProjectService {
	return &ProjectService{
		configPath: filepath.Join(xdg.ConfigHome, "gugacode", "projects.json"),
	}
}

func (p *ProjectService) load() ([]Project, error) {
	data, err := os.ReadFile(p.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Project{}, nil
		}
		return nil, err
	}
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (p *ProjectService) save(projects []Project) error {
	dir := filepath.Dir(p.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.configPath, data, 0644)
}

// GetRecentProjects returns all saved projects, most-recently-opened first.
func (p *ProjectService) GetRecentProjects() ([]Project, error) {
	projects, err := p.load()
	if err != nil {
		return nil, err
	}
	sortProjectsByRecency(projects)
	return projects, nil
}

// AddProject records a project by path. If the path already exists, its
// LastOpened timestamp is updated and no duplicate is created.
func (p *ProjectService) AddProject(path string) (Project, error) {
	projects, err := p.load()
	if err != nil {
		return Project{}, err
	}
	now := time.Now().UnixMilli()
	for i, proj := range projects {
		if proj.Path == path {
			projects[i].LastOpened = now
			if err := p.save(projects); err != nil {
				return Project{}, err
			}
			return projects[i], nil
		}
	}
	proj := Project{
		ID:         fmt.Sprintf("%d", now),
		Name:       filepath.Base(path),
		Path:       path,
		CreatedAt:  now,
		LastOpened: now,
	}
	projects = append(projects, proj)
	if err := p.save(projects); err != nil {
		return Project{}, err
	}
	return proj, nil
}

// RemoveProject deletes a project from the recent list by ID.
func (p *ProjectService) RemoveProject(id string) error {
	projects, err := p.load()
	if err != nil {
		return err
	}
	for i, proj := range projects {
		if proj.ID == id {
			projects = append(projects[:i], projects[i+1:]...)
			return p.save(projects)
		}
	}
	return nil
}

func sortProjectsByRecency(projects []Project) {
	for i := 0; i < len(projects); i++ {
		for j := i + 1; j < len(projects); j++ {
			if projects[j].LastOpened > projects[i].LastOpened {
				projects[i], projects[j] = projects[j], projects[i]
			}
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/ -run ProjectService -v`
Expected: PASS — all 5 tests pass

- [ ] **Step 5: Commit**

```bash
git add services/project_service.go services/project_service_test.go
git commit -m "feat: add ProjectService backend with recent-projects persistence"
```

---

## Task 3: Go Backend — SettingsService

**Files:**
- Create: `services/settings_service.go`
- Create: `services/settings_service_test.go`

- [ ] **Step 1: Write the failing tests**

Create `services/settings_service_test.go`:

```go
package services

import (
	"path/filepath"
	"testing"
)

func TestSettingsService_LoadSettings_returnsDefaultsWhenNoFile(t *testing.T) {
	svc := &SettingsService{configPath: filepath.Join(t.TempDir(), "settings.json")}
	settings, err := svc.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if settings.FontSize != 14 {
		t.Errorf("expected default font size 14, got %d", settings.FontSize)
	}
	if settings.Theme != "dark" {
		t.Errorf("expected default theme 'dark', got '%s'", settings.Theme)
	}
	if settings.WordWrap != true {
		t.Error("expected default wordWrap true")
	}
}

func TestSettingsService_SaveAndLoad(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	settings := defaultSettings()
	settings.FontSize = 18
	settings.TabSize = 4
	settings.WordWrap = false

	err := svc.SaveSettings(settings)
	if err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	svc2 := &SettingsService{configPath: configPath}
	loaded, err := svc2.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if loaded.FontSize != 18 {
		t.Errorf("expected font size 18, got %d", loaded.FontSize)
	}
	if loaded.TabSize != 4 {
		t.Errorf("expected tab size 4, got %d", loaded.TabSize)
	}
	if loaded.WordWrap != false {
		t.Error("expected wordWrap false")
	}
}

func TestSettingsService_LoadSettings_corruptFileReturnsDefaults(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	// Write corrupt JSON
	svc.configPath = configPath
	writeCorruptSettings(t, configPath)

	settings, err := svc.LoadSettings()
	if err != nil {
		t.Fatalf("should not return error for corrupt file: %v", err)
	}
	if settings.FontSize != 14 {
		t.Errorf("expected defaults from corrupt file, got font size %d", settings.FontSize)
	}
}
```

- [ ] **Step 2: Add the test helper**

Append to `services/settings_service_test.go` (after the test functions above):

```go
func writeCorruptSettings(t *testing.T, path string) {
	t.Helper()
	import_writeFile := func(p string, b []byte) {
		// local helper to avoid extra import block churn
	}
	_ = import_writeFile
}
```

**Correction** — replace the helper above with this correct version (uses `os.WriteFile`):

```go
import (
	"os"
	"path/filepath"
	"testing"
)

func writeCorruptSettings(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}
}
```

(Merge the `os` import into the existing import block at the top of the test file.)

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./services/ -run SettingsService -v`
Expected: FAIL — `SettingsService` and `defaultSettings` undefined

- [ ] **Step 4: Write minimal implementation**

Create `services/settings_service.go`:

```go
package services

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

// Settings holds all persisted application settings.
type Settings struct {
	Language    string `json:"language"`
	Theme       string `json:"theme"`
	FontSize    int    `json:"fontSize"`
	FontFamily  string `json:"fontFamily"`
	TabSize     int    `json:"tabSize"`
	WordWrap    bool   `json:"wordWrap"`
	LineNumbers bool   `json:"lineNumbers"`
	Minimap     bool   `json:"minimap"`
}

// SettingsService loads and saves settings as JSON in the config directory.
type SettingsService struct {
	configPath string
}

// NewSettingsService creates a SettingsService using the XDG config path.
func NewSettingsService() *SettingsService {
	return &SettingsService{
		configPath: filepath.Join(xdg.ConfigHome, "gugacode", "settings.json"),
	}
}

// LoadSettings reads settings from disk, falling back to defaults if the file
// is missing or corrupt.
func (s *SettingsService) LoadSettings() (Settings, error) {
	settings := defaultSettings()
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, nil
		}
		return settings, err
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return defaultSettings(), nil
	}
	return settings, nil
}

// SaveSettings writes settings to disk as pretty-printed JSON.
func (s *SettingsService) SaveSettings(settings Settings) error {
	dir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.configPath, data, 0644)
}

func defaultSettings() Settings {
	return Settings{
		Language:    "en",
		Theme:       "dark",
		FontSize:    14,
		FontFamily:  "JetBrains Mono",
		TabSize:     2,
		WordWrap:    true,
		LineNumbers: true,
		Minimap:     false,
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./services/ -run SettingsService -v`
Expected: PASS — all 3 tests pass

- [ ] **Step 6: Commit**

```bash
git add services/settings_service.go services/settings_service_test.go
git commit -m "feat: add SettingsService backend with JSON persistence"
```

---

## Task 4: Go Backend — WindowService

**Files:**
- Create: `services/window_service.go`

> **Note:** WindowService methods require a live Wails window and cannot be unit-tested in isolation. Verification is done in Task 6 when the service is wired into `main.go` and tested manually.

- [ ] **Step 1: Write the implementation**

Create `services/window_service.go`:

```go
package services

import "github.com/wailsapp/wails/v3/pkg/application"

// WindowService exposes native window controls to the frontend.
// The window reference is injected after app creation in main.go.
type WindowService struct {
	window *application.WebviewWindow
}

// SetWindow injects the active window. Called from main.go after the window is created.
func (w *WindowService) SetWindow(window *application.WebviewWindow) {
	w.window = window
}

// Minimise minimises the window.
func (w *WindowService) Minimise() {
	if w.window != nil {
		w.window.Minimise()
	}
}

// Maximise toggles the maximised state of the window.
func (w *WindowService) Maximise() {
	if w.window != nil {
		w.window.Maximise()
	}
}

// Close closes the window.
func (w *WindowService) Close() {
	if w.window != nil {
		w.window.Close()
	}
}

// ToggleFullscreen toggles fullscreen mode.
func (w *WindowService) ToggleFullscreen() {
	if w.window != nil {
		w.window.ToggleFullscreen()
	}
}

// SetTitle updates the window title bar text.
func (w *WindowService) SetTitle(title string) {
	if w.window != nil {
		w.window.SetTitle(title)
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./services/`
Expected: builds with no errors

- [ ] **Step 3: Commit**

```bash
git add services/window_service.go
git commit -m "feat: add WindowService backend for native window controls"
```

---

## Task 5: Register All Services in main.go

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Update main.go to register all services and wire the window**

Replace the entire contents of `main.go` with:

```go
package main

import (
	"embed"
	"log"
	"time"

	"changeme/services"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[string]("time")
}

func main() {
	fileService := &services.FileService{}
	projectService := services.NewProjectService()
	settingsService := services.NewSettingsService()
	windowService := &services.WindowService{}

	app := application.New(application.Options{
		Name:        "gugacode",
		Description: "AI-Powered Code Editor",
		Services: []application.Service{
			application.NewService(fileService),
			application.NewService(projectService),
			application.NewService(settingsService),
			application.NewService(windowService),
			application.NewService(&GreetService{}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "gugacode",
		Width:  1000,
		Height: 618,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	windowService.SetWindow(window)

	go func() {
		for {
			now := time.Now().Format(time.RFC1123)
			app.Event.Emit("time", now)
			time.Sleep(time.Second)
		}
	}()

	err := app.Run()
	if err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: builds with no errors

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat: register FileService, ProjectService, SettingsService, WindowService in main"
```

---

## Task 6: Frontend — Install Dependencies and Vitest

**Files:**
- Modify: `frontend/package.json`
- Create: `frontend/vitest.config.ts`

- [ ] **Step 1: Install packages**

Run from `frontend/`:

```bash
npm install monaco-editor @guolao/vue-monaco-editor
npm install -D vitest @vue/test-utils jsdom
```

- [ ] **Step 2: Add test script to package.json**

In `frontend/package.json`, add to the `"scripts"` block:

```json
"test": "vitest run",
"test:watch": "vitest"
```

- [ ] **Step 3: Create vitest config**

Create `frontend/vitest.config.ts`:

```ts
import { defineConfig } from "vitest/config";
import vue from "@vitejs/plugin-vue";
import { resolve } from "path";

export default defineConfig({
  plugins: [vue()],
  test: {
    environment: "jsdom",
    globals: true,
  },
  resolve: {
    alias: {
      "@": resolve(__dirname, "src"),
    },
  },
});
```

- [ ] **Step 4: Verify vitest runs (no tests yet)**

Run from `frontend/`: `npx vitest run`
Expected: "No test files found" — vitest is configured and working

- [ ] **Step 5: Commit**

```bash
git add frontend/package.json frontend/package-lock.json frontend/vitest.config.ts
git commit -m "chore: add Monaco editor and Vitest to frontend dependencies"
```

---

## Task 7: Frontend — Shared Types and API Layer

**Files:**
- Create: `frontend/src/types/index.ts`
- Create: `frontend/src/api/services.ts`

- [ ] **Step 1: Create shared types**

Create `frontend/src/types/index.ts`:

```ts
export interface DirEntry {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
  modified: number;
}

export interface Project {
  id: string;
  name: string;
  path: string;
  createdAt: number;
  lastOpened: number;
}

export interface Settings {
  language: string;
  theme: string;
  fontSize: number;
  fontFamily: string;
  tabSize: number;
  wordWrap: boolean;
  lineNumbers: boolean;
  minimap: boolean;
}
```

- [ ] **Step 2: Create API layer**

Create `frontend/src/api/services.ts`:

```ts
// Re-export the auto-generated Wails bindings with TypeScript types.
// The bindings are regenerated by the Wails Vite plugin during dev/build
// and live in frontend/bindings/changeme/.
import * as FileServiceBindings from "../../bindings/changeme/file_service.js";
import * as ProjectServiceBindings from "../../bindings/changeme/project_service.js";
import * as SettingsServiceBindings from "../../bindings/changeme/settings_service.js";
import * as WindowServiceBindings from "../../bindings/changeme/window_service.js";
import type { DirEntry, Project, Settings } from "@/types";

export const fileService = {
  listDirectory: (path: string) =>
    FileServiceBindings.ListDirectory(path) as Promise<DirEntry[]>,
  readFile: (path: string) =>
    FileServiceBindings.ReadFile(path) as Promise<string>,
  writeFile: (path: string, content: string) =>
    FileServiceBindings.WriteFile(path, content) as Promise<void>,
  createFile: (path: string) =>
    FileServiceBindings.CreateFile(path) as Promise<void>,
  createDirectory: (path: string) =>
    FileServiceBindings.CreateDirectory(path) as Promise<void>,
  deletePath: (path: string) =>
    FileServiceBindings.DeletePath(path) as Promise<void>,
  renamePath: (oldPath: string, newPath: string) =>
    FileServiceBindings.RenamePath(oldPath, newPath) as Promise<void>,
  pickDirectory: () =>
    FileServiceBindings.PickDirectory() as Promise<string>,
};

export const projectService = {
  getRecentProjects: () =>
    ProjectServiceBindings.GetRecentProjects() as Promise<Project[]>,
  addProject: (path: string) =>
    ProjectServiceBindings.AddProject(path) as Promise<Project>,
  removeProject: (id: string) =>
    ProjectServiceBindings.RemoveProject(id) as Promise<void>,
};

export const settingsService = {
  loadSettings: () =>
    SettingsServiceBindings.LoadSettings() as Promise<Settings>,
  saveSettings: (settings: Settings) =>
    SettingsServiceBindings.SaveSettings(settings) as Promise<void>,
};

export const windowService = {
  minimise: () => WindowServiceBindings.Minimise(),
  maximise: () => WindowServiceBindings.Maximise(),
  close: () => WindowServiceBindings.Close(),
  toggleFullscreen: () => WindowServiceBindings.ToggleFullscreen(),
  setTitle: (title: string) => WindowServiceBindings.SetTitle(title),
};
```

> **Note:** The binding files (`file_service.js`, etc.) are auto-generated. Run `wails3 dev` once (or build) after Task 5 to generate them. If the exact import path differs, adjust the `../../bindings/changeme/` prefix to match the generated structure.

- [ ] **Step 3: Verify TypeScript compiles**

Run from `frontend/`: `npx vue-tsc --noEmit`
Expected: no errors (binding files must exist — run `wails3 dev` first if they don't)

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types/index.ts frontend/src/api/services.ts
git commit -m "feat: add typed API layer wrapping Wails service bindings"
```

---

## Task 8: Frontend — Language Detection Utility

**Files:**
- Create: `frontend/src/lib/language.ts`
- Create: `frontend/src/lib/language.test.ts`

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/language.test.ts`:

```ts
import { describe, it, expect } from "vitest";
import { detectLanguage } from "./language";

describe("detectLanguage", () => {
  it("detects TypeScript", () => {
    expect(detectLanguage("foo.ts")).toBe("typescript");
    expect(detectLanguage("foo.tsx")).toBe("typescript");
  });

  it("detects JavaScript", () => {
    expect(detectLanguage("foo.js")).toBe("javascript");
    expect(detectLanguage("foo.jsx")).toBe("javascript");
  });

  it("detects Vue", () => {
    expect(detectLanguage("App.vue")).toBe("html");
  });

  it("detects Go", () => {
    expect(detectLanguage("main.go")).toBe("go");
  });

  it("detects JSON", () => {
    expect(detectLanguage("package.json")).toBe("json");
  });

  it("detects CSS", () => {
    expect(detectLanguage("style.css")).toBe("css");
  });

  it("detects Markdown", () => {
    expect(detectLanguage("README.md")).toBe("markdown");
  });

  it("returns plaintext for unknown extensions", () => {
    expect(detectLanguage("file.xyz")).toBe("plaintext");
  });

  it("returns plaintext for files with no extension", () => {
    expect(detectLanguage("Makefile")).toBe("plaintext");
  });

  it("handles paths with directories", () => {
    expect(detectLanguage("src/components/App.vue")).toBe("html");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run from `frontend/`: `npx vitest run src/lib/language.test.ts`
Expected: FAIL — `detectLanguage` is not exported (module not found)

- [ ] **Step 3: Write minimal implementation**

Create `frontend/src/lib/language.ts`:

```ts
const extensionToLanguage: Record<string, string> = {
  ts: "typescript",
  tsx: "typescript",
  js: "javascript",
  jsx: "javascript",
  mjs: "javascript",
  cjs: "javascript",
  vue: "html",
  html: "html",
  htm: "html",
  css: "css",
  scss: "scss",
  sass: "sass",
  less: "less",
  go: "go",
  py: "python",
  rs: "rust",
  java: "java",
  c: "c",
  cpp: "cpp",
  cs: "csharp",
  rb: "ruby",
  php: "php",
  swift: "swift",
  kt: "kotlin",
  json: "json",
  xml: "xml",
  yaml: "yaml",
  yml: "yaml",
  toml: "ini",
  ini: "ini",
  md: "markdown",
  markdown: "markdown",
  sh: "shell",
  bash: "shell",
  zsh: "shell",
  sql: "sql",
  dockerfile: "dockerfile",
};

export function detectLanguage(filePath: string): string {
  const fileName = filePath.split(/[/\\]/).pop() ?? filePath;
  const lowerName = fileName.toLowerCase();
  if (lowerName === "dockerfile") return "dockerfile";
  const ext = lowerName.split(".").pop() ?? "";
  return extensionToLanguage[ext] ?? "plaintext";
}
```

- [ ] **Step 4: Run test to verify it passes**

Run from `frontend/`: `npx vitest run src/lib/language.test.ts`
Expected: PASS — all 10 tests pass

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/language.ts frontend/src/lib/language.test.ts
git commit -m "feat: add file-extension language detection utility"
```

---

## Task 9: Frontend — Editor Store

**Files:**
- Create: `frontend/src/stores/editor.ts`
- Create: `frontend/src/stores/editor.test.ts`

- [ ] **Step 1: Write the failing tests**

Create `frontend/src/stores/editor.test.ts`:

```ts
import { describe, it, expect, beforeEach } from "vitest";
import { editorState, openFile, closeFile, updateContent, markSaved } from "./editor";

describe("editor store", () => {
  beforeEach(() => {
    editorState.openFiles = [];
    editorState.activeFilePath = null;
  });

  it("openFile adds a file and sets it active", () => {
    openFile("/src/app.ts", "const x = 1;");
    expect(editorState.openFiles).toHaveLength(1);
    expect(editorState.openFiles[0].name).toBe("app.ts");
    expect(editorState.activeFilePath).toBe("/src/app.ts");
    expect(editorState.openFiles[0].isDirty).toBe(false);
  });

  it("openFile does not duplicate an already-open file", () => {
    openFile("/src/app.ts", "const x = 1;");
    openFile("/src/app.ts", "const x = 1;");
    expect(editorState.openFiles).toHaveLength(1);
  });

  it("openFile reactivates an existing tab without changing content", () => {
    openFile("/src/app.ts", "const x = 1;");
    updateContent("/src/app.ts", "const x = 2;");
    openFile("/src/app.ts", "ignored — already open");
    expect(editorState.openFiles[0].content).toBe("const x = 2;");
  });

  it("updateContent marks file dirty when content changes", () => {
    openFile("/src/app.ts", "original");
    updateContent("/src/app.ts", "changed");
    expect(editorState.openFiles[0].isDirty).toBe(true);
    expect(editorState.openFiles[0].content).toBe("changed");
  });

  it("updateContent does not mark dirty if content equals original", () => {
    openFile("/src/app.ts", "original");
    updateContent("/src/app.ts", "original");
    expect(editorState.openFiles[0].isDirty).toBe(false);
  });

  it("markSaved clears dirty flag and updates original content", () => {
    openFile("/src/app.ts", "original");
    updateContent("/src/app.ts", "new content");
    markSaved("/src/app.ts");
    expect(editorState.openFiles[0].isDirty).toBe(false);
    expect(editorState.openFiles[0].originalContent).toBe("new content");
  });

  it("closeFile removes the file from the list", () => {
    openFile("/src/a.ts", "a");
    openFile("/src/b.ts", "b");
    closeFile("/src/a.ts");
    expect(editorState.openFiles).toHaveLength(1);
    expect(editorState.openFiles[0].path).toBe("/src/b.ts");
  });

  it("closeFile of the active tab selects a neighbor", () => {
    openFile("/src/a.ts", "a");
    openFile("/src/b.ts", "b");
    closeFile("/src/b.ts");
    expect(editorState.activeFilePath).toBe("/src/a.ts");
  });

  it("closeFile of the only tab clears active path", () => {
    openFile("/src/a.ts", "a");
    closeFile("/src/a.ts");
    expect(editorState.openFiles).toHaveLength(0);
    expect(editorState.activeFilePath).toBeNull();
  });

  it("openFile sets language from extension", () => {
    openFile("/src/app.ts", "");
    expect(editorState.openFiles[0].language).toBe("typescript");
    openFile("/src/main.go", "");
    expect(editorState.openFiles[1].language).toBe("go");
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run from `frontend/`: `npx vitest run src/stores/editor.test.ts`
Expected: FAIL — module not found

- [ ] **Step 3: Write minimal implementation**

Create `frontend/src/stores/editor.ts`:

```ts
import { reactive, computed } from "vue";
import { detectLanguage } from "@/lib/language";

export interface OpenFile {
  path: string;
  name: string;
  content: string;
  originalContent: string;
  language: string;
  isDirty: boolean;
}

interface EditorState {
  openFiles: OpenFile[];
  activeFilePath: string | null;
}

export const editorState = reactive<EditorState>({
  openFiles: [],
  activeFilePath: null,
});

export const activeFile = computed<OpenFile | null>(() =>
  editorState.openFiles.find((f) => f.path === editorState.activeFilePath) ?? null
);

export function openFile(path: string, content: string): void {
  const existing = editorState.openFiles.find((f) => f.path === path);
  if (existing) {
    editorState.activeFilePath = path;
    return;
  }
  const name = path.split(/[/\\]/).pop() ?? path;
  editorState.openFiles.push({
    path,
    name,
    content,
    originalContent: content,
    language: detectLanguage(path),
    isDirty: false,
  });
  editorState.activeFilePath = path;
}

export function closeFile(path: string): void {
  const idx = editorState.openFiles.findIndex((f) => f.path === path);
  if (idx === -1) return;
  editorState.openFiles.splice(idx, 1);
  if (editorState.activeFilePath === path) {
    const next = editorState.openFiles[idx] ?? editorState.openFiles[idx - 1] ?? null;
    editorState.activeFilePath = next?.path ?? null;
  }
}

export function updateContent(path: string, content: string): void {
  const file = editorState.openFiles.find((f) => f.path === path);
  if (file) {
    file.content = content;
    file.isDirty = content !== file.originalContent;
  }
}

export function markSaved(path: string): void {
  const file = editorState.openFiles.find((f) => f.path === path);
  if (file) {
    file.originalContent = file.content;
    file.isDirty = false;
  }
}

export function getDirtyFiles(): OpenFile[] {
  return editorState.openFiles.filter((f) => f.isDirty);
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run from `frontend/`: `npx vitest run src/stores/editor.test.ts`
Expected: PASS — all 10 tests pass

- [ ] **Step 5: Commit**

```bash
git add frontend/src/stores/editor.ts frontend/src/stores/editor.test.ts
git commit -m "feat: add editor store for open files, tabs, and dirty state"
```

---

## Task 10: Frontend — Wire App Store with Settings Persistence

**Files:**
- Modify: `frontend/src/stores/app.ts`

- [ ] **Step 1: Add settings load/save to the app store**

Open `frontend/src/stores/app.ts`. Add these imports at the top (after the existing `import` line):

```ts
import { settingsService } from "@/api/services";
import type { Settings } from "@/types";
```

Add these functions at the end of the file (after `setPanelTab`):

```ts
let saveTimer: ReturnType<typeof setTimeout> | null = null;

export async function loadSettings(): Promise<void> {
  try {
    const settings = await settingsService.loadSettings();
    appState.language = settings.language;
    appState.theme = settings.theme;
    appState.fontSize = settings.fontSize;
    appState.fontFamily = settings.fontFamily;
    appState.tabSize = settings.tabSize;
    appState.wordWrap = settings.wordWrap;
    appState.lineNumbers = settings.lineNumbers;
    appState.minimap = settings.minimap;
  } catch (e) {
    console.error("Failed to load settings:", e);
  }
}

export function saveSettings(): void {
  if (saveTimer) clearTimeout(saveTimer);
  saveTimer = setTimeout(async () => {
    const settings: Settings = {
      language: appState.language,
      theme: appState.theme,
      fontSize: appState.fontSize,
      fontFamily: appState.fontFamily,
      tabSize: appState.tabSize,
      wordWrap: appState.wordWrap,
      lineNumbers: appState.lineNumbers,
      minimap: appState.minimap,
    };
    try {
      await settingsService.saveSettings(settings);
    } catch (e) {
      console.error("Failed to save settings:", e);
    }
  }, 500);
}

export function openProject(name: string, path: string): void {
  appState.currentProject = path;
  appState.projectName = name;
}
```

- [ ] **Step 2: Call loadSettings on app startup**

Open `frontend/src/main.ts`. Add the import and call after `app.use(router)`:

```ts
import { loadSettings } from "@/stores/app";

// After app.use(router); and before app.mount("#app");
loadSettings();
```

The updated `main.ts` should look like:

```ts
import { createApp } from "vue";
import ElementPlus from "element-plus";
import "element-plus/dist/index.css";
import "element-plus/theme-chalk/dark/css-vars.css";
import * as ElementPlusIconsVue from "@element-plus/icons-vue";
import { vueMonacoEditor } from "@guolao/vue-monaco-editor";
import "animate.css";
import "./assets/styles/main.css";
import App from "./App.vue";
import router from "./router";
import { loadSettings } from "@/stores/app";

const app = createApp(App);

for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component);
}

app.use(ElementPlus, { size: "default" });
app.use(router);
app.use(vueMonacoEditor);

loadSettings();
app.mount("#app");
```

- [ ] **Step 3: Verify it compiles**

Run from `frontend/`: `npx vue-tsc --noEmit`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add frontend/src/stores/app.ts frontend/src/main.ts
git commit -m "feat: wire app store to SettingsService with debounced auto-save"
```

---

## Task 11: Frontend — Wire TitleBar Window Controls

**Files:**
- Modify: `frontend/src/components/layout/TitleBar.vue`

- [ ] **Step 1: Add window service calls to TitleBar**

Open `frontend/src/components/layout/TitleBar.vue`. Replace the `<script setup>` block with:

```ts
<script setup lang="ts">
import { useRouter } from "vue-router";
import { appState } from "@/stores/app";
import { windowService } from "@/api/services";
import { Minus, FullScreen, Close } from "@element-plus/icons-vue";

const router = useRouter();

const menuItems = [
  { label: "File", action: "file" },
  { label: "Edit", action: "edit" },
  { label: "View", action: "view" },
  { label: "Terminal", action: "terminal" },
  { label: "Help", action: "help" },
] as const;

function handleMenu(action: string) {
  switch (action) {
    case "file":
      router.push("/welcome");
      break;
    case "terminal":
      router.push("/editor");
      break;
    case "help":
      window.open("https://v3.wails.io/", "_blank");
      break;
  }
}

function handleMinimise() {
  windowService.minimise();
}
function handleMaximise() {
  windowService.maximise();
}
function handleClose() {
  windowService.close();
}
</script>
```

- [ ] **Step 2: Wire the template handlers**

In the same file, update the `<template>` — replace the menu buttons and window control buttons:

```html
    <!-- Center: Menu items -->
    <nav class="titlebar__menu" role="menubar" aria-label="Main menu">
      <button
        v-for="item in menuItems"
        :key="item.action"
        class="titlebar__menu-item"
        role="menuitem"
        :aria-label="item.label + ' menu'"
        @click="handleMenu(item.action)"
      >
        {{ item.label }}
      </button>
    </nav>

    <!-- Right: Window controls -->
    <div class="titlebar__controls" role="group" aria-label="Window controls">
      <button
        class="titlebar__control"
        aria-label="Minimize"
        title="Minimize"
        @click="handleMinimise"
      >
        <el-icon :size="12"><Minus /></el-icon>
      </button>
      <button
        class="titlebar__control"
        aria-label="Maximize"
        title="Maximize"
        @click="handleMaximise"
      >
        <el-icon :size="12"><FullScreen /></el-icon>
      </button>
      <button
        class="titlebar__control titlebar__control--close"
        aria-label="Close"
        title="Close"
        @click="handleClose"
      >
        <el-icon :size="12"><Close /></el-icon>
      </button>
    </div>
```

- [ ] **Step 3: Verify it compiles**

Run from `frontend/`: `npx vue-tsc --noEmit`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/layout/TitleBar.vue
git commit -m "feat: wire TitleBar window controls and menu navigation"
```

---

## Task 12: Frontend — Wire WelcomeView Open Project

**Files:**
- Modify: `frontend/src/views/WelcomeView.vue`

- [ ] **Step 1: Replace the script block**

Open `frontend/src/views/WelcomeView.vue`. Replace the entire `<script setup lang="ts">` block with:

```ts
<script setup lang="ts">
import { useRouter } from "vue-router";
import { FolderOpened, DocumentAdd, Clock, Monitor, Setting, Key, Notebook } from "@element-plus/icons-vue";
import { fileService, projectService } from "@/api/services";
import { openProject } from "@/stores/app";

const router = useRouter();

async function handleOpenProject() {
  const path = await fileService.pickDirectory();
  if (!path) return;
  const project = await projectService.addProject(path);
  openProject(project.name, project.path);
  router.push("/editor");
}

function handleNewProject() {
  router.push("/projects");
}

function handleRecentProjects() {
  router.push("/projects");
}

function handleQuickAction(action: string) {
  switch (action) {
    case "terminal":
      router.push("/editor");
      break;
    case "settings":
      router.push("/settings");
      break;
    case "shortcuts":
      router.push("/settings");
      break;
    case "docs":
      window.open("https://v3.wails.io/", "_blank");
      break;
  }
}
</script>
```

- [ ] **Step 2: Verify it compiles**

Run from `frontend/`: `npx vue-tsc --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/WelcomeView.vue
git commit -m "feat: wire WelcomeView Open Project to folder picker and project service"
```

---

## Task 13: Frontend — Wire ProjectsView

**Files:**
- Modify: `frontend/src/views/ProjectsView.vue`

- [ ] **Step 1: Replace the script and template**

Open `frontend/src/views/ProjectsView.vue`. Replace the entire `<script setup lang="ts">` block with:

```ts
<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useRouter } from "vue-router";
import { Plus, FolderOpened, Delete } from "@element-plus/icons-vue";
import { fileService, projectService } from "@/api/services";
import { openProject } from "@/stores/app";
import type { Project } from "@/types";

const router = useRouter();
const projects = ref<Project[]>([]);
const loading = ref(false);

async function loadProjects() {
  loading.value = true;
  try {
    projects.value = await projectService.getRecentProjects();
  } finally {
    loading.value = false;
  }
}

async function handleOpenFolder() {
  const path = await fileService.pickDirectory();
  if (!path) return;
  const project = await projectService.addProject(path);
  await loadProjects();
  openProject(project.name, project.path);
  router.push("/editor");
}

async function handleOpenProject(project: Project) {
  openProject(project.name, project.path);
  await projectService.addProject(project.path);
  router.push("/editor");
}

async function handleRemoveProject(id: string) {
  await projectService.removeProject(id);
  await loadProjects();
}

onMounted(loadProjects);
</script>
```

Replace the `<template>` block with:

```html
<template>
  <div class="projects-view">
    <div class="projects-header">
      <h1 class="projects-title">Projects</h1>
      <el-button
        type="primary"
        :icon="Plus"
        size="default"
        class="btn-primary"
        aria-label="Open folder to add project"
        @click="handleOpenFolder"
      >
        Open Folder
      </el-button>
    </div>

    <div class="projects-body">
      <!-- Empty state -->
      <div v-if="projects.length === 0 && !loading" class="projects-empty">
        <el-icon :size="48" class="projects-empty-icon">
          <FolderOpened />
        </el-icon>
        <h2 class="projects-empty-title">No projects yet</h2>
        <p class="projects-empty-desc">Open a folder to add it as a project</p>
        <el-button
          size="default"
          :icon="FolderOpened"
          class="btn-outline"
          aria-label="Open folder to add project"
          @click="handleOpenFolder"
        >
          Open Folder
        </el-button>
      </div>

      <!-- Project list -->
      <div v-else class="projects-list">
        <div
          v-for="project in projects"
          :key="project.id"
          class="project-card"
          @click="handleOpenProject(project)"
        >
          <el-icon :size="24" class="project-card__icon">
            <FolderOpened />
          </el-icon>
          <div class="project-card__info">
            <span class="project-card__name">{{ project.name }}</span>
            <span class="project-card__path">{{ project.path }}</span>
          </div>
          <button
            class="project-card__remove"
            aria-label="Remove project"
            @click.stop="handleRemoveProject(project.id)"
          >
            <el-icon :size="14"><Delete /></el-icon>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
```

- [ ] **Step 2: Add styles for the project list**

In the same file's `<style scoped>`, replace the `.projects-grid-placeholder` and `.grid-slot` rules with:

```css
.projects-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 100%;
  max-width: 680px;
}

.project-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-elevated);
  cursor: pointer;
  transition: border-color var(--duration-fast) var(--ease-out-expo),
              background-color var(--duration-fast) var(--ease-out-expo);
}

.project-card:hover {
  border-color: var(--color-primary);
  background-color: var(--color-bg-overlay);
}

.project-card__icon {
  color: var(--color-primary);
  flex-shrink: 0;
}

.project-card__info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
}

.project-card__name {
  font-size: 14px;
  font-weight: 500;
  color: var(--color-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.project-card__path {
  font-size: 11px;
  color: var(--color-text-tertiary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.project-card__remove {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  flex-shrink: 0;
  transition: color var(--duration-fast) var(--ease-out-expo),
              background-color var(--duration-fast) var(--ease-out-expo);
}

.project-card__remove:hover {
  color: var(--color-error);
  background-color: color-mix(in srgb, var(--color-error) 10%, transparent);
}
```

- [ ] **Step 3: Verify it compiles**

Run from `frontend/`: `npx vue-tsc --noEmit`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/ProjectsView.vue
git commit -m "feat: wire ProjectsView to list, open, and remove recent projects"
```

---

## Task 14: Frontend — FileTree Component

**Files:**
- Create: `frontend/src/components/explorer/FileTree.vue`

- [ ] **Step 1: Create the recursive file tree component**

Create `frontend/src/components/explorer/FileTree.vue`:

```vue
<script setup lang="ts">
import { ref } from "vue";
import { fileService } from "@/api/services";
import type { DirEntry } from "@/types";
import { ChevronRight, Folder, Document } from "@element-plus/icons-vue";

const props = withDefaults(defineProps<{
  path: string;
  name: string;
  depth?: number;
}>(), {
  depth: 0,
});

const emit = defineEmits<{
  (e: "select", path: string): void;
}>();

const expanded = ref(false);
const loading = ref(false);
const children = ref<DirEntry[]>([]);

async function toggle() {
  if (!expanded.value) {
    loading.value = true;
    try {
      children.value = await fileService.listDirectory(props.path);
    } catch (err) {
      console.error("Failed to list directory:", err);
    } finally {
      loading.value = false;
    }
  }
  expanded.value = !expanded.value;
}

function handleClick() {
  emit("select", props.path);
}

const indent = { paddingLeft: `${props.depth * 12 + 8}px` };
</script>

<template>
  <div class="file-tree">
    <div
      class="file-tree__row"
      :style="indent"
      @click="depth === 0 ? toggle() : (emit('select', path))"
    >
      <button
        v-if="depth > 0"
        class="file-tree__chevron"
        :class="{ 'file-tree__chevron--expanded': expanded }"
        @click.stop="toggle"
        aria-label="Toggle folder"
      >
        <el-icon :size="12"><ChevronRight /></el-icon>
      </button>
      <span v-else class="file-tree__chevron-placeholder" />

      <el-icon :size="14" class="file-tree__icon">
        <Folder v-if="depth === 0 || true" />
      </el-icon>

      <span class="file-tree__name" @click="depth > 0 && !expanded ? toggle() : undefined">
        {{ name }}
      </span>
    </div>

    <div v-if="expanded && loading" class="file-tree__loading">
      Loading...
    </div>

    <div v-if="expanded && !loading" class="file-tree__children">
      <FileTree
        v-for="child in children"
        :key="child.path"
        :path="child.path"
        :name="child.name"
        :depth="depth + 1"
        @select="emit('select', $event)"
      />
    </div>
  </div>
</template>

<style scoped>
.file-tree__row {
  display: flex;
  align-items: center;
  gap: 4px;
  height: 24px;
  cursor: pointer;
  user-select: none;
  border-radius: var(--radius-sm);
  transition: background-color var(--duration-micro) var(--ease-out-expo);
}

.file-tree__row:hover {
  background-color: color-mix(in srgb, var(--color-text-primary) 6%, transparent);
}

.file-tree__chevron {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border: none;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  transition: transform var(--duration-fast) var(--ease-out-expo);
}

.file-tree__chevron--expanded {
  transform: rotate(90deg);
}

.file-tree__chevron-placeholder {
  width: 16px;
  flex-shrink: 0;
}

.file-tree__icon {
  color: var(--color-text-secondary);
  flex-shrink: 0;
}

.file-tree__name {
  font-size: 12px;
  color: var(--color-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.file-tree__loading {
  padding: 4px 12px;
  font-size: 11px;
  color: var(--color-text-tertiary);
}

.file-tree__children {
  /* children render with their own indentation */
}
</style>
```

- [ ] **Step 2: Verify it compiles**

Run from `frontend/`: `npx vue-tsc --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/explorer/FileTree.vue
git commit -m "feat: add recursive FileTree component for project explorer"
```

---

## Task 15: Frontend — Wire SidePanel Explorer Tab

**Files:**
- Modify: `frontend/src/components/layout/SidePanel.vue`

- [ ] **Step 1: Integrate FileTree into the explorer tab**

Open `frontend/src/components/layout/SidePanel.vue`. Replace the `<script setup>` block with:

```ts
<script setup lang="ts">
import { computed } from "vue";
import { appState, toggleSidebar } from "@/stores/app";
import { Close } from "@element-plus/icons-vue";
import FileTree from "@/components/explorer/FileTree.vue";

const panelTitles: Record<string, string> = {
  explorer: "Explorer",
  search: "Search",
  git: "Source Control",
  extensions: "Extensions",
  ai: "AI Assistant",
};

const isCollapsed = computed(() => appState.sidebarCollapsed);
const currentTab = computed(() => appState.panelTab);
const panelTitle = computed(() => panelTitles[currentTab.value] || "Explorer");
const projectPath = computed(() => appState.currentProject);
const projectName = computed(() => appState.projectName ?? "Project");

const emit = defineEmits<{
  (e: "file-select", path: string): void;
}>();

function handleFileSelect(path: string) {
  emit("file-select", path);
}

const emptyMessages: Record<string, string> = {
  explorer: "Open a project to start",
  search: "Type to search across files",
  git: "No source control providers",
  extensions: "No extensions installed",
  ai: "AI assistant ready",
};
</script>
```

Replace the panel body section in `<template>` (the `<!-- Panel body -->` div) with:

```html
      <!-- Panel body -->
      <div class="side-panel__body">
        <!-- Search input (shown only for search tab) -->
        <div v-if="currentTab === 'search'" class="side-panel__search-wrap">
          <input
            type="text"
            class="side-panel__search-input"
            placeholder="Search files..."
            name="search-files"
            autocomplete="off"
            aria-label="Search files"
          />
        </div>

        <!-- Explorer: file tree -->
        <div v-else-if="currentTab === 'explorer' && projectPath" class="side-panel__explorer">
          <div class="side-panel__project-header">{{ projectName }}</div>
          <FileTree :path="projectPath" :name="projectName" :depth="0" @select="handleFileSelect" />
        </div>

        <!-- Empty state for other tabs -->
        <div v-else class="side-panel__empty">
          <div class="side-panel__empty-line" aria-hidden="true" />
          <p class="side-panel__empty-text">{{ emptyMessages[currentTab] || panelTitle }}</p>
        </div>
      </div>
```

Add this style inside `<style scoped>`:

```css
.side-panel__explorer {
  padding: 0 4px;
}

.side-panel__project-header {
  padding: 6px 12px 4px;
  font-size: 10px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--color-text-tertiary);
}
```

- [ ] **Step 2: Verify it compiles**

Run from `frontend/`: `npx vue-tsc --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/layout/SidePanel.vue
git commit -m "feat: wire SidePanel explorer tab to FileTree component"
```

---

## Task 16: Frontend — CodeEditor Component (Monaco)

**Files:**
- Create: `frontend/src/components/editor/CodeEditor.vue`

- [ ] **Step 1: Create the Monaco wrapper component**

Create `frontend/src/components/editor/CodeEditor.vue`:

```vue
<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { VueMonacoEditor } from "@guolao/vue-monaco-editor";
import { appState } from "@/stores/app";

const props = defineProps<{
  modelValue: string;
  language: string;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", value: string): void;
  (e: "cursor-change", line: number, column: number): void;
}>();

const editorRef = ref();

const options = computed(() => ({
  fontSize: appState.fontSize,
  fontFamily: appState.fontFamily,
  tabSize: appState.tabSize,
  wordWrap: appState.wordWrap ? "on" : "off",
  lineNumbers: appState.lineNumbers ? "on" : "off",
  minimap: { enabled: appState.minimap },
  automaticLayout: true,
  scrollBeyondLastLine: false,
  smoothScrolling: true,
  cursorBlinking: "smooth",
  renderWhitespace: "selection",
  bracketPairColorization: { enabled: true },
}));

function handleMount(editor: any) {
  editorRef.value = editor;
  editor.onDidChangeCursorPosition((e: any) => {
    emit("cursor-change", e.position.lineNumber, e.position.column);
  });
}

function handleChange(value: string | undefined) {
  emit("update:modelValue", value ?? "");
}

watch(() => props.language, (lang) => {
  if (editorRef.value) {
    const model = editorRef.value.getModel();
    if (model) {
      editorRef.value.monaco.editor.setModelLanguage(model, lang);
    }
  }
});
</script>

<template>
  <div class="code-editor">
    <VueMonacoEditor
      :value="modelValue"
      :language="language"
      theme="vs-dark"
      :options="options"
      @mount="handleMount"
      @change="handleChange"
    />
  </div>
</template>

<style scoped>
.code-editor {
  width: 100%;
  height: 100%;
}

.code-editor :deep(.monaco-editor) {
  background-color: var(--color-bg-base);
}

.code-editor :deep(.monaco-editor .margin) {
  background-color: var(--color-bg-base);
}
</style>
```

- [ ] **Step 2: Verify it compiles**

Run from `frontend/`: `npx vue-tsc --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/editor/CodeEditor.vue
git commit -m "feat: add CodeEditor Monaco wrapper component"
```

---

## Task 17: Frontend — TabBar Component

**Files:**
- Create: `frontend/src/components/editor/TabBar.vue`

- [ ] **Step 1: Create the tab bar component**

Create `frontend/src/components/editor/TabBar.vue`:

```vue
<script setup lang="ts">
import { editorState, closeFile } from "@/stores/editor";
import { Close } from "@element-plus/icons-vue";

const emit = defineEmits<{
  (e: "select", path: string): void;
  (e: "close", path: string): void;
}>();

function handleSelect(path: string) {
  emit("select", path);
}

function handleClose(path: string) {
  emit("close", path);
}
</script>

<template>
  <div v-if="editorState.openFiles.length > 0" class="tab-bar">
    <div
      v-for="file in editorState.openFiles"
      :key="file.path"
      class="tab-bar__tab"
      :class="{ 'tab-bar__tab--active': file.path === editorState.activeFilePath }"
      @click="handleSelect(file.path)"
    >
      <span class="tab-bar__name">{{ file.name }}</span>
      <span v-if="file.isDirty" class="tab-bar__dirty" aria-hidden="true">●</span>
      <button
        class="tab-bar__close"
        aria-label="Close tab"
        @click.stop="handleClose(file.path)"
      >
        <el-icon :size="12"><Close /></el-icon>
      </button>
    </div>
  </div>
</template>

<style scoped>
.tab-bar {
  display: flex;
  align-items: center;
  height: 34px;
  padding: 0 8px;
  background-color: var(--color-bg-surface);
  box-shadow: 0 1px 0 var(--color-border-subtle);
  overflow-x: auto;
  gap: 2px;
}

.tab-bar__tab {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 8px 0 12px;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-tertiary);
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
  transition: background-color var(--duration-micro) var(--ease-out-expo),
              color var(--duration-micro) var(--ease-out-expo);
}

.tab-bar__tab:hover {
  background-color: color-mix(in srgb, var(--color-text-primary) 6%, transparent);
  color: var(--color-text-secondary);
}

.tab-bar__tab--active {
  background-color: var(--color-bg-base);
  color: var(--color-text-primary);
}

.tab-bar__name {
  font-weight: 400;
}

.tab-bar__tab--active .tab-bar__name {
  font-weight: 500;
}

.tab-bar__dirty {
  color: var(--color-primary);
  font-size: 10px;
  line-height: 1;
}

.tab-bar__close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  transition: background-color var(--duration-micro) var(--ease-out-expo),
              color var(--duration-micro) var(--ease-out-expo);
}

.tab-bar__close:hover {
  background-color: color-mix(in srgb, var(--color-text-primary) 12%, transparent);
  color: var(--color-text-primary);
}
</style>
```

- [ ] **Step 2: Verify it compiles**

Run from `frontend/`: `npx vue-tsc --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/editor/TabBar.vue
git commit -m "feat: add TabBar component with dirty indicators and close buttons"
```

---

## Task 18: Frontend — Wire EditorView with Tabs and Monaco

**Files:**
- Modify: `frontend/src/views/EditorView.vue`

- [ ] **Step 1: Replace EditorView with integrated editor**

Open `frontend/src/views/EditorView.vue`. Replace the entire file with:

```vue
<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Document } from "@element-plus/icons-vue";
import CodeEditor from "@/components/editor/CodeEditor.vue";
import TabBar from "@/components/editor/TabBar.vue";
import { appState } from "@/stores/app";
import {
  editorState,
  activeFile,
  openFile,
  closeFile,
  updateContent,
  markSaved,
} from "@/stores/editor";
import { fileService } from "@/api/services";

const cursorLine = ref(1);
const cursorColumn = ref(1);

const hasOpenFiles = computed(() => editorState.openFiles.length > 0);
const activeContent = computed(() => activeFile.value?.content ?? "");

async function handleFileSelect(path: string) {
  try {
    const content = await fileService.readFile(path);
    openFile(path, content);
  } catch (err) {
    console.error("Failed to read file:", err);
  }
}

function handleTabSelect(path: string) {
  editorState.activeFilePath = path;
}

async function handleTabClose(path: string) {
  closeFile(path);
}

function handleContentChange(value: string) {
  if (editorState.activeFilePath) {
    updateContent(editorState.activeFilePath, value);
  }
}

function handleCursorChange(line: number, column: number) {
  cursorLine.value = line;
  cursorColumn.value = column;
  appState.cursorLine = line;
  appState.cursorColumn = column;
}

async function handleSave() {
  if (!activeFile.value) return;
  try {
    await fileService.writeFile(activeFile.value.path, activeFile.value.content);
    markSaved(activeFile.value.path);
  } catch (err) {
    console.error("Failed to save file:", err);
  }
}

function handleKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key === "s") {
    e.preventDefault();
    handleSave();
  }
}

watch(
  () => activeFile.value?.language,
  (lang) => {
    if (lang) {
      appState.languageMode = lang.charAt(0).toUpperCase() + lang.slice(1);
    }
  }
);
</script>

<template>
  <div class="editor-view" @keydown="handleKeydown">
    <TabBar @select="handleTabSelect" @close="handleTabClose" />

    <div class="editor-area">
      <CodeEditor
        v-if="hasOpenFiles && activeFile"
        :model-value="activeContent"
        :language="activeFile.language"
        @update:model-value="handleContentChange"
        @cursor-change="handleCursorChange"
      />

      <div v-else class="editor-empty-state">
        <span class="empty-prompt">&gt;_</span>
        <p class="empty-hint">Open a file to start editing</p>
        <p class="empty-sub">Select a file from the explorer</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.editor-view {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  background-color: var(--color-bg-base);
  color: var(--color-text-primary);
}

.editor-area {
  flex: 1;
  display: flex;
  overflow: hidden;
}

.editor-empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  text-align: center;
  padding: 24px;
  user-select: none;
  width: 100%;
}

.empty-prompt {
  font-family: var(--font-family-mono);
  font-size: 18px;
  color: var(--color-text-tertiary);
  line-height: 1;
}

.empty-hint {
  margin: 0;
  font-size: 13px;
  color: var(--color-text-secondary);
}

.empty-sub {
  margin: 0;
  font-size: 12px;
  color: var(--color-text-tertiary);
}
</style>
```

- [ ] **Step 2: Wire file selection from SidePanel to EditorView**

Open `frontend/src/components/layout/MainLayout.vue`. Update the `<script setup>` and the center area to pass the `file-select` event through:

Replace the `<script setup lang="ts">` block with:

```ts
<script setup lang="ts">
import { appState, toggleTerminal, toggleAiChat } from "@/stores/app";
import ActivityBar from "./ActivityBar.vue";
import TitleBar from "./TitleBar.vue";
import SidePanel from "./SidePanel.vue";
import AiChatPanel from "./AiChatPanel.vue";
import TerminalPanel from "./TerminalPanel.vue";
import StatusBar from "./StatusBar.vue";
import { ref } from "vue";

const editorView = ref();
</script>
```

In the template, update the SidePanel and slot area:

```html
      <!-- SidePanel (collapsible) -->
      <SidePanel @file-select="handleFileSelect" />

      <!-- Center area: Main content + Terminal -->
      <div class="main-layout__center">
        <slot>
          <div class="main-layout__editor">
            <p class="main-layout__empty-text">No file open</p>
          </div>
        </slot>

        <!-- Terminal Panel -->
        <TerminalPanel />
      </div>
```

**Correction** — the `editorView` ref and `handleFileSelect` are not needed in MainLayout because the slot (router-view) handles it. Instead, the file-select event needs to reach EditorView. Since SidePanel and EditorView are siblings under the router-view slot, use a shared store callback.

Revert MainLayout to its original script. Instead, add a file-selection bridge to the editor store.

Open `frontend/src/stores/editor.ts` and add at the end:

```ts
import { fileService } from "@/api/services";

export async function openFileFromPath(path: string): Promise<void> {
  try {
    const content = await fileService.readFile(path);
    openFile(path, content);
  } catch (err) {
    console.error("Failed to read file:", err);
  }
}
```

Now update `SidePanel.vue` to call `openFileFromPath` directly instead of emitting. Replace the `handleFileSelect` function in `SidePanel.vue`:

```ts
import { openFileFromPath } from "@/stores/editor";

function handleFileSelect(path: string) {
  openFileFromPath(path);
}
```

(Remove the `emit` declaration for `file-select` from SidePanel since it's no longer needed. Keep the `FileTree` `@select` handler calling `handleFileSelect`.)

And in `EditorView.vue`, remove the now-unused `handleFileSelect` function (the store handles it). The `TabBar` `@select` and `@close` handlers stay.

- [ ] **Step 3: Verify it compiles**

Run from `frontend/`: `npx vue-tsc --noEmit`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/EditorView.vue frontend/src/components/layout/MainLayout.vue frontend/src/components/layout/SidePanel.vue frontend/src/stores/editor.ts
git commit -m "feat: integrate Monaco editor with tab bar and file explorer"
```

---

## Task 19: Frontend — Wire StatusBar with Real Data

**Files:**
- Modify: `frontend/src/components/layout/StatusBar.vue`

- [ ] **Step 1: Update StatusBar to reflect real editor state**

Open `frontend/src/components/layout/StatusBar.vue`. Replace the `<script setup>` block with:

```ts
<script setup lang="ts">
import { appState, toggleTerminal } from "@/stores/app";
import { editorState, activeFile } from "@/stores/editor";
import { computed } from "vue";

const branchName = computed(() => appState.branchName || "—");
const errors = computed(() => appState.errors);
const warnings = computed(() => appState.warnings);
const cursorLine = computed(() => appState.cursorLine);
const cursorColumn = computed(() => appState.cursorColumn);
const encoding = computed(() => appState.encoding);
const languageMode = computed(() => activeFile.value?.language ?? appState.languageMode);
const hasProblems = computed(() => errors.value > 0 || warnings.value > 0);
const hasOpenFile = computed(() => editorState.openFiles.length > 0);
</script>
```

Update the template's right side to hide cursor info when no file is open:

```html
    <!-- Right side -->
    <div class="statusbar__right">
      <button
        v-if="hasOpenFile"
        type="button"
        class="statusbar__item"
        :aria-label="'Line ' + cursorLine + ', Column ' + cursorColumn"
      >
        Ln {{ cursorLine }}, Col {{ cursorColumn }}
      </button>
      <button
        v-if="hasOpenFile"
        type="button"
        class="statusbar__item"
        :aria-label="'Encoding: ' + encoding"
      >
        {{ encoding }}
      </button>
      <button
        type="button"
        class="statusbar__item"
        :aria-label="'Language: ' + languageMode"
      >
        {{ languageMode }}
      </button>
    </div>
```

- [ ] **Step 2: Verify it compiles**

Run from `frontend/`: `npx vue-tsc --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/layout/StatusBar.vue
git commit -m "feat: wire StatusBar to real editor cursor position and language"
```

---

## Task 20: Integration — Manual Verification

**Files:** None (verification only)

- [ ] **Step 1: Run all Go tests**

Run from project root: `go test ./services/ -v`
Expected: all FileService, ProjectService, SettingsService tests PASS

- [ ] **Step 2: Run all frontend tests**

Run from `frontend/`: `npx vitest run`
Expected: all language detection and editor store tests PASS

- [ ] **Step 3: Verify TypeScript compiles**

Run from `frontend/`: `npx vue-tsc --noEmit`
Expected: no errors

- [ ] **Step 4: Start dev mode and manually verify**

Run from project root: `wails3 dev`

Manual test checklist:
1. App launches, shows Welcome page
2. Click "Open Project" → native folder picker opens → select a folder → app navigates to Editor view
3. Explorer panel shows the folder name and file tree
4. Click a folder in the tree → it expands/collapses
5. Click a file → it opens in a new tab in the editor
6. Monaco editor shows the file content with syntax highlighting
7. Edit the file → tab shows a dirty dot (●)
8. Press Ctrl+S → dirty dot disappears (file saved)
9. Open a second file → both tabs visible → click between them
10. Close a tab → it disappears, neighbor becomes active
11. Status bar shows correct line/column when cursor moves
12. Status bar shows the file's language
13. Title bar minimize/maximize/close buttons work
14. Navigate to Settings → change font size → it persists after restart
15. Navigate to Projects → recent project appears in the list
16. Remove a project from the list → it disappears

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "chore: integration verification for core IDE foundation"
```

---

## Self-Review Notes

**Spec coverage:** Every UI shell with a TODO is wired: Welcome (open project), Projects (list/add/remove), Editor (Monaco + tabs), Settings (persistence), TitleBar (window controls), SidePanel (file tree), StatusBar (cursor/language). Terminal, AI chat, Git, Search, and Plugins are explicitly deferred to Plans 2–4.

**Placeholder scan:** No TBD/TODO in implementation steps. The `PickDirectory` Wails API method names (`OpenDirectoryDialog().SetTitle().PromptForSingle()`) are the engineer's best starting point — if the exact alpha API differs, the dev build will surface the mismatch immediately. All other code is complete.

**Type consistency:** `DirEntry`, `Project`, `Settings` structs in Go match the TypeScript interfaces in `types/index.ts`. `OpenFile` interface is consistent between `stores/editor.ts` and `TabBar.vue`. `openFile`, `closeFile`, `updateContent`, `markSaved`, `openFileFromPath` signatures match across all importing files.

---

## Follow-Up Plans (Outlined)

### Plan 2: Terminal & AI Chat
- **Terminal:** Add `TerminalService` (Go) using `os/exec` with a PTY (`github.com/creack/pty`), stream output via Wails events. Frontend: install `@xterm/xterm` + `@xterm/addon-fit`, wire to `TerminalPanel.vue`, handle input/output streaming.
- **AI Chat:** Add `AIService` (Go) with HTTP client for OpenAI-compatible APIs. Frontend: wire `AiChatPanel.vue` with message list, streaming responses via Wails events, model selector from settings.
- **Estimated tasks:** ~12

### Plan 3: Git & Search
- **Git:** Add `GitService` (Go) using `go-git` or shelling out to `git`. Frontend: populate the `git` SidePanel tab with changed files, stage/unstage, commit. Branch name in StatusBar from real `git rev-parse`.
- **Search:** Frontend: implement the `search` SidePanel tab — `FileService` gains a `SearchInFiles` method (Go, using `filepath.WalkDir` + `strings.Contains`), results displayed in the panel, click to open at line.
- **Estimated tasks:** ~10

### Plan 4: Plugins & Extensions (Recommend Deferring)
- **Recommendation:** Building a real extension marketplace is a massive undertaking (extension host, sandboxing, API surface, registry). **Recommend removing the PluginsView** for now or replacing it with a static "Coming soon" page. Focus engineering effort on Plans 1–3 first.
- If pursued later: define an extension manifest format, a JS-based plugin host in the frontend, and a `PluginService` (Go) for install/uninstall. ~20+ tasks.
