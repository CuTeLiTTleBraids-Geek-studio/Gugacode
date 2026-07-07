package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// PluginPermission is a capability declared by a plugin manifest and
// enforced by the frontend nknk.* API before privileged calls (Plan 49).
// Built-in permission scopes:
//   - "fs.read"      — read files inside the workspace
//   - "fs.write"     — write files inside the workspace
//   - "shell.exec"   — execute commands via the agent service
//   - "net"          — outbound network access (future)
//   - "ai.send"      — send messages to the AI service (future)
//
// "commands.register" and "views.register" are always allowed and do not
// need to be declared.
type PluginPermission string

// PluginContribution describes what a plugin contributes to the IDE
// (commands, views, etc.). For v1 only command and view contributions
// are parsed; unknown contribution kinds are preserved as raw JSON for
// forward compatibility.
type PluginContribution struct {
	// Commands is a list of command contributions. Each command has an
	// id (e.g. "myext.hello"), title (display label), and optional
	// keybinding (e.g. "ctrl+alt+h").
	Commands []PluginCommandContribution `json:"commands,omitempty"`
	// Views is a list of view contributions. Each view has an id, title,
	// and target location ("sidebar" | "panel" | "statusbar").
	Views []PluginViewContribution `json:"views,omitempty"`
}

// PluginCommandContribution declares a command the plugin contributes
// to the command palette.
type PluginCommandContribution struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Category   string `json:"category,omitempty"`
	Keybinding string `json:"keybinding,omitempty"`
	// Public controls whether other plugins can invoke this command via
	// nknk.commands.execute (Proposal E). When false (the default), only
	// the owning plugin may execute the command; cross-plugin callers
	// get a permission error. When true, any plugin may invoke it.
	Public bool `json:"public,omitempty"`
}

// PluginViewContribution declares a view the plugin contributes to a
// dock location.
type PluginViewContribution struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Location string `json:"location,omitempty"` // "sidebar" | "panel" | "statusbar"
}

// PluginManifest is the parsed plugin.json descriptor (Plan 49). It
// declares a plugin's identity, entry point, required permissions, and
// IDE contributions. The schema is intentionally minimal for v1.
type PluginManifest struct {
	// SchemaVersion is the manifest format version. Currently 1. If
	// unset (0), the manifest is treated as v1 for backward compat.
	// Unknown future versions are rejected by Validate() (Proposal A).
	SchemaVersion int `json:"schemaVersion,omitempty"`
	// Name is the unique plugin identifier. Must match [a-z0-9-]+ and
	// be unique across both user and project layers. Project-layer
	// plugins with the same name as a user-layer plugin override it.
	Name string `json:"name"`
	// Version is a semantic version string (e.g. "1.0.0"). Validated
	// with a relaxed semver regex.
	Version string `json:"version"`
	// Description is a short human-readable summary shown in the UI.
	Description string `json:"description,omitempty"`
	// Author is the plugin author's name or handle.
	Author string `json:"author,omitempty"`
	// Repository is the URL to the plugin's source repository
	// (Proposal D). Optional metadata for the plugin catalog.
	Repository string `json:"repository,omitempty"`
	// Homepage is the URL to the plugin's homepage/documentation
	// (Proposal D). Optional metadata for the plugin catalog.
	Homepage string `json:"homepage,omitempty"`
	// License is the SPDX license identifier (e.g. "MIT", "Apache-2.0")
	// (Proposal D). Optional metadata for the plugin catalog.
	License string `json:"license,omitempty"`
	// Main is the entry point file relative to the plugin directory.
	// Must end in ".js" and not contain path traversal. The file must
	// export an `activate(context)` function.
	Main string `json:"main"`
	// Permissions is the list of capability scopes the plugin requires.
	// The frontend nknk.* API checks this list before privileged calls.
	Permissions []PluginPermission `json:"permissions,omitempty"`
	// ActivationEvents lists the events that trigger the plugin's
	// activation. For v1, supported values are:
	//   - "onStartup"       — activate immediately after load
	//   - "onCommand:<id>"  — activate when the command is invoked
	//   - "onLanguage:<id>" — activate for a language (future)
	ActivationEvents []string `json:"activationEvents,omitempty"`
	// Contributes declares the IDE features the plugin adds.
	Contributes PluginContribution `json:"contributes,omitempty"`
}

// currentPluginSchemaVersion is the manifest schema version this build
// supports. Manifests with a higher version are rejected.
const currentPluginSchemaVersion = 1

// PluginSource identifies where a plugin was discovered.
type PluginSource string

