package services

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// marketplace_service_test.go — G-VSC-01 tests.
//
// These tests exercise the security gates and lifecycle of the marketplace
// service without hitting the network. They build mock VSIX (zip) files in
// memory and drive installFromVSIXData directly, covering:
//   - VSIX path traversal protection (G-SEC-12: malicious "../../" entries)
//   - SHA-256 verification (G-SEC-12 req. 3: mismatched hash rejected)
//   - Default-disabled on install (G-SEC-12 req. 2 / G-VSC-03 req. 2)
//   - ListInstalledExtensions
//   - UninstallExtension

// newTestMarketplaceService returns a MarketplaceService rooted at a temp
// config dir. The temp dir is cleaned up automatically by testing.T.
func newTestMarketplaceService(t *testing.T) (*MarketplaceService, string) {
	t.Helper()
	dir := t.TempDir()
	return NewMarketplaceService(dir), dir
}

// zipEntry is a single file to write into a mock VSIX.
type zipEntry struct {
	Name string
	Data []byte
	// Mode is the zip entry's file mode (used to simulate symlinks). Zero
	// means a regular file; directories are inferred from a trailing "/".
	Mode uint32
}

// buildVSIX builds a VSIX (zip) in memory from the given entries. Returns
// the raw bytes and the hex-encoded SHA-256 of those bytes.
func buildVSIX(t *testing.T, entries []zipEntry) ([]byte, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{
			Name:   e.Name,
			Method: zip.Deflate,
		}
		if e.Mode != 0 {
			hdr.SetMode(os.FileMode(e.Mode))
		}
		f, err := w.CreateHeader(hdr)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", e.Name, err)
		}
		if _, err := f.Write(e.Data); err != nil {
			t.Fatalf("write zip entry %q: %v", e.Name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	data := buf.Bytes()
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:])
}

// validPackageJSON is a minimal VS Code extension package.json payload used
// by the well-formed VSIX fixtures.
const validPackageJSON = `{
  "name": "hello",
  "publisher": "acme",
  "version": "1.0.0",
  "displayName": "Hello",
  "description": "A test extension",
  "engines": { "vscode": "^1.80.0" },
  "activationEvents": ["onStartupFinished"],
  "contributes": { "commands": [{ "command": "acme.hello", "title": "Hello" }] },
  "capabilities": { "untrustedWorkspaces": { "supported": true } }
}`

// buildValidVSIX builds a well-formed VSIX with extension/package.json and a
// dummy runtime file. Returns the bytes and their SHA-256.
func buildValidVSIX(t *testing.T) ([]byte, string) {
	t.Helper()
	return buildVSIX(t, []zipEntry{
		{Name: "extension/package.json", Data: []byte(validPackageJSON)},
		{Name: "extension/main.js", Data: []byte("export function activate() {}\n")},
		{Name: "[Content_Types].xml", Data: []byte("<Types/>")},
	})
}

// --- SHA-256 verification (G-SEC-12 req. 3) ---

// TestMarketplaceInstall_Sha256MismatchRejected verifies that a VSIX whose
// computed SHA-256 does not match the registry-provided hash is rejected
// before any file is extracted.
func TestMarketplaceInstall_Sha256MismatchRejected(t *testing.T) {
	svc, _ := newTestMarketplaceService(t)
	vsix, _ := buildValidVSIX(t)
	// Deliberately wrong hash.
	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"
	err := svc.installFromVSIXData(vsix, wrongHash, "acme", "hello", "1.0.0")
	if err == nil {
		t.Fatal("expected SHA-256 mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "SHA-256 verification failed") {
		t.Fatalf("expected SHA-256 verification error, got: %v", err)
	}
	// No extension directory should have been created on rejection.
	dir := filepath.Join(svc.configDir, extensionsSubdir, "acme.hello")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("extension dir should not exist after a rejected install; stat err=%v", err)
	}
}

