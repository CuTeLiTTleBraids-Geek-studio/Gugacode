package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
)

// expectFile reads a file under dir/rel and fails the test if it is missing.
func expectFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("expected file %s under %s: %v", rel, dir, err)
	}
	return string(b)
}

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

// ============================================================================
// G-FEAT-01: New Project scaffolding wizard tests.
// ============================================================================

func TestProjectService_ListProjectTemplates(t *testing.T) {
	svc := &ProjectService{}
	templates := svc.ListProjectTemplates()
	if len(templates) != 5 {
		t.Fatalf("expected 5 templates, got %d", len(templates))
	}
	ids := map[string]bool{}
	for _, tpl := range templates {
		if tpl.ID == "" || tpl.Name == "" || tpl.Description == "" || tpl.Language == "" {
			t.Errorf("template has empty field: %+v", tpl)
		}
		ids[tpl.ID] = true
	}
	for _, want := range []string{"go", "typescript", "javascript", "monorepo", "fullstack"} {
		if !ids[want] {
			t.Errorf("expected template ID %q in list", want)
		}
	}
}

func TestProjectService_CreateProject_Go(t *testing.T) {
	svc := &ProjectService{}
	dir := t.TempDir()
	req := CreateProjectRequest{
		TemplateID:  "go",
		ProjectName: "demo-app",
		TargetDir:   dir,
		ModuleName:  "github.com/example/demo-app",
	}
	out, err := svc.CreateProject(req)
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}
	// Verify the expected files exist.
	goMod := expectFile(t, out, "go.mod")
	if !strings.Contains(goMod, "module github.com/example/demo-app") {
		t.Errorf("go.mod missing module line, got:\n%s", goMod)
	}
	expectFile(t, out, "cmd/main.go")
	expectFile(t, out, "Makefile")
	expectFile(t, out, ".golangci.yml")
	expectFile(t, out, "Dockerfile")
	ci := expectFile(t, out, filepath.FromSlash(".github/workflows/ci.yml"))
	if !strings.Contains(ci, "go-version") {
		t.Errorf("CI workflow missing go-version, got:\n%s", ci)
	}
	// main.go should reference the project name.
	mainGo := expectFile(t, out, "cmd/main.go")
	if !strings.Contains(mainGo, "demo-app") {
		t.Errorf("main.go should reference project name, got:\n%s", mainGo)
	}
}

func TestProjectService_CreateProject_TypeScript(t *testing.T) {
	svc := &ProjectService{}
	dir := t.TempDir()
	out, err := svc.CreateProject(CreateProjectRequest{
		TemplateID:  "typescript",
		ProjectName: "ts-demo",
		TargetDir:   dir,
	})
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}
	pkg := expectFile(t, out, "package.json")
	if !strings.Contains(pkg, `"name": "ts-demo"`) {
		t.Errorf("package.json missing name, got:\n%s", pkg)
	}
	tsconfig := expectFile(t, out, "tsconfig.json")
	if !strings.Contains(tsconfig, `"strict": true`) {
		t.Errorf("tsconfig should be strict, got:\n%s", tsconfig)
	}
	expectFile(t, out, "src/index.ts")
	expectFile(t, out, "eslint.config.js")
	expectFile(t, out, "vitest.config.ts")
}

func TestProjectService_CreateProject_JavaScript(t *testing.T) {
	svc := &ProjectService{}
	dir := t.TempDir()
	out, err := svc.CreateProject(CreateProjectRequest{
		TemplateID:  "javascript",
		ProjectName: "js-demo",
		TargetDir:   dir,
	})
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}
	pkg := expectFile(t, out, "package.json")
	if !strings.Contains(pkg, `"name": "js-demo"`) {
		t.Errorf("package.json missing name, got:\n%s", pkg)
	}
	expectFile(t, out, "src/index.js")
	expectFile(t, out, "eslint.config.js")
	expectFile(t, out, "vitest.config.ts")
}

func TestProjectService_CreateProject_Monorepo(t *testing.T) {
	svc := &ProjectService{}
	dir := t.TempDir()
	out, err := svc.CreateProject(CreateProjectRequest{
		TemplateID:  "monorepo",
		ProjectName: "mono-demo",
		TargetDir:   dir,
	})
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}
	ws := expectFile(t, out, "pnpm-workspace.yaml")
	if !strings.Contains(ws, "apps/*") || !strings.Contains(ws, "packages/*") {
		t.Errorf("pnpm-workspace.yaml missing globs, got:\n%s", ws)
	}
	expectFile(t, out, "package.json")
	expectFile(t, out, "tsconfig.base.json")
	webPkg := expectFile(t, out, filepath.FromSlash("apps/web/package.json"))
	if !strings.Contains(webPkg, "@mono-demo/web") {
		t.Errorf("web package.json missing scoped name, got:\n%s", webPkg)
	}
	sharedPkg := expectFile(t, out, filepath.FromSlash("packages/shared/package.json"))
	if !strings.Contains(sharedPkg, "@mono-demo/shared") {
		t.Errorf("shared package.json missing scoped name, got:\n%s", sharedPkg)
	}
}

