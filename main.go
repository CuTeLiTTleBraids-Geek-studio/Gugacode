package main

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gugacode/services"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[string]("time")
	application.RegisterEvent[map[string]string]("terminal:output")
	// prompt-6 Task 2: AI stream events carry {streamId, data|busy|...}.
	application.RegisterEvent[map[string]interface{}]("ai:chunk")
	application.RegisterEvent[map[string]interface{}]("ai:done")
	application.RegisterEvent[map[string]interface{}]("ai:error")
	// prompt-5 Task B + prompt-6 Task 2: busy flag with streamId.
	application.RegisterEvent[map[string]interface{}]("ai:stream-busy")
	// prompt-5 Task H: native tool_calls JSON (wrapped with streamId).
	application.RegisterEvent[map[string]interface{}]("ai:tool_calls")
	// Plan 65 / Proposal B: file:saved is emitted by FileService.WriteFile
	// so the frontend can trigger workflows with matching runOn triggers.
	application.RegisterEvent[string]("file:saved")
	// N-152: 前端标题栏根据 window:maximised 事件切换放大/还原图标。
	application.RegisterEvent[bool]("window:maximised")
	// prompt-4 Task 5: 主窗口选中代码发送到 AI 独立窗口。
	application.RegisterEvent[map[string]string]("ai:selection")
	// prompt-4 Task 5: AI 窗口「应用到编辑器」→ 主窗口应用代码。
	application.RegisterEvent[map[string]string]("ai:apply-to-editor")
	// prompt-6 Task 1: dual-window SSOT sync bus.
	application.RegisterEvent[map[string]interface{}]("settings:changed")
	application.RegisterEvent[map[string]interface{}]("conversation:saved")
	application.RegisterEvent[map[string]interface{}]("agent:pending-updated")
}

