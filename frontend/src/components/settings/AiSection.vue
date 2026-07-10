<script setup lang="ts">
import { ref, watch, onMounted, computed } from "vue";
import {
  appState,
  activateAIConfig,
  saveAIConfig,
  deleteAIConfig,
  createNewAIConfig,
  saveSettings,
} from "@/stores/app";
import type { AIProviderConfig } from "@/types";
import { PROVIDER_PRESETS, getProviderPreset } from "@/lib/aiProviders";
import { aiService, settingsService } from "@/api/services";
import { notifySuccess, notifyError } from "@/lib/notifications";
import { errorMessage } from "@/lib/errors";
import { Hide, View, Lock, Unlock, Plus, Edit, Delete, Check } from "@element-plus/icons-vue";
import { useI18n } from "@/lib/i18n";

const { t } = useI18n();

// A normalized draft with all optional fields filled in. We always populate
// defaults when cloning a config into the edit form, so the template can bind
// v-model directly without worrying about `undefined` (el-slider's modelValue
// doesn't accept undefined, etc.).
interface DraftConfig {
  id: string;
  name: string;
  provider: string;
  protocol: string;
  apiKey: string;
  apiKeyConfigured: boolean;
  baseUrl: string;
  model: string;
  temperature: number;
  maxTokens: number;
  systemPrompt: string;
}

const showApiKey = ref(false);
const testingConnection = ref(false);
const testResult = ref<string | null>(null);
const loadingPrompt = ref<"default" | "agent" | null>(null);
const apiKeyStorageMethod = ref<string>("none");
// Which config is currently being edited (expanded inline). null = no edit open.
const editingConfigId = ref<string | null>(null);
// Local draft of the config being edited. Changes here don't persist until the
// user clicks "Save"; cancelling just discards this draft.
const editingDraft = ref<DraftConfig | null>(null);
// G-SEC-07: the actual key is NOT pre-filled into the edit form (to avoid
// surfacing it in the DOM). We remember whether the edited config already has
// a key so we can show a "Key configured" placeholder and preserve the
// existing key on save when the user doesn't enter a new one.
const editingOriginalApiKey = ref<string>("");

// All preset Base URLs — used to detect when the user hasn't customized the
// Base URL yet (so switching providers can auto-fill the new preset's URL).
const presetBaseUrls = new Set(
  PROVIDER_PRESETS.map((p) => p.baseUrl).filter((u) => u !== ""),
);

// When we clone a config into editingDraft (open edit / new config), the
// provider watcher would otherwise fire and clobber an existing custom
// Base URL. This flag suppresses that first fire after population.
let suppressProviderAutoFill = false;

async function refreshApiKeyStorageMethod() {
  try {
    apiKeyStorageMethod.value = await settingsService.getAPIKeyStorageMethod();
  } catch {
    apiKeyStorageMethod.value = "none";
  }
}

onMounted(refreshApiKeyStorageMethod);

// After saving, refresh the encryption badge. saveSettings is debounced
// (500ms) so we wait a bit longer before reading the on-disk state.
function saveSettingsAndRefreshEncryption() {
  saveSettings();
  setTimeout(refreshApiKeyStorageMethod, 800);
}

const apiKeyEncryptionLabel = computed(() => {
  switch (apiKeyStorageMethod.value) {
    case "dpapi": return t("aiSection.encryptedDpapi");
    case "aes": return t("aiSection.encryptedAes");
    case "plain": return t("aiSection.notEncrypted");
    default: return "";
  }
});

const apiKeyEncrypted = computed(() =>
  apiKeyStorageMethod.value === "dpapi" || apiKeyStorageMethod.value === "aes",
);

// Non-null view of the draft for use inside the edit form (guarded by v-if).
const draft = computed(() => editingDraft.value as DraftConfig);