func TestProjectService_CreateProject_Fullstack(t *testing.T) {
	svc := &ProjectService{}
	dir := t.TempDir()
	out, err := svc.CreateProject(CreateProjectRequest{
		TemplateID:  "fullstack",
		ProjectName: "fs-demo",
		TargetDir:   dir,
		ModuleName:  "github.com/example/fs-demo",
	})
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}
	goMod := expectFile(t, out, filepath.FromSlash("backend/go.mod"))
	if !strings.Contains(goMod, "module github.com/example/fs-demo") {
		t.Errorf("backend go.mod missing module line, got:\n%s", goMod)
	}
	expectFile(t, out, filepath.FromSlash("backend/cmd/main.go"))
	fePkg := expectFile(t, out, filepath.FromSlash("frontend/package.json"))
	if !strings.Contains(fePkg, "fs-demo-frontend") {
		t.Errorf("frontend package.json missing name, got:\n%s", fePkg)
	}
	expectFile(t, out, filepath.FromSlash("frontend/src/main.ts"))
	expectFile(t, out, filepath.FromSlash("frontend/tsconfig.json"))
}

func TestProjectService_CreateProject_RejectsInvalidTemplateID(t *testing.T) {
	svc := &ProjectService{}
	dir := t.TempDir()
	_, err := svc.CreateProject(CreateProjectRequest{
		TemplateID:  "../../../etc",
		ProjectName: "x",
		TargetDir:   dir,
	})
	if err == nil {
		t.Fatal("expected error for path-traversal template ID")
	}
	if !strings.Contains(err.Error(), "invalid template ID") {
		t.Errorf("error should mention 'invalid template ID', got: %v", err)
	}
}

func TestProjectService_CreateProject_RejectsEmptyProjectName(t *testing.T) {
	svc := &ProjectService{}
	_, err := svc.CreateProject(CreateProjectRequest{
		TemplateID:  "typescript",
		ProjectName: "   ",
		TargetDir:   t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for empty project name")
	}
	if !strings.Contains(err.Error(), "project name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProjectService_CreateProject_RejectsGoTemplateWithoutModuleName(t *testing.T) {
	svc := &ProjectService{}
	_, err := svc.CreateProject(CreateProjectRequest{
		TemplateID:  "go",
		ProjectName: "demo",
		TargetDir:   t.TempDir(),
		// ModuleName intentionally empty.
	})
	if err == nil {
		t.Fatal("expected error for Go template without module name")
	}
	if !strings.Contains(err.Error(), "module name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestProjectService_CreateProject_EscapesShellInjection verifies that a
// module name containing shell metacharacters (`"; rm -rf /`) is rejected
// rather than rendered into go.mod, so the generated go.mod stays valid.
func TestProjectService_CreateProject_EscapesShellInjection(t *testing.T) {
	svc := &ProjectService{}
	dir := t.TempDir()
	_, err := svc.CreateProject(CreateProjectRequest{
		TemplateID:  "go",
		ProjectName: "demo",
		TargetDir:   dir,
		ModuleName:  `"; rm -rf /`,
	})
	if err == nil {
		t.Fatal("expected error for shell-injection module name")
	}
	if !strings.Contains(err.Error(), "invalid module name") {
		t.Errorf("error should mention 'invalid module name', got: %v", err)
	}
	// The target directory should not contain any generated files.
	entries, _ := os.ReadDir(filepath.Join(dir, "demo"))
	if len(entries) != 0 {
		t.Errorf("no files should be written when validation fails, got %d entries", len(entries))
	}
}

// TestProjectService_CreateProject_EscapesProjectNameInjection verifies that
// a project name with shell metacharacters is rejected so package.json
// cannot be corrupted.
func TestProjectService_CreateProject_EscapesProjectNameInjection(t *testing.T) {
	svc := &ProjectService{}
	_, err := svc.CreateProject(CreateProjectRequest{
		TemplateID:  "typescript",
		ProjectName: `evil"; require("child_process")`,
		TargetDir:   t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for shell-injection project name")
	}
	if !strings.Contains(err.Error(), "invalid project name") {
		t.Errorf("error should mention 'invalid project name', got: %v", err)
	}
}

func TestProjectService_CreateProject_RejectsNonExistentTargetDirParent(t *testing.T) {
	svc := &ProjectService{}
	// A target dir whose parent doesn't exist — MkdirAll should still create
	// it, but the validation path should not panic. Use a deeply nested path.
	missing := filepath.Join(t.TempDir(), "does", "not", "exist", "yet")
	out, err := svc.CreateProject(CreateProjectRequest{
		TemplateID:  "typescript",
		ProjectName: "deep",
		TargetDir:   missing,
	})
	if err != nil {
		t.Fatalf("CreateProject should create nested parents: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "package.json")); err != nil {
		t.Errorf("expected package.json under %s: %v", out, err)
	}
}

func TestProjectService_CreateProject_RefusesNonEmptyTarget(t *testing.T) {
	svc := &ProjectService{}
	dir := t.TempDir()
	// First creation succeeds.
	if _, err := svc.CreateProject(CreateProjectRequest{
		TemplateID:  "typescript",
		ProjectName: "exists",
		TargetDir:   dir,
	}); err != nil {
		t.Fatalf("first CreateProject failed: %v", err)
	}
	// Second creation into the same dir/project should be refused.
	_, err := svc.CreateProject(CreateProjectRequest{
		TemplateID:  "typescript",
		ProjectName: "exists",
		TargetDir:   dir,
	})
	if err == nil {
		t.Fatal("expected error when target directory is non-empty")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got: %v", err)
	}
}