func main() {
	// Initialize structured logging (N-11) before any service is created
	// so all services inherit the configured default logger. The cleanup
	// function closes the log file on shutdown.
	closeLogger := services.InitLogger(slog.LevelInfo)
	defer closeLogger()

	configDir, _ := os.UserConfigDir()
	// G-QUAL-05: acquire a single-instance lock before any service reads
	// or writes settings.json. Without this, two concurrently running
	// gugacode instances can both write settings.json and corrupt it.
	// The lock lives in the gugacode config directory next to settings.json.
	gugacodeDir := filepath.Join(configDir, "gugacode")
	if merr := os.MkdirAll(gugacodeDir, 0o755); merr != nil {
		log.Fatalf("create config directory: %v", merr)
	}
	instanceLock := services.NewInstanceLock(gugacodeDir)
	if err := instanceLock.Acquire(); err != nil {
		log.Fatalf("G-QUAL-05: %v", err)
	}
	// Safety net for panic paths. The normal shutdown path calls
	// Release explicitly before log.Fatal (deferred funcs are skipped
	// by os.Exit). Release is idempotent, so double-release is safe.
	defer instanceLock.Release()

	// prompt-5 Task I: construct services into an appBundle for registration.
	bundle := &appBundle{}
	fileService := &services.FileService{}
	bundle.File = fileService
	terminalService := services.NewTerminalService()
	bundle.Terminal = terminalService
	agentService := services.NewAgentService()
	bundle.Agent = agentService
	aiService := services.NewAIService()
	bundle.AI = aiService
	// N-17: pass configDir so the user-global preset layer
	// (<configDir>/gugacode/presets/*.json) is active. Without this,
	// SaveUserPreset/ListPresetsWithSource would skip the user layer.
	presetService := services.NewPresetService(configDir)
	bundle.Preset = presetService
	aiService.SetPresetService(presetService)
	projectService := services.NewProjectService(fileService, terminalService, agentService, aiService)
	bundle.Project = projectService
	// Plan 50: ProfileService manages multi-profile directory structure.
	// Created before SettingsService so the active profile's settings
	// path can be passed to the SettingsService constructor.
	profileService := services.NewProfileService(configDir)
	bundle.Profile = profileService
	activeSettingsPath, perr := profileService.ActiveSettingsPath()
	if perr != nil || activeSettingsPath == "" {
		activeSettingsPath = filepath.Join(configDir, "gugacode", "settings.json")
	}
	settingsService := services.NewSettingsServiceWithPath(activeSettingsPath)
	bundle.Settings = settingsService
	// Plan 72 / N-25: LayoutService stores the layout tree JSON in
	// layout.json in the same profile directory as settings.json.
	activeLayoutPath := filepath.Join(filepath.Dir(activeSettingsPath), "layout.json")
	layoutService := services.NewLayoutServiceWithPath(activeLayoutPath)
	bundle.Layout = layoutService
	// Wire profile switch callback so SettingsService and LayoutService
	// are redirected to the new profile's directory on switch.
	profileService.SetOnSwitch(func(p string) {
		settingsService.SetConfigPath(p)
		layoutService.SetLayoutPath(filepath.Join(filepath.Dir(p), "layout.json"))
	})
	windowService := &services.WindowService{}
	bundle.Window = windowService
	gitService := &services.GitService{}
	bundle.Git = gitService
	searchService := &services.SearchService{}
	bundle.Search = searchService
	// G-FEAT-02: LSPService provides offline code completion (gopls/tsserver).
	// workspaceRoot is empty at startup; ProjectService updates it when a
	// project is opened so language servers run in the project's context.
	lspService := services.NewLSPService("")
	bundle.LSP = lspService
	// N-67: wire GitService and SearchService into ProjectService so their
	// workspace roots are updated when a project is added. This sandbox
	// prevents the frontend from operating on paths outside the open project.
	projectService.SetGitService(gitService)
	projectService.SetSearchService(searchService)
	// G-FEAT-02: wire LSPService so its workspace root follows the open project.
	projectService.SetLSPService(lspService)
	// G-FEAT-03: ToolchainService exposes Go/TS/JS build/test/lint/format
	// commands for the command palette and editor context menu. Its workspace
	// root follows the open project (set below via SetToolchainService) and
	// tool path overrides are synced from Settings.ToolPaths on load/save.
	toolchainService := services.NewToolchainService()
	bundle.Toolchain = toolchainService
	projectService.SetToolchainService(toolchainService)
	// G-VSC-01: MarketplaceService searches/browses/downloads/installs VS Code
	// extensions (VSIX) from the Open VSX Registry by default. Installed
	// extensions live under <configDir>/gugacode/extensions/ and are disabled
	// by default (G-SEC-12 req. 2). Downloads are SHA-256 verified (req. 3).
	marketplaceService := services.NewMarketplaceService(configDir)
	bundle.Marketplace = marketplaceService
	// G-SEC-12: ExtensionSecurityService tracks install/enable state,
	// blacklist, and permission classification for VS Code extensions.
	// Wired into MarketplaceService so installs are registered and
	// blacklisted extensions are rejected before they land on disk.
	extensionSecurityService := services.NewExtensionSecurityService(configDir)
	bundle.ExtensionSecurity = extensionSecurityService
	marketplaceService.SetSecurityService(extensionSecurityService)
	conversationService := services.NewConversationService("")
	bundle.Conversation = conversationService
	taskService := services.NewTaskService()
	bundle.Task = taskService
	workflowService := services.NewWorkflowService()
	bundle.Workflow = workflowService
	logLevelService := services.NewLogLevelService()
	bundle.LogLevel = logLevelService
	rulesService := services.NewRulesService(configDir)
	bundle.Rules = rulesService
	// Plan 49: PluginService discovers user-global and project-scoped
	// plugins. Pass configDir so the user layer
	// (<configDir>/gugacode/plugins/<name>/) is active.
	pluginService := services.NewPluginService(configDir)
	bundle.Plugin = pluginService
	// Plan 11 Task 4: MCPService manages MCP server connections (stdio/SSE/HTTP).
	// Wired into AgentService so mcp.<server>.<tool> calls are dispatched via
	// MCPService.CallTool after CheckCommand approval (G-SEC-02).
	mcpService := services.NewMCPService()
	bundle.MCP = mcpService
	agentService.SetMCPService(mcpService)
	// Plan 11 Task 5: SkillsService discovers and loads skill YAML files
	// (project-level .nknk/skills/ + user-level <configDir>/gugacode/skills/).
	// Wired into AgentService so SetWorkspaceRoot propagates to SkillsService
	// (reloading project-scoped skills on project switch) and so the agent
	// can apply SystemPrompt + AllowedTools from matched skills (G-SEC-02/03).
	skillsService := services.NewSkillsService(configDir)
	bundle.Skills = skillsService
	agentService.SetSkillsService(skillsService)
	// Plan 11 Task 6: ComputerUseService manages screenshot/mouse/keyboard
	// automation (5 tools). G-SEC-12: disabled by default; enabling is an
	// explicit approval action (Restricted). Operations require user
	// confirmation (ConfirmationRequired) and are audit-logged.
	computerUseService := services.NewComputerUseService(configDir)
	bundle.ComputerUse = computerUseService
	// Plan 11 Task 7: IMService manages Slack/Discord/飞书/企业微信
	// integration (send + receive + notifications). G-SEC-07: Bot Token /
	// Webhook URL encrypted via EncryptSecret (AES-256-GCM/DPAPI), LoadConfig
	// returns only configured booleans. G-SEC-12: sending requires Approve.
	imService := services.NewIMService(configDir)
	bundle.IM = imService
	// Plan 11 Task 8: PersonaService manages built-in + custom Personas
	// (AI assistant roles). 7 built-in (Go Guru / TypeScript Master / etc.)
	// + user-defined persisted to <configDir>/gugacode/personas/*.json (0600).
	personaService := services.NewPersonaService(configDir)
	bundle.Persona = personaService
	// Plan 11 Task 9: AIPlanService manages Plan mode (plan-first-then-execute).
	// Plan 模式下 AI 只能用 plan 工具生成步骤，用户审批后执行。
	// G-SEC-02（Step 9）：每步 Tool 调用经 AgentService.CheckCommand。
	// Plan 与 Goal 互斥（Step 8）：active 跟踪当前活动 Plan。
	aiPlanService := services.NewAIPlanService()
	bundle.AIPlan = aiPlanService
	// Plan 11 Task 10: AIGoalService manages Goal mode (autonomous goal-driven).
	// Goal 模式下 AI 自治连续执行：规划→执行→评估→调整，每轮 Checkpoint。
	// G-SEC-02（Step 9）：每轮工具调用经 CheckCommand。
	// G-SEC-03（Step 10）：Goal 视同不可信 workflow，创建需显式确认。
	// Step 8 安全边界：禁删工作区外文件/禁 git push --force/禁 RiskDangerous。
	aiGoalService := services.NewAIGoalService()
	bundle.AIGoal = aiGoalService
	// Plan 11 Task 12: AIPermissionService manages per-operation model
	// assignment + fallback + usage statistics + cost optimization.
	// 模型权限分配：每个操作（chat/agent/review/etc.）可指定不同模型 + fallback。
	// G-SEC-07（Step 9）：所有调用走 UseStoredKey+ConfigID。
	// Step 6：操作级权限（某些操作可禁用）。
	aiPermissionService := services.NewAIPermissionService(configDir)
	bundle.AIPermission = aiPermissionService
	// Plan 11 Task 13: DiffService 提供结构化多文件 diff / 三方合并 / AI 审查标注 /
	// 行内评论 / Apply·Reject / PR 审查 / Markdown·HTML·unified diff 导出。
	// 纯计算服务（无文件系统副作用），故无需注入 configDir。
	diffService := services.NewDiffService()
	bundle.Diff = diffService
	// Plan 11 Task 14: SnapshotService 智能回滚（快照 + 内容寻址存储 + 清理策略）。
	// 存储于 <configDir>/gugacode/snapshots/，metadata 0600，blob 内容寻址去重。
	snapshotService := services.NewSnapshotService(configDir)
	bundle.Snapshot = snapshotService
	// prompt-10 10-G / 10-H: Delve headless DAP + coverage profile services.
	debugService := services.NewDebugService()
	bundle.Debug = debugService
	coverageService := services.NewCoverageService()
	bundle.Coverage = coverageService
	// prompt-13 13-B: long-lived ESLint (eslint_d preferred)
	eslintService := services.NewEslintService()
	bundle.Eslint = eslintService
	// Step 8: 注入 GitService 捕获 Git 状态。
	snapshotService.SetGitService(gitService)
	// Step 3: 注入 SnapshotService 到 Plan/Goal/Diff，使每步骤前/检查点/Apply 前
	// best-effort 创建快照。workspaceRoot 为空时为 no-op；项目打开后由前端
	// 通过 bindings 设置工作区根（setSnapshotWorkspaceRoot）激活手动触发。
	aiPlanService.SetSnapshotService(snapshotService, "")
	aiGoalService.SetSnapshotService(snapshotService, "")
	diffService.SetSnapshotService(snapshotService, "")
	// 注入内部 executor/checker：前端通过 Wails bindings 调用 RunGoal/ResumeGoal/
	// ExecuteStep 时无法传递 Go 接口实例，回退到这些用 AgentService 实现的适配器。
	// workspaceRoot 为空（项目打开后由前端设置）；此处注入保证 executor 非 nil。
	aiPlanService.SetInternalExecutor(services.NewDefaultStepExecutor(agentService, ""))
	aiGoalService.SetInternalExecutor(
		services.NewDefaultGoalExecutor(agentService, ""),
		services.NewDefaultSecurityChecker(agentService, ""),
	)
	bundle.InstanceLock = instanceLock

	app := application.New(application.Options{
		Name:        "gugacode",
		Description: "AI-Powered Code Editor",
		// prompt-5 Task I: service list lives in bootstrap_services.go
		Services: bundle.wailsServices(),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
			// Plan 58 / N-21: intercept /_plugins/<name>/<path> requests and
			// serve them from PluginService.ServePluginAsset. Wails v3
			// alpha2.111 has no public API for registering custom URL
			// schemes (nknk-plugin://), so we route plugin assets under
			// the existing asset handler's scheme via a path prefix.
			//
			// Plan 66 / N-14: the middleware also injects security headers
			// (Content-Security-Policy, X-Content-Type-Options,
			// X-Frame-Options, Referrer-Policy) on every response. The CSP
			// restricts connect-src to 'self' because all AI/network calls
			// are made from Go, not from the webview. Plugin assets served
			// from /_plugins/ are covered by the same origin.
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasPrefix(r.URL.Path, "/_plugins/") {
						// Plugin assets are JS/CSS/JSON — never HTML.
						// Apply the static CSP (no 'unsafe-inline' for
						// script-src) and serve directly.
						applySecurityHeaders(w)
						servePluginAsset(w, r, pluginService)
						return
					}
					// N-34: For HTML responses, generate a per-request
					// nonce, inject it into <script> tags, and set the
					// CSP header with 'nonce-<N>' instead of 'unsafe-inline'.
					// For non-HTML responses, apply the static CSP.
					rec := &responseRecorder{ResponseWriter: w, buf: &bytes.Buffer{}, statusCode: http.StatusOK}
					next.ServeHTTP(rec, r)

					// prompt-6 Task 6 / BUG-M10: preserve the real status from
					// the downstream AssetServer (do not force 200).
					status := rec.statusCode
					if status == 0 {
						status = http.StatusOK
					}

					ct := rec.Header().Get("Content-Type")
					if strings.HasPrefix(ct, "text/html") {
						// G-SEC-10: refuse to serve the page if CSP nonce
						// generation fails — a predictable/empty nonce would
						// weaken script-src and let an attacker inject scripts.
						nonce, err := generateNonce()
						if err != nil {
							slog.Error("CSP nonce generation failed", "err", err)
							http.Error(w, "internal server error", http.StatusInternalServerError)
							return
						}
						body := injectNonceIntoHTML(rec.buf.Bytes(), nonce)
					csp := fmt.Sprintf(contentSecurityPolicyWithNonce, nonce)
					w.Header().Set("Content-Security-Policy", csp)
					w.Header().Set("X-Content-Type-Options", "nosniff")
					w.Header().Set("X-Frame-Options", "DENY")
					w.Header().Set("Referrer-Policy", "no-referrer")
					w.Header().Set("Content-Length", strconv.Itoa(len(body)))
					w.WriteHeader(status)
					_, _ = w.Write(body)
					return
					}
					// Non-HTML response: apply static CSP and copy the
					// buffered body through unchanged with real status.
					applySecurityHeaders(w)
					w.WriteHeader(status)
					_, _ = w.Write(rec.buf.Bytes())
				})
			},
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   "main",
		Title:  "gugacode",
		Width:  1000,
		Height: 618,
		// Frameless 移除原生标题栏与边框，使用前端的 TitleBar.vue 作为自定义标题栏。
		// Windows 上保留 FramelessWindowDecorations（阴影 + Win11 圆角 + resize）。
		// macOS 上配合 MacTitleBarHiddenInset 实现隐藏标题栏但保留交通灯按钮。
		Frameless: true,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		Windows: application.WindowsWindow{
			// false = 保留 Aero 阴影、Win11 圆角和原生 resize 八个把手
			DisableFramelessWindowDecorations: false,
		},
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	// prompt-4 Task 1: WindowService 管理主窗口 + AI 独立窗口。
	windowService.SetApp(app)
	windowService.SetWindow(window)

	// N-152: 监听窗口最大化/还原事件，向前端推送布尔状态。
	// 前端标题栏据此切换放大 ↔ 还原图标，无需轮询 IsMaximised。
	// 同时监听 WindowRestore 和 WindowUnMaximise：不同平台在还原时
	// 发出的事件不同（Windows 倾向 WindowRestore，部分场景 WindowUnMaximise）。
	window.OnWindowEvent(events.Common.WindowMaximise, func(_ *application.WindowEvent) {
		app.Event.Emit("window:maximised", true)
	})
	window.OnWindowEvent(events.Common.WindowRestore, func(_ *application.WindowEvent) {
		app.Event.Emit("window:maximised", false)
	})
	window.OnWindowEvent(events.Common.WindowUnMaximise, func(_ *application.WindowEvent) {
		app.Event.Emit("window:maximised", false)
	})

	// prompt-4 Task 1/6: 关闭主窗口时联动关闭 AI 窗口。
	window.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		windowService.CloseAIWindow()
	})

	// prompt-5 Task C / BUG-L6: 仅在设置 openAIWindowOnStartup=true 时启动即开 AI 窗。
	// 默认 false；用户关闭后仍可通过 OpenAIWindow / ToggleAIWindow 打开。
	if settings, err := settingsService.LoadSettings(); err == nil && settings.OpenAIWindowOnStartup {
		windowService.OpenAIWindow()
	}

	// Link AI service to app for event-based streaming
	aiService.SetApp(app)
	// G-SEC-07: give AIService access to SettingsService so it can fetch
	// stored API keys without the key ever crossing the Wails binding.
	aiService.SetSettingsService(settingsService)
	// Plan 11 Task 12 Step 3: inject AIPermissionService so AIService can
	// resolve per-operation model assignments via ResolveModelFor.
	aiService.SetPermissionService(aiPermissionService)
	aiPermissionService.SetSettingsService(settingsService)

	// Link terminal service to app for event-based output emission
	terminalService.SetApp(app)

	// Link file service to app so WriteFile can emit "file:saved" events
	// for workflow triggers (Plan 65 / Proposal B).
	fileService.SetApp(app)

	go func() {
		for {
			now := time.Now().Format(time.RFC1123)
			app.Event.Emit("time", now)
			time.Sleep(time.Second)
		}
	}()

	err := app.Run()
	// N-95 / Proposal AC: clean up terminal readLoop goroutines on shutdown
	// so the process exits cleanly without leaking goroutines blocked on
	// PTY reads. Safe to call multiple times.
	terminalService.Shutdown()
	// N-103: close the agent audit log file handle so it doesn't leak.
	agentService.Close()
	// Plan 11 Task 4: stop all running MCP server connections.
	if mcpErr := mcpService.Close(); mcpErr != nil {
		log.Printf("mcp close: %v", mcpErr)
	}
	// G-FEAT-02: stop any running LSP server processes on shutdown.
	lspService.StopAll()
	// G-QUAL-05: release the single-instance lock so the next gugacode
	// launch can acquire it. Must run before log.Fatal, which calls
	// os.Exit and skips deferred functions.
	instanceLock.Release()
	if err != nil {
		log.Fatal(err)
	}
}

