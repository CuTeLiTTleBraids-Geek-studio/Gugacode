<script setup lang="ts">
import { appState, saveSettings } from "@/stores/app";
import { useI18n } from "@/lib/i18n";

const { t } = useI18n();
</script>

<template>
  <section class="settings-section">
    <h2 class="section-title">{{ t("editorSection.title") }}</h2>

    <div class="setting-row">
      <label class="setting-label">{{ t("editorSection.fontSize") }}</label>
      <div class="setting-control">
        <el-input-number
          v-model="appState.fontSize"
          :min="8"
          :max="32"
          :step="1"
          size="default"
          :aria-label="t('editorSection.fontSizeAria')"
          @change="saveSettings"
        />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("editorSection.fontFamily") }}</label>
      <div class="setting-control">
        <el-input
          v-model="appState.fontFamily"
          size="default"
          style="width: 320px"
          :aria-label="t('editorSection.fontFamilyAria')"
          @change="saveSettings"
        />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("editorSection.tabSize") }}</label>
      <div class="setting-control">
        <el-input-number
          v-model="appState.tabSize"
          :min="1"
          :max="8"
          :step="1"
          size="default"
          :aria-label="t('editorSection.tabSizeAria')"
          @change="saveSettings"
        />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("editorSection.wordWrap") }}</label>
      <div class="setting-control">
        <el-switch v-model="appState.wordWrap" :aria-label="t('editorSection.wordWrapAria')" @change="saveSettings" />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("editorSection.lineNumbers") }}</label>
      <div class="setting-control">
        <el-switch v-model="appState.lineNumbers" :aria-label="t('editorSection.lineNumbersAria')" @change="saveSettings" />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("editorSection.minimap") }}</label>
      <div class="setting-control">
        <el-switch v-model="appState.minimap" :aria-label="t('editorSection.minimapAria')" @change="saveSettings" />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("editorSection.cursorBlinking") }}</label>
      <div class="setting-control">
        <el-select
          v-model="appState.cursorBlinking"
          size="default"
          style="width: 180px"
          :aria-label="t('editorSection.cursorBlinkingAria')"
          @change="saveSettings"
        >
          <el-option :label="t('editorSection.cursorBlinkBlink')" value="blink" />
          <el-option :label="t('editorSection.cursorBlinkSmooth')" value="smooth" />
          <el-option :label="t('editorSection.cursorBlinkPhase')" value="phase" />
          <el-option :label="t('editorSection.cursorBlinkExpand')" value="expand" />
          <el-option :label="t('editorSection.cursorBlinkSolid')" value="solid" />
        </el-select>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("editorSection.cursorStyle") }}</label>
      <div class="setting-control">
        <el-select
          v-model="appState.cursorStyle"
          size="default"
          style="width: 180px"
          :aria-label="t('editorSection.cursorStyleAria')"
          @change="saveSettings"
        >
          <el-option :label="t('editorSection.cursorStyleLine')" value="line" />
          <el-option :label="t('editorSection.cursorStyleBlock')" value="block" />
          <el-option :label="t('editorSection.cursorStyleUnderline')" value="underline" />
          <el-option :label="t('editorSection.cursorStyleLineThin')" value="line-thin" />
          <el-option :label="t('editorSection.cursorStyleBlockOutline')" value="block-outline" />
          <el-option :label="t('editorSection.cursorStyleUnderlineThin')" value="underline-thin" />
        </el-select>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("editorSection.bracketColorization") }}</label>
      <div class="setting-control">
        <el-switch v-model="appState.bracketColorization" :aria-label="t('editorSection.bracketColorizationAria')" @change="saveSettings" />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("editorSection.autoSave") }}</label>
      <div class="setting-control">
        <el-switch v-model="appState.autoSave" :aria-label="t('editorSection.autoSaveAria')" @change="saveSettings" />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("editorSection.autoSaveDelay") }}</label>
      <div class="setting-control">
        <el-select
          v-model="appState.autoSaveDelay"
          size="default"
          style="width: 180px"
          :disabled="!appState.autoSave"
          :aria-label="t('editorSection.autoSaveDelayAria')"
          @change="saveSettings"
        >
          <el-option :label="t('editorSection.autoSaveDelay1s')" value="1000" />
          <el-option :label="t('editorSection.autoSaveDelay5s')" value="5000" />
          <el-option :label="t('editorSection.autoSaveDelay10s')" value="10000" />
          <el-option :label="t('editorSection.autoSaveDelay30s')" value="30000" />
        </el-select>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">{{ t("editorSection.inlineCompletion") }}</label>
      <div class="setting-control">
        <el-switch
          v-model="appState.inlineCompletionEnabled"
          :aria-label="t('editorSection.inlineCompletionAria')"
          @change="saveSettings"
        />
        <span class="setting-hint">{{ t("editorSection.inlineCompletionHint") }}</span>
      </div>
    </div>
  </section>
</template>

<style scoped>
.setting-hint {
  margin-left: 12px;
  font-size: 11px;
  color: var(--color-text-tertiary, #909399);
}
</style>
