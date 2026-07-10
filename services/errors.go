package services

import "errors"

// Sentinel errors for consistent error checking across services (G-QUAL-01).
//
// Use errors.Is(err, services.ErrNotFound) to discriminate rather than
// string-matching on err.Error(). Wrap these with fmt.Errorf("...: %w", err)
// when returning so callers can still unwrap them.
var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrInvalidInput  = errors.New("invalid input")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrTimeout       = errors.New("timeout")
	// ErrNotAllowed 表示操作被安全策略拒绝（G-SEC-02 / G-SEC-12）。
	// 例如：Computer Use 未启用、坐标落入禁止区域、快捷键在黑名单中。
	ErrNotAllowed = errors.New("not allowed")
	// ErrPlatformUnsupported 表示当前平台不支持该操作。
	// 例如：Linux/macOS 上的 Computer Use 截图/鼠标键盘原生操作。
	ErrPlatformUnsupported = errors.New("platform unsupported")
)