// pluginAssetPathPrefix is the URL path prefix for plugin asset requests.
const pluginAssetPathPrefix = "/_plugins/"

// contentSecurityPolicyWithNonce is the CSP template applied to HTML
// responses (Plan 66 / N-14, N-34). Each HTML response gets a fresh
// per-request nonce that is injected into inline <script> tags and the
// CSP header's script-src directive, replacing 'unsafe-inline'.
//
// The policy is intentionally strict:
//   - default-src 'self'        — base default, restricted to same origin
//   - script-src 'self' 'nonce-<N>' blob: — Vite-built scripts, nonce-tagged
//     inline scripts, and blob: workers (Monaco)
//   - style-src 'self' 'unsafe-inline' — Vue scoped styles and theme CSS
//     (style-src keeps 'unsafe-inline' because Vue's scoped-style runtime
//     injects <style> tags dynamically; nonceing styles is invasive and
//     not the security win that nonceing scripts is)
//   - img-src 'self' data: blob: — data-URI icons and generated previews
//   - font-src 'self' data: — embedded fonts
//   - connect-src 'self' — same-origin only; AI/network calls go through Go
//   - worker-src 'self' blob: — Monaco and other web workers
//   - frame-ancestors 'none' — disallow embedding the app in any frame
//
// N-34 (prompt-4.md): replaces the previous 'unsafe-inline' for script-src
// with a per-request nonce. The nonce is generated fresh on every HTML
// response, so an attacker who learns one nonce cannot reuse it on a
// subsequent request. Non-HTML responses (JS/CSS/assets) keep the static
// CSP without the nonce, since they don't contain inline scripts.
const contentSecurityPolicyWithNonce = "default-src 'self'; " +
	"script-src 'self' 'nonce-%s' blob:; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"worker-src 'self' blob:; " +
	"frame-ancestors 'none'"

