package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
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

func TestProjectService_AddProjectSetsWorkspaceRoot(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "projects.json")
	workspace := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "blocked.txt")

	fs := &FileService{}
	svc := &ProjectService{configPath: configPath, fileService: fs}

	// Adding a project should set the workspace root
	_, err := svc.AddProject(workspace)
	if err != nil {
		t.Fatalf("AddProject failed: %v", err)
	}

	// Writing outside the workspace should now be blocked
	if err := fs.WriteFile(outsideFile, "data"); err == nil {
		t.Error("WriteFile outside workspace should be blocked after AddProject")
	}

	// Writing inside the workspace should work
	insideFile := filepath.Join(workspace, "allowed.txt")
	if err := fs.WriteFile(insideFile, "data"); err != nil {
		t.Errorf("WriteFile inside workspace should succeed: %v", err)
	}
}

func TestProjectService_AddProjectNonExistentPathWithFileService(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "projects.json")
	fs := &FileService{}
	svc := &ProjectService{configPath: configPath, fileService: fs}

	// Adding a non-existent project path should fail because SetWorkspaceRoot validates
	_, err := svc.AddProject("/nonexistent/path/xyz")
	if err == nil {
		t.Error("AddProject with non-existent path should fail when fileService is linked")
	}
}

func TestSortProjectsByRecency_DescendingOrder(t *testing.T) {
	projects := []Project{
		{ID: "a", LastOpened: 100},
		{ID: "b", LastOpened: 300},
		{ID: "c", LastOpened: 200},
	}
	sortProjectsByRecency(projects)
	if projects[0].ID != "b" {
		t.Errorf("expected 'b' (300) first, got %s", projects[0].ID)
	}
	if projects[1].ID != "c" {
		t.Errorf("expected 'c' (200) second, got %s", projects[1].ID)
	}
	if projects[2].ID != "a" {
		t.Errorf("expected 'a' (100) third, got %s", projects[2].ID)
	}
}

func TestSortProjectsByRecency_EmptyAndSingle(t *testing.T) {
	sortProjectsByRecency(nil)
	sortProjectsByRecency([]Project{{ID: "x", LastOpened: 1}})
}

func TestProjectService_AddProjectGeneratesUniqueIDs(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "projects.json")
	svc := &ProjectService{configPath: configPath}

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	p1, err := svc.AddProject(dir1)
	if err != nil {
		t.Fatalf("AddProject dir1 failed: %v", err)
	}
	p2, err := svc.AddProject(dir2)
	if err != nil {
		t.Fatalf("AddProject dir2 failed: %v", err)
	}

	if p1.ID == p2.ID {
		t.Errorf("expected unique IDs, got duplicate %s", p1.ID)
	}
	if len(p1.ID) < 16 {
		t.Errorf("expected ID length >= 16, got %d", len(p1.ID))
	}
}

func TestProjectService_RemoveProject_RejectsInvalidID(t *testing.T) {
	svc := NewProjectService(nil, nil, nil, nil)
	err := svc.RemoveProject("../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path-traversal ID")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error should mention 'invalid', got: %v", err)
	}
}

func TestProjectService_RemoveProject_RejectsEmptyID(t *testing.T) {
	svc := NewProjectService(nil, nil, nil, nil)
	err := svc.RemoveProject("")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

// N-67 / Proposal AJ: AddProject must propagate SetWorkspaceRoot to
// GitService so git operations on paths outside the project are rejected.
func TestProjectService_AddProject_PropagatesWorkspaceRootToGitService(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "projects.json")
	workspace := t.TempDir()
	outsideRepo := t.TempDir()

	gitSvc := &GitService{}
	svc := &ProjectService{
		configPath:  configPath,
		gitService:  gitSvc,
		fileService: &FileService{}, // needed so AddProject doesn't nil-deref
	}
	if _, err := svc.AddProject(workspace); err != nil {
		t.Fatalf("AddProject failed: %v", err)
	}
	// Init a git repo outside the workspace — GitService should reject it.
	if _, err := git.PlainInit(outsideRepo, false); err != nil {
		t.Fatalf("git.PlainInit failed: %v", err)
	}
	if _, err := gitSvc.GetStatus(outsideRepo); err == nil {
		t.Error("GitService.GetStatus on outside repo should be blocked after AddProject")
	}
	// Init a git repo inside the workspace — should be allowed.
	insideRepo := filepath.Join(workspace, "myrepo")
	if err := os.MkdirAll(insideRepo, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := git.PlainInit(insideRepo, false); err != nil {
		t.Fatalf("git.PlainInit inside failed: %v", err)
	}
	if _, err := gitSvc.GetStatus(insideRepo); err != nil {
		t.Errorf("GitService.GetStatus inside workspace should succeed: %v", err)
	}
}

// N-67 / Proposal AJ: AddProject must propagate SetWorkspaceRoot to
// SearchService so search/replace operations on paths outside the
// project are rejected.
func TestProjectService_AddProject_PropagatesWorkspaceRootToSearchService(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "projects.json")
	workspace := t.TempDir()
	outside := t.TempDir()

	searchSvc := &SearchService{}
	svc := &ProjectService{
		configPath:    configPath,
		searchService: searchSvc,
		fileService:   &FileService{},
	}
	if _, err := svc.AddProject(workspace); err != nil {
		t.Fatalf("AddProject failed: %v", err)
	}
	// Search outside the workspace should be blocked.
	writeFile(t, outside, "target.txt", "hello")
	if _, err := searchSvc.Search(outside, "hello", false); err == nil {
		t.Error("SearchService.Search outside workspace should be blocked after AddProject")
	}
	// Search inside the workspace should work.
	writeFile(t, workspace, "target.txt", "hello")
	if _, err := searchSvc.Search(workspace, "hello", false); err != nil {
		t.Errorf("SearchService.Search inside workspace should succeed: %v", err)
	}
}
