package services

import (
	crypto_rand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/adrg/xdg"
)

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
	configPath      string
	fileService     *FileService
	terminalService *TerminalService
	agentService    *AgentService
	aiService       *AIService
	gitService      *GitService
	searchService   *SearchService
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
