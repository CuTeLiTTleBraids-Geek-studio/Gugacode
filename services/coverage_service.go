package services

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CoverageHit is a simplified per-line coverage flag for gutter UI (prompt-9/10/11).
// File is always stored as a cleaned slash-normalized path (may still be package-relative).
type CoverageHit struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Covered bool   `json:"covered"`
}

// NormalizeCoveragePath cleans and slash-normalizes a coverprofile path so
// same-basename files under different directories do not collide (prompt-11 11-B).
func NormalizeCoveragePath(p string) string {
	if p == "" {
		return ""
	}
	// Windows drive paths: keep as-is after Clean; always use forward slashes.
	p = filepath.Clean(p)
	p = strings.ReplaceAll(p, "\\", "/")
	// strip redundant ./ prefix
	p = strings.TrimPrefix(p, "./")
	return p
}

// CoveragePathsMatch reports whether a cover hit path refers to the same file
// as editorPath. Never matches on basename alone when either side has directories
// (prompt-11 11-B — avoid cross-package gutter bleed).
func CoveragePathsMatch(hitPath, editorPath string) bool {
	h := NormalizeCoveragePath(hitPath)
	e := NormalizeCoveragePath(editorPath)
	if h == "" || e == "" {
		return false
	}
	if strings.EqualFold(h, e) {
		return true
	}
	hParts := strings.Split(h, "/")
	eParts := strings.Split(e, "/")
	// Basename-only paths only match other basename-only paths.
	if len(hParts) == 1 || len(eParts) == 1 {
		return len(hParts) == 1 && len(eParts) == 1 && strings.EqualFold(h, e)
	}
	// Prefer full relative suffix (pkg/a/foo.go vs /abs/pkg/a/foo.go).
	hl, el := strings.ToLower(h), strings.ToLower(e)
	if strings.HasSuffix(el, "/"+hl) || strings.HasSuffix(hl, "/"+el) {
		return true
	}
	// Require last two path segments (dir + file) to match.
	return strings.EqualFold(hParts[len(hParts)-1], eParts[len(eParts)-1]) &&
		strings.EqualFold(hParts[len(hParts)-2], eParts[len(eParts)-2])
}

// CoverageRunResult is returned after go test -coverprofile (prompt-10 10-H).
type CoverageRunResult struct {
	Success  bool          `json:"success"`
	Output   string        `json:"output"`
	Hits     []CoverageHit `json:"hits"`
	Profile  string        `json:"profile"`
	Duration int64         `json:"durationMs"`
}

// CoverageService parses go cover profiles and can run coverage for a package.
type CoverageService struct {
	workspaceRoot string
}

// NewCoverageService creates the coverage helper.
func NewCoverageService() *CoverageService {
	return &CoverageService{}
}

// SetWorkspaceRoot sets the default package directory for RunPackageCoverage.
func (c *CoverageService) SetWorkspaceRoot(root string) {
	c.workspaceRoot = root
}

// ParseCoverProfile reads a go cover profile and returns per-line hits.
// Format: file:startLine.startCol,endLine.endCol numStmt count
func (c *CoverageService) ParseCoverProfile(profilePath string) ([]CoverageHit, error) {
	f, err := os.Open(profilePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []CoverageHit
	sc := bufio.NewScanner(f)
	// skip mode line
	if sc.Scan() {
		// mode: set
	}
	for sc.Scan() {
		line := sc.Text()
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		count, _ := strconv.Atoi(parts[2])
		loc := parts[0]
		colon := strings.LastIndex(loc, ":")
		if colon < 0 {
			continue
		}
		file := loc[:colon]
		rangePart := loc[colon+1:]
		comma := strings.Index(rangePart, ",")
		if comma < 0 {
			continue
		}
		start := rangePart[:comma]
		end := rangePart[comma+1:]
		dot := strings.Index(start, ".")
		if dot < 0 {
			continue
		}
		startLine, _ := strconv.Atoi(start[:dot])
		endDot := strings.Index(end, ".")
		endLine := startLine
		if endDot >= 0 {
			endLine, _ = strconv.Atoi(end[:endDot])
		}
		covered := count > 0
		normFile := NormalizeCoveragePath(file)
		for ln := startLine; ln <= endLine; ln++ {
			out = append(out, CoverageHit{
				File:    normFile,
				Line:    ln,
				Covered: covered,
			})
		}
	}
	if err := sc.Err(); err != nil {
		return out, fmt.Errorf("scan cover profile: %w", err)
	}
	return out, nil
}

// RunPackageCoverage runs `go test -coverprofile=<tmp> .` in packageDir and parses hits.
func (c *CoverageService) RunPackageCoverage(packageDir string) (CoverageRunResult, error) {
	dir := packageDir
	if dir == "" {
		dir = c.workspaceRoot
	}
	if dir == "" {
		return CoverageRunResult{Success: false, Output: "no package directory"}, nil
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		return CoverageRunResult{Success: false, Output: "go not found"}, nil
	}
	tmp, err := os.CreateTemp("", "guga-cover-*.out")
	if err != nil {
		return CoverageRunResult{}, err
	}
	profile := tmp.Name()
	_ = tmp.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := commandContext(ctx, goBin, "test", "-count=1", "-coverprofile="+profile, ".")
	cmd.Dir = dir
	start := time.Now()
	out, runErr := cmd.CombinedOutput()
	hits, perr := c.ParseCoverProfile(profile)
	if perr != nil {
		hits = nil
	}
	return CoverageRunResult{
		Success:  runErr == nil,
		Output:   string(out),
		Hits:     hits,
		Profile:  profile,
		Duration: time.Since(start).Milliseconds(),
	}, nil
}