// TestMarketplaceInstall_Sha256MatchAccepted verifies that a matching hash
// allows the install to proceed.
func TestMarketplaceInstall_Sha256MatchAccepted(t *testing.T) {
	svc, _ := newTestMarketplaceService(t)
	vsix, wantHash := buildValidVSIX(t)
	if err := svc.installFromVSIXData(vsix, wantHash, "acme", "hello", "1.0.0"); err != nil {
		t.Fatalf("install with matching hash failed: %v", err)
	}
	// The extension directory should now exist with the extracted payload.
	manifestPath := filepath.Join(svc.configDir, extensionsSubdir, "acme.hello", "extension", "package.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("extracted package.json should exist: %v", err)
	}
}

// --- Path traversal protection (G-SEC-12) ---

// TestMarketplaceInstall_PathTraversalRejected verifies that a malicious VSIX
// containing entries with "../" traversal is rejected and nothing is written
// outside the install directory.
func TestMarketplaceInstall_PathTraversalRejected(t *testing.T) {
	svc, configDir := newTestMarketplaceService(t)
	// Build a VSIX whose entry escapes the extension directory.
	malicious, wantHash := buildVSIX(t, []zipEntry{
		{Name: "extension/package.json", Data: []byte(validPackageJSON)},
		{Name: "../../evil.txt", Data: []byte("pwned")},
	})
	err := svc.installFromVSIXData(malicious, wantHash, "acme", "hello", "1.0.0")
	if err == nil {
		t.Fatal("expected path traversal rejection, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "traversal") && !strings.Contains(strings.ToLower(err.Error()), "escapes") && !strings.Contains(strings.ToLower(err.Error()), "outside") {
		t.Fatalf("expected traversal-related error, got: %v", err)
	}
	// The malicious payload must NOT have escaped into the config dir's parent.
	evilPath := filepath.Join(configDir, "evil.txt")
	if _, err := os.Stat(evilPath); !os.IsNotExist(err) {
		t.Fatalf("traversal payload leaked to %s; stat err=%v", evilPath, err)
	}
	// And not in the parent of configDir either.
	parentEvil := filepath.Join(filepath.Dir(configDir), "evil.txt")
	if _, err := os.Stat(parentEvil); !os.IsNotExist(err) {
		t.Fatalf("traversal payload leaked to parent %s; stat err=%v", parentEvil, err)
	}
	// The install directory should have been cleaned up (no half-installed ext).
	dir := filepath.Join(svc.configDir, extensionsSubdir, "acme.hello")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("extension dir should not exist after a rejected install; stat err=%v", err)
	}
}

// TestMarketplaceInstall_AbsolutePathEntryRejected verifies that an absolute
// entry path is rejected (another traversal vector).
func TestMarketplaceInstall_AbsolutePathEntryRejected(t *testing.T) {
	svc, _ := newTestMarketplaceService(t)
	// Use a Unix-style absolute path entry. (Drive-letter absolute forms are
	// platform-specific; the leading-slash form is the cross-platform guard.)
	abs, wantHash := buildVSIX(t, []zipEntry{
		{Name: "extension/package.json", Data: []byte(validPackageJSON)},
		{Name: "/etc/evil.txt", Data: []byte("pwned")},
	})
	err := svc.installFromVSIXData(abs, wantHash, "acme", "hello", "1.0.0")
	if err == nil {
		t.Fatal("expected absolute-path rejection, got nil")
	}
}

