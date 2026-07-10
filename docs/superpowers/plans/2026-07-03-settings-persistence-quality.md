# Settings Persistence & Code Quality Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix B-4 (14 unpersisted settings) and resolve Q-1 (SSE duplication), Q-4 (aiService/aiServiceV2 merge), Q-6 (Icon component mapping) to achieve full settings persistence and code quality.

**Architecture:** Extend the `Settings` type on both backend and frontend, merge duplicate AI service wrappers, extract SSE parsing into a shared helper, and convert preset icons from string class names to component references.

**Tech Stack:** Go 1.25, Vue 3 + TypeScript, Element Plus, Wails v3

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `services/settings_service.go` | Settings persistence backend | Modify (add fields) |
| `frontend/src/types/index.ts` | Frontend type definitions | Modify (extend Settings) |
| `frontend/src/stores/app.ts` | AppState reactive store | Modify (add fields + load/save) |
| `frontend/src/views/SettingsView.vue` | Settings UI | Modify (bind all to appState) |
| `services/ai_service.go` | AI service backend | Modify (extract SSE helper) |
| `frontend/src/api/services.ts` | API service wrappers | Modify (merge aiServiceV2) |
| `frontend/src/stores/ai.ts` | AI store | Modify (use merged aiService) |
| `frontend/src/stores/ai.test.ts` | AI store tests | Modify (update mocks) |
| `frontend/src/components/editor/CodeEditor.vue` | Editor with AI actions | Modify (icon mapping) |

---

### Task 1: Extend backend Settings struct (B-4)

**Files:**
- Modify: `services/settings_service.go`

- [x] **Step 1: Read current Settings struct**

Read `services/settings_service.go` to find the `Settings` struct definition.

- [x] **Step 2: Add 14 new fields to Settings struct**

Add these fields to the `Settings` struct:

```go
type Settings struct {
	Language       string `json:"language"`
	Theme          string `json:"theme"`
	FontSize       int    `json:"fontSize"`
	FontFamily     string `json:"fontFamily"`
	TabSize        int    `json:"tabSize"`
	WordWrap       bool   `json:"wordWrap"`
	LineNumbers    bool   `json:"lineNumbers"`
	Minimap        bool   `json:"minimap"`
	AIAPIKey       string `json:"aiApiKey"`
	AIBaseURL      string `json:"aiBaseUrl"`
	AIModel        string `json:"aiModel"`
	AISystemPrompt string `json:"aiSystemPrompt"`
	// New persisted fields (B-4)
	CursorBlinking          string  `json:"cursorBlinking"`
	CursorStyle             string  `json:"cursorStyle"`
	BracketColorization     bool    `json:"bracketColorization"`
	AutoSave                bool    `json:"autoSave"`
	AutoSaveDelay           string  `json:"autoSaveDelay"`
	AIProvider              string  `json:"aiProvider"`
	Temperature             float64 `json:"temperature"`
	MaxTokens               int     `json:"maxTokens"`
	DefaultShell            string  `json:"defaultShell"`
	TerminalFontSize        int     `json:"terminalFontSize"`
	TerminalCursorStyle     string  `json:"terminalCursorStyle"`
	Scrollback              int     `json:"scrollback"`
	UIDensity               string  `json:"uiDensity"`
	FontSizeScaling         int     `json:"fontSizeScaling"`
}
```

- [x] **Step 3: Update defaultSettings to include new fields**

Find the `defaultSettings` variable and add:

```go
var defaultSettings = Settings{
	Language:       "en",
	Theme:          "dark",
	FontSize:       14,
	FontFamily:     "JetBrains Mono",
	TabSize:        2,
	WordWrap:       true,
	LineNumbers:    true,
	Minimap:        false,
	AIBaseURL:      "https://api.openai.com",
	AIModel:        "gpt-4o",
	// New defaults
	CursorBlinking:      "blink",
	CursorStyle:         "line",
	BracketColorization: true,
	AutoSave:            false,
	AutoSaveDelay:       "afterDelay",
	AIProvider:          "",
	Temperature:         0.7,
	MaxTokens:           4096,
	DefaultShell:        "",
	TerminalFontSize:    13,
	TerminalCursorStyle: "block",
	Scrollback:          10000,
	UIDensity:           "comfortable",
	FontSizeScaling:     100,
}
```

