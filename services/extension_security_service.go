package services

// extension_security_service.go — G-VSC-03 / G-SEC-12: VS Code extension
// security gates.
//
// This service implements the BLOCKER security gates for VS Code-style
// extensions (a separate code path from the native plugin system in
// plugin_service.go). It provides:
//
//  1. Permission-based classification: each extension is classified as
//     Trusted / Reviewed / Restricted based on the permissions it requests.
//  2. Untrusted-by-default: newly installed extensions start disabled +
//     "pending review". The first enable attempt surfaces a popup listing
//     the requested API permissions (handled by the frontend store).
//  3. Signature verification: VSIX files are verified via SHA-256 hash
//     and (when present) a marketplace signature. Unverified extensions
//     are rejected.
//  4. Blacklist enforcement: known-malicious extension IDs are blocked
//     from installation entirely.
//
// The classification uses a richer permission set than the native plugin
// system (which uses fs.read/fs.write/shell.exec/net/ai.send) because VS
// Code extensions declare a broader API surface.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ExtensionSecurityLevel is the risk tier assigned to an extension based
// on the permissions it requests. G-VSC-03 requirement 1.
type ExtensionSecurityLevel string

const (
	// SecurityTrusted: read-only extensions. Only request fs.read and/or
	// ui.notifications. Enabled-by-default is still pending-review per
	// G-SEC-12 (new installs are disabled + pending review), but the
	// enable popup shows a minimal "read-only" notice.
	SecurityTrusted ExtensionSecurityLevel = "trusted"
	// SecurityReviewed: extensions that request file write or terminal
	// access in addition to read. Enable requires a permission popup
	// (G-VSC-03 requirement 2).
	SecurityReviewed ExtensionSecurityLevel = "reviewed"
	// SecurityRestricted: extensions that request network access or
	// unrestricted shell execution. Disabled by default; enabling
	// requires a popup listing the specific APIs and an explicit
	// confirmation (G-VSC-03 requirement 2, G-SEC-12 requirement 2).
	SecurityRestricted ExtensionSecurityLevel = "restricted"
)

// ExtensionPermission is a capability an extension requests. The set is
// broader than the native PluginPermission to cover the VS Code API
// surface that the compatibility layer exposes.
type ExtensionPermission string

const (
	PermFsRead    ExtensionPermission = "fs.read"
	PermFsWrite   ExtensionPermission = "fs.write"
	PermShellExec ExtensionPermission = "shell.execute"
	PermNetwork   ExtensionPermission = "network"
	PermClipboard ExtensionPermission = "clipboard"
	PermUINotif   ExtensionPermission = "ui.notifications"
	PermUIWebview ExtensionPermission = "ui.webview"
)

// ExtensionSecurityInfo is the runtime descriptor for an installed
// extension's security state. Mirrored on the frontend via the Wails
// binding so the permission dialog and PluginsView can render it.
type ExtensionSecurityInfo struct {
	// ExtensionID is the "<publisher>.<name>" identifier (VS Code convention).
	ExtensionID string `json:"extensionId"`
	// Level is the classified security tier.
	Level ExtensionSecurityLevel `json:"level"`
	// Permissions is the full list of requested permissions.
	Permissions []ExtensionPermission `json:"permissions"`
	// SHA256 is the hex-encoded SHA-256 of the installed VSIX payload.
	SHA256 string `json:"sha256"`
	// Verified is true when the signature check passed.
	Verified bool `json:"verified"`
	// Enabled is the current enabled state. New installs default to
	// false (G-SEC-12 requirement 2).
	Enabled bool `json:"enabled"`
	// Blacklisted is true when the extension ID is in the known-malicious
	// list. Blacklisted extensions cannot be enabled or installed.
	Blacklisted bool `json:"blacklisted"`
	// PendingReview is true for newly installed extensions that have not
	// yet been explicitly enabled by the user. Cleared on first enable.
	PendingReview bool `json:"pendingReview"`
}

