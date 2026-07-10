package services

import (
	crypto_rand "crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/adrg/xdg"
)

//go:embed all:templates
var templateFS embed.FS

func generateProjectID() string {
	b := make([]byte, 8)
	_, _ = crypto_rand.Read(b)
	return hex.EncodeToString(b)
}

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
	configPath       string
	fileService      *FileService
	terminalService  *TerminalService
	agentService     *AgentService
	aiService        *AIService
	gitService       *GitService
	searchService    *SearchService
	toolchainService *ToolchainService
	lspService       *LSPService
}

// NewProjectService creates a ProjectService that stores data in the
// OS-specific config directory (via XDG). If fileService is non-nil, it
// sets the workspace root for path sandboxing when a project is added.
// If terminalService is non-nil, the terminal workspace root is also set.
// If agentService is non-nil, the agent workspace root (command sandbox)
// is also set (N-1). If aiService is non-nil, the AI project root is set
// for project-level preset lookups (N-17).
// If gitService is non-nil (N-67), the git workspace root is set.
// If searchService is non-nil (N-67), the search workspace root is set.
func NewProjectService(fs *FileService, ts *TerminalService, as *AgentService, ais *AIService) *ProjectService {
	return &ProjectService{
		configPath:      filepath.Join(xdg.ConfigHome, "gugacode", "projects.json"),
		fileService:     fs,
		terminalService: ts,
		agentService:    as,
		aiService:       ais,
	}
}

// SetGitService links the GitService so its workspace root is updated when
// a project is added (N-67). Called from main.go after construction.
func (p *ProjectService) SetGitService(g *GitService) {
	p.gitService = g
}

// SetSearchService links the SearchService so its workspace root is updated
// when a project is added (N-67). Called from main.go after construction.
func (p *ProjectService) SetSearchService(s *SearchService) {
	p.searchService = s
}

// SetLSPService links the LSPService so its workspace root is updated when
// a project is added (G-FEAT-02). Called from main.go after construction.
func (p *ProjectService) SetLSPService(l *LSPService) {
	p.lspService = l
}

// SetToolchainService links the ToolchainService so its workspace root is
// updated when a project is added (G-FEAT-03). Called from main.go after
// construction.
func (p *ProjectService) SetToolchainService(t *ToolchainService) {
	p.toolchainService = t
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
	// M-5: atomic write (temp+rename+0600) prevents half-written state.
	return atomicWriteJSON(p.configPath, projects, 0600)
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
// If a FileService is linked, the workspace root is set for path sandboxing.
// N-67: GitService and SearchService workspace roots are also set.
func (p *ProjectService) AddProject(path string) (Project, error) {
	if p.fileService != nil {
		if err := p.fileService.SetWorkspaceRoot(path); err != nil {
			return Project{}, err
		}
	}
	if p.terminalService != nil {
		if err := p.terminalService.SetWorkspaceRoot(path); err != nil {
			return Project{}, err
		}
	}
	if p.agentService != nil {
		if err := p.agentService.SetWorkspaceRoot(path); err != nil {
			return Project{}, err
		}
	}
	if p.gitService != nil {
		if err := p.gitService.SetWorkspaceRoot(path); err != nil {
			return Project{}, err
		}
	}
	if p.searchService != nil {
		if err := p.searchService.SetWorkspaceRoot(path); err != nil {
			return Project{}, err
		}
	}
	if p.aiService != nil {
		p.aiService.SetProjectRoot(path)
	}
	if p.lspService != nil {
		p.lspService.SetWorkspaceRoot(path)
	}
	if p.toolchainService != nil {
		if err := p.toolchainService.SetWorkspaceRoot(path); err != nil {
			return Project{}, err
		}
	}
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
		ID:         generateProjectID(),
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

// isValidProjectID checks that an ID is a valid hex string with optional hyphens.
// This prevents path traversal attacks through the ID (which is used in filenames).
func isValidProjectID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '-') {
			return false
		}
	}
	return true
}

// RemoveProject deletes a project from the recent list by ID.
func (p *ProjectService) RemoveProject(id string) error {
	if !isValidProjectID(id) {
		return fmt.Errorf("invalid project ID: %s", id)
	}
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
	return fmt.Errorf("project not found: %s", id)
}