const baseUrlPlaceholder = computed(() =>
  draft.value?.protocol === "anthropic"
    ? t("aiSection.baseUrlAnthropicPlaceholder")
    : t("aiSection.baseUrlOpenaiPlaceholder"),
);

// G-SEC-07: when editing a config that already has a key, hint that a key is
// configured instead of pre-filling the actual key into the input.
const apiKeyPlaceholder = computed(() =>
  editingOriginalApiKey.value
    ? t("aiSection.apiKeyConfigured")
    : "sk-...",
);

const protocolDisabled = computed(() => draft.value?.provider === "anthropic");

function providerLabel(providerId: string): string {
  return getProviderPreset(providerId)?.label ?? providerId;
}

// --- Provider switch auto-fill -------------------------------------------------
// When the user changes the provider in the edit form, auto-fill the Base URL
// (only if it's empty or matches another preset's default) and the protocol.
// These are just default values — the user can still override them manually.
watch(
  () => editingDraft.value?.provider,
  (newProvider, oldProvider) => {
    if (suppressProviderAutoFill) {
      suppressProviderAutoFill = false;
      return;
    }
    if (!editingDraft.value || newProvider === oldProvider) return;
    const preset = getProviderPreset(newProvider ?? "");
    if (!preset) return;
    const currentUrl = editingDraft.value.baseUrl.trim();
    if (currentUrl === "" || presetBaseUrls.has(currentUrl)) {
      editingDraft.value.baseUrl = preset.baseUrl;
    }
    if (preset.protocol) {
      editingDraft.value.protocol = preset.protocol;
    }
  },
);

// --- Config list actions ------------------------------------------------------

function handleNewConfig() {
  const cfg = createNewAIConfig();
  // Push into the list and persist immediately (so the new config survives a
  // crash even before the user finishes editing it).
  saveAIConfig(cfg);
  activateAIConfig(cfg.id);
  // Open the edit form with a clone so edits stay local until "Save".
  suppressProviderAutoFill = true;
  editingDraft.value = normalizeDraft(cfg);
  editingConfigId.value = cfg.id;
  // G-SEC-07: new config has no stored key yet.
  editingOriginalApiKey.value = "";
  testResult.value = null;
  showApiKey.value = false;
}

function handleEdit(id: string) {
  const cfg = appState.aiProviderConfigs.find((c) => c.id === id);
  if (!cfg) return;
  suppressProviderAutoFill = true;
  editingDraft.value = normalizeDraft(cfg);
  // G-SEC-07: the backend strips apiKey from configs. Track whether a key
  // is configured (not the plaintext) so save can preserve it.
  editingOriginalApiKey.value = cfg.apiKeyConfigured ? "___stored___" : "";
  editingDraft.value.apiKey = "";
  editingConfigId.value = id;
  testResult.value = null;
  showApiKey.value = false;
}

function handleSetActive(id: string) {
  activateAIConfig(id);
}

function handleDelete(id: string) {
  if (appState.aiProviderConfigs.length <= 1) {
    notifyError(t("aiSection.deleteFailed"));
    return;
  }
  // Close the edit form if we're deleting the config being edited.
  if (editingConfigId.value === id) {
    editingConfigId.value = null;
    editingDraft.value = null;
    testResult.value = null;
  }
  const ok = deleteAIConfig(id);
  if (!ok) {
    notifyError(t("aiSection.deleteFailed"));
  }
}

function normalizeDraft(cfg: AIProviderConfig): DraftConfig {
  return {
    id: cfg.id,
    name: cfg.name,
    provider: cfg.provider,
    protocol: cfg.protocol ?? "openai",
    apiKey: cfg.apiKey,
    apiKeyConfigured: cfg.apiKeyConfigured ?? false,
    baseUrl: cfg.baseUrl,
    model: cfg.model,
    temperature: cfg.temperature ?? 0.7,
    maxTokens: cfg.maxTokens ?? 4096,
    systemPrompt: cfg.systemPrompt ?? "",
  };
}

