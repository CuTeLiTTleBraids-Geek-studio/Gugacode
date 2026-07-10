package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// ProfileService manages multiple user profiles (Plan 50). Each profile
// is a directory under <configDir>/gugacode/profiles/<name>/ that
// contains a settings.json file (and potentially other per-profile
// state in the future). The active profile is tracked in
// <configDir>/gugacode/profiles-state.json.
//
// Migration: on first call, if the profiles directory does not exist
// but a legacy <configDir>/gugacode/settings.json does, the legacy
// file is copied into profiles/default/settings.json and "default"
// becomes the active profile. This preserves backward compatibility —
// existing users get a "default" profile with their current settings.
//
// The SettingsService is made profile-aware by having its config path
// point at the active profile's settings.json. ProfileService.SetActive
// updates the state file and (via a callback registered by main.go)
// redirects the SettingsService to the new path.
type ProfileService struct {
	configDir    string // original config dir (empty = disabled)
	rootDir      string // <configDir>/gugacode
	profilesDir  string // <rootDir>/profiles
	statePath    string // <rootDir>/profiles-state.json
	legacyPath   string // <rootDir>/settings.json (pre-profile migration source)
	onSwitchFunc func(settingsPath string) // callback to redirect SettingsService
}

// ProfileInfo is the runtime descriptor for a profile. Pairs the
// profile name with filesystem metadata and the active flag.
type ProfileInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   int64  `json:"createdAt,omitempty"`
	ModifiedAt  int64  `json:"modifiedAt,omitempty"`
	Active      bool   `json:"active"`
}

// profileStateFile is the on-disk shape of profiles-state.json.
type profileStateFile struct {
	ActiveProfile string `json:"activeProfile"`
}

// profileMeta is an optional metadata file inside each profile
// directory (profile.json). Currently only stores Description; other
// fields are derived from filesystem timestamps.
type profileMeta struct {
	Description string `json:"description,omitempty"`
}

// profileNameRe restricts profile names to lowercase kebab-case so
// they are filesystem-safe across platforms. "default" is reserved
// and always exists.
var profileNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// defaultProfileName is the name of the auto-created default profile.
const defaultProfileName = "default"

// NewProfileService constructs a ProfileService rooted at
// <configDir>/gugacode. Pass empty configDir to disable all
// filesystem operations (used in some tests).
func NewProfileService(configDir string) *ProfileService {
	root := filepath.Join(configDir, "gugacode")
	return &ProfileService{
		configDir:   configDir,
		rootDir:     root,
		profilesDir: filepath.Join(root, "profiles"),
		statePath:   filepath.Join(root, "profiles-state.json"),
		legacyPath:  filepath.Join(root, "settings.json"),
	}
}

// SetOnSwitch registers a callback invoked after a successful profile
// switch. The callback receives the absolute path to the newly active
// profile's settings.json so the SettingsService can be redirected.
// This decouples ProfileService from SettingsService at construction
// time (avoids an import cycle concern in main.go wiring order).
func (s *ProfileService) SetOnSwitch(fn func(settingsPath string)) {
	s.onSwitchFunc = fn
}

// ensureProfilesDir creates the profiles directory if missing and
// performs legacy migration. Idempotent.
func (s *ProfileService) ensureProfilesDir() error {
	if s.configDir == "" {
		return fmt.Errorf("config directory is not configured")
	}
	if err := os.MkdirAll(s.profilesDir, 0o755); err != nil {
		return fmt.Errorf("create profiles directory: %w", err)
	}
	// Auto-create default profile if missing.
	defaultDir := filepath.Join(s.profilesDir, defaultProfileName)
	if _, err := os.Stat(defaultDir); os.IsNotExist(err) {
		if err := os.MkdirAll(defaultDir, 0o755); err != nil {
			return fmt.Errorf("create default profile: %w", err)
		}
		// Migrate legacy settings.json if present, otherwise write defaults.
		defaultSettingsPath := filepath.Join(defaultDir, "settings.json")
		if data, rerr := os.ReadFile(s.legacyPath); rerr == nil {
			// Legacy file exists — copy to default profile.
			if werr := atomicWriteFile(defaultSettingsPath, data, 0600); werr != nil {
				return fmt.Errorf("migrate legacy settings: %w", werr)
			}
		} else {
			// No legacy file — write defaults.
			if werr := atomicWriteFile(defaultSettingsPath, defaultsBytes(), 0600); werr != nil {
				return fmt.Errorf("write default profile settings: %w", werr)
			}
		}
	}
	// Ensure state file exists with "default" as active if missing.
	if _, err := os.Stat(s.statePath); os.IsNotExist(err) {
		if werr := atomicWriteFile(s.statePath, []byte(`{"activeProfile":"default"}`), 0600); werr != nil {
			return fmt.Errorf("create profiles state: %w", werr)
		}
	}
	return nil
}