const (
	// PluginSourceUser is a user-global plugin installed under
	// <configDir>/gugacode/plugins/<name>/.
	PluginSourceUser PluginSource = "user"
	// PluginSourceProject is a project-scoped plugin installed under
	// <projectRoot>/.nknk/plugins/<name>/. Project plugins override
	// user plugins with the same name.
	PluginSourceProject PluginSource = "project"
)

// PluginInfo is the runtime descriptor for an installed plugin. It
// pairs the parsed manifest with discovery metadata (source layer,
// install path, enabled state).
type PluginInfo struct {
	Manifest PluginManifest `json:"manifest"`
	// Path is the absolute path to the plugin directory.
	Path string `json:"path"`
	// Source is the discovery layer ("user" or "project").
	Source PluginSource `json:"source"`
	// Enabled is whether the user has enabled the plugin. Disabled
	// plugins are listed but not activated.
	Enabled bool `json:"enabled"`
	// MainExists is true if the manifest's Main file exists on disk.
	// surfaced to the UI so install problems are visible.
	MainExists bool `json:"mainExists"`
	// LoadError is set when the plugin's manifest could not be read,
	// parsed, or validated (N-111). Previously such plugins were
	// silently dropped from the list — making them appear "missing"
	// rather than "broken". The frontend shows this string so the user
	// can repair or remove the broken plugin. Empty when healthy.
	LoadError string `json:"loadError,omitempty"`
}

// pluginStateEntry is one row in the persisted enabled/disabled state
// file. Missing entries default to enabled=true (opt-out).
type pluginStateEntry struct {
	Enabled bool `json:"enabled"`
}

// pluginStateFile is the on-disk shape of plugins-state.json.
type pluginStateFile struct {
	Plugins map[string]pluginStateEntry `json:"plugins"`
}

// isPluginPathOutsideRoot reports whether relPath escapes the plugin
// directory. It rejects:
//  1. Windows absolute paths (filepath.IsAbs returns true)
//  2. Unix-style absolute paths (leading "/") — filepath.IsAbs does
//     not detect these on Windows, which would let "/abs/main.js"
//     slip through.
//  3. Volume-relative paths on Windows (e.g. "\abs" or "C:foo")
//  4. Parent traversal (filepath.Clean(path) starts with "..")
//
// This is used to validate both the manifest Main entry and
// ReadPluginFile's relPath argument.
func isPluginPathOutsideRoot(relPath string) bool {
	if relPath == "" {
		return false
	}
	// Reject Unix-style absolute paths that filepath.IsAbs misses on
	// Windows. Also reject leading-backslash paths (Windows UNC/relative
	// to current drive root).
	if strings.HasPrefix(relPath, "/") || strings.HasPrefix(relPath, "\\") {
		return true
	}
	// filepath.IsAbs catches Windows drive paths ("C:\...") and UNC
	// paths ("\\server\share\...").
	if filepath.IsAbs(relPath) {
		return true
	}
	// Reject Windows volume-relative form (e.g. "C:foo"). A drive
	// letter followed by anything other than a separator is
	// volume-relative and could escape the plugin dir on that drive.
	if len(relPath) >= 2 && relPath[1] == ':' {
		if len(relPath) == 2 || (relPath[2] != '/' && relPath[2] != '\\') {
			return true
		}
	}
	// Reject parent traversal. filepath.Clean collapses ".." segments;
	// if the cleaned path is exactly ".." or starts with ".." followed
	// by a separator, it would escape the plugin dir. We check both
	// separator forms for cross-platform test consistency.
	cleaned := filepath.Clean(relPath)
	if cleaned == ".." {
		return true
	}
	if strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "..\\") {
		return true
	}
	return false
}

// pluginNameRe restricts plugin names to lowercase kebab-case. This
// keeps names filesystem-safe and avoids collisions with built-in
// feature IDs.
var pluginNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// semverRe is a relaxed semver regex: MAJOR.MINOR.PATCH with optional
// pre-release and build metadata. Borrowed from the semver spec.
var semverRe = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// validPermissions is the set of permission scopes recognized by the
// frontend. Unknown permissions are kept (forward-compat) but the UI
// shows a warning.
var validPermissions = map[PluginPermission]bool{
	"fs.read":    true,
	"fs.write":   true,
	"shell.exec": true,
	"net":        true,
	"ai.send":    true,
}

// userPluginsSubdir is the user-global plugins directory name, relative
// to the config dir.
const userPluginsSubdir = "gugacode/plugins"

// projectPluginsRel is the project-scoped plugins directory, relative
// to the project root.
const projectPluginsRel = ".nknk/plugins"

