<script setup lang="ts">
import { ref } from "vue";
import { appState, saveSettings } from "@/stores/app";
import { fileService, logLevelService } from "@/api/services";
import { Folder, Document } from "@element-plus/icons-vue";
import { useI18n } from "@/lib/i18n";

const { t } = useI18n();

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

// --- Application log viewer (N-11) ---
const logPath = ref<string>("");
const logContent = ref<string>("");
const logModalVisible = ref(false);
const logLoading = ref(false);
const logError = ref<string>("");

async function loadLogPath() {
  try {
    logPath.value = await logLevelService.getLogPath();
  } catch (e) {
    console.error("Failed to get log path:", e);
    logPath.value = "";
  }
}

async function handleViewLog() {
  logModalVisible.value = true;
  logLoading.value = true;
  logError.value = "";
  logContent.value = "";
  try {
    if (!logPath.value) {
      await loadLogPath();
    }
    // 64 KiB tail is plenty for in-app inspection.
    logContent.value = await logLevelService.readLog(64 * 1024);
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e);
    logError.value = msg;
  } finally {
    logLoading.value = false;
  }
}

// Load the path lazily when the section is first rendered so the UI can
// display it next to the View Log button.
loadLogPath();
</script>

<template>
  <section class="settings-section">
    <h2 class="section-title">{{ t("settings.general") }}</h2>

    <div class="setting-row">
      <label class="setting-label">{{ t("general.language") }}</label>
      <div class="setting-control">
        <el-select
          v-model="appState.language"
          size="default"
          style="width: 180px"
          :aria-label="t('general.language')"
          @change="saveSettings"
        >
          <el-option label="English" value="en" />
          <el-option label="中文" value="zh" />
          <el-option label="日本語" value="ja" />
        </el-select>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("general.openAIWindowOnStartup") }}</label>
      <div class="setting-control">
        <el-switch
          v-model="appState.openAIWindowOnStartup"
          :aria-label="t('general.openAIWindowOnStartup')"
          @change="saveSettings"
        />
      </div>
      <p class="setting-hint">{{ t("general.openAIWindowOnStartupHint") }}</p>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("general.autoUpdate") }}</label>
      <div class="setting-control">
        <el-switch v-model="appState.autoUpdate" :aria-label="t('general.autoUpdate')" @change="saveSettings" />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("general.pluginSandbox") }}</label>
      <div class="setting-control">
        <el-switch
          v-model="appState.enablePluginSandbox"
          :aria-label="t('general.pluginSandbox')"
          @change="saveSettings"
        />
        <span class="setting-hint">{{ t("general.pluginSandboxHint") }}</span>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("general.dataFolder") }}</label>
      <div class="setting-control">
        <el-input
          v-model="appState.dataFolderPath"
          size="default"
          style="width: 320px"
          readonly
          :aria-label="t('general.dataFolder')"
        >
          <template #append>
            <el-button :icon="Folder" @click="handleBrowseFolder" :aria-label="t('common.browse')" />
          </template>
        </el-input>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("general.applicationLog") }}</label>
      <div class="setting-control log-control">
        <span class="log-path" :title="logPath">{{ logPath || t("general.logUnavailable") }}</span>
        <el-button :icon="Document" size="default" @click="handleViewLog" :aria-label="t('general.viewLog')">
          {{ t("general.viewLog") }}
        </el-button>
      </div>
    </div>

    <el-dialog
      v-model="logModalVisible"
      :title="t('general.logViewerTitle')"
      width="80%"
      top="6vh"
      :close-on-click-modal="false"
      :aria-label="t('general.logViewerTitle')"
    >
      <div v-loading="logLoading" class="log-modal-body">
        <p v-if="logError" class="log-error" role="alert">{{ t("general.logReadFailed", { error: logError }) }}</p>
        <pre v-else-if="logContent" class="log-pre">{{ logContent }}</pre>
        <p v-else class="log-empty">{{ t("general.logEmpty") }}</p>
      </div>
      <template #footer>
        <el-button @click="logModalVisible = false">{{ t("general.logClose") }}</el-button>
        <el-button type="primary" :loading="logLoading" @click="handleViewLog">{{ t("general.logRefresh") }}</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.setting-hint {
  margin-left: 12px;
  font-size: 12px;
  color: var(--color-text-tertiary, #888);
}

.log-control {
  align-items: center;
  gap: 12px;
}

.log-path {
  display: inline-block;
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-mono, monospace);
  font-size: 12px;
  color: var(--color-text-tertiary, #888);
}

.log-modal-body {
  min-height: 200px;
  max-height: 70vh;
  overflow-y: auto;
}

.log-pre {
  margin: 0;
  padding: 12px;
  background: var(--color-bg-surface-container, #1e1e1e);
  color: var(--color-text-primary, #d4d4d4);
  font-family: var(--font-mono, monospace);
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
  border-radius: 4px;
}

.log-error {
  color: var(--color-danger, #f56c6c);
  font-size: 13px;
}

.log-empty {
  color: var(--color-text-tertiary, #888);
  font-size: 13px;
  font-style: italic;
}
</style>
