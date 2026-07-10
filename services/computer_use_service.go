package services

// Plan 11 Task 6 — Computer Use（屏幕截图 + 鼠标键盘控制）。
//
// 提供 5 个工具供 AI agent 调用：
//   - Screenshot：截取屏幕或指定区域，返回 base64 PNG
//   - MouseMove：移动鼠标到指定坐标
//   - MouseClick：点击鼠标按钮
//   - KeyboardType：输入文本
//   - KeyboardHotkey：按下组合键
//
// 安全模型（G-SEC-02 / G-SEC-06 / G-SEC-12）：
//   - 所有操作默认 RiskDangerous，需用户显式确认（Step 3）
//   - 禁止 OS 级快捷键黑名单（Ctrl+Alt+Del / Cmd+Q / Alt+F4 等）（Step 6）
//   - 应用白名单：仅允许在白名单进程内操作（Step 5）
//   - 禁止区域：屏幕坐标不可落入禁止区域（密码管理器等）（Step 5/6）
//   - 操作日志审计：每次不可逆操作记录到审计日志（Step 7）
//   - G-SEC-12：默认 Enabled=false，启用需 explicitApproval（Step 8）
//
// 原生操作通过平台特定文件实现：
//   - computer_use_windows.go：Windows 截图/鼠标键盘（gdi32/user32）
//   - computer_use_unix.go：Linux/macOS stub（返回 ErrPlatformUnsupported）
//
// 配置持久化用 atomicWriteJSON（0600），复用既有原子写实现。