// TestMarketplaceInstall_SymlinkEntryRejected verifies that a symlink zip
// entry is rejected (symlinks could point outside the install dir).
func TestMarketplaceInstall_SymlinkEntryRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("symlink test skipped in short mode")
	}
	svc, _ := newTestMarketplaceService(t)
	// archive/zip's FileHeader.SetMode checks for Go's fs.ModeSymlink type
	// bit (1<<27), NOT the Unix S_IFLNK bits. So to simulate a real symlink
	// entry (the way a Unix zip tool would encode one), we pass the Go
	// FileMode os.ModeSymlink|0777. SetMode then encodes S_IFLNK into the
	// upper 16 bits of ExternalAttrs; on read-back msModeToFileMode maps
	// that back to fs.ModeSymlink, so f.Mode()&os.ModeSymlink triggers the
	// production guard. Passing raw 0xA1FF would NOT work — SetMode would
	// treat it as a regular file (no ModeSymlink bit) and store S_IFREG.
	symlinkVSIX, wantHash := buildVSIX(t, []zipEntry{
		{Name: "extension/package.json", Data: []byte(validPackageJSON)},
		{Name: "extension/link", Data: []byte("../../../../etc/passwd"), Mode: uint32(os.ModeSymlink | 0o777)},
	})
	err := svc.installFromVSIXData(symlinkVSIX, wantHash, "acme", "hello", "1.0.0")
	if err == nil {
		t.Fatal("expected symlink rejection, got nil")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got: %v", err)
	}
}

// --- Default disabled (G-SEC-12 req. 2 / G-VSC-03 req. 2) ---

// TestMarketplaceInstall_DefaultDisabled verifies that a freshly installed
// extension is disabled by default.
func TestMarketplaceInstall_DefaultDisabled(t *testing.T) {
	svc, _ := newTestMarketplaceService(t)
	vsix, wantHash := buildValidVSIX(t)
	if err := svc.installFromVSIXData(vsix, wantHash, "acme", "hello", "1.0.0"); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	installed, err := svc.ListInstalledExtensions()
	if err != nil {
		t.Fatalf("list installed: %v", err)
	}
	if len(installed) != 1 {
		t.Fatalf("expected 1 installed extension, got %d", len(installed))
	}
	ext := installed[0]
	if ext.Enabled {
		t.Errorf("newly installed extension should be disabled by default (G-SEC-12 req. 2); got Enabled=true")
	}
	if ext.Publisher != "acme" || ext.Name != "hello" {
		t.Errorf("unexpected identity: publisher=%q name=%q", ext.Publisher, ext.Name)
	}
	if ext.Version != "1.0.0" {
		t.Errorf("unexpected version: %q", ext.Version)
	}
}

// --- ListInstalledExtensions ---

// TestMarketplaceListInstalled verifies listing multiple installed extensions
// and that the result is sorted by publisher then name.
func TestMarketplaceListInstalled(t *testing.T) {
	svc, _ := newTestMarketplaceService(t)

	// Install two extensions with different publishers.
	vsix, hash := buildValidVSIX(t)
	// Tweak the package.json publisher/name per install by building distinct
	// VSIXes so the metadata files differ.
	pkgA := strings.ReplaceAll(strings.ReplaceAll(validPackageJSON, `"acme"`, `"alpha"`), `"hello"`, `"one"`)
	vsixA, hashA := buildVSIX(t, []zipEntry{
		{Name: "extension/package.json", Data: []byte(pkgA)},
		{Name: "extension/main.js", Data: []byte("export function activate() {}\n")},
	})
	pkgB := strings.ReplaceAll(strings.ReplaceAll(validPackageJSON, `"acme"`, `"beta"`), `"hello"`, `"two"`)
	vsixB, hashB := buildVSIX(t, []zipEntry{
		{Name: "extension/package.json", Data: []byte(pkgB)},
		{Name: "extension/main.js", Data: []byte("export function activate() {}\n")},
	})
	_ = vsix
	_ = hash
	if err := svc.installFromVSIXData(vsixA, hashA, "alpha", "one", "1.0.0"); err != nil {
		t.Fatalf("install alpha.one: %v", err)
	}
	if err := svc.installFromVSIXData(vsixB, hashB, "beta", "two", "2.0.0"); err != nil {
		t.Fatalf("install beta.two: %v", err)
	}

	installed, err := svc.ListInstalledExtensions()
	if err != nil {
		t.Fatalf("list installed: %v", err)
	}
	if len(installed) != 2 {
		t.Fatalf("expected 2 installed extensions, got %d", len(installed))
	}
	// Sorted: alpha.one before beta.two.
	if installed[0].Publisher != "alpha" || installed[0].Name != "one" {
		t.Errorf("expected alpha.one first, got %s.%s", installed[0].Publisher, installed[0].Name)
	}
	if installed[1].Publisher != "beta" || installed[1].Name != "two" {
		t.Errorf("expected beta.two second, got %s.%s", installed[1].Publisher, installed[1].Name)
	}
	// Both disabled by default.
	for _, e := range installed {
		if e.Enabled {
			t.Errorf("%s.%s should be disabled by default", e.Publisher, e.Name)
		}
	}
}

