package services

// Plan 11 — GoalExecutor / StepExecutor / SecurityChecker 的默认实现。
//
// 前端通过 Wails bindings 调用 RunGoal/ResumeGoal/ExecuteStep 时无法传递
// Go 接口实例（executor/checker 会被序列化为 nil）。这些适配器在 main.go
// 中注入到 AIGoalService / AIPlanService，当参数为 nil 时自动回退使用。

import (
	"fmt"
	"path/filepath"
)

// defaultSecurityChecker 用 AgentService.CheckCommand 实现 SecurityChecker。
type defaultSecurityChecker struct {
	agent *AgentService
	root  string
}

func (c *defaultSecurityChecker) CheckCommand(command string) CommandCheck {
	if c.agent == nil {
		return CommandCheck{Blocked: false}
	}
	return c.agent.CheckCommand(command)
}

func (c *defaultSecurityChecker) IsWorkspacePath(path string) bool {
	if c.root == "" {
		return true // 未设置根目录时不阻断
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return !IsPathOutsideRoot(c.root, abs)
}

// defaultStepExecutor 用 AgentService.ExecCommand 实现 StepExecutor。
type defaultStepExecutor struct {
	agent *AgentService
	root  string
}

func (e *defaultStepExecutor) Execute(tool, args string) (string, error) {
	if e.agent == nil {
		return "", fmt.Errorf("agent service not injected: %w", ErrInvalidInput)
	}
	// 将 tool+args 组合为命令执行。
	// tool 可以是 "shell" / "mcp" / "file_read" 等；args 是 JSON 参数。
	// 简化实现：直接把 args 当作命令行执行（适用于 shell tool）。
	cmd := args
	if cmd == "" {
		cmd = tool
	}
	result, err := e.agent.ExecCommand(cmd, e.root)
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

// defaultGoalExecutor 用 AgentService 实现 GoalExecutor（简化版）。
type defaultGoalExecutor struct {
	agent *AgentService
	root  string
}

func (e *defaultGoalExecutor) Plan(goal *Goal) (string, error) {
	// 简化实现：返回基于 goal.Description 的固定规划。
	// 完整实现应调用 AIService 让 AI 生成步骤。
	return fmt.Sprintf("Plan for goal %q: analyze requirements, execute steps, verify", goal.Description), nil
}

func (e *defaultGoalExecutor) Execute(goal *Goal, steps string) (GoalRoundResult, error) {
	if e.agent == nil {
		return GoalRoundResult{}, fmt.Errorf("agent service not injected: %w", ErrInvalidInput)
	}
	// 简化实现：执行一个无害的命令来证明 executor 可用。
	result, err := e.agent.ExecCommand("echo goal-step", e.root)
	if err != nil {
		return GoalRoundResult{Error: err.Error()}, err
	}
	return GoalRoundResult{
		Success:  result.ExitCode == 0,
		Snapshot: "",
		Note:     fmt.Sprintf("executed step for goal %q", goal.ID),
	}, nil
}

func (e *defaultGoalExecutor) Evaluate(goal *Goal) (bool, error) {
	// 简化实现：不自动判定达成。完整实现应调用 AIService 评估。
	return false, nil
}

// NewDefaultSecurityChecker 创建默认 SecurityChecker。
func NewDefaultSecurityChecker(agent *AgentService, workspaceRoot string) SecurityChecker {
	return &defaultSecurityChecker{agent: agent, root: workspaceRoot}
}

// NewDefaultStepExecutor 创建默认 StepExecutor。
func NewDefaultStepExecutor(agent *AgentService, workspaceRoot string) StepExecutor {
	return &defaultStepExecutor{agent: agent, root: workspaceRoot}
}

// NewDefaultGoalExecutor 创建默认 GoalExecutor。
func NewDefaultGoalExecutor(agent *AgentService, workspaceRoot string) GoalExecutor {
	return &defaultGoalExecutor{agent: agent, root: workspaceRoot}
}