- [x] **Step 4: Verify build**

Run: `go build ./services/...`
Expected: success

- [x] **Step 5: Commit**

```bash
git add services/settings_service.go
git commit -m "feat: extend Settings struct with 14 new persisted fields (B-4)"
```

---

### Task 2: Extend frontend Settings type and appState (B-4)

**Files:**
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/stores/app.ts`

- [x] **Step 1: Extend Settings interface in types**

In `frontend/src/types/index.ts`, find the `Settings` interface and add:

```typescript
export interface Settings {
  language: string;
  theme: string;
  fontSize: number;
  fontFamily: string;
  tabSize: number;
  wordWrap: boolean;
  lineNumbers: boolean;
  minimap: boolean;
  aiApiKey: string;
  aiBaseUrl: string;
  aiModel: string;
  aiSystemPrompt: string;
  // New persisted fields (B-4)
  cursorBlinking: string;
  cursorStyle: string;
  bracketColorization: boolean;
  autoSave: boolean;
  autoSaveDelay: string;
  aiProvider: string;
  temperature: number;
  maxTokens: number;
  defaultShell: string;
  terminalFontSize: number;
  terminalCursorStyle: string;
  scrollback: number;
  uiDensity: string;
  fontSizeScaling: number;
}
```

- [x] **Step 2: Extend AppState interface in app.ts**

In `frontend/src/stores/app.ts`, add the new fields to `AppState`:

```typescript
  // Editor extras
  cursorBlinking: string;
  cursorStyle: string;
  bracketColorization: boolean;
  autoSave: boolean;
  autoSaveDelay: string;
  // AI extras
  aiProvider: string;
  temperature: number;
  maxTokens: number;
  // Terminal
  defaultShell: string;
  terminalFontSize: number;
  terminalCursorStyle: string;
  scrollback: number;
  // Appearance
  uiDensity: string;
  fontSizeScaling: number;
```

- [x] **Step 3: Add defaults to appState reactive**

In the `reactive<AppState>()` call, add:

```typescript
  cursorBlinking: "blink",
  cursorStyle: "line",
  bracketColorization: true,
  autoSave: false,
  autoSaveDelay: "afterDelay",
  aiProvider: "",
  temperature: 0.7,
  maxTokens: 4096,
  defaultShell: "",
  terminalFontSize: 13,
  terminalCursorStyle: "block",
  scrollback: 10000,
  uiDensity: "comfortable",
  fontSizeScaling: 100,
```

- [x] **Step 4: Update loadSettings to load new fields**

In `loadSettings()`, add:

```typescript
    appState.cursorBlinking = settings.cursorBlinking;
    appState.cursorStyle = settings.cursorStyle;
    appState.bracketColorization = settings.bracketColorization;
    appState.autoSave = settings.autoSave;
    appState.autoSaveDelay = settings.autoSaveDelay;
    appState.aiProvider = settings.aiProvider;
    appState.temperature = settings.temperature;
    appState.maxTokens = settings.maxTokens;
    appState.defaultShell = settings.defaultShell;
    appState.terminalFontSize = settings.terminalFontSize;
    appState.terminalCursorStyle = settings.terminalCursorStyle;
    appState.scrollback = settings.scrollback;
    appState.uiDensity = settings.uiDensity;
    appState.fontSizeScaling = settings.fontSizeScaling;
```

- [x] **Step 5: Update saveSettings to save new fields**

In `saveSettings()`, add to the `settings: Settings` object:

```typescript
      cursorBlinking: appState.cursorBlinking,
      cursorStyle: appState.cursorStyle,
      bracketColorization: appState.bracketColorization,
      autoSave: appState.autoSave,
      autoSaveDelay: appState.autoSaveDelay,
      aiProvider: appState.aiProvider,
      temperature: appState.temperature,
      maxTokens: appState.maxTokens,
      defaultShell: appState.defaultShell,
      terminalFontSize: appState.terminalFontSize,
      terminalCursorStyle: appState.terminalCursorStyle,
      scrollback: appState.scrollback,
      uiDensity: appState.uiDensity,
      fontSizeScaling: appState.fontSizeScaling,