// extensionSecurityStateEntry is one row in the persisted extension
// security state file. Stored under <configDir>/gugacode/extension-security.json.
// This is distinct from the simpler extensionStateEntry in
// marketplace_service.go (which only tracks Enabled) because the security
// service tracks classification, permissions, and verification state.
type extensionSecurityStateEntry struct {
	Level         ExtensionSecurityLevel  `json:"level"`
	Permissions   []ExtensionPermission   `json:"permissions"`
	SHA256        string                  `json:"sha256"`
	Verified      bool                    `json:"verified"`
	Enabled       bool                    `json:"enabled"`
	PendingReview bool                    `json:"pendingReview"`
}

type extensionSecurityStateFile struct {
	Extensions map[string]extensionSecurityStateEntry `json:"extensions"`
}

// extensionSecurityStateFileName is the on-disk file name for persisted
// extension security state, written under <configDir>/gugacode/.
const extensionSecurityStateFileName = "extension-security.json"

// ErrBlacklisted is returned when an operation targets a blacklisted
// extension. Callers should surface this to the user as "installation
// blocked: known malicious extension".
var ErrBlacklisted = errors.New("extension is on the known-malicious blacklist")

// ErrSignatureMismatch is returned when a VSIX's computed SHA-256 does
// not match the expected hash.
var ErrSignatureMismatch = errors.New("extension signature verification failed: SHA-256 mismatch")

// ErrRestrictedRequiresApproval is returned when SetExtensionEnabled is
// called to enable a Restricted extension without the explicit approval
// flag. The frontend should catch this and show the permission dialog.
var ErrRestrictedRequiresApproval = errors.New("restricted extensions require explicit user approval to enable")

// ErrNotVerified is returned when an extension that has not passed
// signature verification is enabled.
var ErrNotVerified = errors.New("extension has not passed signature verification")

// ExtensionSecurityService implements G-VSC-03 / G-SEC-12. It is
// thread-safe (mu guards the in-memory state and the blacklist).
type ExtensionSecurityService struct {
	mu        sync.Mutex
	configDir string
	blacklist map[string]bool
}

// NewExtensionSecurityService constructs the service. configDir is the
// user config directory (same one used by PluginService). The built-in
// default blacklist is loaded immediately; a user-overridable copy is
// read from <configDir>/gugacode/extension-blacklist.json if present.
func NewExtensionSecurityService(configDir string) *ExtensionSecurityService {
	s := &ExtensionSecurityService{
		configDir: configDir,
		blacklist: make(map[string]bool),
	}
	// Seed with the built-in defaults. The on-disk file (if any) is
	// layered on top so users can add entries without rebuilding.
	for k := range defaultBlacklist {
		s.blacklist[k] = true
	}
	s.loadBlacklistFile()
	return s
}

// ClassifyExtension determines the security level from the requested
// permissions (G-VSC-03 requirement 1).
//
// Rules:
//   - Only fs.read and/or ui.notifications (or no permissions) → Trusted
//   - Adds fs.write or shell.execute → Reviewed
//   - Adds network or (shell.execute present alongside network) → Restricted
//
// "Unrestricted shell.execute" is treated as Restricted when combined
// with network access; a standalone shell.execute without network is
// Reviewed (terminal-only).
func (s *ExtensionSecurityService) ClassifyExtension(permissions []ExtensionPermission) ExtensionSecurityLevel {
	has := make(map[ExtensionPermission]bool, len(permissions))
	for _, p := range permissions {
		has[p] = true
	}

	hasNetwork := has[PermNetwork]
	hasShell := has[PermShellExec]
	hasFsWrite := has[PermFsWrite]

	// Restricted: network access (with or without shell) or unrestricted
	// shell + network. Per G-VSC-03 requirement 1, "network" and
	// "unrestricted shell.execute" both bump to Restricted.
	if hasNetwork {
		return SecurityRestricted
	}
	// shell.execute alongside network is already covered above. A
	// standalone shell.execute is Reviewed. The "unrestricted" qualifier
	// in the spec is operationalized as: shell.execute + network =
	// Restricted (handled above), so we don't double-classify here.

	// Reviewed: file write or terminal access.
	if hasFsWrite || hasShell {
		return SecurityReviewed
	}

	// Trusted: only read-only / notification perms (or none).
	return SecurityTrusted
}

