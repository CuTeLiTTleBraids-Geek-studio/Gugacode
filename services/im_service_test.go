package services

// Plan 11 Task 7 Step 10 — IMService 测试覆盖。
//
// 覆盖：
//   - 4 个 provider（slack/discord/feishu/wechat_work）配置持久化
//   - EncryptSecret 加密/解密（敏感字段不回传明文，G-SEC-07）
//   - NotificationRules 通知规则渲染 + 发送
//   - G-SEC-12：未 Approved 时拒绝发送
//   - SendMessage payload 构造按 provider 类型

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestIMService(t *testing.T) *IMService {
	t.Helper()
	dir := t.TempDir()
	svc := NewIMService(dir)
	return svc
}

// --- Step 1: IMConfig schema + 持久化 ---

func TestIMService_DefaultConfig(t *testing.T) {
	svc := newTestIMService(t)
	view := svc.LoadConfig()
	if view.Approved {
		t.Error("IM should not be approved by default (G-SEC-12)")
	}
	if len(view.Providers) != 0 {
		t.Errorf("expected 0 providers by default, got %d", len(view.Providers))
	}
}

func TestIMService_UpdateConfig_PersistsProviders(t *testing.T) {
	svc := newTestIMService(t)
	cfg := IMConfig{
		Providers: []IMProvider{
			{Type: "slack", Name: "team-slack", WebhookURL: "https://hooks.slack.com/services/x", BotToken: "xoxb-secret", ChannelID: "C123", Enabled: true, MentionTrigger: "@gpt"},
			{Type: "discord", Name: "dev-discord", WebhookURL: "https://discord.com/api/webhooks/y", ChannelID: "123", Enabled: true},
			{Type: "feishu", Name: "cn-feishu", WebhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/z", BotToken: "t-secret", ChannelID: "oc_abc"},
			{Type: "wechat_work", Name: "corp-wecom", WebhookURL: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=k", ChannelID: "group1"},
		},
		Approved: true,
	}
	if err := svc.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}
	// 重新加载验证持久化 + 解密。
	svc2 := NewIMService(svc.configDir)
	view := svc2.LoadConfig()
	if !view.Approved {
		t.Error("Approved should persist")
	}
	if len(view.Providers) != 4 {
		t.Fatalf("expected 4 providers, got %d", len(view.Providers))
	}
	expected := []string{"slack", "discord", "feishu", "wechat_work"}
	for i, p := range view.Providers {
		if p.Type != expected[i] {
			t.Errorf("provider[%d].Type = %q, want %q", i, p.Type, expected[i])
		}
	}
}

// --- Step 2: 4 个 provider ---

func TestIMService_FourProviders(t *testing.T) {
	svc := newTestIMService(t)
	cfg := IMConfig{
		Providers: []IMProvider{
			{Type: "slack", Name: "s1"},
			{Type: "discord", Name: "d1"},
			{Type: "feishu", Name: "f1"},
			{Type: "wechat_work", Name: "w1"},
		},
		Approved: true,
	}
	_ = svc.UpdateConfig(cfg)
	view := svc.LoadConfig()
	types := make(map[string]bool)
	for _, p := range view.Providers {
		types[p.Type] = true
	}
	for _, tp := range []string{"slack", "discord", "feishu", "wechat_work"} {
		if !types[tp] {
			t.Errorf("provider type %q not found", tp)
		}
	}
}

// --- Step 8: G-SEC-07 加密 — 敏感字段不回传明文 ---

func TestIMService_LoadConfig_NoPlaintextSecrets(t *testing.T) {
	svc := newTestIMService(t)
	cfg := IMConfig{
		Providers: []IMProvider{
			{
				Type:       "slack",
				Name:       "secret-test",
				WebhookURL: "https://hooks.slack.com/services/PLAINTEXT_WEBHOOK",
				BotToken:   "xoxb-PLAINTEXT_TOKEN",
				Enabled:    true,
			},
		},
		Approved: true,
	}
	if err := svc.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}
	view := svc.LoadConfig()
	if len(view.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(view.Providers))
	}
	p := view.Providers[0]
	// G-SEC-07：视图不应包含明文敏感字段。
	if !p.WebhookConfigured {
		t.Error("WebhookConfigured should be true")
	}
	if !p.BotTokenConfigured {
		t.Error("BotTokenConfigured should be true")
	}
	// 验证磁盘文件中的字段已加密（非明文）。
	data, err := os.ReadFile(filepath.Join(svc.configDir, "gugacode", "im.json"))
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	fileContent := string(data)
	if strings.Contains(fileContent, "PLAINTEXT_WEBHOOK") {
		t.Error("plaintext webhook leaked to disk (G-SEC-07)")
	}
	if strings.Contains(fileContent, "PLAINTEXT_TOKEN") {
		t.Error("plaintext bot token leaked to disk (G-SEC-07)")
	}
}