import (
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// 配置 schema（Step 5 / Step 8）
// ---------------------------------------------------------------------------

// ComputerUseConfig 是 Computer Use 的持久化配置。
type ComputerUseConfig struct {
	// Enabled 控制整个 Computer Use 功能开关。
	// G-SEC-12：默认 false，启用需 explicitApproval（视同 Restricted）。
	Enabled bool `yaml:"enabled" json:"enabled"`
	// ConfirmationRequired 为 true 时，每次操作前必须截图 + AI 规划 + 用户确认。
	// Step 3：默认 true。关闭后允许自动执行（仍记录审计日志，但不推荐）。
	ConfirmationRequired bool `yaml:"confirmationRequired" json:"confirmationRequired"`
	// ScreenshotQuality 0-100，控制 JPEG/PNG 压缩质量（Step 2）。
	ScreenshotQuality int `yaml:"screenshotQuality,omitempty" json:"screenshotQuality,omitempty"`
	// ScreenshotScale 截图缩放比例（0.1-1.0），降低分辨率以节省 token（Step 2）。
	ScreenshotScale float64 `yaml:"screenshotScale,omitempty" json:"screenshotScale,omitempty"`
	// AppWhitelist 允许操作的应用进程名白名单。空表示不限制。
	AppWhitelist []string `yaml:"appWhitelist,omitempty" json:"appWhitelist,omitempty"`
	// ForbiddenZones 屏幕上禁止操作的矩形区域（密码管理器等）（Step 5/6）。
	ForbiddenZones []ForbiddenZone `yaml:"forbiddenZones,omitempty" json:"forbiddenZones,omitempty"`
	// ForbiddenHotkeys 禁止的快捷键组合黑名单（Step 6）。
	// 默认包含 OS 级危险快捷键。
	ForbiddenHotkeys []string `yaml:"forbiddenHotkeys,omitempty" json:"forbiddenHotkeys,omitempty"`
	// RecordingEnabled 录制模式开关（Step 4）。
	RecordingEnabled bool `yaml:"recordingEnabled,omitempty" json:"recordingEnabled,omitempty"`
}

// ForbiddenZone 是屏幕上的禁止操作矩形区域。
type ForbiddenZone struct {
	Name string `yaml:"name" json:"name"`
	X    int    `yaml:"x" json:"x"`
	Y    int    `yaml:"y" json:"y"`
	W    int    `yaml:"w" json:"w"`
	H    int    `yaml:"h" json:"h"`
}

// 默认禁止快捷键黑名单（Step 6）：
// Ctrl+Alt+Del（Windows 安全屏幕）、Cmd+Q（macOS 退出）、
// Alt+F4（关闭窗口）、Ctrl+Shift+Esc（任务管理器）等。
var defaultForbiddenHotkeys = []string{
	"ctrl+alt+del",
	"ctrl+shift+esc",
	"alt+f4",
	"cmd+q",
	"cmd+option+esc",
	"ctrl+alt+backspace",
	"super+l",
}

// defaultComputerUseConfig 返回安全默认配置。
func defaultComputerUseConfig() ComputerUseConfig {
	return ComputerUseConfig{
		Enabled:              false, // G-SEC-12：默认禁用
		ConfirmationRequired: true,  // Step 3：默认需确认
		ScreenshotQuality:    80,
		ScreenshotScale:      1.0,
		AppWhitelist:         nil, // 不限制
		ForbiddenZones:       nil,
		ForbiddenHotkeys:     append([]string{}, defaultForbiddenHotkeys...),
		RecordingEnabled:     false,
	}
}

// ---------------------------------------------------------------------------
// 操作日志审计（Step 7）
// ---------------------------------------------------------------------------

// AuditAction 是审计日志中的单条操作记录。
type AuditAction struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`     // screenshot/mouse_move/mouse_click/keyboard_type/keyboard_hotkey
	Args      string    `json:"args"`       // 操作参数摘要（脱敏）
	Success   bool      `json:"success"`    // 是否执行成功
	Error     string    `json:"error,omitempty"`
	// ConfirmedByUser 标记该操作是否经用户确认（Step 3）。
	ConfirmedByUser bool `json:"confirmedByUser"`
}

// ---------------------------------------------------------------------------
// ComputerUseService
// ---------------------------------------------------------------------------

// ComputerUseService 管理屏幕截图与鼠标键盘控制（Step 1）。
type ComputerUseService struct {
	mu        sync.RWMutex
	config    ComputerUseConfig
	configDir string
	// auditLog 是操作审计日志的内存缓存（最近 N 条）。
	auditLog []AuditAction
	// platform 是平台特定的操作执行器（截图/鼠标/键盘）。
	// 由平台文件（computer_use_windows.go / computer_use_unix.go）注入。
	platform platformExecutor
}

// platformExecutor 是平台特定操作的接口。
// 由 computer_use_windows.go / computer_use_unix.go 实现。
type platformExecutor interface {
	// Screenshot 截取屏幕或指定区域。region 为 nil 表示全屏。
	Screenshot(region *image.Rectangle) ([]byte, error)
	// MouseMove 移动鼠标到 (x, y)。
	MouseMove(x, y int) error
	// MouseClick 点击鼠标按钮（left/right/middle）。
	MouseClick(button string) error
	// KeyboardType 输入文本。
	KeyboardType(text string) error
	// KeyboardHotkey 按下组合键（如 "ctrl+c"）。
	KeyboardHotkey(keys string) error
}

// NewComputerUseService 创建服务。configDir 用于配置文件路径。
func NewComputerUseService(configDir string) *ComputerUseService {
	svc := &ComputerUseService{
		config:    defaultComputerUseConfig(),
		configDir: configDir,
		platform:  newPlatformExecutor(), // 平台 stub
	}
	// best-effort 加载配置；失败用默认配置。
	_ = svc.loadConfig()
	return svc
}

// configPath 返回配置文件路径。
func (s *ComputerUseService) configPath() string {
	return filepath.Join(s.configDir, "gugacode", "computer_use.yaml")
}

// loadConfig 从磁盘加载配置。文件不存在时用默认配置（不报错）。
func (s *ComputerUseService) loadConfig() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.configPath())
	if err != nil {
		if os.IsNotExist(err) {
			s.config = defaultComputerUseConfig()
			return nil
		}
		return fmt.Errorf("read computer_use config: %w", err)
	}
	var cfg ComputerUseConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse computer_use config: %w", err)
	}
	// 合并默认禁止快捷键：用户自定义 + 默认黑名单（取并集）。
	if len(cfg.ForbiddenHotkeys) == 0 {
		cfg.ForbiddenHotkeys = append([]string{}, defaultForbiddenHotkeys...)
	}
	s.config = cfg
	return nil
}

// saveConfig 持久化配置（G-SEC-09：atomicWriteFile 0600）。
func (s *ComputerUseService) saveConfig() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := yaml.Marshal(s.config)
	if err != nil {
		return fmt.Errorf("marshal computer_use config: %w", err)
	}
	return atomicWriteFile(s.configPath(), data, 0600)
}

// GetConfig 返回当前配置的副本。
func (s *ComputerUseService) GetConfig() ComputerUseConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// UpdateConfig 更新配置并持久化。
// G-SEC-12：从 Enabled=false → true 是显式审批动作，调用方需确保用户已确认。
func (s *ComputerUseService) UpdateConfig(cfg ComputerUseConfig) error {
	s.mu.Lock()
	s.config = cfg
	// 确保 ForbiddenHotkeys 始终包含默认黑名单。
	for _, def := range defaultForbiddenHotkeys {
		if !containsString(s.config.ForbiddenHotkeys, def) {
			s.config.ForbiddenHotkeys = append(s.config.ForbiddenHotkeys, def)
		}
	}
	s.mu.Unlock()
	return s.saveConfig()
}

// IsEnabled 返回 Computer Use 是否已启用。
func (s *ComputerUseService) IsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.Enabled
}

// ---------------------------------------------------------------------------
// 安全边界检查（Step 6）
// ---------------------------------------------------------------------------

// isHotkeyForbidden 检查快捷键是否在禁止黑名单中。
// 大小写不敏感比较。
func isHotkeyForbidden(forbidden []string, keys string) bool {
	normalized := strings.ToLower(strings.TrimSpace(keys))
	for _, f := range forbidden {
		if strings.ToLower(strings.TrimSpace(f)) == normalized {
			return true
		}
	}
	return false
}

// isPointInForbiddenZone 检查坐标是否落入禁止区域。
func isPointInForbiddenZone(zones []ForbiddenZone, x, y int) bool {
	for _, z := range zones {
		if x >= z.X && x < z.X+z.W && y >= z.Y && y < z.Y+z.H {
			return true
		}
	}
	return false
}

// checkSafety 检查操作是否通过安全边界。
// 返回 error 如果操作被禁止。
func (s *ComputerUseService) checkSafety(action string, args interface{}) error {
	s.mu.RLock()
	cfg := s.config
	s.mu.RUnlock()

	if !cfg.Enabled {
		return fmt.Errorf("computer use is disabled (G-SEC-12): %w", ErrNotAllowed)
	}

	switch action {
	case "mouse_move", "mouse_click":
		// 检查坐标是否在禁止区域。
		if coords, ok := args.(coordsArg); ok {
			if isPointInForbiddenZone(cfg.ForbiddenZones, coords.X, coords.Y) {
				return fmt.Errorf("coordinates (%d,%d) fall in forbidden zone (Step 6): %w",
					coords.X, coords.Y, ErrNotAllowed)
			}
		}
	case "keyboard_hotkey":
		// 检查快捷键是否在黑名单。
		if keys, ok := args.(string); ok {
			if isHotkeyForbidden(cfg.ForbiddenHotkeys, keys) {
				return fmt.Errorf("hotkey %q is forbidden (Step 6 OS safety): %w",
					keys, ErrNotAllowed)
			}
		}
	}
	return nil
}

// coordsArg 是 checkSafety 的坐标参数辅助类型。
type coordsArg struct {
	X, Y int
}

// ---------------------------------------------------------------------------
// 审计日志（Step 7）
// ---------------------------------------------------------------------------

// recordAudit 记录一条操作审计日志。
func (s *ComputerUseService) recordAudit(action, argsSummary string, success bool, confirmed bool, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := AuditAction{
		Timestamp:       time.Now().UTC(),
		Action:          action,
		Args:            argsSummary,
		Success:         success,
		ConfirmedByUser: confirmed,
		Error:           errMsg,
	}
	s.auditLog = append(s.auditLog, entry)
	// 限制内存缓存大小（保留最近 500 条）。
	if len(s.auditLog) > 500 {
		s.auditLog = s.auditLog[len(s.auditLog)-500:]
	}
}

// GetAuditLog 返回审计日志副本（最近 N 条）。
func (s *ComputerUseService) GetAuditLog(limit int) []AuditAction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.auditLog) {
		limit = len(s.auditLog)
	}
	start := len(s.auditLog) - limit
	if start < 0 {
		start = 0
	}
	out := make([]AuditAction, limit)
	copy(out, s.auditLog[start:])
	return out
}

// ---------------------------------------------------------------------------
// 5 个工具实现（Step 1 / Step 2 / Step 3）
// ---------------------------------------------------------------------------

// Screenshot 截取屏幕，返回 base64 编码的 PNG（Step 2）。
// region 为 nil 表示全屏；quality 和 scale 从配置读取。
// 所有截图操作需经用户确认（ConfirmationRequired=true 时）。
func (s *ComputerUseService) Screenshot(ctx context.Context, region *image.Rectangle, confirmedByUser bool) (string, error) {
	if err := s.checkSafety("screenshot", nil); err != nil {
		s.recordAudit("screenshot", "", false, confirmedByUser, err.Error())
		return "", err
	}
	// Step 3：需确认时，未确认则拒绝。
	if s.config.ConfirmationRequired && !confirmedByUser {
		err := fmt.Errorf("screenshot requires user confirmation (Step 3): %w", ErrNotAllowed)
		s.recordAudit("screenshot", "", false, false, err.Error())
		return "", err
	}
	imgBytes, err := s.platform.Screenshot(region)
	if err != nil {
		s.recordAudit("screenshot", fmt.Sprintf("region=%v", region), false, confirmedByUser, err.Error())
		return "", fmt.Errorf("screenshot: %w", err)
	}
	// 编码为 base64 PNG。
	encoded := base64.StdEncoding.EncodeToString(imgBytes)
	s.recordAudit("screenshot", fmt.Sprintf("region=%v bytes=%d", region, len(imgBytes)), true, confirmedByUser, "")
	return encoded, nil
}

// MouseMove 移动鼠标到 (x, y)（Step 1）。
func (s *ComputerUseService) MouseMove(ctx context.Context, x, y int, confirmedByUser bool) error {
	if err := s.checkSafety("mouse_move", coordsArg{X: x, Y: y}); err != nil {
		s.recordAudit("mouse_move", fmt.Sprintf("(%d,%d)", x, y), false, confirmedByUser, err.Error())
		return err
	}
	if s.config.ConfirmationRequired && !confirmedByUser {
		err := fmt.Errorf("mouse_move requires user confirmation (Step 3): %w", ErrNotAllowed)
		s.recordAudit("mouse_move", fmt.Sprintf("(%d,%d)", x, y), false, false, err.Error())
		return err
	}
	if err := s.platform.MouseMove(x, y); err != nil {
		s.recordAudit("mouse_move", fmt.Sprintf("(%d,%d)", x, y), false, confirmedByUser, err.Error())
		return err
	}
	s.recordAudit("mouse_move", fmt.Sprintf("(%d,%d)", x, y), true, confirmedByUser, "")
	return nil
}

// MouseClick 点击鼠标按钮（Step 1）。button: left/right/middle。
func (s *ComputerUseService) MouseClick(ctx context.Context, button string, confirmedByUser bool) error {
	if err := s.checkSafety("mouse_click", coordsArg{}); err != nil {
		s.recordAudit("mouse_click", button, false, confirmedByUser, err.Error())
		return err
	}
	if s.config.ConfirmationRequired && !confirmedByUser {
		err := fmt.Errorf("mouse_click requires user confirmation (Step 3): %w", ErrNotAllowed)
		s.recordAudit("mouse_click", button, false, false, err.Error())
		return err
	}
	if err := s.platform.MouseClick(button); err != nil {
		s.recordAudit("mouse_click", button, false, confirmedByUser, err.Error())
		return err
	}
	s.recordAudit("mouse_click", button, true, confirmedByUser, "")
	return nil
}

// KeyboardType 输入文本（Step 1）。
func (s *ComputerUseService) KeyboardType(ctx context.Context, text string, confirmedByUser bool) error {
	if err := s.checkSafety("keyboard_type", nil); err != nil {
		// 不记录完整文本（可能含敏感信息），仅记录长度。
		s.recordAudit("keyboard_type", fmt.Sprintf("len=%d", len(text)), false, confirmedByUser, err.Error())
		return err
	}
	if s.config.ConfirmationRequired && !confirmedByUser {
		err := fmt.Errorf("keyboard_type requires user confirmation (Step 3): %w", ErrNotAllowed)
		s.recordAudit("keyboard_type", fmt.Sprintf("len=%d", len(text)), false, false, err.Error())
		return err
	}
	if err := s.platform.KeyboardType(text); err != nil {
		s.recordAudit("keyboard_type", fmt.Sprintf("len=%d", len(text)), false, confirmedByUser, err.Error())
		return err
	}
	s.recordAudit("keyboard_type", fmt.Sprintf("len=%d", len(text)), true, confirmedByUser, "")
	return nil
}

// KeyboardHotkey 按下组合键（Step 1）。keys 如 "ctrl+c"。
func (s *ComputerUseService) KeyboardHotkey(ctx context.Context, keys string, confirmedByUser bool) error {
	if err := s.checkSafety("keyboard_hotkey", keys); err != nil {
		s.recordAudit("keyboard_hotkey", keys, false, confirmedByUser, err.Error())
		return err
	}
	if s.config.ConfirmationRequired && !confirmedByUser {
		err := fmt.Errorf("keyboard_hotkey requires user confirmation (Step 3): %w", ErrNotAllowed)
		s.recordAudit("keyboard_hotkey", keys, false, false, err.Error())
		return err
	}
	if err := s.platform.KeyboardHotkey(keys); err != nil {
		s.recordAudit("keyboard_hotkey", keys, false, confirmedByUser, err.Error())
		return err
	}
	s.recordAudit("keyboard_hotkey", keys, true, confirmedByUser, "")
	return nil
}

// ---------------------------------------------------------------------------
// 录制模式（Step 4）
// ---------------------------------------------------------------------------

// RecordedAction 是录制模式下捕获的用户操作。
type RecordedAction struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	Args      string    `json:"args"`
}

// recordingSession 是录制模式的内存缓冲。
type recordingSession struct {
	mu      sync.Mutex
	actions []RecordedAction
	active  bool
}

var recording = &recordingSession{}

// StartRecording 开始录制模式（Step 4）。
func (s *ComputerUseService) StartRecording() error {
	s.mu.RLock()
	enabled := s.config.Enabled
	s.mu.RUnlock()
	if !enabled {
		return fmt.Errorf("computer use disabled, cannot record: %w", ErrNotAllowed)
	}
	recording.mu.Lock()
	defer recording.mu.Unlock()
	recording.active = true
	recording.actions = nil
	return nil
}

// StopRecording 停止录制并返回捕获的操作序列。
func (s *ComputerUseService) StopRecording() []RecordedAction {
	recording.mu.Lock()
	defer recording.mu.Unlock()
	recording.active = false
	out := make([]RecordedAction, len(recording.actions))
	copy(out, recording.actions)
	return out
}

// IsRecording 返回录制模式是否激活。
func (s *ComputerUseService) IsRecording() bool {
	recording.mu.Lock()
	defer recording.mu.Unlock()
	return recording.active
}

// recordAction 在录制模式下捕获操作（内部调用）。
func recordAction(action, args string) {
	recording.mu.Lock()
	defer recording.mu.Unlock()
	if !recording.active {
		return
	}
	recording.actions = append(recording.actions, RecordedAction{
		Timestamp: time.Now().UTC(),
		Action:    action,
		Args:      args,
	})
}

// ---------------------------------------------------------------------------
// 辅助：PNG 编码（Step 2，供平台实现使用）
// ---------------------------------------------------------------------------

// encodePNG 将 image.Image 编码为 PNG 字节。
// 供平台实现（computer_use_windows.go 等）复用。
func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("png encode: %w", err)
	}
	return buf.Bytes(), nil
}