// defaultsBytes returns the JSON encoding of defaultSettings(). Used
// during initial profile creation. Errors are ignored (defaults are
// simple and always encodable).
func defaultsBytes() []byte {
	data, err := json.MarshalIndent(defaultSettings(), "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return data
}

// loadState reads the active profile name from the state file. Returns
// "default" if the file is missing or corrupt.
func (s *ProfileService) loadState() string {
	data, err := os.ReadFile(s.statePath)
	if err != nil {
		return defaultProfileName
	}
	var state profileStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return defaultProfileName
	}
	if state.ActiveProfile == "" {
		return defaultProfileName
	}
	return state.ActiveProfile
}

// saveState writes the active profile name to the state file.
func (s *ProfileService) saveState(name string) error {
	if s.configDir == "" {
		return fmt.Errorf("config directory is not configured")
	}
	state := profileStateFile{ActiveProfile: name}
	// G-SEC-09: atomic write (temp file + rename).
	if err := atomicWriteJSON(s.statePath, state, 0644); err != nil {
		return fmt.Errorf("save profiles state: %w", err)
	}
	return nil
}

// ListProfiles returns all profiles, sorted by name, with the active
// profile flagged. Performs initial migration if needed.
func (s *ProfileService) ListProfiles() ([]ProfileInfo, error) {
	if err := s.ensureProfilesDir(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.profilesDir)
	if err != nil {
		return nil, fmt.Errorf("read profiles directory: %w", err)
	}
	active := s.loadState()
	var out []ProfileInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !profileNameRe.MatchString(name) {
			continue // skip non-conforming directories
		}
		info, err := s.buildProfileInfo(name, active)
		if err != nil {
			continue // skip broken profiles
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		// "default" always sorts first
		if out[i].Name == defaultProfileName {
			return true
		}
		if out[j].Name == defaultProfileName {
			return false
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// buildProfileInfo constructs a ProfileInfo from filesystem metadata.
func (s *ProfileService) buildProfileInfo(name, active string) (ProfileInfo, error) {
	dir := filepath.Join(s.profilesDir, name)
	info, err := os.Stat(dir)
	if err != nil {
		return ProfileInfo{}, err
	}
	pi := ProfileInfo{
		Name:        name,
		CreatedAt:   info.ModTime().Unix(),
		ModifiedAt:  info.ModTime().Unix(),
		Active:      name == active,
	}
	// Load optional description from profile.json.
	metaPath := filepath.Join(dir, "profile.json")
	if data, merr := os.ReadFile(metaPath); merr == nil {
		var meta profileMeta
		if jerr := json.Unmarshal(data, &meta); jerr == nil {
			pi.Description = meta.Description
		}
	}
	// If settings.json was modified more recently, use its mtime.
	settingsPath := filepath.Join(dir, "settings.json")
	if sinfo, serr := os.Stat(settingsPath); serr == nil {
		pi.ModifiedAt = sinfo.ModTime().Unix()
	}
	return pi, nil
}

// GetActiveProfile returns the name of the currently active profile.
// Performs initial migration if needed. Returns "default" if no
// profile has been set.
func (s *ProfileService) GetActiveProfile() (string, error) {
	if err := s.ensureProfilesDir(); err != nil {
		return "", err
	}
	return s.loadState(), nil
}

// ActiveSettingsPath returns the absolute path to the active
// profile's settings.json. Used by main.go to initialize
// SettingsService with the correct path.
func (s *ProfileService) ActiveSettingsPath() (string, error) {
	if err := s.ensureProfilesDir(); err != nil {
		return "", err
	}
	active := s.loadState()
	return filepath.Join(s.profilesDir, active, "settings.json"), nil
}

// SetActiveProfile switches the active profile. Returns an error if
// the profile does not exist. On success, saves the state file and
// invokes the onSwitch callback (if registered) with the new
// settings.json path so SettingsService can be redirected.
func (s *ProfileService) SetActiveProfile(name string) error {
	if !profileNameRe.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: must be lowercase kebab-case", name)
	}
	if err := s.ensureProfilesDir(); err != nil {
		return err
	}
	dir := filepath.Join(s.profilesDir, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("profile %q does not exist", name)
	}
	if err := s.saveState(name); err != nil {
		return fmt.Errorf("save active profile: %w", err)
	}
	if s.onSwitchFunc != nil {
		s.onSwitchFunc(filepath.Join(dir, "settings.json"))
	}
	return nil
}

// CreateProfile creates a new profile directory. If fromCurrent is
// true, copies the active profile's settings.json; otherwise writes
// default settings. Returns an error if the profile already exists.
func (s *ProfileService) CreateProfile(name string, fromCurrent bool) error {
	if !profileNameRe.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: must be lowercase kebab-case", name)
	}
	if name == defaultProfileName {
		return fmt.Errorf("profile name %q is reserved", name)
	}
	if err := s.ensureProfilesDir(); err != nil {
		return err
	}
	dir := filepath.Join(s.profilesDir, name)
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("profile %q already exists", name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}
	settingsPath := filepath.Join(dir, "settings.json")
	if fromCurrent {
		// N-110: ActiveSettingsPath failure was silently swallowed. If it
		// fails (e.g. profiles dir can't be created), we can't copy the
		// current settings — log the failure and fall through to the
		// defaults path below so the new profile is still usable.
		active, aerr := s.ActiveSettingsPath()
		if aerr != nil {
			slog.Warn("could not resolve active settings path; using defaults for new profile",
				"name", name, "err", aerr)
		}
		if active != "" {
			if data, rerr := os.ReadFile(active); rerr == nil {
				if werr := atomicWriteFile(settingsPath, data, 0600); werr != nil {
					return fmt.Errorf("copy settings to new profile: %w", werr)
				}
			} else {
				// Fall back to defaults if active profile has no settings.
				if werr := atomicWriteFile(settingsPath, defaultsBytes(), 0600); werr != nil {
					return fmt.Errorf("write default settings: %w", werr)
				}
			}
		} else {
			// No active path resolved — write defaults.
			if werr := atomicWriteFile(settingsPath, defaultsBytes(), 0600); werr != nil {
				return fmt.Errorf("write default settings: %w", werr)
			}
		}
	} else {
		if werr := atomicWriteFile(settingsPath, defaultsBytes(), 0600); werr != nil {
			return fmt.Errorf("write default settings: %w", werr)
		}
	}
	return nil
}

// DeleteProfile removes a profile directory. The "default" profile
// and the currently active profile cannot be deleted.
func (s *ProfileService) DeleteProfile(name string) error {
	if !profileNameRe.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: must be lowercase kebab-case", name)
	}
	if name == defaultProfileName {
		return fmt.Errorf("cannot delete the default profile")
	}
	if err := s.ensureProfilesDir(); err != nil {
		return err
	}
	active := s.loadState()
	if name == active {
		return fmt.Errorf("cannot delete the active profile (switch to another profile first)")
	}
	dir := filepath.Join(s.profilesDir, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("profile %q does not exist", name)
	}
	if err := removeProfileDir(dir); err != nil {
		return fmt.Errorf("delete profile directory: %w", err)
	}
	return nil
}