// pluginStateFileName is the file name for persisted enabled/disabled
// state, written under <configDir>/gugacode/.
const pluginStateFileName = "plugins-state.json"

// Validate checks the manifest for required fields and well-formed
// values. Returns a descriptive error describing the first problem
// found, or nil if the manifest is valid.
func (m *PluginManifest) Validate() error {
	// SchemaVersion: 0 means unset (treated as 1 for backward compat).
	// Reject manifests declaring a future schema version we don't support.
	if m.SchemaVersion < 0 {
		return fmt.Errorf("plugin %q: schemaVersion cannot be negative", m.Name)
	}
	if m.SchemaVersion > currentPluginSchemaVersion {
		return fmt.Errorf("plugin %q: unsupported schemaVersion %d (this build supports up to %d); please update gugacode", m.Name, m.SchemaVersion, currentPluginSchemaVersion)
	}
	if !pluginNameRe.MatchString(m.Name) {
		return fmt.Errorf("invalid plugin name %q: must be lowercase kebab-case (e.g. \"my-plugin\")", m.Name)
	}
	if !semverRe.MatchString(m.Version) {
		return fmt.Errorf("invalid plugin version %q: expected semver (e.g. \"1.0.0\")", m.Version)
	}
	if m.Main == "" {
		return fmt.Errorf("plugin %q: main entry point is required", m.Name)
	}
	if isPluginPathOutsideRoot(m.Main) {
		return fmt.Errorf("plugin %q: main must be a relative path within the plugin directory (got %q)", m.Name, m.Main)
	}
	if !strings.HasSuffix(m.Main, ".js") {
		return fmt.Errorf("plugin %q: main entry point must be a .js file (got %q)", m.Name, m.Main)
	}
	if len(m.ActivationEvents) == 0 {
		return fmt.Errorf("plugin %q: at least one activationEvent is required (e.g. \"onStartup\")", m.Name)
	}
	for _, ev := range m.ActivationEvents {
		if ev == "" {
			return fmt.Errorf("plugin %q: activationEvent cannot be empty", m.Name)
		}
		if !strings.HasPrefix(ev, "on") {
			return fmt.Errorf("plugin %q: activationEvent %q must start with \"on\" (e.g. \"onStartup\", \"onCommand:foo\")", m.Name, ev)
		}
	}
	for _, p := range m.Permissions {
		if p == "" {
			return fmt.Errorf("plugin %q: permission cannot be empty", m.Name)
		}
		if !validPermissions[p] {
			return fmt.Errorf("plugin %q: unknown permission %q (recognized: fs.read, fs.write, shell.exec, net, ai.send)", m.Name, p)
		}
	}
	for i, c := range m.Contributes.Commands {
		if c.ID == "" {
			return fmt.Errorf("plugin %q: contributes.commands[%d].id is required", m.Name, i)
		}
		if c.Title == "" {
			return fmt.Errorf("plugin %q: contributes.commands[%d].title is required", m.Name, i)
		}
	}
	for i, v := range m.Contributes.Views {
		if v.ID == "" {
			return fmt.Errorf("plugin %q: contributes.views[%d].id is required", m.Name, i)
		}
		if v.Title == "" {
			return fmt.Errorf("plugin %q: contributes.views[%d].title is required", m.Name, i)
		}
		switch v.Location {
		case "", "sidebar", "panel", "statusbar":
			// ok
		default:
			return fmt.Errorf("plugin %q: contributes.views[%d].location %q is invalid (expected sidebar|panel|statusbar)", m.Name, i, v.Location)
		}
	}
	return nil
}

// PluginService discovers, validates, and tracks installed plugins
// (Plan 49). Plugins are discovered from two layers:
//   - User-global: <configDir>/gugacode/plugins/<name>/plugin.json
//   - Project-scoped: <projectRoot>/.nknk/plugins/<name>/plugin.json
//
// Project plugins override user plugins with the same name. The
// enabled/disabled state is persisted in <configDir>/gugacode/
// plugins-state.json so it survives project switches.
type PluginService struct {
	// configDir is the user-level config directory (e.g. ~/.config on
	// Linux, %APPDATA% on Windows). If empty, the user layer is
	// skipped and only project plugins are discovered.
	configDir string
}

// NewPluginService constructs a PluginService. configDir is the user
// config directory; pass empty to disable the user-global plugin layer.
func NewPluginService(configDir string) *PluginService {
	return &PluginService{configDir: configDir}
}

