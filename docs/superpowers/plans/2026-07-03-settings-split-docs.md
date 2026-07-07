# SettingsView Split + Architecture Docs Plan (Q-5)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the 977-line SettingsView.vue into 6 focused section components, and create ARCHITECTURE.md and SECURITY.md documentation for open-source readiness.

**Architecture:** Each settings section becomes a self-contained Vue component in `frontend/src/components/settings/`. Components import `appState` and `saveSettings` directly from the stores (they're global singletons). The parent SettingsView.vue becomes a thin shell with navigation + dynamic section rendering. Documentation files go in the project root.

**Tech Stack:** Vue 3 SFCs, TypeScript, Go project structure

---

## File Structure

- Create: `frontend/src/components/settings/GeneralSection.vue` — Task 1
- Create: `frontend/src/components/settings/EditorSection.vue` — Task 2
- Create: `frontend/src/components/settings/AiSection.vue` — Task 3
- Create: `frontend/src/components/settings/TerminalSection.vue` — Task 4
- Create: `frontend/src/components/settings/ShortcutsSection.vue` — Task 5
- Create: `frontend/src/components/settings/AppearanceSection.vue` — Task 6
- Modify: `frontend/src/views/SettingsView.vue` — Task 7 (becomes thin shell)
- Create: `ARCHITECTURE.md` — Task 8
- Create: `SECURITY.md` — Task 9

---

### Task 1: Create GeneralSection.vue

**Files:**
- Create: `frontend/src/components/settings/GeneralSection.vue`

Extracts the "general" section (lines 115-170 of original SettingsView.vue). Contains: language selector, telemetry toggle, auto-update toggle, data folder path picker.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/settings/GeneralSection.vue`:

```vue
<script setup lang="ts">
import { appState, saveSettings } from "@/stores/app";
import { fileService } from "@/api/services";
import { Folder } from "@element-plus/icons-vue";

async function handleBrowseFolder() {
  try {
    const path = await fileService.pickDirectory();
    if (path) {
      appState.dataFolderPath = path;
      saveSettings();
    }
  } catch (e) {
    console.error("Failed to pick directory:", e);
  }
}
</script>

<template>
  <section class="settings-section">
    <h2 class="section-title">General</h2>

    <div class="setting-row">
      <label class="setting-label">Language</label>
      <div class="setting-control">
        <el-select
          v-model="appState.language"
          size="default"
          style="width: 180px"
          aria-label="Interface language"
          @change="saveSettings"
        >
          <el-option label="English" value="en" />
          <el-option label="中文" value="zh" />
          <el-option label="日本語" value="ja" />
        </el-select>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Telemetry</label>
      <div class="setting-control">
        <el-switch v-model="appState.telemetry" aria-label="Telemetry toggle" @change="saveSettings" />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Auto Update</label>
      <div class="setting-control">
        <el-switch v-model="appState.autoUpdate" aria-label="Auto update toggle" @change="saveSettings" />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Data Folder</label>
      <div class="setting-control">
        <el-input
          v-model="appState.dataFolderPath"
          size="default"
          style="width: 320px"
          readonly
          aria-label="Data folder path"
        >
          <template #append>
            <el-button :icon="Folder" @click="handleBrowseFolder" aria-label="Browse folder" />
          </template>
        </el-input>
      </div>
    </div>
  </section>
</template>
```

---

### Task 2: Create EditorSection.vue

**Files:**
- Create: `frontend/src/components/settings/EditorSection.vue`

Extracts the "editor" section (lines 173-306). Contains: font size, font family, tab size, word wrap, line numbers, minimap, cursor blinking, cursor style, bracket colorization, auto-save + delay.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/settings/EditorSection.vue`:

```vue
<script setup lang="ts">
import { appState, saveSettings } from "@/stores/app";
</script>

<template>
  <section class="settings-section">
    <h2 class="section-title">Editor</h2>

    <div class="setting-row">
      <label class="setting-label">Font Size</label>
      <div class="setting-control">
        <el-input-number
          v-model="appState.fontSize"
          :min="8"
          :max="32"
          :step="1"
          size="default"
          aria-label="Editor font size"
          @change="saveSettings"
        />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Font Family</label>
      <div class="setting-control">
        <el-input
          v-model="appState.fontFamily"
          size="default"
          style="width: 320px"
          aria-label="Editor font family"
          @change="saveSettings"
        />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Tab Size</label>
      <div class="setting-control">
        <el-input-number
          v-model="appState.tabSize"
          :min="1"
          :max="8"
          :step="1"
          size="default"
          aria-label="Tab size"
          @change="saveSettings"
        />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Word Wrap</label>
      <div class="setting-control">
        <el-switch v-model="appState.wordWrap" aria-label="Word wrap toggle" @change="saveSettings" />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Line Numbers</label>
      <div class="setting-control">
        <el-switch v-model="appState.lineNumbers" aria-label="Line numbers toggle" @change="saveSettings" />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Minimap</label>
      <div class="setting-control">
        <el-switch v-model="appState.minimap" aria-label="Minimap toggle" @change="saveSettings" />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Cursor Blinking</label>
      <div class="setting-control">
        <el-select
          v-model="appState.cursorBlinking"
          size="default"
          style="width: 180px"
          aria-label="Cursor blinking style"
          @change="saveSettings"
        >
          <el-option label="Blink" value="blink" />
          <el-option label="Smooth" value="smooth" />
          <el-option label="Phase" value="phase" />
          <el-option label="Expand" value="expand" />
          <el-option label="Solid" value="solid" />
        </el-select>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Cursor Style</label>
      <div class="setting-control">
        <el-select
          v-model="appState.cursorStyle"
          size="default"
          style="width: 180px"
          aria-label="Cursor style"
          @change="saveSettings"
        >
          <el-option label="Line" value="line" />
          <el-option label="Block" value="block" />
          <el-option label="Underline" value="underline" />
          <el-option label="Line Thin" value="line-thin" />
          <el-option label="Block Outline" value="block-outline" />
          <el-option label="Underline Thin" value="underline-thin" />
        </el-select>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Bracket Pair Colorization</label>
      <div class="setting-control">
        <el-switch v-model="appState.bracketColorization" aria-label="Bracket pair colorization toggle" @change="saveSettings" />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Auto Save</label>
      <div class="setting-control">
        <el-switch v-model="appState.autoSave" aria-label="Auto save toggle" @change="saveSettings" />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Auto Save Delay</label>
      <div class="setting-control">
        <el-select
          v-model="appState.autoSaveDelay"
          size="default"
          style="width: 180px"
          :disabled="!appState.autoSave"
          aria-label="Auto save delay"
          @change="saveSettings"
        >
          <el-option label="After 1s" value="1000" />
          <el-option label="After 5s" value="5000" />
          <el-option label="After 10s" value="10000" />
          <el-option label="After 30s" value="30000" />
        </el-select>
      </div>
    </div>
  </section>
</template>
```

---

### Task 3: Create AiSection.vue

**Files:**
- Create: `frontend/src/components/settings/AiSection.vue`

Extracts the "ai" section (lines 309-455). Contains: AI provider, API key, base URL, model, temperature, max tokens, system prompt, test connection.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/settings/AiSection.vue`:

```vue
<script setup lang="ts">
import { ref } from "vue";
import { appState, saveSettings } from "@/stores/app";
import { aiService } from "@/api/services";
import { Hide, View } from "@element-plus/icons-vue";

const showApiKey = ref(false);
const testingConnection = ref(false);
const testResult = ref<string | null>(null);

function resetSystemPrompt() {
  appState.aiSystemPrompt = "";
  saveSettings();
}

async function handleTestConnection() {
  testingConnection.value = true;
  testResult.value = null;
  try {
    aiService.setConfig({
      apiKey: appState.aiApiKey,
      baseUrl: appState.aiBaseUrl,
      model: appState.aiModel,
      systemPrompt: appState.aiSystemPrompt,
    });
    const response = await aiService.send([{ role: "user", content: "ping" }]);
    if (response) {
      testResult.value = "Success: received response from AI API";
    } else {
      testResult.value = "Warning: empty response from AI API";
    }
  } catch (e: any) {
    testResult.value = `Error: ${e?.message ?? "Connection failed"}`;
  } finally {
    testingConnection.value = false;
  }
}
</script>

<template>
  <section class="settings-section">
    <h2 class="section-title">AI & Models</h2>

    <div class="setting-row">
      <label class="setting-label">Provider</label>
      <div class="setting-control">
        <el-select
          v-model="appState.aiProvider"
          size="default"
          style="width: 180px"
          aria-label="AI provider"
          @change="saveSettings"
        >
          <el-option label="OpenAI" value="openai" />
          <el-option label="Azure OpenAI" value="azure" />
          <el-option label="Ollama" value="ollama" />
          <el-option label="LM Studio" value="lmstudio" />
          <el-option label="Custom" value="custom" />
        </el-select>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">API Key</label>
      <div class="setting-control">
        <el-input
          v-model="appState.aiApiKey"
          size="default"
          style="width: 320px"
          :type="showApiKey ? 'text' : 'password'"
          placeholder="sk-..."
          aria-label="API key"
        >
          <template #suffix>
            <el-button
              :icon="showApiKey ? View : Hide"
              link
              @click="showApiKey = !showApiKey"
              aria-label="Toggle API key visibility"
            />
          </template>
        </el-input>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Base URL</label>
      <div class="setting-control">
        <el-input
          v-model="appState.aiBaseUrl"
          size="default"
          style="width: 320px"
          placeholder="https://api.openai.com"
          aria-label="API base URL"
        />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Model</label>
      <div class="setting-control">
        <el-input
          v-model="appState.aiModel"
          size="default"
          style="width: 320px"
          placeholder="gpt-4o"
          aria-label="AI model name"
          @change="saveSettings"
        />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Temperature</label>
      <div class="setting-control">
        <el-slider
          v-model="appState.temperature"
          :min="0"
          :max="2"
          :step="0.1"
          style="width: 320px"
          aria-label="Temperature"
          @change="saveSettings"
        />
        <span class="slider-value">{{ appState.temperature.toFixed(1) }}</span>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Max Tokens</label>
      <div class="setting-control">
        <el-input-number
          v-model="appState.maxTokens"
          :min="1"
          :max="128000"
          :step="256"
          size="default"
          aria-label="Max tokens"
          @change="saveSettings"
        />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">System Prompt</label>
      <div class="setting-control" style="flex-direction: column; align-items: stretch">
        <el-input
          v-model="appState.aiSystemPrompt"
          type="textarea"
          :rows="6"
          placeholder="Leave empty to use the default system prompt..."
          aria-label="Custom system prompt"
        />
        <div style="margin-top: 8px">
          <el-button size="small" @click="resetSystemPrompt">Reset to Default</el-button>
        </div>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Connection</label>
      <div class="setting-control">
        <el-button
          type="primary"
          size="default"
          :loading="testingConnection"
          @click="handleTestConnection"
        >
          Test Connection
        </el-button>
        <span v-if="testResult" style="margin-left: 12px; font-size: 12px">{{ testResult }}</span>
      </div>
    </div>
  </section>
</template>
```

---

### Task 4: Create TerminalSection.vue

**Files:**
- Create: `frontend/src/components/settings/TerminalSection.vue`

Extracts the "terminal" section (lines 458-526).

- [ ] **Step 1: Create the component**

Create `frontend/src/components/settings/TerminalSection.vue`:

```vue
<script setup lang="ts">
import { appState, saveSettings } from "@/stores/app";
</script>

<template>
  <section class="settings-section">
    <h2 class="section-title">Terminal</h2>

    <div class="setting-row">
      <label class="setting-label">Default Shell</label>
      <div class="setting-control">
        <el-select
          v-model="appState.defaultShell"
          size="default"
          style="width: 180px"
          aria-label="Default shell"
          @change="saveSettings"
        >
          <el-option label="PowerShell" value="powershell" />
          <el-option label="Command Prompt" value="cmd" />
          <el-option label="Git Bash" value="bash" />
          <el-option label="WSL" value="wsl" />
        </el-select>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Font Size</label>
      <div class="setting-control">
        <el-input-number
          v-model="appState.terminalFontSize"
          :min="8"
          :max="32"
          :step="1"
          size="default"
          aria-label="Terminal font size"
          @change="saveSettings"
        />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Cursor Style</label>
      <div class="setting-control">
        <el-select
          v-model="appState.terminalCursorStyle"
          size="default"
          style="width: 180px"
          aria-label="Terminal cursor style"
          @change="saveSettings"
        >
          <el-option label="Block" value="block" />
          <el-option label="Underline" value="underline" />
          <el-option label="Bar" value="bar" />
        </el-select>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Scrollback</label>
      <div class="setting-control">
        <el-input-number
          v-model="appState.scrollback"
          :min="100"
          :max="100000"
          :step="1000"
          size="default"
          aria-label="Terminal scrollback limit"
          @change="saveSettings"
        />
      </div>
    </div>
  </section>
</template>
```

---

### Task 5: Create ShortcutsSection.vue

**Files:**
- Create: `frontend/src/components/settings/ShortcutsSection.vue`

Extracts the "shortcuts" section (lines 529-547).

- [ ] **Step 1: Create the component**

Create `frontend/src/components/settings/ShortcutsSection.vue`:

```vue
<script setup lang="ts">
const shortcuts = [
  { action: "Save File", keys: "Ctrl+S" },
  { action: "Command Palette", keys: "Ctrl+Shift+P" },
  { action: "Find", keys: "Ctrl+F" },
  { action: "Replace", keys: "Ctrl+H" },
  { action: "Toggle Terminal", keys: "Ctrl+`" },
  { action: "Toggle AI Chat", keys: "Ctrl+Shift+A" },
  { action: "Toggle Sidebar", keys: "Ctrl+B" },
];
</script>

<template>
  <section class="settings-section">
    <h2 class="section-title">Shortcuts</h2>

    <div v-for="s in shortcuts" :key="s.action" class="setting-row">
      <label class="setting-label">{{ s.action }}</label>
      <div class="setting-control">
        <kbd class="shortcut-key">{{ s.keys }}</kbd>
      </div>
    </div>
  </section>
</template>
```

---

### Task 6: Create AppearanceSection.vue

**Files:**
- Create: `frontend/src/components/settings/AppearanceSection.vue`

Extracts the "appearance" section (lines 550-619). Contains: theme (dark/light/system), accent color picker, font size scaling, UI density.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/settings/AppearanceSection.vue`:

```vue
<script setup lang="ts">
import { appState, saveSettings, applyAccentTheme, applyMode } from "@/stores/app";
import type { ThemeMode } from "@/stores/app";
import { accentThemes } from "@/lib/monaco-themes";
import type { AccentTheme } from "@/lib/monaco-themes";

const accentColorList = Object.entries(accentThemes).map(([key, meta]) => ({
  key: key as AccentTheme,
  label: meta.label,
  color: meta.color,
}));

function handleThemeChange() {
  applyMode(appState.theme as ThemeMode);
  saveSettings();
}

function selectAccent(key: AccentTheme) {
  applyAccentTheme(key);
  saveSettings();
}
</script>

<template>
  <section class="settings-section">
    <h2 class="section-title">Appearance</h2>

    <div class="setting-row">
      <label class="setting-label">Theme</label>
      <div class="setting-control">
        <el-select
          v-model="appState.theme"
          size="default"
          style="width: 180px"
          aria-label="Application theme"
          @change="handleThemeChange"
        >
          <el-option label="Dark" value="dark" />
          <el-option label="Light" value="light" />
          <el-option label="System" value="system" />
        </el-select>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Color Accent</label>
      <div class="setting-control">
        <div class="color-swatches">
          <button
            v-for="item in accentColorList"
            :key="item.key"
            class="color-swatch"
            :class="{ 'is-selected': appState.accentTheme === item.key }"
            :style="{ backgroundColor: item.color }"
            :aria-label="'Select accent color ' + item.label"
            :aria-pressed="appState.accentTheme === item.key"
            @click="selectAccent(item.key)"
          />
        </div>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">Font Size Scaling</label>
      <div class="setting-control">
        <el-slider
          v-model="appState.fontSizeScaling"
          :min="80"
          :max="150"
          :step="5"
          style="width: 320px"
          aria-label="Font size scaling"
          @change="saveSettings"
        />
        <span class="slider-value">{{ appState.fontSizeScaling }}%</span>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">UI Density</label>
      <div class="setting-control">
        <el-select
          v-model="appState.uiDensity"
          size="default"
          style="width: 180px"
          aria-label="UI density"
          @change="saveSettings"
        >
          <el-option label="Compact" value="compact" />
          <el-option label="Comfortable" value="comfortable" />
          <el-option label="Spacious" value="spacious" />
        </el-select>
      </div>
    </div>
  </section>
</template>
```

---

### Task 7: Rewrite SettingsView.vue as Thin Shell

**Files:**
- Modify: `frontend/src/views/SettingsView.vue` (complete rewrite)

The parent becomes a thin shell with left navigation and dynamic component rendering.

- [ ] **Step 1: Write the new SettingsView.vue**

Overwrite `frontend/src/views/SettingsView.vue` with:

```vue
<script setup lang="ts">
import { ref } from "vue";
import GeneralSection from "@/components/settings/GeneralSection.vue";
import EditorSection from "@/components/settings/EditorSection.vue";
import AiSection from "@/components/settings/AiSection.vue";
import TerminalSection from "@/components/settings/TerminalSection.vue";
import ShortcutsSection from "@/components/settings/ShortcutsSection.vue";
import AppearanceSection from "@/components/settings/AppearanceSection.vue";

type SettingsSection = "general" | "editor" | "ai" | "terminal" | "shortcuts" | "appearance";

const activeSection = ref<SettingsSection>("general");

const navItems: { key: SettingsSection; label: string }[] = [
  { key: "general", label: "General" },
  { key: "editor", label: "Editor" },
  { key: "ai", label: "AI & Models" },
  { key: "terminal", label: "Terminal" },
  { key: "shortcuts", label: "Shortcuts" },
  { key: "appearance", label: "Appearance" },
];

function selectSection(key: SettingsSection) {
  activeSection.value = key;
}
</script>

<template>
  <div class="settings-view">
    <aside class="settings-nav">
      <ul class="settings-nav-list">
        <li
          v-for="item in navItems"
          :key="item.key"
          class="settings-nav-item"
        >
          <button
            class="settings-nav-btn"
            :class="{ 'is-active': activeSection === item.key }"
            :aria-label="item.label"
            :aria-current="activeSection === item.key ? 'page' : undefined"
            @click="selectSection(item.key)"
          >
            {{ item.label }}
          </button>
        </li>
      </ul>
    </aside>

    <main class="settings-content">
      <GeneralSection v-show="activeSection === 'general'" />
      <EditorSection v-show="activeSection === 'editor'" />
      <AiSection v-show="activeSection === 'ai'" />
      <TerminalSection v-show="activeSection === 'terminal'" />
      <ShortcutsSection v-show="activeSection === 'shortcuts'" />
      <AppearanceSection v-show="activeSection === 'appearance'" />
    </main>
  </div>
</template>

<style scoped>
.settings-view {
  display: flex;
  height: 100%;
  overflow: hidden;
}

.settings-nav {
  width: 200px;
  border-right: 1px solid var(--color-border-default);
  background: var(--color-bg-surface);
  padding: 16px 0;
  overflow-y: auto;
}

.settings-nav-list {
  list-style: none;
  padding: 0;
  margin: 0;
}

.settings-nav-item {
  margin: 2px 8px;
}

.settings-nav-btn {
  display: block;
  width: 100%;
  text-align: left;
  padding: 8px 16px;
  border: none;
  background: transparent;
  color: var(--color-text-secondary);
  font-family: var(--font-sans);
  font-size: 13px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.settings-nav-btn:hover {
  background: var(--color-sidebar-hover);
  color: var(--color-text-primary);
}

.settings-nav-btn.is-active {
  background: var(--color-primary-container);
  color: var(--color-on-primary-container);
  font-weight: 500;
}

.settings-content {
  flex: 1;
  overflow-y: auto;
  padding: 24px 32px;
}

.settings-content :deep(.settings-section) {
  max-width: 640px;
}

.settings-content :deep(.section-title) {
  font-size: 18px;
  font-weight: 600;
  margin-bottom: 24px;
  color: var(--color-text-primary);
}

.settings-content :deep(.setting-row) {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 20px;
}

.settings-content :deep(.setting-label) {
  width: 180px;
  flex-shrink: 0;
  font-size: 13px;
  color: var(--color-text-secondary);
}

.settings-content :deep(.setting-control) {
  display: flex;
  align-items: center;
  gap: 8px;
}

.settings-content :deep(.slider-value) {
  font-size: 12px;
  color: var(--color-text-tertiary);
  margin-left: 8px;
}

.settings-content :deep(.color-swatches) {
  display: flex;
  gap: 8px;
}

.settings-content :deep(.color-swatch) {
  width: 28px;
  height: 28px;
  border-radius: var(--radius-full);
  border: 2px solid transparent;
  cursor: pointer;
  transition: border-color var(--transition-fast), transform var(--transition-fast);
}

.settings-content :deep(.color-swatch:hover) {
  transform: scale(1.1);
}

.settings-content :deep(.color-swatch.is-selected) {
  border-color: var(--color-text-primary);
}

.settings-content :deep(.shortcut-key) {
  display: inline-block;
  padding: 2px 8px;
  background: var(--color-bg-surface-container);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-xs);
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--color-text-primary);
}
</style>
```

- [ ] **Step 2: Type-check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [ ] **Step 3: Run tests**

Run: `cd frontend && npx vitest run`
Expected: All tests pass

---

### Task 8: Create ARCHITECTURE.md

**Files:**
- Create: `ARCHITECTURE.md` (project root)

- [ ] **Step 1: Write the file**

Create `e:\gugacode\gugacode\gugacode\ARCHITECTURE.md`:

```markdown
# Architecture

## Overview

gugacode is a desktop IDE built with **Go (Wails v3)** backend and **Vue 3 + TypeScript** frontend, compiled into a single binary. The backend provides services via Wails IPC bindings; the frontend consumes them through auto-generated TypeScript wrappers.

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.25, Wails v3 (alpha2.111) |
| Frontend | Vue 3, TypeScript 5, Vite 8, Tailwind v4 |
| Editor | Monaco Editor 0.55 |
| Terminal | ConPTY (Windows) / creack-pty (Unix) |
| Git | go-git v5.19.1 |
| UI Components | Element Plus 2.14 |
| Charts/Markdown | marked, DOMPurify, highlight.js |

## Project Structure

```
gugacode/
├── main.go                    # App entry: service registration, event wiring
├── go.mod                     # Module: gugacode
├── services/                  # Go backend services
│   ├── file_service.go        # File I/O with workspace sandboxing
│   ├── project_service.go     # Recent projects management
│   ├── settings_service.go    # XDG-path settings persistence
│   ├── window_service.go      # Window controls (min/max/close/fullscreen)
│   ├── terminal_service.go    # ConPTY/pty terminal sessions
│   ├── ai_service.go          # OpenAI-compatible chat + streaming SSE
│   ├── ai_prompts.go          # Default system prompt + 9 preset actions
│   ├── conversation_service.go# AI conversation history persistence
│   ├── git_service.go         # Git status/stage/commit/branch/diff
│   ├── search_service.go      # Regex content search + replace
│   ├── output_buffer.go       # Thread-safe terminal output buffer
│   └── *_test.go              # Go unit tests
├── frontend/
│   ├── src/
│   │   ├── api/services.ts    # Typed Wails binding wrappers
│   │   ├── stores/            # Vue reactive state (app, editor, ai, git, search, terminal)
│   │   ├── components/
│   │   │   ├── editor/        # CodeEditor (Monaco), DiffView, TabBar
│   │   │   ├── explorer/      # FileTree with context menu
│   │   │   ├── layout/        # MainLayout, TitleBar, ActivityBar, SidePanel, GitPanel, SearchPanel, TerminalPanel, AiChatPanel, StatusBar, CommandPalette
│   │   │   └── settings/      # Settings section components (General, Editor, AI, Terminal, Shortcuts, Appearance)
│   │   ├── views/             # WelcomeView, ProjectsView, EditorView, SettingsView, PluginsView
│   │   ├── lib/               # monaco-themes, language detection, markdown, notifications
│   │   ├── composables/       # useKeyboard (global shortcuts)
│   │   └── types/index.ts     # Shared TypeScript interfaces
│   └── bindings/              # Auto-generated Wails JS bindings
├── build/                     # Platform-specific build configs (Windows/macOS/Linux/iOS/Android)
└── docs/                      # Documentation and plans
```

## Service Architecture

Each backend service is a Go struct registered with `application.NewService()`. Wails v3 computes method binding IDs using FNV-1a 32-bit hash of `{modulePath}.{TypeName}.{MethodName}`. The frontend calls these via `$Call.ByID(bindingID, ...args)`.

### Service Registry (main.go)

| Service | Responsibility |
|---|---|
| FileService | File CRUD with path sandboxing (prevents traversal outside workspace) |
| ProjectService | Recent projects list, add/remove, sorted by LastOpened |
| SettingsService | JSON settings persisted to XDG config dir |
| WindowService | Window controls: minimise, maximise, close, fullscreen, set title |
| TerminalService | ConPTY/pty session management, output buffering |
| AIService | OpenAI-compatible chat (send + stream), preset prompts, config |
| GitService | Status, stage/unstage, commit, branch CRUD, diff |
| SearchService | Regex content search + find-and-replace |
| ConversationService | AI conversation save/load/list/delete/rename |

### Event System

Wails v3 events are used for streaming data:
- `terminal:output` — terminal output chunks (emitted from Go poll loop)
- `ai:chunk` — AI streaming response chunks
- `ai:done` — AI stream completion
- `ai:error` — AI stream error
- `time` — clock tick for status bar

### Path Sandboxing

`FileService.SetWorkspaceRoot(path)` sets the allowed directory. All file operations (including `ListDirectory`) validate paths against this root using `filepath.Rel()` to prevent directory traversal. `TerminalService` has an equivalent `validateWorkingDir()` that ensures the working directory is within the workspace.

## Frontend State Management

State is managed via Vue 3 `reactive()` singletons in `stores/`:

- **appState** — global app settings (theme, editor config, AI config, panel visibility)
- **editorState** — open files, active file, dirty state, auto-save
- **aiState** — messages, streaming state, conversations, mentioned files
- **gitState** — changed files, branch info, branches list
- **searchState** — search results, replace state
- **terminalState** — terminal output lines, running state

Settings are persisted to the backend via `saveSettings()` (debounced 500ms) and loaded on startup via `loadSettings()`.

## Theme System

- **Dark/Light mode** — `data-mode` attribute on `<html>` overrides CSS custom properties. Monaco themes are switched between `nknk-{accent}` (dark) and `nknk-light-{accent}` (light) sets.
- **Accent colors** — 8 accent themes (blue, teal, green, amber, pink, purple, cyan, indigo) via `data-theme` attribute. Each accent has coordinated Monaco editor themes.
- **System mode** — Listens to `prefers-color-scheme` media query and auto-switches.

## AI Integration

- **Chat** — OpenAI-compatible API (`/v1/chat/completions`). Supports streaming via SSE.
- **Inline Completion** — Monaco `InlineCompletionItemProvider` calls the AI service with the current file context.
- **Code Actions** — Right-click context menu in Monaco with 9 preset actions (explain, refactor, fix, generate docs, generate tests, optimize, review, security, commit message).
- **@-mention** — Chat input supports `@file` mentions to inject file content as context.
- **Conversation History** — Saved as JSON files in the XDG data directory.

## Testing

- **Go** — `go test ./services/...` (unit tests for all services)
- **Frontend** — `npx vitest run` (Vue component tests + store tests)
- **Type-check** — `npx vue-tsc --noEmit`

## Build

```bash
# Development (hot reload)
wails3 dev

# Production build
wails3 build

# Frontend only (browser dev)
cd frontend && npm run dev
```
```

---

### Task 9: Create SECURITY.md

**Files:**
- Create: `SECURITY.md` (project root)

- [ ] **Step 1: Write the file**

Create `e:\gugacode\gugacode\gugacode\SECURITY.md`:

```markdown
# Security Policy

## Supported Versions

| Version | Supported |
|---|---|
| 1.0.x | ✅ |
| < 1.0 | ❌ |

## Reporting a Vulnerability

If you discover a security vulnerability in gugacode, please report it responsibly:

1. **Do NOT open a public GitHub issue** for security vulnerabilities.
2. Email security@gugacode.dev with a description of the vulnerability, steps to reproduce, and potential impact.
3. You will receive an acknowledgment within 48 hours.
4. We will investigate and provide a fix timeline within 7 days.

Please include:
- Description of the vulnerability
- Steps to reproduce
- Affected components (backend service, frontend component, etc.)
- Potential impact
- Suggested fix (if any)

## Security Measures

### Path Sandboxing
All file operations are sandboxed to the workspace root. `FileService.validatePath()` prevents directory traversal attacks by checking that the resolved path is within the workspace root. Terminal sessions validate their working directory similarly.

### Input Validation
- Project IDs are validated as hex strings to prevent path traversal via filenames.
- AI API responses are checked for non-2xx status codes and parsed for structured error messages.
- HTTP clients disable redirects to prevent SSRF.

### XSS Prevention
- Markdown rendering uses DOMPurify to sanitize HTML before rendering.
- All user input displayed in the UI is escaped by Vue's template engine by default.

### API Key Storage
- API keys are stored in the local settings file (XDG config directory) and never transmitted to any server except the configured AI provider.
- API keys are not logged or included in error messages.

### Dependency Security
- Run `govulncheck ./...` to scan Go dependencies for known vulnerabilities.
- Run `npm audit` in the frontend directory to check npm dependencies.
- Both should be run in CI before releases.

## Security Headers

The Wails v3 webview does not make external network requests except:
- AI provider API calls (user-configured base URL)
- Link clicks in the Help menu (opens in external browser)

No CSRF, ClickJacking, or CORS protections are needed since the app runs in a desktop webview, not a browser.

## Disclosure Timeline

- **Day 0**: Vulnerability reported
- **Day 1-2**: Acknowledgment and initial assessment
- **Day 3-7**: Fix development and testing
- **Day 7-14**: Patch release (severity-dependent)
- **Day 30**: Public disclosure (if applicable)

## Contact

- Security email: security@gugacode.dev
- General issues: [GitHub Issues](https://github.com/gugacode/gugacode/issues)
```

---

### Task 10: Full Verification

- [ ] **Step 1: Go tests**

Run: `cd e:\gugacode\gugacode\gugacode && go test ./services/...`
Expected: ok gugacode/services

- [ ] **Step 2: Frontend type-check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [ ] **Step 3: Frontend tests**

Run: `cd frontend && npx vitest run`
Expected: All tests pass

- [ ] **Step 4: Verify SettingsView is now a thin shell**

Run: `(Get-Content frontend\src\views\SettingsView.vue).Count`
Expected: Less than 200 lines (down from 977)

- [ ] **Step 5: Verify all 6 section components exist**

Run: `Test-Path frontend\src\components\settings\GeneralSection.vue, frontend\src\components\settings\EditorSection.vue, frontend\src\components\settings\AiSection.vue, frontend\src\components\settings\TerminalSection.vue, frontend\src\components\settings\ShortcutsSection.vue, frontend\src\components\settings\AppearanceSection.vue`
Expected: All True

- [ ] **Step 6: Verify docs exist**

Run: `Test-Path ARCHITECTURE.md, SECURITY.md`
Expected: True, True