// removeProfileDir deletes a profile directory by removing its contents
// first, then the directory itself. This is more reliable than
// os.RemoveAll on Windows, where RemoveAll can report success while the
// directory is still visible due to deferred filesystem operations.
func removeProfileDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			if rerr := removeProfileDir(path); rerr != nil {
				return rerr
			}
		} else {
			// Clear read-only attribute if set (Windows).
			_ = os.Chmod(path, 0o644)
			if rerr := os.Remove(path); rerr != nil {
				return rerr
			}
		}
	}
	return os.Remove(dir)
}

// RenameProfile renames a profile. The "default" profile cannot be
// renamed. The target name must not already exist.
func (s *ProfileService) RenameProfile(oldName, newName string) error {
	if !profileNameRe.MatchString(oldName) {
		return fmt.Errorf("invalid old profile name %q", oldName)
	}
	if !profileNameRe.MatchString(newName) {
		return fmt.Errorf("invalid new profile name %q: must be lowercase kebab-case", newName)
	}
	if oldName == defaultProfileName {
		return fmt.Errorf("cannot rename the default profile")
	}
	if newName == defaultProfileName {
		return fmt.Errorf("profile name %q is reserved", newName)
	}
	if oldName == newName {
		return nil
	}
	if err := s.ensureProfilesDir(); err != nil {
		return err
	}
	oldDir := filepath.Join(s.profilesDir, oldName)
	newDir := filepath.Join(s.profilesDir, newName)
	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return fmt.Errorf("profile %q does not exist", oldName)
	}
	if _, err := os.Stat(newDir); err == nil {
		return fmt.Errorf("profile %q already exists", newName)
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		return fmt.Errorf("rename profile: %w", err)
	}
	// If we renamed the active profile, update the state file.
	if s.loadState() == oldName {
		if err := s.saveState(newName); err != nil {
			return fmt.Errorf("update active profile name: %w", err)
		}
	}
	return nil
}

