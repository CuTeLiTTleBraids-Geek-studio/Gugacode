package services

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTasksFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestLoadTasks_NoFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	svc := NewTaskService()
	tasks, err := svc.LoadTasks(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestLoadTasks_EmptyRootReturnsError(t *testing.T) {
	svc := NewTaskService()
	_, err := svc.LoadTasks("")
	if err == nil {
		t.Fatal("expected error for empty root, got nil")
	}
}

func TestLoadTasks_DotNknkTasksJson(t *testing.T) {
	dir := t.TempDir()
	writeTasksFile(t, dir, ".nknk/tasks.json", `{
		"version": "1",
		"tasks": [
			{"label": "build", "command": "go", "args": ["build", "./..."]},
			{"label": "test", "command": "go", "args": ["test", "./..."]}
		]
	}`)
	svc := NewTaskService()
	tasks, err := svc.LoadTasks(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Label != "build" || tasks[1].Label != "test" {
		t.Errorf("unexpected labels: %q, %q", tasks[0].Label, tasks[1].Label)
	}
}

func TestLoadTasks_LegacyTaskJsonAtRoot(t *testing.T) {
	dir := t.TempDir()
	writeTasksFile(t, dir, "task.json", `{
		"version": "1",
		"tasks": [{"label": "run", "command": "npm start"}]
	}`)
	svc := NewTaskService()
	tasks, err := svc.LoadTasks(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Label != "run" {
		t.Errorf("expected 1 task 'run', got %+v", tasks)
	}
}

func TestLoadTasks_DotNknkTakesPriorityOverRoot(t *testing.T) {
	dir := t.TempDir()
	writeTasksFile(t, dir, ".nknk/tasks.json", `{"tasks":[{"label":"from-dotnknk","command":"a"}]}`)
	writeTasksFile(t, dir, "task.json", `{"tasks":[{"label":"from-root","command":"b"}]}`)
	svc := NewTaskService()
	tasks, err := svc.LoadTasks(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Label != "from-dotnknk" {
		t.Errorf("expected from-dotnknk, got %+v", tasks)
	}
}

func TestLoadTasks_InvalidJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	writeTasksFile(t, dir, ".nknk/tasks.json", `{not valid json`)
	svc := NewTaskService()
	_, err := svc.LoadTasks(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestLoadTasks_SkipsTasksWithoutLabelOrCommand(t *testing.T) {
	dir := t.TempDir()
	writeTasksFile(t, dir, ".nknk/tasks.json", `{
		"tasks": [
			{"label": "ok", "command": "echo hi"},
			{"label": "", "command": "no-label"},
			{"label": "no-cmd", "command": ""},
			{"command": "no-label-no-cmd"}
		]
	}`)
	svc := NewTaskService()
	tasks, err := svc.LoadTasks(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 valid task, got %d: %+v", len(tasks), tasks)
	}
	if tasks[0].Label != "ok" {
		t.Errorf("expected 'ok', got %q", tasks[0].Label)
	}
}

func TestComposeCommandLine_NoArgs(t *testing.T) {
	td := TaskDef{Command: "ls"}
	if got := td.ComposeCommandLine(); got != "ls" {
		t.Errorf("expected 'ls', got %q", got)
	}
}

func TestComposeCommandLine_WithArgs(t *testing.T) {
	td := TaskDef{Command: "go", Args: []string{"build", "./..."}}
	got := td.ComposeCommandLine()
	want := "go 'build' './...'"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestComposeCommandLine_EscapesSingleQuotes(t *testing.T) {
	td := TaskDef{Command: "echo", Args: []string{"it's"}}
	got := td.ComposeCommandLine()
	// Should wrap arg in single quotes and escape the embedded quote.
	want := "echo 'it'\\''s'"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}