// contentSecurityPolicyStatic is the CSP applied to non-HTML responses
// (JS/CSS/assets/plugin assets). These responses don't contain inline
// scripts, so they don't need the nonce. 'unsafe-inline' is omitted from
// script-src entirely — only 'self' and blob: are allowed.
const contentSecurityPolicyStatic = "default-src 'self'; " +
	"script-src 'self' blob:; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"worker-src 'self' blob:; " +
	"frame-ancestors 'none'"

// applySecurityHeaders injects security-related HTTP headers on the
// response. It is called by the asset middleware for every request,
// including plugin asset requests. The headers tighten the webview's
// security posture (Plan 66 / N-14):
//   - Content-Security-Policy: restricts resource loading to same origin
//   - X-Content-Type-Options: nosniff — prevents MIME-type sniffing
//   - X-Frame-Options: DENY — legacy frame-embedding guard (CSP
//     frame-ancestors is the modern equivalent, but X-Frame-Options is
//     kept for older webviews)
//   - Referrer-Policy: no-referrer — never leak the app's origin/path
func applySecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", contentSecurityPolicyStatic)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// generateNonce returns a fresh 16-byte hex-encoded random nonce for use
// in CSP script-src 'nonce-<N>' and inline <script nonce="<N>"> tags.
// Each HTML response gets its own nonce; nonces are not reused across
// requests so an attacker who learns one cannot inject scripts later.
//
// G-SEC-10: If crypto/rand.Read fails we return an error instead of
// falling back to a predictable time-derived nonce. A predictable nonce
// defeats the purpose of CSP, so the caller must refuse to serve the
// page (HTTP 500) rather than ship a weak nonce.
//
// N-34 (prompt-4.md).
func generateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate CSP nonce: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// scriptTagPattern matches <script ...> opening tags so we can inject a
// nonce attribute. Handles <script>, <script type="module">, and
// <script src="...">. Captures the tag's attributes in group 1.
var scriptTagPattern = regexp.MustCompile(`<script(\s[^>]*)?>`)