// SetProfileDescription updates the description metadata for a
// profile. Creates profile.json if it does not exist.
func (s *ProfileService) SetProfileDescription(name, description string) error {
	if !profileNameRe.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: must be lowercase kebab-case", name)
	}
	if err := s.ensureProfilesDir(); err != nil {
		return err
	}
	dir := filepath.Join(s.profilesDir, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("profile %q does not exist", name)
	}
	metaPath := filepath.Join(dir, "profile.json")
	meta := profileMeta{Description: description}
	// G-SEC-09: atomic write (temp file + rename).
	if err := atomicWriteJSON(metaPath, meta, 0644); err != nil {
		return fmt.Errorf("write profile meta: %w", err)
	}
	// Update ModifiedAt on the directory. N-110: this was silently
	// ignored. Chtimes failure is cosmetic (the mtime is used for
	// sorting/display only), so we log rather than fail — but it
	// shouldn't be invisible if e.g. permissions are wrong.
	if cerr := os.Chtimes(dir, time.Now(), time.Now()); cerr != nil {
		slog.Debug("could not update profile directory mtime", "dir", dir, "err", cerr)
	}
	return nil
}

// ExportProfile returns the JSON content of a profile's settings.json
// along with optional metadata, packaged as a single JSON blob for
// the frontend to save as a file. Returns an error if the profile
// does not exist.
type ProfileExport struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Settings    json.RawMessage `json:"settings"`
	ExportedAt  int64           `json:"exportedAt"`
}

func (s *ProfileService) ExportProfile(name string) (ProfileExport, error) {
	if !profileNameRe.MatchString(name) {
		return ProfileExport{}, fmt.Errorf("invalid profile name %q: must be lowercase kebab-case", name)
	}
	if err := s.ensureProfilesDir(); err != nil {
		return ProfileExport{}, err
	}
	dir := filepath.Join(s.profilesDir, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return ProfileExport{}, fmt.Errorf("profile %q does not exist", name)
	}
	settingsPath := filepath.Join(dir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return ProfileExport{}, fmt.Errorf("read profile settings: %w", err)
	}
	export := ProfileExport{
		Name:       name,
		Settings:   json.RawMessage(data),
		ExportedAt: time.Now().Unix(),
	}
	// Include description from profile.json if present.
	if metaData, merr := os.ReadFile(filepath.Join(dir, "profile.json")); merr == nil {
		var meta profileMeta
		if jerr := json.Unmarshal(metaData, &meta); jerr == nil {
			export.Description = meta.Description
		}
	}
	return export, nil
}

// ImportProfile creates a new profile from an exported JSON blob.
// The name in the export is used unless it conflicts with an existing
// profile or "default", in which case a numeric suffix is appended.
// Returns the actual name used.
func (s *ProfileService) ImportProfile(export ProfileExport) (string, error) {
	if err := s.ensureProfilesDir(); err != nil {
		return "", err
	}
	name := export.Name
	if !profileNameRe.MatchString(name) || name == defaultProfileName {
		name = "imported"
	}
	// Avoid collisions by appending -2, -3, etc.
	finalName := name
	counter := 2
	for {
		dir := filepath.Join(s.profilesDir, finalName)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			break
		}
		finalName = fmt.Sprintf("%s-%d", name, counter)
		counter++
		if counter > 1000 {
			return "", fmt.Errorf("could not find a unique name for imported profile")
		}
	}
	dir := filepath.Join(s.profilesDir, finalName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create imported profile directory: %w", err)
	}
	settingsPath := filepath.Join(dir, "settings.json")
	if err := atomicWriteFile(settingsPath, []byte(export.Settings), 0600); err != nil {
		return "", fmt.Errorf("write imported settings: %w", err)
	}
	// Write description if provided. N-110: the previous implementation
	// ignored both marshal and write errors, reporting success even when
	// disk-full or permission denied prevented the metadata from being
	// saved. The settings.json is already written, so the profile is
	// usable — but we surface the meta-write failure so the caller can
	// inform the user that the description wasn't saved.
	if export.Description != "" {
		meta := profileMeta{Description: export.Description}
		// G-SEC-09: atomic write (temp file + rename).
		if werr := atomicWriteJSON(filepath.Join(dir, "profile.json"), meta, 0644); werr != nil {
			return finalName, fmt.Errorf("write imported profile meta: %w", werr)
		}
	}
	return finalName, nil
}