// --- Edit form actions --------------------------------------------------------

function handleSave() {
  if (!editingDraft.value) return;
  // G-SEC-07: if the user did not enter a new key, set apiKeyConfigured so
  // the backend preserves the existing on-disk key. When a new key was
  // entered, it will be saved and apiKeyConfigured will be true.
  if (!editingDraft.value.apiKey && editingOriginalApiKey.value) {
    editingDraft.value.apiKeyConfigured = true;
  } else if (editingDraft.value.apiKey) {
    editingDraft.value.apiKeyConfigured = true;
  } else {
    editingDraft.value.apiKeyConfigured = false;
  }
  saveAIConfig(editingDraft.value);
  // Refresh the encryption badge after a delay (apiKey may have changed).
  saveSettingsAndRefreshEncryption();
  editingConfigId.value = null;
  editingDraft.value = null;
  editingOriginalApiKey.value = "";
  testResult.value = null;
}

function handleCancel() {
  editingConfigId.value = null;
  editingDraft.value = null;
  editingOriginalApiKey.value = "";
  testResult.value = null;
}

async function loadPrompt(name: "default" | "agent") {
  if (!editingDraft.value) return;
  loadingPrompt.value = name;
  try {
    const text = await aiService.getSystemPrompt(name);
    editingDraft.value.systemPrompt = text;
    notifySuccess(
      name === "agent" ? t("aiSection.agentPromptLoaded") : t("aiSection.defaultPromptLoaded"),
    );
  } catch (e: unknown) {
    notifyError(
      t("aiSection.loadFailed", { name, error: errorMessage(e) || t("aiSection.unknownError") }),
    );
  } finally {
    loadingPrompt.value = null;
  }
}

function resetSystemPrompt() {
  if (!editingDraft.value) return;
  editingDraft.value.systemPrompt = "";
  notifySuccess(t("aiSection.systemPromptCleared"));
}

async function handleTestConnection() {
  if (!editingDraft.value) return;
  testingConnection.value = true;
  testResult.value = null;
  try {
    // G-SEC-07: when the user has not entered a new key, use the stored key
    // (backend fetches from SettingsService via configId). When a new key
    // was entered, send it directly for the test.
    const newKey = editingDraft.value.apiKey;
    aiService.setConfig({
      apiKey: newKey || undefined,
      useStoredKey: !newKey,
      configId: editingDraft.value.id,
      baseUrl: editingDraft.value.baseUrl,
      model: editingDraft.value.model,
      systemPrompt: editingDraft.value.systemPrompt ?? "",
      temperature: editingDraft.value.temperature ?? 0.7,
      protocol: editingDraft.value.protocol ?? "openai",
    });
    const response = await aiService.send([{ role: "user", content: "ping" }]);
    if (response) {
      testResult.value = t("aiSection.testSuccess");
    } else {
      testResult.value = t("aiSection.testEmpty");
    }
  } catch (e: unknown) {
    testResult.value = t("aiSection.testError", { error: errorMessage(e) || t("aiSection.connectionFailed") });
  } finally {
    testingConnection.value = false;
  }
}
</script>

