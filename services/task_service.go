package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TaskDef describes a single named command runnable from the UI.
// It mirrors a subset of VS Code's tasks.json schema (simplified).
type TaskDef struct {
	Label   string   `json:"label"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Cwd     string   `json:"cwd,omitempty"`
	// Shell, when true, runs the command line through the user's shell
	// (`sh -c` on unix, `cmd /c` on windows). When false (default), the
	// command is executed directly. The frontend currently always uses
	// shell mode by writing the command into a terminal session, so this
	// field is informational for future non-terminal execution.
	Shell bool `json:"shell,omitempty"`
}

// TaskFile is the on-disk schema for .nknk/tasks.json.
type TaskFile struct {
	Version string    `json:"version"`
	Tasks   []TaskDef `json:"tasks"`
}

// TaskService exposes project-scoped task definitions to the frontend.
type TaskService struct{}

// NewTaskService creates a new TaskService.
func NewTaskService() *TaskService {
	return &TaskService{}
}

// LoadTasks reads the project's task definitions. Returns an empty list
// (not an error) when no tasks file exists, so the frontend can always
// render the Tasks panel.
func (s *TaskService) LoadTasks(projectRoot string) ([]TaskDef, error) {
	if projectRoot == "" {
		return nil, fmt.Errorf("projectRoot is required")
	}
	// Try .nknk/tasks.json first, then task.json at the root.
	candidates := []string{
		filepath.Join(projectRoot, ".nknk", "tasks.json"),
		filepath.Join(projectRoot, "task.json"),
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		var tf TaskFile
		if err := json.Unmarshal(data, &tf); err != nil {
			return nil, fmt.Errorf("parse %s: %w", p, err)
		}
		// Validate: each task must have a label and a command.
		out := make([]TaskDef, 0, len(tf.Tasks))
		for _, t := range tf.Tasks {
			if t.Label == "" || t.Command == "" {
				continue
			}
			out = append(out, t)
		}
		return out, nil
	}
	return []TaskDef{}, nil
}

// ComposeCommandLine builds a single shell-ready command line from a TaskDef.
// The frontend uses this to write the command into a terminal session.
func (t TaskDef) ComposeCommandLine() string {
	out := t.Command
	for _, a := range t.Args {
		out += " " + shellQuote(a)
	}
	return out
}

// shellQuote wraps a single argument in single quotes, escaping any embedded
// single quotes. This is a best-effort cross-platform quoting; for the
// terminal-write use case the user's shell will re-parse the line.
func shellQuote(s string) string {
	// Replace every ' with '\'' (close quote, escaped quote, reopen quote).
	escaped := ""
	for _, r := range s {
		if r == '\'' {
			escaped += `'\''`
		} else {
			escaped += string(r)
		}
	}
	return "'" + escaped + "'"
}
