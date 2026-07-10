package main

// prompt-5 Task I: service registration extracted from main.go so the entry
// point stays focused on lifecycle (lock → wire → window → run).

import (
	"gugacode/services"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// appBundle holds every backend service constructed during bootstrap.
// Fields are exported only for tests that need to inspect wiring.
type appBundle struct {
	File               *services.FileService
	Project            *services.ProjectService
	Settings           *services.SettingsService
	Window             *services.WindowService
	Terminal           *services.TerminalService
	AI                 *services.AIService
	Git                *services.GitService
	Search             *services.SearchService
	Conversation       *services.ConversationService
	Task               *services.TaskService
	Workflow           *services.WorkflowService
	Agent              *services.AgentService
	Rules              *services.RulesService
	LogLevel           *services.LogLevelService
	Plugin             *services.PluginService
	Profile            *services.ProfileService
	Layout             *services.LayoutService
	LSP                *services.LSPService
	Toolchain          *services.ToolchainService
	Marketplace        *services.MarketplaceService
	ExtensionSecurity  *services.ExtensionSecurityService
	MCP                *services.MCPService
	Skills             *services.SkillsService
	ComputerUse        *services.ComputerUseService
	IM                 *services.IMService
	Persona            *services.PersonaService
	AIPlan             *services.AIPlanService
	AIGoal             *services.AIGoalService
	AIPermission       *services.AIPermissionService
	Diff               *services.DiffService
	Snapshot           *services.SnapshotService
	Preset             *services.PresetService
	Debug              *services.DebugService
	Coverage           *services.CoverageService
	InstanceLock       *services.InstanceLock
}

// wailsServices returns the application.Service slice registered with Wails.
func (b *appBundle) wailsServices() []application.Service {
	return []application.Service{
		application.NewService(b.File),
		application.NewService(b.Project),
		application.NewService(b.Settings),
		application.NewService(b.Window),
		application.NewService(b.Terminal),
		application.NewService(b.AI),
		application.NewService(b.Git),
		application.NewService(b.Search),
		application.NewService(b.Conversation),
		application.NewService(b.Task),
		application.NewService(b.Workflow),
		application.NewService(b.Agent),
		application.NewService(b.Rules),
		application.NewService(b.LogLevel),
		application.NewService(b.Plugin),
		application.NewService(b.Profile),
		application.NewService(b.Layout),
		application.NewService(b.LSP),
		application.NewService(b.Toolchain),
		application.NewService(b.Marketplace),
		application.NewService(b.ExtensionSecurity),
		application.NewService(b.MCP),
		application.NewService(b.Skills),
		application.NewService(b.ComputerUse),
		application.NewService(b.IM),
		application.NewService(b.Persona),
		application.NewService(b.AIPlan),
		application.NewService(b.AIGoal),
		application.NewService(b.AIPermission),
		application.NewService(b.Diff),
		application.NewService(b.Snapshot),
		application.NewService(b.Debug),
		application.NewService(b.Coverage),
	}
}