// injectNonceIntoHTML replaces the CSP nonce placeholder in the given HTML
// body and adds a nonce attribute to every <script> tag. This is the core
// of N-34: it lets us drop 'unsafe-inline' from script-src in production.
//
// The function is applied to HTML responses only (content-type text/html).
// JS/CSS/asset responses are passed through unchanged.
func injectNonceIntoHTML(body []byte, nonce string) []byte {
	// Add nonce="..." to every <script> tag that doesn't already have one.
	injected := scriptTagPattern.ReplaceAllFunc(body, func(match []byte) []byte {
		// If the tag already has a nonce attribute, leave it alone.
		if bytes.Contains(match, []byte("nonce=")) {
			return match
		}
		// Insert nonce after "<script".
		insert := ` nonce="` + nonce + `"`
		// Find the position right after "<script".
		insertPos := 7 // len("<script")
		result := make([]byte, 0, len(match)+len(insert))
		result = append(result, match[:insertPos]...)
		result = append(result, []byte(insert)...)
		result = append(result, match[insertPos:]...)
		return result
	})
	return injected
}

// responseRecorder is an http.ResponseWriter that buffers the response body
// in memory so the middleware can post-process it (e.g. inject CSP nonces
// into HTML). Write() appends to the buffer; WriteHeader records the status
// without forwarding (middleware writes the final response).
//
// N-34 (prompt-4.md): the middleware reads the buffer after the downstream
// handler finishes, decides whether the response is HTML, and either
// injects the nonce + nonce-CSP (HTML) or applies the static CSP (other).
//
// prompt-6 Task 6 / BUG-M10: statusCode is preserved and written through
// so AssetServer 404/500 is not rewritten as 200.
type responseRecorder struct {
	http.ResponseWriter
	buf           *bytes.Buffer
	statusCode    int
	headerWritten bool
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.buf.Write(b)
}