func sortProjectsByRecency(projects []Project) {
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].LastOpened > projects[j].LastOpened
	})
}

// ============================================================================
// G-FEAT-01: New Project scaffolding wizard.
//
// The wizard generates Go/TypeScript/JavaScript/Monorepo/Fullstack projects
// from embedded templates (services/templates/*). Template variables are
// strictly validated before rendering so that user-supplied module/project
// names cannot inject content into go.mod / package.json or escape the target
// directory.
// ============================================================================

// ProjectTemplate describes a scaffolding template the wizard can generate.
type ProjectTemplate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Language    string `json:"language"`
}

// CreateProjectRequest is the payload for CreateProject.
type CreateProjectRequest struct {
	TemplateID  string `json:"templateId"`
	ProjectName string `json:"projectName"`
	TargetDir   string `json:"targetDir"`
	ModuleName  string `json:"moduleName"` // for Go: module path
}

// projectTemplateData is the data passed to text/template when rendering.
type projectTemplateData struct {
	ProjectName string
	ModuleName  string
	TemplateID  string
}

// moduleAndProjectNamePattern restricts template inputs to a safe character
// set. Go module paths allow letters, digits, and the punctuation ".", "-",
// "_", "/". Project/package names used in package.json are tighter (no "/").
// Crucially this rejects shell metacharacters (";", spaces, quotes, "|", "&",
// "<", ">", "$", backticks, backslashes, "*") so values like
// `"; rm -rf /"` cannot be rendered into go.mod / package.json.
var (
	moduleNamePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	projectNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// builtInTemplates is the static catalog returned by ListProjectTemplates.
// The IDs map to subdirectories under services/templates/.
var builtInTemplates = []ProjectTemplate{
	{
		ID:          "go",
		Name:        "Go Service",
		Description: "HTTP server with Makefile, golangci-lint config, Dockerfile, and CI.",
		Language:    "Go",
	},
	{
		ID:          "typescript",
		Name:        "TypeScript Project",
		Description: "Strict tsconfig, ESLint flat config, Vitest, and an entry point.",
		Language:    "TypeScript",
	},
	{
		ID:          "javascript",
		Name:        "JavaScript Project",
		Description: "ESM JavaScript project with ESLint and Vitest.",
		Language:    "JavaScript",
	},
	{
		ID:          "monorepo",
		Name:        "Monorepo",
		Description: "pnpm workspace with a web app and a shared package.",
		Language:    "TypeScript",
	},
	{
		ID:          "fullstack",
		Name:        "Fullstack",
		Description: "Go backend API and a Vue/Vite frontend in one repo.",
		Language:    "Go + TypeScript",
	},
}

// ListProjectTemplates returns the available project templates for the wizard.
func (s *ProjectService) ListProjectTemplates() []ProjectTemplate {
	out := make([]ProjectTemplate, len(builtInTemplates))
	copy(out, builtInTemplates)
	return out
}

// CreateProject generates a new project from the named template into
// TargetDir. It validates the template ID, sanitizes/validates the project
// and module names, ensures the target directory is safe, walks the embedded
// template tree, renders each .tmpl file with text/template, and writes the
// results to disk. The created project path is returned.
func (s *ProjectService) CreateProject(req CreateProjectRequest) (string, error) {
	if !isValidTemplateID(req.TemplateID) {
		return "", fmt.Errorf("invalid template ID: %q", req.TemplateID)
	}
	name, err := sanitizeProjectName(req.ProjectName)
	if err != nil {
		return "", err
	}
	moduleName, err := sanitizeModuleName(req.ModuleName, req.TemplateID)
	if err != nil {
		return "", err
	}
	targetDir, err := resolveTargetDir(req.TargetDir, name)
	if err != nil {
		return "", err
	}
	data := projectTemplateData{
		ProjectName: name,
		ModuleName:  moduleName,
		TemplateID:  req.TemplateID,
	}
	if err := renderTemplateTree(req.TemplateID, targetDir, data); err != nil {
		return "", err
	}
	return targetDir, nil
}

// isValidTemplateID checks that the ID is one of the known template IDs.
// This prevents path traversal through the ID (which is joined into the
// embed.FS path).
func isValidTemplateID(id string) bool {
	for _, t := range builtInTemplates {
		if t.ID == id {
			return true
		}
	}
	return false
}

// sanitizeProjectName validates that name contains only safe characters for
// use as a package name / directory name. Empty names are rejected.
func sanitizeProjectName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("project name is required")
	}
	if len(name) > 214 {
		return "", fmt.Errorf("project name too long (max 214 chars)")
	}
	if !projectNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid project name %q: only letters, digits, '.', '-', '_' allowed (must start alphanumeric)", name)
	}
	return name, nil
}

