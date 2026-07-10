package services

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// marketplace_service.go — G-VSC-01: VS Code extension marketplace client.
//
// This service searches, browses, downloads, verifies, and installs VS Code
// extensions (VSIX packages) from a registry. The default registry is the
// Open VSX Registry (https://open-vsx.org/vscode/registry/api), which has no
// legal restrictions on programmatic access. The VS Code Marketplace is
// optional and requires explicit user opt-in (set via SetRegistryURL).
//
// Security (G-SEC-12):
//   - requirement 2: newly installed extensions are disabled by default.
//   - requirement 3: every downloaded VSIX is verified against a registry-
//     provided SHA-256 hash; a mismatch aborts the install.
//   - All extracted paths are validated to stay within the target directory
//     (path traversal protection) — see extractVSIX.

// defaultOpenVSXRegistryAPI is the default registry base URL (Open VSX).
// Open VSX has no legal restrictions on API use, unlike the VS Code
// Marketplace whose ToS require the official client.
const defaultOpenVSXRegistryAPI = "https://open-vsx.org/vscode/registry/api"

// extensionsSubdir is the on-disk directory for installed VSIX extensions,
// relative to the config dir: <configDir>/gugacode/extensions/<publisher>.<name>/
const extensionsSubdir = "gugacode/extensions"

// extensionsStateFileName is the persisted enabled/disabled state file for
// VS Code extensions, written under <configDir>/gugacode/.
const extensionsStateFileName = "extensions-state.json"

// installedExtMetaFile is the small metadata file written into each
// installed extension directory recording the installed version. It lets
// ListInstalledExtensions report the version without re-parsing package.json
// (which may not carry the exact installed version after updates).
const installedExtMetaFile = "gugacode-ext.json"

// vsixExtensionPrefix is the path prefix inside a VSIX zip for the
// extension payload (VSIX packages place package.json under extension/).
const vsixExtensionPrefix = "extension/"

// MarketplaceService searches, downloads, verifies, and installs VS Code
// extensions (VSIX) from a registry. The default registry is Open VSX.
type MarketplaceService struct {
	mu          sync.Mutex
	configDir   string
	registryURL string
	httpClient  *http.Client
	// G-SEC-12: securityService is used to register installs and check
	// the blacklist before installation. Without this, marketplace installs
	// bypass security classification.
	securityService *ExtensionSecurityService
}