// WriteHeader records the downstream status without forwarding to the
// underlying ResponseWriter. Middleware later emits the real status.
func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.headerWritten = true
}

func (r *responseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// servePluginAsset handles /_plugins/<plugin-name>/<rel-path> requests by
// delegating to PluginService.ServePluginAsset. It reads the current
// project root from the appState (forwarded as a query parameter by the
// frontend) so project-scoped plugins can be resolved.
//
// Plan 58 / N-21: This is the runtime side of the plugin protocol handler.
// Wails v3 alpha2.111 does not expose a public API for registering custom
// URL schemes, so we intercept requests on the existing asset handler's
// scheme (http://wails.localhost on Windows, wails://localhost on
// macOS/Linux) via AssetOptions.Middleware.
func servePluginAsset(w http.ResponseWriter, r *http.Request, svc *services.PluginService) {
	// Strip the prefix to get "<plugin-name>/<rel-path>".
	rest := strings.TrimPrefix(r.URL.Path, pluginAssetPathPrefix)
	if rest == "" {
		http.Error(w, "plugin name is required", http.StatusBadRequest)
		return
	}
	// Split into plugin name and relative path. The first path segment
	// is the plugin name; the rest is the relative path within the plugin
	// directory. We use strings.SplitN to handle rel-paths that contain
	// "/" separators.
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "plugin name is required", http.StatusBadRequest)
		return
	}
	pluginName := parts[0]
	if len(parts) < 2 || parts[1] == "" {
		http.Error(w, "file path is required", http.StatusBadRequest)
		return
	}
	relPath := parts[1]
	// The project root is forwarded by the frontend as a query parameter
	// so project-scoped plugins can be resolved. Empty means user-global only.
	projectRoot := r.URL.Query().Get("projectRoot")
	data, mime, err := svc.ServePluginAsset(pluginName, relPath, projectRoot)
	if err != nil {
		slog.Error("plugin asset serve failed",
			"plugin", pluginName,
			"path", relPath,
			"error", err)
		http.Error(w, "plugin asset not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", mime)
	// Allow dynamic import() of plugin scripts. Without
	// Cross-Origin-Resource-Policy, some webview versions block the
	// response from being used as a module.
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	w.Header().Set("Cache-Control", "no-cache")
	if _, err := w.Write(data); err != nil {
		slog.Warn("plugin asset write failed", "error", err)
	}
}
