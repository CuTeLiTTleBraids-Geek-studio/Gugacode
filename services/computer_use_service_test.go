package services

// Plan 11 Task 6 Step 9 — Computer Use service tests.
//
// 覆盖：
//   - 配置加载/保存（默认值 + G-SEC-12 默认禁用）
//   - 安全边界：禁止快捷键黑名单（Step 6）
//   - 安全边界：坐标落入禁止区域（Step 5/6）
//   - 审计日志记录（Step 7）
//   - ConfirmationRequired 强制确认（Step 3）
//   - 录制模式 start/stop（Step 4）
//   - 平台 stub 返回 ErrPlatformUnsupported

import (
	"context"
	"errors"
	"image"
	"path/filepath"
	"strings"
	"testing"
)

func newTestComputerUseService(t *testing.T) *ComputerUseService {
	t.Helper()
	dir := t.TempDir()
	svc := NewComputerUseService(dir)
	return svc
}

// --- Step 1/2: 配置与默认值 ---

func TestComputerUseService_DefaultConfig(t *testing.T) {
	svc := newTestComputerUseService(t)
	cfg := svc.GetConfig()
	// G-SEC-12：默认禁用。
	if cfg.Enabled {
		t.Error("default Enabled should be false (G-SEC-12)")
	}
	// Step 3：默认需确认。
	if !cfg.ConfirmationRequired {
		t.Error("default ConfirmationRequired should be true")
	}
	// Step 6：默认禁止快捷键黑名单应包含 OS 级危险快捷键。
	required := []string{"ctrl+alt+del", "alt+f4", "cmd+q"}
	for _, r := range required {
		if !containsString(cfg.ForbiddenHotkeys, r) {
			t.Errorf("default ForbiddenHotkeys should contain %q, got %v", r, cfg.ForbiddenHotkeys)
		}
	}
}