// NewMarketplaceService constructs a MarketplaceService rooted at configDir.
// Installed extensions live under <configDir>/gugacode/extensions/. The
// registry defaults to Open VSX; call SetRegistryURL to switch (e.g. to the
// VS Code Marketplace after user opt-in).
func NewMarketplaceService(configDir string) *MarketplaceService {
	return &MarketplaceService{
		configDir:   configDir,
		registryURL: defaultOpenVSXRegistryAPI,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// SetSecurityService injects the ExtensionSecurityService so marketplace
// installs can be registered for security classification and blacklist
// checking (G-SEC-12).
func (s *MarketplaceService) SetSecurityService(ss *ExtensionSecurityService) {
	s.mu.Lock()
	s.securityService = ss
	s.mu.Unlock()
}

// SetRegistryURL overrides the registry base URL. Used to opt in to the VS
// Code Marketplace API (the user must consent, as its ToS restrict programmatic
// access to the official client). Pass an empty string to reset to Open VSX.
func (s *MarketplaceService) SetRegistryURL(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if url == "" {
		s.registryURL = defaultOpenVSXRegistryAPI
	} else {
		s.registryURL = strings.TrimRight(url, "/")
	}
}

// ExtensionSearchResult is a single hit from a registry search.
type ExtensionSearchResult struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	DisplayName   string  `json:"displayName"`
	Publisher     string  `json:"publisher"`
	Description   string  `json:"description"`
	Version       string  `json:"version"`
	Rating        float64 `json:"rating"`
	RatingCount   int     `json:"ratingCount"`
	DownloadCount int     `json:"downloadCount"`
	IconURL       string  `json:"iconUrl"`
}

// ExtensionVersion is a single published version of an extension.
type ExtensionVersion struct {
	Version     string `json:"version"`
	DownloadURL string `json:"downloadUrl"`
	Date        string `json:"date"`
}

// ExtensionDetail is the full metadata for a single extension.
type ExtensionDetail struct {
	ExtensionSearchResult
	Categories []string           `json:"categories"`
	Tags       []string           `json:"tags"`
	License    string             `json:"license"`
	Repository string             `json:"repository"`
	Readme     string             `json:"readme"`
	Versions   []ExtensionVersion `json:"versions"`
}

// InstalledExtension is a locally installed VS Code extension.
type InstalledExtension struct {
	Publisher string `json:"publisher"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Path      string `json:"path"`
	Enabled   bool   `json:"enabled"`
}

// VSCodeExtensionManifest is the subset of extension/package.json parsed
// after extraction (G-VSC-01 Step 3). Unknown fields are ignored.
type VSCodeExtensionManifest struct {
	Name             string          `json:"name"`
	Publisher        string          `json:"publisher"`
	Version          string          `json:"version"`
	DisplayName      string          `json:"displayName"`
	Description      string          `json:"description"`
	Engines          map[string]string `json:"engines"`
	ActivationEvents []string        `json:"activationEvents"`
	Contributes      json.RawMessage `json:"contributes"`
	Capabilities     json.RawMessage `json:"capabilities"`
}

// mpExtensionStateEntry is one row in the marketplace's persisted
// enabled/disabled state file (extensions-state.json). This is intentionally
// separate from ExtensionSecurityService's extensionStateEntry, which tracks
// the richer G-VSC-03 security classification in extension-security.json.
type mpExtensionStateEntry struct {
	Enabled bool `json:"enabled"`
}

// mpExtensionStateFile is the on-disk shape of extensions-state.json.
type mpExtensionStateFile struct {
	Extensions map[string]mpExtensionStateEntry `json:"extensions"`
}

// installedExtMeta is the on-disk shape of gugacode-ext.json.
type installedExtMeta struct {
	Publisher string `json:"publisher"`
	Name      string `json:"name"`
	Version   string `json:"version"`
}

// --- Open VSX API response shapes (subset) ---

// ovsxFileMap maps file roles (download/readme/icon/license) to URLs within
// an Open VSX version entry. The sha256 key carries the download hash.
type ovsxFileMap map[string]string

// ovsxVersion is a single version entry in the Open VSX response.
type ovsxVersion struct {
	Version   string     `json:"version"`
	Timestamp string     `json:"timestamp"`
	Files     ovsxFileMap `json:"files"`
}

// ovsxExtension is the Open VSX extension object (search hit or detail).
type ovsxExtension struct {
	Name          string        `json:"name"`
	Namespace     string        `json:"namespace"`
	DisplayName   string        `json:"displayName"`
	Description   string        `json:"description"`
	Version       string        `json:"version"`
	License       string        `json:"license"`
	Repository    string        `json:"repository"`
	Categories    []string      `json:"categories"`
	Tags          []string      `json:"tags"`
	DownloadCount int           `json:"downloadCount"`
	AverageRating float64       `json:"averageRating"`
	ReviewCount   int           `json:"reviewCount"`
	Files         ovsxFileMap   `json:"files"`
	Versions      []ovsxVersion `json:"versions"`
}

// ovsxSearchResponse is the search endpoint envelope.
type ovsxSearchResponse struct {
	Extensions []ovsxExtension `json:"extensions"`
}

// SearchExtensions searches the registry for extensions matching query.
// page is 1-based; pageSize caps the result count. Returns an empty slice
// (not nil) when no results are found.
func (s *MarketplaceService) SearchExtensions(query string, page int, pageSize int) ([]ExtensionSearchResult, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize
	reqURL := fmt.Sprintf("%s/-/search?query=%s&size=%d&offset=%d",
		s.registryURL, urlEscape(query), pageSize, offset)
	data, err := s.httpGetJSON(reqURL)
	if err != nil {
		return nil, fmt.Errorf("search extensions: %w", err)
	}
	var resp ovsxSearchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse search response: %w", err)
	}
	out := make([]ExtensionSearchResult, 0, len(resp.Extensions))
	for _, e := range resp.Extensions {
		out = append(out, ovsxToSearchResult(e))
	}
	return out, nil
}

// GetExtensionDetail gets detailed info about a specific extension by
// publisher (namespace) and name.
func (s *MarketplaceService) GetExtensionDetail(publisher, name string) (*ExtensionDetail, error) {
	if err := validateExtensionIdent(publisher, name); err != nil {
		return nil, err
	}
	reqURL := fmt.Sprintf("%s/%s/%s", s.registryURL, urlEscape(publisher), urlEscape(name))
	data, err := s.httpGetJSON(reqURL)
	if err != nil {
		return nil, fmt.Errorf("get extension detail: %w", err)
	}
	var e ovsxExtension
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parse extension detail: %w", err)
	}
	detail := &ExtensionDetail{
		ExtensionSearchResult: ovsxToSearchResult(e),
		Categories:            append([]string(nil), e.Categories...),
		Tags:                  append([]string(nil), e.Tags...),
		License:               e.License,
		Repository:            e.Repository,
		Readme:                fileURL(e.Files, "readme"),
	}
	for _, v := range e.Versions {
		detail.Versions = append(detail.Versions, ExtensionVersion{
			Version:     v.Version,
			DownloadURL: fileURL(v.Files, "download"),
			Date:        v.Timestamp,
		})
	}
	return detail, nil
}

// DownloadAndInstallExtension downloads a VSIX for the given extension
// version, verifies its SHA-256 against the registry-provided hash, and
// installs it under <configDir>/gugacode/extensions/<publisher>.<name>/.
// Newly installed extensions are disabled by default (G-SEC-12 req. 2).
// A hash mismatch aborts the install (G-SEC-12 req. 3).
func (s *MarketplaceService) DownloadAndInstallExtension(publisher, name, version string) error {
	if err := validateExtensionIdent(publisher, name); err != nil {
		return err
	}
	if s.configDir == "" {
		return fmt.Errorf("config directory is not configured")
	}
	reqURL := fmt.Sprintf("%s/%s/%s", s.registryURL, urlEscape(publisher), urlEscape(name))
	data, err := s.httpGetJSON(reqURL)
	if err != nil {
		return fmt.Errorf("fetch extension metadata: %w", err)
	}
	var e ovsxExtension
	if err := json.Unmarshal(data, &e); err != nil {
		return fmt.Errorf("parse extension metadata: %w", err)
	}
	// Resolve the target version entry. If version is empty, use the latest
	// (first) version returned by the registry.
	ver, err := pickVersion(e.Versions, version)
	if err != nil {
		return err
	}
	downloadURL := fileURL(ver.Files, "download")
	if downloadURL == "" {
		return fmt.Errorf("extension %s.%s version %s has no download URL", publisher, name, ver.Version)
	}
	wantHash := fileURL(ver.Files, "sha256")
	if wantHash == "" {
		// G-SEC-12 req. 3: refuse to install when the registry did not
		// provide a hash to verify against. Installing without verification
		// would defeat the integrity gate.
		return fmt.Errorf("extension %s.%s version %s has no SHA-256 hash from the registry; refusing to install unverified", publisher, name, ver.Version)
	}
	vsixData, err := s.httpGetBytes(downloadURL)
	if err != nil {
		return fmt.Errorf("download VSIX: %w", err)
	}
	return s.installFromVSIXData(vsixData, wantHash, publisher, name, ver.Version)
}

// installFromVSIXData is the testable core of the install path: it verifies
// the SHA-256 of the VSIX bytes, extracts them with path-traversal protection,
// parses the extension manifest, and records the install as disabled-by-default.
// Separating this from DownloadAndInstallExtension lets tests exercise the
// security gates (hash verification, traversal rejection) without network.
func (s *MarketplaceService) installFromVSIXData(vsix []byte, wantHash, publisher, name, version string) error {
	if s.configDir == "" {
		return fmt.Errorf("config directory is not configured")
	}
	if err := validateExtensionIdent(publisher, name); err != nil {
		return err
	}
	// G-SEC-12 req. 3: SHA-256 verification. Reject on mismatch.
	gotHash := sha256Hex(vsix)
	if !strings.EqualFold(gotHash, wantHash) {
		return fmt.Errorf("SHA-256 verification failed for %s.%s: expected %s, got %s (G-SEC-12 req. 3)", publisher, name, wantHash, gotHash)
	}
	// G-SEC-12: check the blacklist before installing. Reject known-malicious
	// extensions early so they never land on disk.
	if s.securityService != nil {
		if s.securityService.IsBlacklisted(publisher, name) {
			return fmt.Errorf("extension %s.%s is blacklisted (G-SEC-12)", publisher, name)
		}
	}
	targetDir := s.extensionDir(publisher, name)
	// Extract into a sibling temp dir first, then swap into place. This keeps
	// a failed/aborted extraction from leaving a half-installed extension.
	tmpDir := targetDir + ".installing"
	if err := os.RemoveAll(tmpDir); err != nil {
		return fmt.Errorf("clean stale install temp dir: %w", err)
	}
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return fmt.Errorf("create install temp dir: %w", err)
	}
	if err := extractVSIX(bytes.NewReader(vsix), int64(len(vsix)), tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return err
	}
	// Parse the manifest from the extracted payload for metadata (Step 3).
	if _, err := parseVSIXManifest(tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("parse extension manifest: %w", err)
	}
	// Record installed version metadata.
	meta := installedExtMeta{Publisher: publisher, Name: name, Version: version}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, installedExtMetaFile), metaBytes, 0o644); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("write extension metadata: %w", err)
	}
	// Swap into place. Remove any prior install first.
	_ = os.RemoveAll(targetDir)
	if err := os.Rename(tmpDir, targetDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("finalize install: %w", err)
	}
	// G-SEC-12: register the install with the security service so the
	// extension is classified, blacklist-checked, and recorded. Without
	// this, the marketplace install path bypasses security review.
	if s.securityService != nil {
		extensionID := publisher + "." + name
		tmpVsix := targetDir + ".vsix"
		if err := os.WriteFile(tmpVsix, vsix, 0o644); err != nil {
			return fmt.Errorf("write vsix for security registration: %w", err)
		}
		_, regErr := s.securityService.RegisterInstall(extensionID, nil, tmpVsix, wantHash)
		_ = os.Remove(tmpVsix)
		if regErr != nil {
			_ = os.RemoveAll(targetDir)
			return fmt.Errorf("security registration failed: %w", regErr)
		}
	}
	// G-SEC-12 req. 2 / G-VSC-03 req. 2: default disabled. Newly installed
	// extensions must not auto-activate until the user explicitly enables them.
	return s.setExtensionEnabled(publisher, name, false)
}

// ListInstalledExtensions returns all installed VS Code extensions with their
// enabled state. The result is sorted by publisher then name for deterministic
// display. Missing state entries default to disabled (safe default).
func (s *MarketplaceService) ListInstalledExtensions() ([]InstalledExtension, error) {
	if s.configDir == "" {
		return nil, fmt.Errorf("config directory is not configured")
	}
	dir := filepath.Join(s.configDir, extensionsSubdir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []InstalledExtension{}, nil
		}
		return nil, fmt.Errorf("list installed extensions: %w", err)
	}
	state := s.loadExtensionState()
	out := make([]InstalledExtension, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		extDir := filepath.Join(dir, entry.Name())
		meta, err := readInstalledMeta(extDir)
		if err != nil {
			// A directory without metadata isn't a tracked install — skip it.
			continue
		}
		key := extensionStateKey(meta.Publisher, meta.Name)
		enabled := false
		if st, ok := state.Extensions[key]; ok {
			enabled = st.Enabled
		}
		out = append(out, InstalledExtension{
			Publisher: meta.Publisher,
			Name:      meta.Name,
			Version:   meta.Version,
			Path:      extDir,
			Enabled:   enabled,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Publisher != out[j].Publisher {
			return out[i].Publisher < out[j].Publisher
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// UninstallExtension removes an installed extension from disk and clears its
// enabled/disabled state. Returns nil if the extension was not installed.
func (s *MarketplaceService) UninstallExtension(publisher, name string) error {
	if err := validateExtensionIdent(publisher, name); err != nil {
		return err
	}
	if s.configDir == "" {
		return fmt.Errorf("config directory is not configured")
	}
	targetDir := s.extensionDir(publisher, name)
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("uninstall extension %s.%s: %w", publisher, name, err)
	}
	// Remove any leftover install temp dir from a failed install.
	_ = os.RemoveAll(targetDir + ".installing")
	return s.setExtensionEnabled(publisher, name, false, true)
}

// SetExtensionEnabled persists the enabled/disabled state for an installed
// extension. Exposed for the frontend to toggle extensions on/off.
func (s *MarketplaceService) SetExtensionEnabled(publisher, name string, enabled bool) error {
	if err := validateExtensionIdent(publisher, name); err != nil {
		return err
	}
	return s.setExtensionEnabled(publisher, name, enabled)
}

// GetExtensionManifest reads and parses the extension/package.json from an
// installed extension directory, returning the manifest subset (Step 3).
func (s *MarketplaceService) GetExtensionManifest(publisher, name string) (*VSCodeExtensionManifest, error) {
	if err := validateExtensionIdent(publisher, name); err != nil {
		return nil, err
	}
	if s.configDir == "" {
		return nil, fmt.Errorf("config directory is not configured")
	}
	return parseVSIXManifest(s.extensionDir(publisher, name))
}

// --- internal helpers ---

// extensionDir returns the absolute install path for an extension.
func (s *MarketplaceService) extensionDir(publisher, name string) string {
	return filepath.Join(s.configDir, extensionsSubdir, extensionStateKey(publisher, name))
}

// setExtensionEnabled updates the persisted state. When remove is true the
// entry is deleted instead of written.
func (s *MarketplaceService) setExtensionEnabled(publisher, name string, enabled bool, remove ...bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.configDir == "" {
		return fmt.Errorf("config directory is not configured")
	}
	state := s.loadExtensionStateLocked()
	if state.Extensions == nil {
		state.Extensions = make(map[string]mpExtensionStateEntry)
	}
	key := extensionStateKey(publisher, name)
	if len(remove) > 0 && remove[0] {
		delete(state.Extensions, key)
	} else {
		state.Extensions[key] = mpExtensionStateEntry{Enabled: enabled}
	}
	return s.saveExtensionStateLocked(state)
}

// loadExtensionState reads the persisted enabled/disabled state (best-effort).
func (s *MarketplaceService) loadExtensionState() mpExtensionStateFile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadExtensionStateLocked()
}

func (s *MarketplaceService) loadExtensionStateLocked() mpExtensionStateFile {
	if s.configDir == "" {
		return mpExtensionStateFile{Extensions: map[string]mpExtensionStateEntry{}}
	}
	path := filepath.Join(s.configDir, "gugacode", extensionsStateFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return mpExtensionStateFile{Extensions: map[string]mpExtensionStateEntry{}}
	}
	var state mpExtensionStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return mpExtensionStateFile{Extensions: map[string]mpExtensionStateEntry{}}
	}
	if state.Extensions == nil {
		state.Extensions = map[string]mpExtensionStateEntry{}
	}
	return state
}

func (s *MarketplaceService) saveExtensionStateLocked(state mpExtensionStateFile) error {
	path := filepath.Join(s.configDir, "gugacode", extensionsStateFileName)
	// M-5: atomic write (temp+rename+0600) prevents half-written state.
	return atomicWriteJSON(path, state, 0600)
}

// extensionStateKey is the map key for an extension's persisted state, also
// used as the on-disk directory name: "<publisher>.<name>".
func extensionStateKey(publisher, name string) string {
	return publisher + "." + name
}

// httpGetJSON fetches a JSON document from the registry.
func (s *MarketplaceService) httpGetJSON(url string) ([]byte, error) {
	return s.httpGet(url, "application/json")
}

// httpGetBytes fetches raw bytes (e.g. a VSIX) from a URL.
func (s *MarketplaceService) httpGetBytes(url string) ([]byte, error) {
	return s.httpGet(url, "")
}

func (s *MarketplaceService) httpGet(url, accept string) ([]byte, error) {
	s.mu.Lock()
	client := s.httpClient
	s.mu.Unlock()
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	req.Header.Set("User-Agent", "gugacode-marketplace/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("registry returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

// extractVSIX extracts a VSIX (zip) into targetDir, rejecting any entry whose
// path escapes targetDir (path traversal protection). VSIX entries use forward
// slashes; we normalize and validate each one with filepath.Secure-style
// checks plus a resolved-prefix check, mirroring the plugin service's defense.
func extractVSIX(r io.ReaderAt, size int64, targetDir string) error {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return fmt.Errorf("open VSIX zip: %w", err)
	}
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("resolve target dir: %w", err)
	}
	for _, f := range zr.File {
		if err := extractZipEntry(f, absTarget); err != nil {
			return err
		}
	}
	return nil
}

// extractZipEntry writes a single zip entry into absTarget, after validating
// the resolved path stays within absTarget. Directory entries are created;
// file entries are written with their contents. Symlinks (via the zip's Unix
// symlink bit) are rejected — an extension should not install symlinks.
func extractZipEntry(f *zip.File, absTarget string) error {
	name := f.Name
	// Normalize separators to the OS form and clean. Reject absolute paths
	// and parent traversal before joining.
	name = strings.ReplaceAll(name, "\\", "/")
	cleaned := filepath.Clean(name)
	if strings.HasPrefix(cleaned, "..") || cleaned == ".." {
		return fmt.Errorf("VSIX entry %q escapes the install directory (path traversal)", f.Name)
	}
	// Reject absolute entry paths (Unix "/" or Windows drive/UNC). filepath.IsAbs
	// catches drive/UNC on Windows; the leading-slash check covers Unix-style
	// and a backslash-rooted entry.
	if strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "\\") || filepath.IsAbs(cleaned) {
		return fmt.Errorf("VSIX entry %q is an absolute path (path traversal)", f.Name)
	}
	// Reject Windows volume-relative form ("C:foo").
	if len(cleaned) >= 2 && cleaned[1] == ':' && (len(cleaned) == 2 || (cleaned[2] != '/' && cleaned[2] != '\\')) {
		return fmt.Errorf("VSIX entry %q uses a Windows volume-relative path (path traversal)", f.Name)
	}
	dest := filepath.Join(absTarget, cleaned)
	// Resolve and verify the destination is within absTarget. This catches
	// symlink-based escapes that the lexical check would miss.
	destResolved, err := evalSymlinksAllowMissing(dest)
	if err != nil {
		return fmt.Errorf("resolve VSIX entry path %q: %w", f.Name, err)
	}
	if destResolved != absTarget && !strings.HasPrefix(destResolved, absTarget+string(filepath.Separator)) {
		return fmt.Errorf("VSIX entry %q resolves outside the install directory (path traversal)", f.Name)
	}
	// Reject symlink entries (Unix mode bit). A VSIX should ship real files.
	if f.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("VSIX entry %q is a symlink; symlinks are not allowed", f.Name)
	}
	if f.FileInfo().IsDir() {
		return os.MkdirAll(dest, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create dir for %q: %w", f.Name, err)
	}
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open VSIX entry %q: %w", f.Name, err)
	}
	defer rc.Close()
	w, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create file %q: %w", f.Name, err)
	}
	defer w.Close()
	if _, err := io.Copy(w, rc); err != nil {
		return fmt.Errorf("write VSIX entry %q: %w", f.Name, err)
	}
	return nil
}

// parseVSIXManifest reads extension/package.json from an extracted VSIX
// directory and returns the manifest subset (Step 3). Returns an error if the
// manifest is missing or malformed.
func parseVSIXManifest(extDir string) (*VSCodeExtensionManifest, error) {
	manifestPath := filepath.Join(extDir, vsixExtensionPrefix, "package.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read extension manifest: %w", err)
	}
	var m VSCodeExtensionManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse extension manifest: %w", err)
	}
	return &m, nil
}

// readInstalledMeta reads the gugacode-ext.json metadata from an installed
// extension directory.
func readInstalledMeta(extDir string) (*installedExtMeta, error) {
	data, err := os.ReadFile(filepath.Join(extDir, installedExtMetaFile))
	if err != nil {
		return nil, err
	}
	var meta installedExtMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// pickVersion resolves the version entry to install. Empty version selects
// the latest (first) version returned by the registry.
func pickVersion(versions []ovsxVersion, version string) (ovsxVersion, error) {
	if len(versions) == 0 {
		return ovsxVersion{}, fmt.Errorf("extension has no published versions")
	}
	if version == "" {
		return versions[0], nil
	}
	for _, v := range versions {
		if v.Version == version {
			return v, nil
		}
	}
	return ovsxVersion{}, fmt.Errorf("version %q not found for extension", version)
}

// ovsxToSearchResult maps an Open VSX extension object to the public struct.
func ovsxToSearchResult(e ovsxExtension) ExtensionSearchResult {
	publisher := e.Namespace
	if publisher == "" {
		publisher = e.Name
	}
	icon := fileURL(e.Files, "icon")
	return ExtensionSearchResult{
		ID:            extensionStateKey(publisher, e.Name),
		Name:          e.Name,
		DisplayName:   e.DisplayName,
		Publisher:     publisher,
		Description:   e.Description,
		Version:       e.Version,
		Rating:        e.AverageRating,
		RatingCount:   e.ReviewCount,
		DownloadCount: e.DownloadCount,
		IconURL:       icon,
	}
}

// fileURL returns the URL for a file role from an Open VSX file map.
func fileURL(files ovsxFileMap, role string) string {
	if files == nil {
		return ""
	}
	return files[role]
}

// sha256Hex returns the hex-encoded SHA-256 of data.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// validateExtensionIdent rejects empty or path-bearing publisher/name values
// before they are joined into a filesystem path.
func validateExtensionIdent(publisher, name string) error {
	if publisher == "" {
		return fmt.Errorf("publisher is required")
	}
	if name == "" {
		return fmt.Errorf("extension name is required")
	}
	if strings.ContainsAny(publisher, `/\`) || strings.Contains(publisher, "..") {
		return fmt.Errorf("invalid publisher %q", publisher)
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid extension name %q", name)
	}
	return nil
}

// urlEscape percent-encodes a path segment for use in a registry URL.
// Uses net/url's PathEscape which encodes spaces as %20 and other reserved
// characters as required for URL path segments.
func urlEscape(s string) string {
	return url.PathEscape(s)
}