// TestMarketplaceListInstalled_Empty verifies listing when nothing is
// installed returns an empty (non-nil) slice.
func TestMarketplaceListInstalled_Empty(t *testing.T) {
	svc, _ := newTestMarketplaceService(t)
	installed, err := svc.ListInstalledExtensions()
	if err != nil {
		t.Fatalf("list installed: %v", err)
	}
	if installed == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(installed) != 0 {
		t.Fatalf("expected 0 installed extensions, got %d", len(installed))
	}
}

// --- SetExtensionEnabled ---

// TestMarketplaceSetExtensionEnabled verifies that toggling enabled state
// persists and is reflected by ListInstalledExtensions.
func TestMarketplaceSetExtensionEnabled(t *testing.T) {
	svc, _ := newTestMarketplaceService(t)
	vsix, wantHash := buildValidVSIX(t)
	if err := svc.installFromVSIXData(vsix, wantHash, "acme", "hello", "1.0.0"); err != nil {
		t.Fatalf("install: %v", err)
	}
	// Enable it.
	if err := svc.SetExtensionEnabled("acme", "hello", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	installed, err := svc.ListInstalledExtensions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(installed) != 1 || !installed[0].Enabled {
		t.Fatalf("expected enabled extension, got %+v", installed)
	}
	// Disable it again.
	if err := svc.SetExtensionEnabled("acme", "hello", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	installed, err = svc.ListInstalledExtensions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(installed) != 1 || installed[0].Enabled {
		t.Fatalf("expected disabled extension, got %+v", installed)
	}
}

// --- UninstallExtension ---

// TestMarketplaceUninstall verifies that uninstalling removes the extension
// directory and clears it from the listing.
func TestMarketplaceUninstall(t *testing.T) {
	svc, _ := newTestMarketplaceService(t)
	vsix, wantHash := buildValidVSIX(t)
	if err := svc.installFromVSIXData(vsix, wantHash, "acme", "hello", "1.0.0"); err != nil {
		t.Fatalf("install: %v", err)
	}
	dir := filepath.Join(svc.configDir, extensionsSubdir, "acme.hello")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("extension dir should exist before uninstall: %v", err)
	}
	if err := svc.UninstallExtension("acme", "hello"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("extension dir should be removed after uninstall; stat err=%v", err)
	}
	installed, err := svc.ListInstalledExtensions()
	if err != nil {
		t.Fatalf("list after uninstall: %v", err)
	}
	if len(installed) != 0 {
		t.Fatalf("expected 0 installed after uninstall, got %d", len(installed))
	}
}

// TestMarketplaceUninstall_NotInstalled verifies uninstalling an extension
// that was never installed does not error (idempotent removal).
func TestMarketplaceUninstall_NotInstalled(t *testing.T) {
	svc, _ := newTestMarketplaceService(t)
	if err := svc.UninstallExtension("acme", "ghost"); err != nil {
		t.Fatalf("uninstalling a non-installed extension should not error: %v", err)
	}
}

// --- Manifest parsing (Step 3) ---

// TestMarketplaceGetExtensionManifest verifies that the manifest is parsed
// from the extracted extension/package.json after install.
func TestMarketplaceGetExtensionManifest(t *testing.T) {
	svc, _ := newTestMarketplaceService(t)
	vsix, wantHash := buildValidVSIX(t)
	if err := svc.installFromVSIXData(vsix, wantHash, "acme", "hello", "1.0.0"); err != nil {
		t.Fatalf("install: %v", err)
	}
	m, err := svc.GetExtensionManifest("acme", "hello")
	if err != nil {
		t.Fatalf("get manifest: %v", err)
	}
	if m.Name != "hello" {
		t.Errorf("expected name hello, got %q", m.Name)
	}
	if m.Engines["vscode"] != "^1.80.0" {
		t.Errorf("expected engines.vscode ^1.80.0, got %q", m.Engines["vscode"])
	}
	if len(m.ActivationEvents) != 1 || m.ActivationEvents[0] != "onStartupFinished" {
		t.Errorf("unexpected activationEvents: %v", m.ActivationEvents)
	}
	if len(m.Contributes) == 0 {
		t.Errorf("expected non-empty contributes")
	}
	if len(m.Capabilities) == 0 {
		t.Errorf("expected non-empty capabilities")
	}
}

// --- Reinstall / update ---

// TestMarketplaceReinstall_Overwrites verifies that installing an extension
// that is already installed replaces the prior version cleanly.
func TestMarketplaceReinstall_Overwrites(t *testing.T) {
	svc, _ := newTestMarketplaceService(t)
	vsix, wantHash := buildValidVSIX(t)
	if err := svc.installFromVSIXData(vsix, wantHash, "acme", "hello", "1.0.0"); err != nil {
		t.Fatalf("install v1: %v", err)
	}
	// Enable it, then reinstall — the reinstall should reset to disabled.
	if err := svc.SetExtensionEnabled("acme", "hello", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := svc.installFromVSIXData(vsix, wantHash, "acme", "hello", "1.0.0"); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	installed, err := svc.ListInstalledExtensions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(installed) != 1 {
		t.Fatalf("expected 1 installed, got %d", len(installed))
	}
	if installed[0].Enabled {
		t.Errorf("reinstall should reset enabled to default-disabled; got Enabled=true")
	}
}

// --- ValidateExtensionIdent ---

// TestMarketplaceValidateIdent verifies the publisher/name guard rejects
// path-bearing identifiers that could escape the extensions directory.
func TestMarketplaceValidateIdent(t *testing.T) {
	cases := []struct {
		name      string
		publisher string
		ext       string
		wantErr   bool
	}{
		{"valid", "acme", "hello", false},
		{"empty publisher", "", "hello", true},
		{"empty name", "acme", "", true},
		{"publisher traversal", "..", "hello", true},
		{"name traversal", "acme", "..", true},
		{"publisher slash", "a/b", "hello", true},
		{"name backslash", "acme", "h\\i", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateExtensionIdent(c.publisher, c.ext)
			if c.wantErr && err == nil {
				t.Errorf("expected error for publisher=%q name=%q", c.publisher, c.ext)
			}
			if !c.wantErr && err != nil {
				t.Errorf("unexpected error for publisher=%q name=%q: %v", c.publisher, c.ext, err)
			}
		})
	}
}

// --- State persistence across instances ---

// TestMarketplaceStatePersistsAcrossInstances verifies that enabled state
// written by one service instance is read by a fresh instance pointed at the
// same config dir (the state lives on disk, not in memory).
func TestMarketplaceStatePersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	svc1 := NewMarketplaceService(dir)
	vsix, wantHash := buildValidVSIX(t)
	if err := svc1.installFromVSIXData(vsix, wantHash, "acme", "hello", "1.0.0"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := svc1.SetExtensionEnabled("acme", "hello", true); err != nil {
		t.Fatalf("enable: %v", err)
	}

	svc2 := NewMarketplaceService(dir)
	installed, err := svc2.ListInstalledExtensions()
	if err != nil {
		t.Fatalf("list via second instance: %v", err)
	}
	if len(installed) != 1 {
		t.Fatalf("expected 1 installed, got %d", len(installed))
	}
	if !installed[0].Enabled {
		t.Errorf("enabled state should have persisted across instances; got Enabled=false")
	}
}

// --- State file shape ---

// TestMarketplaceStateFileShape verifies the on-disk state file is valid JSON
// with the expected key after a default-disabled install.
func TestMarketplaceStateFileShape(t *testing.T) {
	svc, _ := newTestMarketplaceService(t)
	vsix, wantHash := buildValidVSIX(t)
	if err := svc.installFromVSIXData(vsix, wantHash, "acme", "hello", "1.0.0"); err != nil {
		t.Fatalf("install: %v", err)
	}
	statePath := filepath.Join(svc.configDir, "gugacode", extensionsStateFileName)
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	var state mpExtensionStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("parse state file: %v", err)
	}
	entry, ok := state.Extensions["acme.hello"]
	if !ok {
		t.Fatalf("expected state entry for acme.hello; got %+v", state.Extensions)
	}
	if entry.Enabled {
		t.Errorf("state entry should be Enabled=false (default disabled); got true")
	}
}