// ListPlugins returns all installed plugins from both layers, with
// project plugins overriding user plugins of the same name. The
// enabled state is loaded from the persisted state file. The result is
// sorted by plugin name for deterministic UI display.
func (s *PluginService) ListPlugins(projectRoot string) ([]PluginInfo, error) {
	merged := make(map[string]PluginInfo)

	// User layer first (lower priority).
	if s.configDir != "" {
		userDir := filepath.Join(s.configDir, userPluginsSubdir)
		if entries, err := scanPluginDir(userDir, PluginSourceUser); err == nil {
			for _, p := range entries {
				merged[p.Manifest.Name] = p
			}
		}
	}

	// Project layer (higher priority — overrides user).
	if projectRoot != "" {
		projDir := filepath.Join(projectRoot, projectPluginsRel)
		if entries, err := scanPluginDir(projDir, PluginSourceProject); err == nil {
			for _, p := range entries {
				merged[p.Manifest.Name] = p
			}
		}
	}

	// Apply enabled state.
	state := s.loadPluginState()
	out := make([]PluginInfo, 0, len(merged))
	for name, info := range merged {
		// N-111: a broken plugin (LoadError set) can never be activated
		// — its manifest couldn't be parsed/validated, so there's no
		// entry point to load. Force it disabled regardless of state.
		if info.LoadError != "" {
			info.Enabled = false
			out = append(out, info)
			continue
		}
		if entry, ok := state.Plugins[name]; ok {
			info.Enabled = entry.Enabled
		} else {
			info.Enabled = true // opt-out: enabled by default
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Manifest.Name < out[j].Manifest.Name
	})
	return out, nil
}

// GetPlugin returns a single plugin by name, or an error if not found.
func (s *PluginService) GetPlugin(name, projectRoot string) (PluginInfo, error) {
	plugins, err := s.ListPlugins(projectRoot)
	if err != nil {
		return PluginInfo{}, err
	}
	for _, p := range plugins {
		if p.Manifest.Name == name {
			return p, nil
		}
	}
	return PluginInfo{}, fmt.Errorf("plugin %q not found", name)
}

// SetPluginEnabled persists the enabled/disabled state for a plugin.
// The state is stored globally (not per-project) so a user's choice
// survives project switches.
func (s *PluginService) SetPluginEnabled(name string, enabled bool) error {
	if name == "" {
		return fmt.Errorf("plugin name is required")
	}
	state := s.loadPluginState()
	if state.Plugins == nil {
		state.Plugins = make(map[string]pluginStateEntry)
	}
	state.Plugins[name] = pluginStateEntry{Enabled: enabled}
	return s.savePluginState(state)
}

// ReadPluginFile reads a file from a plugin's directory, enforcing
// that relPath stays within the plugin directory (no traversal). Used
// by the frontend to dynamically import the plugin's main.js entry
// point and any bundled assets. Returns the file contents as bytes.
//
// N-56: filepath.Abs only does lexical cleaning — it does NOT resolve
// symlinks. A symlink inside the plugin dir pointing outside would
// pass the lexical prefix check. We resolve symlinks on both the
// target and the plugin root before the prefix comparison.
func (s *PluginService) ReadPluginFile(pluginName, relPath, projectRoot string) ([]byte, error) {
	info, err := s.GetPlugin(pluginName, projectRoot)
	if err != nil {
		return nil, err
	}
	if isPluginPathOutsideRoot(relPath) {
		return nil, fmt.Errorf("plugin file path must be relative to the plugin directory: %s", relPath)
	}
	full := filepath.Join(info.Path, relPath)
	// Resolve and verify the cleaned path is still inside the plugin dir.
	absFull, err := filepath.Abs(full)
	if err != nil {
		return nil, err
	}
	absRoot, err := filepath.Abs(info.Path)
	if err != nil {
		return nil, err
	}
	absFullResolved, err := evalSymlinksAllowMissing(absFull)
	if err != nil {
		return nil, err
	}
	absRootResolved, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		absRootResolved = absRoot
	}
	if !strings.HasPrefix(absFullResolved, absRootResolved+string(filepath.Separator)) && absFullResolved != absRootResolved {
		return nil, fmt.Errorf("plugin file path escapes plugin directory: %s", relPath)
	}
	data, err := os.ReadFile(absFull)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// pluginAssetMimeTypes maps file extensions to MIME types for serving
// plugin assets. Unknown extensions default to application/octet-stream.
var pluginAssetMimeTypes = map[string]string{
	".js":  "application/javascript",
	".mjs": "application/javascript",
	".json": "application/json",
	".css": "text/css",
	".html": "text/html",
	".svg": "image/svg+xml",
	".png": "image/png",
	".jpg": "image/jpeg",
	".jpeg": "image/jpeg",
	".gif": "image/gif",
	".woff": "font/woff",
	".woff2": "font/woff2",
	".ttf":  "font/ttf",
	".ico":  "image/x-icon",
}

