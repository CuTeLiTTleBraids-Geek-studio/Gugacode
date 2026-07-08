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
	application.RegisterEvent[string]("ai:chunk")
	application.RegisterEvent[string]("ai:done")
	application.RegisterEvent[string]("ai:error")
	// Plan 65 / Proposal B: file:saved is emitted by FileService.WriteFile
	// so the frontend can trigger workflows with matching runOn triggers.
	application.RegisterEvent[string]("file:saved")
	// N-152: 前端标题栏根据 window:maximised 事件切换放大/还原图标。
	application.RegisterEvent[bool]("window:maximised")
}

func main() {
	// Initialize structured logging (N-11) before any service is created
	// so all services inherit the configured default logger. The cleanup
	// function closes the log file on shutdown.
	closeLogger := services.InitLogger(slog.LevelInfo)
	defer closeLogger()

	configDir, _ := os.UserConfigDir()
	fileService := &services.FileService{}
	terminalService := services.NewTerminalService()
	agentService := services.NewAgentService()
	aiService := services.NewAIService()
	// N-17: pass configDir so the user-global preset layer
	// (<configDir>/gugacode/presets/*.json) is active. Without this,
	// SaveUserPreset/ListPresetsWithSource would skip the user layer.
	presetService := services.NewPresetService(configDir)
	aiService.SetPresetService(presetService)
	projectService := services.NewProjectService(fileService, terminalService, agentService, aiService)
	// Plan 50: ProfileService manages multi-profile directory structure.
	// Created before SettingsService so the active profile's settings
	// path can be passed to the SettingsService constructor.
	profileService := services.NewProfileService(configDir)
	activeSettingsPath, perr := profileService.ActiveSettingsPath()
	if perr != nil || activeSettingsPath == "" {
		activeSettingsPath = filepath.Join(configDir, "gugacode", "settings.json")
	}
	settingsService := services.NewSettingsServiceWithPath(activeSettingsPath)
	// Plan 72 / N-25: LayoutService stores the layout tree JSON in
	// layout.json in the same profile directory as settings.json.
	activeLayoutPath := filepath.Join(filepath.Dir(activeSettingsPath), "layout.json")
	layoutService := services.NewLayoutServiceWithPath(activeLayoutPath)
	// Wire profile switch callback so SettingsService and LayoutService
	// are redirected to the new profile's directory on switch.
	profileService.SetOnSwitch(func(p string) {
		settingsService.SetConfigPath(p)
		layoutService.SetLayoutPath(filepath.Join(filepath.Dir(p), "layout.json"))
	})
	windowService := &services.WindowService{}
	gitService := &services.GitService{}
	searchService := &services.SearchService{}
	// N-67: wire GitService and SearchService into ProjectService so their
	// workspace roots are updated when a project is added. This sandbox
	// prevents the frontend from operating on paths outside the open project.
	projectService.SetGitService(gitService)
	projectService.SetSearchService(searchService)
	conversationService := services.NewConversationService("")
	taskService := services.NewTaskService()
	workflowService := services.NewWorkflowService()
	logLevelService := services.NewLogLevelService()
	rulesService := services.NewRulesService(configDir)
	// Plan 49: PluginService discovers user-global and project-scoped
	// plugins. Pass configDir so the user layer
	// (<configDir>/gugacode/plugins/<name>/) is active.
	pluginService := services.NewPluginService(configDir)

	app := application.New(application.Options{
		Name:        "gugacode",
		Description: "AI-Powered Code Editor",
		Services: []application.Service{
			application.NewService(fileService),
			application.NewService(projectService),
			application.NewService(settingsService),
			application.NewService(windowService),
			application.NewService(terminalService),
			application.NewService(aiService),
			application.NewService(gitService),
			application.NewService(searchService),
			application.NewService(conversationService),
			application.NewService(taskService),
			application.NewService(workflowService),
			application.NewService(agentService),
			application.NewService(rulesService),
			application.NewService(logLevelService),
			application.NewService(pluginService),
			application.NewService(profileService),
			application.NewService(layoutService),
		},
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
					rec := &responseRecorder{ResponseWriter: w, buf: &bytes.Buffer{}}
					next.ServeHTTP(rec, r)

					ct := rec.Header().Get("Content-Type")
					if strings.HasPrefix(ct, "text/html") {
						nonce := generateNonce()
						body := injectNonceIntoHTML(rec.buf.Bytes(), nonce)
						csp := fmt.Sprintf(contentSecurityPolicyWithNonce, nonce)
						w.Header().Set("Content-Security-Policy", csp)
						w.Header().Set("X-Content-Type-Options", "nosniff")
						w.Header().Set("X-Frame-Options", "DENY")
						w.Header().Set("Referrer-Policy", "no-referrer")
						w.Header().Set("Content-Length", strconv.Itoa(len(body)))
						_, _ = w.Write(body)
						return
					}
					// Non-HTML response: apply static CSP and copy the
					// buffered body through unchanged.
					applySecurityHeaders(w)
					_, _ = w.Write(rec.buf.Bytes())
				})
			},
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
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

	// Link AI service to app for event-based streaming
	aiService.SetApp(app)

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
// N-34 (prompt-4.md).
func generateNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// rand.Read should never fail on a healthy system, but if it
		// does we fall back to a time-based nonce rather than crashing.
		// This is still better than 'unsafe-inline' for script-src.
		now := time.Now().UnixNano()
		for i := range b {
			b[i] = byte(now >> (i % 8 * 8))
		}
	}
	return hex.EncodeToString(b)
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
// into HTML). Header() and WriteHeader() are forwarded to the underlying
// ResponseWriter, but Write() appends to the buffer instead.
//
// N-34 (prompt-4.md): the middleware reads the buffer after the downstream
// handler finishes, decides whether the response is HTML, and either
// injects the nonce + nonce-CSP (HTML) or applies the static CSP (other).
type responseRecorder struct {
	http.ResponseWriter
	buf *bytes.Buffer
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.buf.Write(b)
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