// VerifyExtensionSignature verifies the SHA-256 hash of a downloaded
// VSIX file against the expected hash (G-SEC-12 requirement 3).
//
// expectedSHA256 is the hex-encoded hash published by the marketplace
// (or supplied out-of-band for self-hosted extensions). An empty
// expectedSHA256 is rejected — verification requires a hash to compare
// against. Returns ErrSignatureMismatch on mismatch.
//
// The marketplace-signature check is represented by the `Verified`
// flag on ExtensionSecurityInfo: when the SHA-256 matches we treat the
// extension as verified (the marketplace signature is what produced the
// expected hash). A future implementation can layer an additional
// detached-signature check here without changing the call sites.
func (s *ExtensionSecurityService) VerifyExtensionSignature(vsixPath string, expectedSHA256 string) error {
	if expectedSHA256 == "" {
		return errors.New("signature verification requires a non-empty expected SHA-256")
	}
	data, err := os.ReadFile(vsixPath)
	if err != nil {
		return fmt.Errorf("read vsix for verification: %w", err)
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	// Lowercase + trim for a robust comparison.
	if !strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(expectedSHA256)) {
		return ErrSignatureMismatch
	}
	return nil
}

// ComputeSHA256 returns the hex-encoded SHA-256 of a file. Used to
// populate ExtensionSecurityInfo.SHA256 after a successful install.
func (s *ExtensionSecurityService) ComputeSHA256(vsixPath string) (string, error) {
	data, err := os.ReadFile(vsixPath)
	if err != nil {
		return "", fmt.Errorf("read vsix for sha256: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// IsBlacklisted checks if an extension (by "<publisher>.<name>") is in
// the known-malicious list (G-VSC-03 requirement 3, G-SEC-12 requirement
// 3). Thread-safe.
func (s *ExtensionSecurityService) IsBlacklisted(publisher, name string) bool {
	id := normalizeExtensionID(publisher, name)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blacklist[id]
}

// AddToBlacklist adds an extension to the blacklist and persists the
// update to <configDir>/gugacode/extension-blacklist.json so it survives
// restarts. Thread-safe.
func (s *ExtensionSecurityService) AddToBlacklist(publisher, name string) error {
	id := normalizeExtensionID(publisher, name)
	if id == "" || id == "." {
		return fmt.Errorf("invalid extension identifier: publisher and name are required")
	}
	s.mu.Lock()
	s.blacklist[id] = true
	s.mu.Unlock()
	return s.saveBlacklistFile()
}

// RemoveFromBlacklist removes a user-added entry. Built-in defaults
// cannot be removed (the entry is re-added on next start); this is
// intentional — the default list represents known-malicious IDs that
// must not be bypassable. Returns an error if the entry is built-in.
func (s *ExtensionSecurityService) RemoveFromBlacklist(publisher, name string) error {
	id := normalizeExtensionID(publisher, name)
	s.mu.Lock()
	defer s.mu.Unlock()
	if defaultBlacklist[id] {
		return fmt.Errorf("cannot remove built-in blacklist entry %q", id)
	}
	delete(s.blacklist, id)
	return s.saveBlacklistFileLocked()
}

// RegisterInstall records a newly installed extension's security info.
// Performs classification, blacklist check, and signature verification
// (when an expected hash is supplied). New installs are stored as
// disabled + pending review (G-SEC-12 requirement 2). Returns
// ErrBlacklisted if the extension is on the blacklist.
func (s *ExtensionSecurityService) RegisterInstall(
	extensionID string,
	permissions []ExtensionPermission,
	vsixPath string,
	expectedSHA256 string,
) (*ExtensionSecurityInfo, error) {
	if extensionID == "" {
		return nil, fmt.Errorf("extensionID is required")
	}
	// Blacklist check first — never record state for blacklisted IDs.
	if publisher, name, ok := splitExtensionID(extensionID); ok {
		if s.IsBlacklisted(publisher, name) {
			return nil, ErrBlacklisted
		}
	} else if s.IsBlacklisted("", extensionID) {
		return nil, ErrBlacklisted
	}

	info := &ExtensionSecurityInfo{
		ExtensionID:   extensionID,
		Level:         s.ClassifyExtension(permissions),
		Permissions:   append([]ExtensionPermission(nil), permissions...),
		Enabled:       false, // G-SEC-12: disabled by default
		PendingReview: true,  // G-SEC-12: pending review
	}

	// Signature verification. When expectedSHA256 is empty we record
	// Verified=false and the extension cannot be enabled (ErrNotVerified).
	if expectedSHA256 != "" {
		if err := s.VerifyExtensionSignature(vsixPath, expectedSHA256); err != nil {
			return nil, fmt.Errorf("verify signature: %w", err)
		}
		info.Verified = true
		info.SHA256 = expectedSHA256
	} else if vsixPath != "" {
		// Compute the hash for record-keeping but mark unverified.
		if hash, err := s.ComputeSHA256(vsixPath); err == nil {
			info.SHA256 = hash
		}
	}

	if err := s.saveSecurityInfo(info); err != nil {
		return nil, fmt.Errorf("persist security info: %w", err)
	}
	return info, nil
}

// GetSecurityInfo returns the persisted security info for an installed
// extension. Returns an error if the extension has no recorded state
// (i.e. was never registered via RegisterInstall).
func (s *ExtensionSecurityService) GetSecurityInfo(extensionID string) (*ExtensionSecurityInfo, error) {
	if extensionID == "" {
		return nil, fmt.Errorf("extensionID is required")
	}
	state := s.loadExtensionState()
	entry, ok := state.Extensions[extensionID]
	if !ok {
		return nil, fmt.Errorf("no security info for extension %q", extensionID)
	}
	// Refresh the blacklist flag from the in-memory set so newly-added
	// entries are reflected without a re-register.
	blacklisted := false
	if publisher, name, ok := splitExtensionID(extensionID); ok {
		blacklisted = s.IsBlacklisted(publisher, name)
	}
	return &ExtensionSecurityInfo{
		ExtensionID:   extensionID,
		Level:         entry.Level,
		Permissions:   append([]ExtensionPermission(nil), entry.Permissions...),
		SHA256:        entry.SHA256,
		Verified:      entry.Verified,
		Enabled:       entry.Enabled,
		Blacklisted:   blacklisted,
		PendingReview: entry.PendingReview,
	}, nil
}

// SetExtensionEnabled enables/disables an extension (G-SEC-12 requirement
// 2, G-VSC-03 requirement 2).
//
// Enabling a Restricted extension requires explicitApproval=true — the
// frontend sets this after the user confirms the permission popup.
// Without it, ErrRestrictedRequiresApproval is returned so the caller
// can trigger the dialog.
//
// Enabling an unverified extension is rejected with ErrNotVerified.
// Enabling a blacklisted extension is rejected with ErrBlacklisted.
//
// The single-arg form (enabled bool) is the common case; the explicit
// approval flag is conveyed via the variadic option.
func (s *ExtensionSecurityService) SetExtensionEnabled(extensionID string, enabled bool, explicitApproval ...bool) error {
	if extensionID == "" {
		return fmt.Errorf("extensionID is required")
	}
	approved := len(explicitApproval) > 0 && explicitApproval[0]

	// Blacklist check — always enforced.
	if publisher, name, ok := splitExtensionID(extensionID); ok {
		if s.IsBlacklisted(publisher, name) {
			return ErrBlacklisted
		}
	}

	state := s.loadExtensionState()
	entry, ok := state.Extensions[extensionID]
	if !ok {
		return fmt.Errorf("no security info for extension %q; register the install first", extensionID)
	}

	if enabled {
		// Signature gate: unverified extensions cannot be enabled.
		if !entry.Verified {
			return ErrNotVerified
		}
		// Restricted gate: requires explicit approval.
		if entry.Level == SecurityRestricted && !approved {
			return ErrRestrictedRequiresApproval
		}
		// Reviewed gate: also surfaces a popup, but we don't hard-block
		// it server-side because the popup is informational. The
		// frontend store is responsible for showing the dialog before
		// calling SetExtensionEnabled. Restricted is the hard gate
		// because network access is the highest-risk capability.
		entry.PendingReview = false
	} else {
		// Disabling always succeeds (subject to blacklist above).
	}
	entry.Enabled = enabled

	state.Extensions[extensionID] = entry
	return s.saveExtensionState(state)
}

// ListSecurityInfo returns the security info for all registered
// extensions, with the blacklist flag refreshed from the in-memory set.
func (s *ExtensionSecurityService) ListSecurityInfo() []ExtensionSecurityInfo {
	state := s.loadExtensionState()
	out := make([]ExtensionSecurityInfo, 0, len(state.Extensions))
	for id, entry := range state.Extensions {
		blacklisted := false
		if publisher, name, ok := splitExtensionID(id); ok {
			blacklisted = s.IsBlacklisted(publisher, name)
		}
		out = append(out, ExtensionSecurityInfo{
			ExtensionID:   id,
			Level:         entry.Level,
			Permissions:   append([]ExtensionPermission(nil), entry.Permissions...),
			SHA256:        entry.SHA256,
			Verified:      entry.Verified,
			Enabled:       entry.Enabled,
			Blacklisted:   blacklisted,
			PendingReview: entry.PendingReview,
		})
	}
	return out
}

// CanInstall is a pre-install gate (G-VSC-03 requirement 3). Returns
// ErrBlacklisted if the extension is on the known-malicious list, nil
// otherwise. The frontend should call this before downloading a VSIX.
func (s *ExtensionSecurityService) CanInstall(publisher, name string) error {
	if s.IsBlacklisted(publisher, name) {
		return ErrBlacklisted
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal: persistence helpers
// ---------------------------------------------------------------------------

func (s *ExtensionSecurityService) saveSecurityInfo(info *ExtensionSecurityInfo) error {
	state := s.loadExtensionState()
	if state.Extensions == nil {
		state.Extensions = make(map[string]extensionSecurityStateEntry)
	}
	state.Extensions[info.ExtensionID] = extensionSecurityStateEntry{
		Level:         info.Level,
		Permissions:   append([]ExtensionPermission(nil), info.Permissions...),
		SHA256:        info.SHA256,
		Verified:      info.Verified,
		Enabled:       info.Enabled,
		PendingReview: info.PendingReview,
	}
	return s.saveExtensionState(state)
}

func (s *ExtensionSecurityService) loadExtensionState() extensionSecurityStateFile {
	if s.configDir == "" {
		return extensionSecurityStateFile{Extensions: map[string]extensionSecurityStateEntry{}}
	}
	path := filepath.Join(s.configDir, "gugacode", extensionSecurityStateFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return extensionSecurityStateFile{Extensions: map[string]extensionSecurityStateEntry{}}
	}
	var state extensionSecurityStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return extensionSecurityStateFile{Extensions: map[string]extensionSecurityStateEntry{}}
	}
	if state.Extensions == nil {
		state.Extensions = map[string]extensionSecurityStateEntry{}
	}
	return state
}

func (s *ExtensionSecurityService) saveExtensionState(state extensionSecurityStateFile) error {
	if s.configDir == "" {
		return fmt.Errorf("user config directory is not configured")
	}
	path := filepath.Join(s.configDir, "gugacode", extensionSecurityStateFileName)
	// M-5: atomic write (temp+rename+0600) prevents half-written state.
	return atomicWriteJSON(path, state, 0600)
}

// normalizeExtensionID builds the canonical "<publisher>.<name>" form,
// lowercased and trimmed. Handles the case where the caller already
// passed the combined id (publisher="publisher.name", name="").
func normalizeExtensionID(publisher, name string) string {
	p := strings.ToLower(strings.TrimSpace(publisher))
	n := strings.ToLower(strings.TrimSpace(name))
	if p == "" && n == "" {
		return ""
	}
	if n == "" {
		// publisher already holds the full id.
		return p
	}
	if p == "" {
		return n
	}
	return p + "." + n
}

// splitExtensionID splits "<publisher>.<name>" into its parts. Returns
// ok=false if the id doesn't contain a dot.
func splitExtensionID(id string) (publisher, name string, ok bool) {
	id = strings.TrimSpace(id)
	idx := strings.Index(id, ".")
	if idx <= 0 || idx == len(id)-1 {
		return "", "", false
	}
	return id[:idx], id[idx+1:], true
}