// ServePluginAsset reads a file from a plugin's directory and returns
// the content + MIME type. It is called by the Wails asset middleware
// (registered in main.go) when a request hits /_plugins/<name>/<path>.
//
// This is the runtime side of Plan 58 / N-21: without a registered
// protocol handler, the frontend cannot dynamic-import plugin entry
// points. The middleware routes /_plugins/<name>/<path> to this method.
//
// Security: relPath is validated by isPluginPathOutsideRoot and the
// resolved path is checked to remain within the plugin directory.
// pluginName must match a discovered plugin (projectRoot is used to
// resolve project-scoped plugins).
func (s *PluginService) ServePluginAsset(pluginName, relPath, projectRoot string) ([]byte, string, error) {
	data, err := s.ReadPluginFile(pluginName, relPath, projectRoot)
	if err != nil {
		return nil, "", err
	}
	mime := pluginAssetMimeType(relPath)
	return data, mime, nil
}

// pluginAssetMimeType returns the MIME type for a plugin asset based on
// its file extension. Unknown extensions return application/octet-stream.
func pluginAssetMimeType(relPath string) string {
	ext := strings.ToLower(filepath.Ext(relPath))
	if mime, ok := pluginAssetMimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

// scanPluginDir scans a plugin layer directory for subdirectories
// containing a plugin.json manifest. Returns a map keyed by plugin
// name. Subdirectories without a valid manifest are silently skipped
// (best-effort discovery — a single broken plugin should not break
// listing). N-111: broken plugins (unreadable manifest, malformed JSON,
// or failed validation) are now included in the list with LoadError set
// instead of being silently dropped, so the user can see that a plugin
// is broken rather than wondering why it's missing.
func scanPluginDir(dir string, source PluginSource) ([]PluginInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []PluginInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pluginDir := filepath.Join(dir, entry.Name())
		manifestPath := filepath.Join(pluginDir, "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			// A missing manifest (os.IsNotExist) means the directory
			// isn't a plugin — skip it silently. But if the manifest
			// EXISTS and can't be read (permission denied, I/O error),
			// surface it as a broken plugin (N-111).
			if os.IsNotExist(err) {
				continue
			}
			out = append(out, PluginInfo{
				Manifest:  PluginManifest{Name: entry.Name()},
				Path:      pluginDir,
				Source:    source,
				Enabled:   false,
				LoadError: fmt.Sprintf("cannot read manifest: %v", err),
			})
			continue
		}
		var manifest PluginManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			out = append(out, PluginInfo{
				Manifest:  PluginManifest{Name: entry.Name()},
				Path:      pluginDir,
				Source:    source,
				Enabled:   false,
				LoadError: fmt.Sprintf("malformed manifest JSON: %v", err),
			})
			continue
		}
		if err := manifest.Validate(); err != nil {
			out = append(out, PluginInfo{
				Manifest:  manifest,
				Path:      pluginDir,
				Source:    source,
				Enabled:   false,
				LoadError: fmt.Sprintf("invalid manifest: %v", err),
			})
			continue
		}
		mainPath := filepath.Join(pluginDir, manifest.Main)
		_, statErr := os.Stat(mainPath)
		out = append(out, PluginInfo{
			Manifest:   manifest,
			Path:       pluginDir,
			Source:     source,
			Enabled:    true, // default; refined by caller
			MainExists: statErr == nil,
		})
	}
	return out, nil
}

// loadPluginState reads the persisted enabled/disabled state. Returns
// an empty state if the file is missing or corrupt (best-effort).
func (s *PluginService) loadPluginState() pluginStateFile {
	if s.configDir == "" {
		return pluginStateFile{Plugins: map[string]pluginStateEntry{}}
	}
	path := filepath.Join(s.configDir, "gugacode", pluginStateFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return pluginStateFile{Plugins: map[string]pluginStateEntry{}}
	}
	var state pluginStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return pluginStateFile{Plugins: map[string]pluginStateEntry{}}
	}
	if state.Plugins == nil {
		state.Plugins = map[string]pluginStateEntry{}
	}
	return state
}

// savePluginState writes the enabled/disabled state to disk.
func (s *PluginService) savePluginState(state pluginStateFile) error {
	if s.configDir == "" {
		return fmt.Errorf("user config directory is not configured")
	}
	path := filepath.Join(s.configDir, "gugacode", pluginStateFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create plugins state directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plugins state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write plugins state: %w", err)
	}
	return nil
}