// sanitizeModuleName validates the Go module path. For non-Go templates the
// module name is unused and may be empty; otherwise it must match the module
// path pattern. The moduleNamePattern rejects shell metacharacters so values
// like `"; rm -rf /"` cannot be injected into go.mod.
func sanitizeModuleName(moduleName, templateID string) (string, error) {
	if templateID != "go" && templateID != "fullstack" {
		return "", nil
	}
	moduleName = strings.TrimSpace(moduleName)
	if moduleName == "" {
		return "", fmt.Errorf("module name is required for Go templates")
	}
	if len(moduleName) > 512 {
		return "", fmt.Errorf("module name too long")
	}
	if !moduleNamePattern.MatchString(moduleName) {
		return "", fmt.Errorf("invalid module name %q: only letters, digits, '.', '-', '_', '/' allowed (must start alphanumeric)", moduleName)
	}
	return moduleName, nil
}

// resolveTargetDir resolves and validates the target directory. If targetDir
// is empty, the project is created under the OS temp dir. The resolved path
// must not already exist (to avoid clobbering), and is checked for traversal
// safety via IsRelativePathSafe on the project name component.
func resolveTargetDir(targetDir, projectName string) (string, error) {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		targetDir = os.TempDir()
	}
	abs, err := filepath.Abs(targetDir)
	if err != nil {
		return "", fmt.Errorf("resolve target directory: %w", err)
	}
	// The final project directory is <targetDir>/<projectName>. Validate the
	// name component lexically so a malicious name cannot escape via "..".
	if !IsRelativePathSafe(projectName) {
		return "", fmt.Errorf("unsafe project name for directory: %q", projectName)
	}
	finalDir := filepath.Join(abs, projectName)
	// Refuse to overwrite an existing non-empty directory.
	if info, err := os.Stat(finalDir); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(finalDir)
		if len(entries) > 0 {
			return "", fmt.Errorf("target directory already exists and is not empty: %s", finalDir)
		}
	}
	return finalDir, nil
}

// renderTemplateTree walks the embedded templates/<templateID>/ subtree and
// renders every file (stripping the .tmpl suffix) into targetDir.
func renderTemplateTree(templateID, targetDir string, data projectTemplateData) error {
	root := "templates/" + templateID
	// Verify the template directory exists in the embed.FS.
	if _, err := templateFS.ReadDir(root); err != nil {
		return fmt.Errorf("template %q not found in embed.FS: %w", templateID, err)
	}
	return fs.WalkDir(templateFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("compute relative path for %s: %w", path, err)
		}
		// Convert embed path separators (always '/') to OS separators.
		relOS := filepath.FromSlash(rel)
		// Strip the .tmpl suffix to produce the output filename.
		outName := strings.TrimSuffix(relOS, ".tmpl")
		if outName == "" {
			return fmt.Errorf("template file %s has empty output name", path)
		}
		outPath := filepath.Join(targetDir, outName)
		// Ensure the parent directory exists.
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return fmt.Errorf("create directory for %s: %w", outPath, err)
		}
		raw, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded template %s: %w", path, err)
		}
		// Parse and execute the template. text/template does not auto-escape,
		// but the inputs are pre-validated to a safe charset, so injection
		// into go.mod / package.json is prevented at the validation layer.
		tmpl, err := template.New(path).Parse(string(raw))
		if err != nil {
			return fmt.Errorf("parse template %s: %w", path, err)
		}
		var buf strings.Builder
		if err := tmpl.Execute(&buf, data); err != nil {
			return fmt.Errorf("execute template %s: %w", path, err)
		}
		if err := os.WriteFile(outPath, []byte(buf.String()), 0644); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
		return nil
	})
}