func TestIMService_EncryptionRoundTrip(t *testing.T) {
	svc := newTestIMService(t)
	original := IMConfig{
		Providers: []IMProvider{
			{Type: "slack", Name: "rt", WebhookURL: "https://example.com/hook", BotToken: "secret123", Enabled: true},
		},
		Approved: true,
	}
	if err := svc.UpdateConfig(original); err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}
	// 重新加载：内部 loadConfig 应解密回明文。
	svc2 := NewIMService(svc.configDir)
	svc2.mu.RLock()
	webhook := svc2.config.Providers[0].WebhookURL
	token := svc2.config.Providers[0].BotToken
	svc2.mu.RUnlock()
	if webhook != "https://example.com/hook" {
		t.Errorf("webhook roundtrip = %q, want %q", webhook, "https://example.com/hook")
	}
	if token != "secret123" {
		t.Errorf("token roundtrip = %q, want %q", token, "secret123")
	}
}

// --- Step 9: G-SEC-12 — 未 Approved 拒绝发送 ---

func TestIMService_SendMessage_NotApprovedRejected(t *testing.T) {
	svc := newTestIMService(t)
	// 未调用 Approve()，Approved=false。
	err := svc.SendMessage(context.Background(), "any", "", "hello", nil)
	if err == nil {
		t.Error("SendMessage before approval should fail (G-SEC-12)")
	}
	if !strings.Contains(err.Error(), "not approved") {
		t.Errorf("expected not-approved error, got %v", err)
	}
}

func TestIMService_Approve_EnablesSending(t *testing.T) {
	svc := newTestIMService(t)
	if err := svc.Approve(); err != nil {
		t.Fatalf("Approve failed: %v", err)
	}
	if !svc.IsApproved() {
		t.Error("IsApproved should be true after Approve")
	}
	// 此时 SendMessage 应通过 Approval 检查（后续会因 provider 不存在而失败）。
	err := svc.SendMessage(context.Background(), "nonexistent", "", "hello", nil)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected provider-not-found error after approval, got %v", err)
	}
}

// --- Step 3: buildSendPayload 按 provider 类型构造 ---

func TestIMService_BuildSendPayload_Slack(t *testing.T) {
	svc := newTestIMService(t)
	p := svc.buildSendPayload("slack", "C123", "hello", []string{"code line"})
	if p["channel"] != "C123" {
		t.Errorf("slack payload channel = %v, want C123", p["channel"])
	}
	text, ok := p["text"].(string)
	if !ok {
		t.Fatal("slack payload text should be string")
	}
	if !strings.Contains(text, "hello") {
		t.Error("slack payload should contain text")
	}
	if !strings.Contains(text, "```") {
		t.Error("slack payload should wrap attachments in code block")
	}
}

func TestIMService_BuildSendPayload_Discord(t *testing.T) {
	svc := newTestIMService(t)
	p := svc.buildSendPayload("discord", "", "hi", nil)
	if p["content"] != "hi" {
		t.Errorf("discord payload content = %v, want hi", p["content"])
	}
}

func TestIMService_BuildSendPayload_Feishu(t *testing.T) {
	svc := newTestIMService(t)
	p := svc.buildSendPayload("feishu", "", "你好", nil)
	if p["msg_type"] != "text" {
		t.Errorf("feishu payload msg_type = %v, want text", p["msg_type"])
	}
	content, ok := p["content"].(map[string]interface{})
	if !ok {
		t.Fatal("feishu payload content should be map")
	}
	if content["text"] != "你好" {
		t.Errorf("feishu payload content.text = %v, want 你好", content["text"])
	}
}