<template>
  <section class="settings-section">
    <h2 class="section-title">{{ t("aiSection.title") }}</h2>

    <div class="ai-configs">
      <div class="ai-configs__header">
        <span class="setting-label">{{ t("aiSection.configs") }}</span>
        <el-button size="small" type="primary" :icon="Plus" @click="handleNewConfig">
          {{ t("aiSection.newConfig") }}
        </el-button>
      </div>

      <div
        v-for="cfg in appState.aiProviderConfigs"
        :key="cfg.id"
        class="ai-config-row"
        :class="{ 'ai-config-row--active': cfg.id === appState.activeAIConfigId }"
      >
        <div class="ai-config-row__top">
          <div class="ai-config-row__info">
            <span class="ai-config-row__name">{{ cfg.name }}</span>
            <el-tag size="small" type="info">{{ providerLabel(cfg.provider) }}</el-tag>
            <span class="ai-config-row__model">{{ cfg.model }}</span>
            <el-tag
              v-if="cfg.id === appState.activeAIConfigId"
              size="small"
              type="success"
            >
              {{ t("aiSection.active") }}
            </el-tag>
          </div>
          <div class="ai-config-row__actions">
            <el-button
              v-if="cfg.id !== appState.activeAIConfigId"
              size="small"
              :icon="Check"
              @click="handleSetActive(cfg.id)"
            >
              {{ t("aiSection.setActive") }}
            </el-button>
            <el-button size="small" :icon="Edit" @click="handleEdit(cfg.id)">
              {{ t("aiSection.edit") }}
            </el-button>
            <el-button
              v-if="appState.aiProviderConfigs.length > 1"
              size="small"
              type="danger"
              :icon="Delete"
              @click="handleDelete(cfg.id)"
            >
              {{ t("aiSection.delete") }}
            </el-button>
          </div>
        </div>

        <!-- Inline expanded edit form -->
        <div
          v-if="editingConfigId === cfg.id && editingDraft"
          class="ai-config-edit"
        >
          <div class="setting-row">
            <label class="setting-label">{{ t("aiSection.configName") }}</label>
            <div class="setting-control">
              <el-input
                v-model="draft.name"
                size="default"
                style="width: 320px"
                :placeholder="t('aiSection.configNamePlaceholder')"
              />
            </div>
          </div>

          <div class="setting-row">
            <label class="setting-label">{{ t("aiSection.provider") }}</label>
            <div class="setting-control">
              <el-select
                v-model="draft.provider"
                size="default"
                style="width: 240px"
              >
                <el-option
                  v-for="preset in PROVIDER_PRESETS"
                  :key="preset.id"
                  :label="preset.label"
                  :value="preset.id"
                />
              </el-select>
            </div>
          </div>

          <div class="setting-row">
            <label class="setting-label">{{ t("aiSection.protocol") }}</label>
            <div class="setting-control">
              <el-select
                v-model="draft.protocol"
                size="default"
                style="width: 240px"
                :disabled="protocolDisabled"
              >
                <el-option
                  :label="t('aiSection.protocolOpenai')"
                  value="openai"
                />
                <el-option
                  :label="t('aiSection.protocolAnthropic')"
                  value="anthropic"
                />
              </el-select>
            </div>
          </div>

          <div class="setting-row">
            <label class="setting-label">
              {{ t("aiSection.apiKey") }}
              <span
                v-if="apiKeyEncryptionLabel"
                class="api-key-encryption-badge"
                :class="{ 'api-key-encryption-badge--encrypted': apiKeyEncrypted }"
              >
                <el-icon :size="11">
                  <Lock v-if="apiKeyEncrypted" />
                  <Unlock v-else />
                </el-icon>
                {{ apiKeyEncryptionLabel }}
              </span>
            </label>
            <div class="setting-control">
              <el-input
                v-model="draft.apiKey"
                size="default"
                style="width: 320px"
                :type="showApiKey ? 'text' : 'password'"
                :placeholder="apiKeyPlaceholder"
              >
                <template #suffix>
                  <el-button
                    :icon="showApiKey ? View : Hide"
                    link
                    @click="showApiKey = !showApiKey"
                  />
                </template>
              </el-input>
            </div>
          </div>

          <div class="setting-row">
            <label class="setting-label">{{ t("aiSection.baseUrl") }}</label>
            <div class="setting-control">
              <el-input
                v-model="draft.baseUrl"
                size="default"
                style="width: 320px"
                :placeholder="baseUrlPlaceholder"
              />
            </div>
          </div>

          <div class="setting-row">
            <label class="setting-label">{{ t("aiSection.model") }}</label>
            <div class="setting-control">
              <el-input
                v-model="draft.model"
                size="default"
                style="width: 320px"
                placeholder="gpt-4o"
              />
            </div>
          </div>

          <div class="setting-row">
            <label class="setting-label">{{ t("aiSection.temperature") }}</label>
            <div class="setting-control">
              <el-slider
                v-model="draft.temperature"
                :min="0"
                :max="2"
                :step="0.1"
                style="width: 320px"
              />
              <span class="slider-value">{{ draft.temperature.toFixed(1) }}</span>
            </div>
          </div>

          <div class="setting-row">
            <label class="setting-label">{{ t("aiSection.maxTokens") }}</label>
            <div class="setting-control">
              <el-input-number
                v-model="draft.maxTokens"
                :min="1"
                :max="128000"
                :step="256"
                size="default"
              />
            </div>
          </div>

          <div class="setting-row">
            <label class="setting-label">{{ t("aiSection.systemPrompt") }}</label>
            <div class="setting-control" style="flex-direction: column; align-items: stretch">
              <el-input
                v-model="draft.systemPrompt"
                type="textarea"
                :rows="6"
                :placeholder="t('aiSection.systemPromptPlaceholder')"
              />
              <div class="prompt-actions">
                <el-button
                  size="small"
                  :loading="loadingPrompt === 'default'"
                  @click="loadPrompt('default')"
                >
                  {{ t("aiSection.loadDefault") }}
                </el-button>
                <el-button
                  size="small"
                  :loading="loadingPrompt === 'agent'"
                  @click="loadPrompt('agent')"
                >
                  {{ t("aiSection.loadAgent") }}
                </el-button>
                <el-button size="small" @click="resetSystemPrompt">
                  {{ t("aiSection.clearUseDefault") }}
                </el-button>
              </div>
              <span class="prompt-hint">{{ t("aiSection.promptHint") }}</span>
            </div>
          </div>

          <div class="setting-row">
            <label class="setting-label">{{ t("aiSection.connection") }}</label>
            <div class="setting-control">
              <el-button
                type="primary"
                size="default"
                :loading="testingConnection"
                @click="handleTestConnection"
              >
                {{ t("aiSection.testConnection") }}
              </el-button>
              <span v-if="testResult" class="ai-test-result">{{ testResult }}</span>
            </div>
          </div>

          <div class="ai-config-edit__footer">
            <el-button type="primary" @click="handleSave">
              {{ t("aiSection.save") }}
            </el-button>
            <el-button @click="handleCancel">
              {{ t("aiSection.cancel") }}
            </el-button>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.api-key-encryption-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  margin-left: 8px;
  padding: 1px 6px;
  font-size: 10px;
  font-weight: 500;
  border-radius: 8px;
  vertical-align: middle;
  color: var(--color-text-tertiary);
  background: var(--color-bg-surface-container-low);
  border: 1px solid var(--color-border-subtle);
}

.api-key-encryption-badge--encrypted {
  color: var(--color-success);
  background: var(--color-success-container);
  border-color: var(--color-success);
}

.ai-configs__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.ai-config-row {
  border: 1px solid var(--color-border-subtle);
  border-left: 3px solid transparent;
  border-radius: 8px;
  padding: 12px 14px;
  margin-bottom: 10px;
  background: var(--color-bg-surface-container-low);
  transition: border-color 0.15s ease, background 0.15s ease;
}

.ai-config-row--active {
  border-left-color: var(--color-primary);
  background: var(--color-primary-container);
}

.ai-config-row__top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.ai-config-row__info {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.ai-config-row__name {
  font-weight: 600;
  color: var(--color-text-primary);
}

.ai-config-row__model {
  font-size: 12px;
  color: var(--color-text-tertiary);
}

.ai-config-row__actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.ai-config-edit {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px dashed var(--color-border-subtle);
}

.ai-config-edit__footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
}

.ai-test-result {
  margin-left: 12px;
  font-size: 12px;
}
</style>
