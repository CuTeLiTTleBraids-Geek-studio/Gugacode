package services

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"strings"
	"sync"
)

// SearchMatch describes a single match within a file.
type SearchMatch struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Preview string `json:"preview"`
}

// SearchResult groups all matches in a single file.
type SearchResult struct {
	Path    string        `json:"path"`
	Matches []SearchMatch `json:"matches"`
}

// SearchService exposes file-content search to the frontend.
// N-67: when workspaceRoot is set via SetWorkspaceRoot, all search/replace
// path arguments are validated to be within the workspace. This prevents
// the frontend from searching or replacing in files outside the open project.
type SearchService struct {
	mu            sync.RWMutex
	workspaceRoot string
}

// SetWorkspaceRoot sets the directory within which search and replace
// operations are allowed. Pass an empty string to disable sandboxing.
func (s *SearchService) SetWorkspaceRoot(root string) error {
	if root == "" {
		s.mu.Lock()
		s.workspaceRoot = ""
		s.mu.Unlock()
		return nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace root is not a directory: %s", abs)
	}
	s.mu.Lock()
	s.workspaceRoot = abs
	s.mu.Unlock()
	return nil
}

// validatePath returns nil if path is within the workspace root (or if no
// root is set). Uses the shared ValidatePathWithinRoot from pathsec.go.
func (s *SearchService) validatePath(path string) error {
	s.mu.RLock()
	root := s.workspaceRoot
	s.mu.RUnlock()
	_, err := ValidatePathWithinRoot(root, path)
	return err
}

// ignoredDirs are directory basenames skipped during search.
var ignoredDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	".hg":          true,
	".svn":         true,
	"dist":         true,
	"build":        true,
	"out":          true,
	".next":        true,
	".nuxt":        true,
	"target":       true,
	"vendor":       true,
}

// isBinary returns true if the file content contains a null byte in the first 4KB.
func isBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	buf := make([]byte, 4096)
	n, _ := f.Read(buf)
	return bytes.IndexByte(buf[:n], 0) >= 0
}

// Search walks path recursively and returns files whose content matches the query.
// If ignoreCase is true, the match is case-insensitive. The query is treated as a
// regular expression.
func (s *SearchService) Search(root, query string, ignoreCase bool) ([]SearchResult, error) {
	if err := s.validatePath(root); err != nil {
		return nil, err
	}
	pattern := query
	var flags syntax.Flags
	if ignoreCase {
		flags = syntax.FoldCase
	}
	re, err := regexp.Compile("(?"+flagsToString(flags)+")" + pattern)
	if err != nil {
		// Try without the inline flags wrapper — older Go may not need it.
		re, err = regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		if ignoreCase {
			re = re.Copy()
			// Fallback: lowercase comparison
			re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(pattern))
		}
	}

	var results []SearchResult
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if ignoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "" || strings.HasPrefix(d.Name(), ".") && d.Name() != ".env" {
			// Skip dotfiles except .env
			return nil
		}
		if isBinary(p) {
			return nil
		}
		matches := searchFile(p, re)
		if len(matches) > 0 {
			relPath, _ := filepath.Rel(root, p)
			results = append(results, SearchResult{
				Path:    filepath.ToSlash(relPath),
				Matches: matches,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func flagsToString(f syntax.Flags) string {
	if f&syntax.FoldCase != 0 {
		return "i"
	}
	return ""
}

func searchFile(path string, re *regexp.Regexp) []SearchMatch {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var matches []SearchMatch
	scanner := bufio.NewScanner(f)
	// Allow longer lines (default 64KB is too small for minified files)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		loc := re.FindStringIndex(line)
		if loc != nil {
			matches = append(matches, SearchMatch{
				Line:    lineNum,
				Column:  loc[0] + 1,
				Preview: line,
			})
		}
	}
	return matches
}


// ReplaceResult reports the outcome of a replace operation.
type ReplaceResult struct {
	Replacements int `json:"replacements"`
}

// Replace replaces all occurrences of pattern in the file at filePath with
// replacement. If caseSensitive is false, the match is case-insensitive.
// The pattern is treated as a regular expression. The replacement string
// supports capture group references (e.g., $1).
func (s *SearchService) Replace(filePath string, pattern string, replacement string, caseSensitive bool) (*ReplaceResult, error) {
	if strings.TrimSpace(pattern) == "" {
		return nil, errors.New("pattern cannot be empty")
	}
	if err := s.validatePath(filePath); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	flags := ""
	if !caseSensitive {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex: %w", err)
	}

	count := 0
	newContent := re.ReplaceAllStringFunc(string(data), func(match string) string {
		count++
		return re.ReplaceAllString(match, replacement)
	})

	if count > 0 {
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			return nil, err
		}
	}

	return &ReplaceResult{Replacements: count}, nil
}