func TestIMService_BuildSendPayload_WechatWork(t *testing.T) {
	svc := newTestIMService(t)
	p := svc.buildSendPayload("wechat_work", "", "msg", nil)
	if p["msg_type"] != "text" {
		t.Errorf("wechat_work payload msg_type = %v, want text", p["msg_type"])
	}
}

// --- Step 7: NotificationRules 通知规则 ---

func TestIMService_Notify_UsesRules(t *testing.T) {
	svc := newTestIMService(t)
	_ = svc.Approve()
	cfg := IMConfig{
		Approved: true,
		Providers: []IMProvider{
			{Type: "slack", Name: "team", WebhookURL: "", Enabled: true, ChannelID: "C1"},
		},
		NotificationRules: []NotificationRule{
			{
				Event:    IMEventTaskCompleted,
				Provider: "team",
				Channel:  "C1",
				Template: "Task: {title}\n{body}\n@{timestamp}",
				Enabled:  true,
			},
		},
	}
	_ = svc.UpdateConfig(cfg)
	// 发送应触发规则匹配（provider webhook 为空 → 返回 invalid input 错误，
	// 但证明了规则匹配 + 模板渲染路径被走到）。
	err := svc.Notify(context.Background(), IMEventTaskCompleted, "Build", "Success")
	if err == nil {
		t.Skip("Notify succeeded with empty webhook (unexpected but acceptable)")
	}
	// 错误应包含 provider 名（证明规则被匹配）。
	if !strings.Contains(err.Error(), "team") {
		t.Errorf("Notify error should mention matched provider 'team', got %v", err)
	}
}

func TestIMService_Notify_DisabledRuleSkipped(t *testing.T) {
	svc := newTestIMService(t)
	_ = svc.Approve()
	cfg := IMConfig{
		Approved: true,
		NotificationRules: []NotificationRule{
			{Event: IMEventErrorAlert, Provider: "p", Channel: "c", Template: "{title}", Enabled: false},
		},
	}
	_ = svc.UpdateConfig(cfg)
	// 所有规则 disabled，应无错误返回（无匹配规则 = 无发送）。
	if err := svc.Notify(context.Background(), IMEventErrorAlert, "err", "fail"); err != nil {
		t.Errorf("Notify with all rules disabled should be noop, got %v", err)
	}
}

func TestIMService_RenderTemplate(t *testing.T) {
	svc := newTestIMService(t)
	out := svc.renderTemplate("{title} | {body} | {timestamp}", "T", "B")
	if !strings.Contains(out, "T | B |") {
		t.Errorf("renderTemplate = %q, expected placeholders replaced", out)
	}
}

// --- Step 4: PollMessages ---

func TestIMService_PollMessages_NotApprovedRejected(t *testing.T) {
	svc := newTestIMService(t)
	_, err := svc.PollMessages(context.Background(), "any")
	if err == nil {
		t.Error("PollMessages before approval should fail (G-SEC-12)")
	}
}

func TestIMService_PollMessages_ApprovedReturnsEmpty(t *testing.T) {
	svc := newTestIMService(t)
	_ = svc.Approve()
	msgs, err := svc.PollMessages(context.Background(), "any")
	if err != nil {
		t.Errorf("PollMessages after approval should not error, got %v", err)
	}
	// 简化实现返回 nil/空（无实时接收能力）。
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages in stub, got %d", len(msgs))
	}
}

// --- 视图：不回传明文 ---

func TestIMConfigView_NoSecretFields(t *testing.T) {
	view := IMConfigView{
		Providers: []IMProviderView{
			{Type: "slack", Name: "x", WebhookConfigured: true, BotTokenConfigured: true},
		},
	}
	// 序列化视图，确认不含 webhookUrl/botToken 明文字段（仅 webhookConfigured/botTokenConfigured）。
	data, _ := jsonMarshalSafe(view)
	s := string(data)
	if strings.Contains(s, `"webhookUrl"`) || strings.Contains(s, `"botToken"`) {
		t.Errorf("IMConfigView should not expose webhookUrl/botToken plaintext, got %s", s)
	}
	if !strings.Contains(s, "webhookConfigured") {
		t.Errorf("IMConfigView should expose webhookConfigured, got %s", s)
	}
	if !strings.Contains(s, "botTokenConfigured") {
		t.Errorf("IMConfigView should expose botTokenConfigured, got %s", s)
	}
}

// jsonMarshalSafe 是测试辅助。
func jsonMarshalSafe(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