func TestComputerUseService_UpdateConfig_PersistsForbiddenHotkeys(t *testing.T) {
	svc := newTestComputerUseService(t)
	// 用户自定义快捷键，不应丢失默认黑名单。
	err := svc.UpdateConfig(ComputerUseConfig{
		Enabled:              true,
		ConfirmationRequired: false,
		ForbiddenHotkeys:     []string{"ctrl+c"}, // 自定义
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := svc.GetConfig()
	// 默认黑名单应被合并保留。
	if !containsString(cfg.ForbiddenHotkeys, "ctrl+alt+del") {
		t.Error("default forbidden hotkeys should be preserved after UpdateConfig")
	}
	if !containsString(cfg.ForbiddenHotkeys, "ctrl+c") {
		t.Error("user-defined hotkey should be preserved")
	}
	// 重新加载验证持久化。
	svc2 := NewComputerUseService(svc.configDir)
	cfg2 := svc2.GetConfig()
	if !cfg2.Enabled {
		t.Error("Enabled should persist across reloads")
	}
	if !containsString(cfg2.ForbiddenHotkeys, "ctrl+c") {
		t.Error("user hotkey should persist")
	}
}

// --- Step 6: 安全边界 — 禁止快捷键 ---

func TestIsHotkeyForbidden(t *testing.T) {
	forbidden := []string{"Ctrl+Alt+Del", "Alt+F4"}
	tests := []struct {
		keys string
		want bool
	}{
		{"ctrl+alt+del", true},   // 大小写不敏感
		{"Ctrl+Alt+Del", true},    // 原样
		{"alt+f4", true},
		{"ALT+F4", true},
		{"ctrl+c", false},         // 不在黑名单
		{"ctrl+c ", false},        // 空格 trim
		{"", false},
	}
	for _, tt := range tests {
		if got := isHotkeyForbidden(forbidden, tt.keys); got != tt.want {
			t.Errorf("isHotkeyForbidden(%q) = %v, want %v", tt.keys, got, tt.want)
		}
	}
}

func TestComputerUseService_KeyboardHotkey_ForbiddenRejected(t *testing.T) {
	svc := newTestComputerUseService(t)
	_ = svc.UpdateConfig(ComputerUseConfig{
		Enabled:              true,
		ConfirmationRequired: false, // 跳过确认，测试黑名单
	})
	ctx := context.Background()
	// 禁止快捷键应被拒绝（Step 6）。
	err := svc.KeyboardHotkey(ctx, "ctrl+alt+del", true)
	if !errors.Is(err, ErrNotAllowed) {
		t.Errorf("ctrl+alt+del should be rejected, got %v", err)
	}
	// 非禁止快捷键应通过安全检查（平台 stub 返回 ErrPlatformUnsupported）。
	err = svc.KeyboardHotkey(ctx, "ctrl+c", true)
	if !errors.Is(err, ErrPlatformUnsupported) {
		t.Errorf("ctrl+c should pass safety but fail on platform stub, got %v", err)
	}
}

// --- Step 5/6: 安全边界 — 禁止区域 ---

func TestIsPointInForbiddenZone(t *testing.T) {
	zones := []ForbiddenZone{
		{Name: "password-manager", X: 100, Y: 100, W: 200, H: 100},
	}
	tests := []struct {
		x, y int
		want bool
	}{
		{150, 150, true},   // 中心
		{100, 100, true},    // 左上角（包含）
		{299, 199, true},    // 右下角（包含）
		{300, 200, false},   // 右下角外（不包含）
		{99, 100, false},    // 左侧外
		{0, 0, false},       // 原点外
	}
	for _, tt := range tests {
		if got := isPointInForbiddenZone(zones, tt.x, tt.y); got != tt.want {
			t.Errorf("isPointInForbiddenZone(%d,%d) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}

func TestComputerUseService_MouseMove_ForbiddenZoneRejected(t *testing.T) {
	svc := newTestComputerUseService(t)
	_ = svc.UpdateConfig(ComputerUseConfig{
		Enabled:              true,
		ConfirmationRequired: false,
		ForbiddenZones: []ForbiddenZone{
			{Name: "password-manager", X: 100, Y: 100, W: 200, H: 100},
		},
	})
	ctx := context.Background()
	// 坐标在禁止区域内应被拒绝。
	err := svc.MouseMove(ctx, 150, 150, true)
	if !errors.Is(err, ErrNotAllowed) {
		t.Errorf("move to forbidden zone should be rejected, got %v", err)
	}
	// 坐标在禁止区域外应通过安全检查。
	err = svc.MouseMove(ctx, 500, 500, true)
	if !errors.Is(err, ErrPlatformUnsupported) {
		t.Errorf("move to safe zone should pass safety but fail on platform stub, got %v", err)
	}
}

// --- Step 8: G-SEC-12 默认禁用 ---

func TestComputerUseService_DisabledByDefault(t *testing.T) {
	svc := newTestComputerUseService(t)
	if svc.IsEnabled() {
		t.Error("Computer Use should be disabled by default (G-SEC-12)")
	}
	ctx := context.Background()
	// 未启用时所有操作应被拒绝。
	_, err := svc.Screenshot(ctx, nil, true)
	if !errors.Is(err, ErrNotAllowed) {
		t.Errorf("screenshot when disabled should be ErrNotAllowed, got %v", err)
	}
	err = svc.MouseMove(ctx, 10, 10, true)
	if !errors.Is(err, ErrNotAllowed) {
		t.Errorf("mouse_move when disabled should be ErrNotAllowed, got %v", err)
	}
}

// --- Step 3: ConfirmationRequired ---

func TestComputerUseService_ConfirmationRequired(t *testing.T) {
	svc := newTestComputerUseService(t)
	_ = svc.UpdateConfig(ComputerUseConfig{
		Enabled:              true,
		ConfirmationRequired: true, // 默认值
	})
	ctx := context.Background()
	// 未确认时所有操作应被拒绝。
	_, err := svc.Screenshot(ctx, nil, false)
	if !errors.Is(err, ErrNotAllowed) {
		t.Errorf("screenshot without confirmation should be ErrNotAllowed, got %v", err)
	}
	err = svc.MouseMove(ctx, 10, 10, false)
	if !errors.Is(err, ErrNotAllowed) {
		t.Errorf("mouse_move without confirmation should be ErrNotAllowed, got %v", err)
	}
	// 确认后应通过安全检查（平台 stub 失败）。
	err = svc.MouseMove(ctx, 10, 10, true)
	if !errors.Is(err, ErrPlatformUnsupported) {
		t.Errorf("mouse_move with confirmation should pass safety, got %v", err)
	}
}

// --- Step 7: 审计日志 ---

func TestComputerUseService_AuditLog(t *testing.T) {
	svc := newTestComputerUseService(t)
	_ = svc.UpdateConfig(ComputerUseConfig{
		Enabled:              true,
		ConfirmationRequired: false,
	})
	ctx := context.Background()
	// 执行几个操作（安全检查通过，平台 stub 失败）。
	_ = svc.MouseMove(ctx, 10, 10, true)
	_ = svc.KeyboardHotkey(ctx, "ctrl+c", true)
	// 执行一个被安全检查拒绝的操作。
	_ = svc.KeyboardHotkey(ctx, "ctrl+alt+del", true)

	log := svc.GetAuditLog(10)
	if len(log) < 3 {
		t.Fatalf("expected at least 3 audit entries, got %d", len(log))
	}
	// 验证被拒绝的操作也被记录。
	var foundForbidden, foundMove, foundHotkey bool
	for _, e := range log {
		if e.Action == "mouse_move" && e.Args == "(10,10)" {
			foundMove = true
		}
		if e.Action == "keyboard_hotkey" && e.Args == "ctrl+c" {
			foundHotkey = true
		}
		if e.Action == "keyboard_hotkey" && strings.Contains(e.Error, "forbidden") {
			foundForbidden = true
		}
	}
	if !foundMove {
		t.Error("audit log should record mouse_move")
	}
	if !foundHotkey {
		t.Error("audit log should record keyboard_hotkey ctrl+c")
	}
	if !foundForbidden {
		t.Error("audit log should record rejected forbidden hotkey")
	}
}

func TestComputerUseService_AuditLog_Limit(t *testing.T) {
	svc := newTestComputerUseService(t)
	_ = svc.UpdateConfig(ComputerUseConfig{
		Enabled:              true,
		ConfirmationRequired: false,
	})
	// 记录超过 500 条，验证截断。
	for i := 0; i < 550; i++ {
		svc.recordAudit("mouse_move", "(0,0)", true, true, "")
	}
	log := svc.GetAuditLog(0) // 0 = 全部
	if len(log) != 500 {
		t.Errorf("audit log should be capped at 500, got %d", len(log))
	}
	// 限制返回数量。
	log = svc.GetAuditLog(10)
	if len(log) != 10 {
		t.Errorf("GetAuditLog(10) should return 10 entries, got %d", len(log))
	}
}

// --- Step 4: 录制模式 ---

func TestComputerUseService_Recording(t *testing.T) {
	svc := newTestComputerUseService(t)
	_ = svc.UpdateConfig(ComputerUseConfig{
		Enabled:              true,
		ConfirmationRequired: false,
	})
	if svc.IsRecording() {
		t.Error("should not be recording by default")
	}
	if err := svc.StartRecording(); err != nil {
		t.Fatal(err)
	}
	if !svc.IsRecording() {
		t.Error("should be recording after StartRecording")
	}
	// 模拟录制操作。
	recordAction("mouse_move", "(10,10)")
	recordAction("keyboard_type", "hello")
	actions := svc.StopRecording()
	if len(actions) != 2 {
		t.Fatalf("expected 2 recorded actions, got %d", len(actions))
	}
	if actions[0].Action != "mouse_move" || actions[0].Args != "(10,10)" {
		t.Errorf("first action = %+v", actions[0])
	}
	if svc.IsRecording() {
		t.Error("should not be recording after StopRecording")
	}
}

func TestComputerUseService_Recording_DisabledRejected(t *testing.T) {
	svc := newTestComputerUseService(t)
	// 未启用时不能录制。
	err := svc.StartRecording()
	if !errors.Is(err, ErrNotAllowed) {
		t.Errorf("StartRecording when disabled should be ErrNotAllowed, got %v", err)
	}
}

// --- Step 1/2: 5 个工具接口验证 ---

func TestComputerUseService_FiveToolsExist(t *testing.T) {
	svc := newTestComputerUseService(t)
	_ = svc.UpdateConfig(ComputerUseConfig{
		Enabled:              true,
		ConfirmationRequired: false,
	})
	ctx := context.Background()
	// 所有 5 个工具调用都应到达平台 stub（返回 ErrPlatformUnsupported）。
	// Screenshot 传入 nil region 表示全屏。
	_, err := svc.Screenshot(ctx, nil, true)
	if !errors.Is(err, ErrPlatformUnsupported) {
		t.Errorf("Screenshot should return ErrPlatformUnsupported, got %v", err)
	}
	err = svc.MouseMove(ctx, 10, 10, true)
	if !errors.Is(err, ErrPlatformUnsupported) {
		t.Errorf("MouseMove should return ErrPlatformUnsupported, got %v", err)
	}
	err = svc.MouseClick(ctx, "left", true)
	if !errors.Is(err, ErrPlatformUnsupported) {
		t.Errorf("MouseClick should return ErrPlatformUnsupported, got %v", err)
	}
	err = svc.KeyboardType(ctx, "test", true)
	if !errors.Is(err, ErrPlatformUnsupported) {
		t.Errorf("KeyboardType should return ErrPlatformUnsupported, got %v", err)
	}
	err = svc.KeyboardHotkey(ctx, "ctrl+c", true)
	if !errors.Is(err, ErrPlatformUnsupported) {
		t.Errorf("KeyboardHotkey should return ErrPlatformUnsupported, got %v", err)
	}
}

// --- Step 2: 截图区域参数 ---

func TestComputerUseService_Screenshot_WithRegion(t *testing.T) {
	svc := newTestComputerUseService(t)
	_ = svc.UpdateConfig(ComputerUseConfig{
		Enabled:              true,
		ConfirmationRequired: false,
	})
	ctx := context.Background()
	// 指定区域截图。
	region := image.Rect(0, 0, 100, 100)
	_, err := svc.Screenshot(ctx, &region, true)
	if !errors.Is(err, ErrPlatformUnsupported) {
		t.Errorf("Screenshot with region should reach platform stub, got %v", err)
	}
	// 验证审计日志记录了 region 参数。
	log := svc.GetAuditLog(1)
	if len(log) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(log))
	}
	if !strings.Contains(log[0].Args, "region=") {
		t.Errorf("audit args should contain region, got %q", log[0].Args)
	}
}

// --- 辅助 ---

func TestComputerUseService_ConfigPath(t *testing.T) {
	dir := t.TempDir()
	svc := NewComputerUseService(dir)
	p := svc.configPath()
	expected := filepath.Join(dir, "gugacode", "computer_use.yaml")
	if p != expected {
		t.Errorf("configPath = %q, want %q", p, expected)
	}
}