```

- [x] **Step 6: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [x] **Step 7: Commit**

```bash
git add frontend/src/types/index.ts frontend/src/stores/app.ts
git commit -m "feat: extend frontend Settings type and appState with 14 fields (B-4)"
```

---

### Task 3: Update SettingsView to bind all settings to appState (B-4)

**Files:**
- Modify: `frontend/src/views/SettingsView.vue`

- [x] **Step 1: Remove all local refs for persisted settings**

Remove these local ref declarations:
```typescript
const telemetry = ref(appState.telemetry);
const autoUpdate = ref(appState.autoUpdate);
const dataFolderPath = ref(appState.dataFolderPath);
const cursorBlinking = ref("blink");
const cursorStyle = ref("line");
const bracketColorization = ref(true);
const autoSave = ref(false);
const autoSaveDelay = ref("afterDelay");
const aiProvider = ref("");
const temperature = ref(0.7);
const maxTokens = ref(4096);
const defaultShell = ref("");
const terminalFontSize = ref(13);
const terminalCursorStyle = ref("block");
const scrollback = ref(10000);
const uiDensity = ref("comfortable");
const fontSizeScaling = ref(100);
```

- [x] **Step 2: Update all template v-model bindings**

Replace every `v-model="cursorBlinking"` with `v-model="appState.cursorBlinking"`, and add `@change="saveSettings"`.

Do the same for all 14+ settings:
- `cursorBlinking` → `appState.cursorBlinking` + `@change="saveSettings"`
- `cursorStyle` → `appState.cursorStyle` + `@change="saveSettings"`
- `bracketColorization` → `appState.bracketColorization` + `@change="saveSettings"`
- `autoSave` → `appState.autoSave` + `@change="saveSettings"`
- `autoSaveDelay` → `appState.autoSaveDelay` + `@change="saveSettings"`
- `aiProvider` → `appState.aiProvider` + `@change="saveSettings"`
- `temperature` → `appState.temperature` + `@change="saveSettings"`
- `maxTokens` → `appState.maxTokens` + `@change="saveSettings"`
- `defaultShell` → `appState.defaultShell` + `@input="saveSettings"`
- `terminalFontSize` → `appState.terminalFontSize` + `@change="saveSettings"`
- `terminalCursorStyle` → `appState.terminalCursorStyle` + `@change="saveSettings"`
- `scrollback` → `appState.scrollback` + `@change="saveSettings"`
- `uiDensity` → `appState.uiDensity` + `@change="saveSettings"`
- `fontSizeScaling` → `appState.fontSizeScaling` + `@change="saveSettings"`
- `telemetry` → `appState.telemetry` + `@change="saveSettings"`
- `autoUpdate` → `appState.autoUpdate` + `@change="saveSettings"`
- `dataFolderPath` → `appState.dataFolderPath` + `@input="saveSettings"`

- [x] **Step 3: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [x] **Step 4: Run tests**

Run: `cd frontend && npx vitest run`
Expected: all PASS

- [x] **Step 5: Commit**

```bash
git add frontend/src/views/SettingsView.vue
git commit -m "fix: bind all SettingsView fields to appState with saveSettings (B-4)"
```

---

### Task 4: Merge aiService and aiServiceV2 (Q-4)

**Files:**
- Modify: `frontend/src/api/services.ts`
- Modify: `frontend/src/stores/ai.ts`
- Modify: `frontend/src/stores/ai.test.ts`
- Modify: `frontend/src/views/SettingsView.vue`

- [x] **Step 1: Merge setConfig into aiService with optional systemPrompt**

In `frontend/src/api/services.ts`, update the `aiService.setConfig` to accept `systemPrompt`:

```typescript
export const aiService = {
  setConfig: (config: {
    apiKey: string;
    baseUrl: string;
    model: string;
    systemPrompt?: string;
  }) =>
    AIServiceBindings.SetConfig({
      APIKey: config.apiKey,
      BaseURL: config.baseUrl,
      Model: config.model,
      SystemPrompt: config.systemPrompt ?? "",
    }) as Promise<void>,
  send: (messages: ChatMessage[]) =>
    AIServiceBindings.Send(messages) as Promise<{
      Content: string;
      FinishReason: string;
    } | null>,
  startStream: (messages: ChatMessage[]) =>
    AIServiceBindings.StartStream(messages) as Promise<void>,
  stopStream: () =>
    AIServiceBindings.StopStream() as Promise<void>,
  getDefaultSystemPrompt: () =>
    AIServiceBindings.GetDefaultSystemPrompt() as Promise<string>,
  getPresetPrompt: (name: string) =>
    AIServiceBindings.GetPresetPrompt(name) as Promise<string>,
  listPresets: () =>
    AIServiceBindings.ListPresets() as Promise<PresetMeta[]>,
};
```

Remove the entire `aiServiceV2` export.

- [x] **Step 2: Update ai.ts to use aiService.setConfig**

In `frontend/src/stores/ai.ts`, replace:
```typescript
import { aiService, aiServiceV2, conversationService } from "@/api/services";
```
with:
```typescript
import { aiService, conversationService } from "@/api/services";
```

Replace:
```typescript
    aiServiceV2.setConfig({
      apiKey: appState.aiApiKey,
      baseUrl: appState.aiBaseUrl,
      model: appState.aiModel,
      systemPrompt: appState.aiSystemPrompt,
    });