// --- CRIT-02 / G-SEC-12: marketplace ↔ security service integration ---

// TestMarketplaceInstall_CRIT02_BlacklistedRejectedWithSecurityService
// verifies that when a MarketplaceService has an ExtensionSecurityService
// wired in, installing a blacklisted extension is rejected at the blacklist
// gate (before any files are written) and the extension directory is never
// created.
func TestMarketplaceInstall_CRIT02_BlacklistedRejectedWithSecurityService(t *testing.T) {
	svc, dir := newTestMarketplaceService(t)
	ss := NewExtensionSecurityService(dir)
	svc.SetSecurityService(ss)

	vsix, wantHash := buildValidVSIX(t)
	// "anabarban.anabarban" is in the built-in default blacklist.
	err := svc.installFromVSIXData(vsix, wantHash, "anabarban", "anabarban", "1.0.0")
	if err == nil {
		t.Fatal("expected blacklisted install to be rejected, got nil error (CRIT-02)")
	}
	if !strings.Contains(err.Error(), "blacklisted") {
		t.Errorf("expected blacklisted error, got: %v", err)
	}
	// The extension directory must NOT exist (blacklist gate fires before
	// extraction).
	targetDir := svc.extensionDir("anabarban", "anabarban")
	if _, statErr := os.Stat(targetDir); !os.IsNotExist(statErr) {
		t.Errorf("blacklisted extension directory should not exist: path=%s statErr=%v", targetDir, statErr)
	}
}

// TestMarketplaceInstall_CRIT02_RegistersWithSecurityService verifies that a
// successful install of a legitimate extension triggers
// ExtensionSecurityService.RegisterInstall, producing a security state entry
// with Enabled=false and PendingReview=true (G-SEC-12 req. 2: default
// disabled + pending review).
func TestMarketplaceInstall_CRIT02_RegistersWithSecurityService(t *testing.T) {
	svc, dir := newTestMarketplaceService(t)
	ss := NewExtensionSecurityService(dir)
	svc.SetSecurityService(ss)

	vsix, wantHash := buildValidVSIX(t)
	if err := svc.installFromVSIXData(vsix, wantHash, "acme", "hello", "1.0.0"); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// The security service must have a registered entry for acme.hello.
	info, err := ss.GetSecurityInfo("acme.hello")
	if err != nil {
		t.Fatalf("GetSecurityInfo(acme.hello): %v (CRIT-02: RegisterInstall not called)", err)
	}
	if info.Enabled {
		t.Errorf("registered extension should be Enabled=false (CRIT-02 default disabled); got true")
	}
	if !info.PendingReview {
		t.Errorf("registered extension should be PendingReview=true (CRIT-02 pending review); got false")
	}
}