```
with:
```typescript
    aiService.setConfig({
      apiKey: appState.aiApiKey,
      baseUrl: appState.aiBaseUrl,
      model: appState.aiModel,
      systemPrompt: appState.aiSystemPrompt,
    });
```

- [x] **Step 3: Update SettingsView.vue imports**

In `frontend/src/views/SettingsView.vue`, replace:
```typescript
import { fileService, aiService, aiServiceV2 } from "@/api/services";
```
with:
```typescript
import { fileService, aiService } from "@/api/services";
```

Replace `aiServiceV2.setConfig` with `aiService.setConfig` in `handleTestConnection`.

- [x] **Step 4: Update ai.test.ts mocks**

In `frontend/src/stores/ai.test.ts`, remove the `aiServiceV2` mock and update the `aiService` mock to include `setConfig`:

```typescript
vi.mock("@/api/services", () => ({
  aiService: {
    setConfig: vi.fn().mockResolvedValue(undefined),
    startStream: vi.fn().mockResolvedValue(undefined),
    stopStream: vi.fn().mockResolvedValue(undefined),
    send: vi.fn().mockResolvedValue({ Content: "ok", FinishReason: "stop" }),
    getPresetPrompt: vi.fn().mockResolvedValue("Explain this code."),
    getDefaultSystemPrompt: vi.fn().mockResolvedValue("default prompt"),
    listPresets: vi.fn().mockResolvedValue([]),
  },
  conversationService: {
    save: vi.fn().mockResolvedValue(undefined),
    load: vi.fn().mockResolvedValue({ id: "1", title: "test", created_at: 0, messages: [] }),
    generateId: vi.fn().mockResolvedValue("new-id"),
    generateTitle: vi.fn().mockResolvedValue("test title"),
  },
}));
```

- [x] **Step 5: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [x] **Step 6: Run tests**

Run: `cd frontend && npx vitest run`
Expected: all PASS

- [x] **Step 7: Commit**

```bash
git add frontend/src/api/services.ts frontend/src/stores/ai.ts frontend/src/stores/ai.test.ts frontend/src/views/SettingsView.vue
git commit -m "refactor: merge aiServiceV2 into aiService with optional systemPrompt (Q-4)"
```

---

### Task 5: Extract parseSSEStream helper (Q-1)

**Files:**
- Modify: `services/ai_service.go`
- Test: `services/ai_service_test.go`

- [x] **Step 1: Write the failing test**

Add to `services/ai_service_test.go`:

```go
func TestParseSSEStream_EmitsChunks(t *testing.T) {
	// SSE format: "data: {json}\n\n"
	body := "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\ndata: [DONE]\n\n"
	var chunks []string
	err := parseSSEStream(strings.NewReader(body), func(c string) {
		chunks = append(chunks, c)
	})
	if err != nil {
		t.Fatalf("parseSSEStream failed: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0] != "hello" || chunks[1] != " world" {
		t.Errorf("expected ['hello', ' world'], got %v", chunks)
	}
}

func TestParseSSEStream_EmptyInput(t *testing.T) {
	err := parseSSEStream(strings.NewReader(""), func(c string) {})
	if err != nil {
		t.Fatalf("parseSSEStream on empty input should not error: %v", err)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./services/ -run TestParseSSEStream -v`
Expected: FAIL (function not defined)

- [x] **Step 3: Implement parseSSEStream**

Add this function to `services/ai_service.go`:

```go
// parseSSEStream reads a Server-Sent Events stream and calls onChunk for each
// content delta. Returns nil on normal completion or the scanner error.
func parseSSEStream(r io.Reader, onChunk func(string)) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 6 || line[:6] != "data: " {
			continue
		}
		data := line[6:]
		if data == "[DONE]" {
			break
		}

		var result struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &result); err != nil {
			continue
		}
		if len(result.Choices) > 0 && result.Choices[0].Delta.Content != "" {
			onChunk(result.Choices[0].Delta.Content)
		}
	}
	return scanner.Err()
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./services/ -run TestParseSSEStream -v`
Expected: PASS

- [x] **Step 5: Refactor SendStreamWithContext to use parseSSEStream**

Replace the SSE parsing loop in `SendStreamWithContext` with:

```go
	return parseSSEStream(resp.Body, onChunk)
```

Replace the SSE parsing loop in `streamWithEvents` with:

```go
	return parseSSEStream(resp.Body, func(chunk string) {
		a.app.Event.Emit("ai:chunk", chunk)
	})
```

- [x] **Step 6: Run full test suite**

Run: `go test ./services/ -count=1`
Expected: all PASS

- [x] **Step 7: Commit**

```bash
git add services/ai_service.go services/ai_service_test.go
git commit -m "refactor: extract parseSSEStream helper to eliminate SSE duplication (Q-1)"
```

---

### Task 6: PresetMeta Icon component mapping (Q-6)

**Files:**
- Modify: `frontend/src/components/editor/CodeEditor.vue`

- [x] **Step 1: Create icon name-to-component mapping**

In `frontend/src/components/editor/CodeEditor.vue`, add an import and mapping in the `<script setup>`:

```typescript
import {
  Info,
  Refresh,
  MagicStick,
  Document,
  CircleCheck,
  Cpu,
  View,
  Lock,
  Edit,
} from "@element-plus/icons-vue";

const presetIconMap: Record<string, any> = {
  "el-icon-info": Info,
  "el-icon-refresh-left": Refresh,
  "el-icon-magic-stick": MagicStick,
  "el-icon-document": Document,
  "el-icon-circle-check": CircleCheck,
  "el-icon-cpu": Cpu,
  "el-icon-view": View,
  "el-icon-lock": Lock,
  "el-icon-edit": Edit,
};
```

- [x] **Step 2: Update the AI action menu to use component icons**

Where the AI actions are rendered in the context menu, use:

```html
<el-icon><component :is="presetIconMap[action.icon] || Info" /></el-icon>
```

- [x] **Step 3: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [x] **Step 4: Run tests**

Run: `cd frontend && npx vitest run`
Expected: all PASS

- [x] **Step 5: Commit**

```bash
git add frontend/src/components/editor/CodeEditor.vue
git commit -m "fix: map PresetMeta icon strings to Element Plus components (Q-6)"
```

---

### Task 7: Full verification

- [x] **Step 1: Run Go tests**

Run: `go test ./services/... -count=1 -timeout 60s`
Expected: all PASS

- [x] **Step 2: Run Go build**

Run: `go build .`
Expected: success

- [x] **Step 3: Run frontend type check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [x] **Step 4: Run frontend tests**

Run: `cd frontend && npx vitest run`
Expected: all PASS

- [x] **Step 5: Commit if any fixup needed**

```bash
git add -A
git commit -m "chore: verification fixes for Plan 10"
```
