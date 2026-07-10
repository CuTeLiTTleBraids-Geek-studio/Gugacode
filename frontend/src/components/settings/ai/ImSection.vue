<script setup lang="ts">
/**
 * Plan 11 Task 7 Step 6 �?IM 集成设置分区�? *
 * 展示并编�?IMConfig�? *   - provider 列表（Slack/Discord/飞书/企业微信�? 添加/编辑/删除
 *   - 每个 provider �?webhook/token/channel/mentionTrigger（G-SEC-07：编辑时明文，保存后加密�? *   - 连接测试按钮（发送测试消息）
 *   - 通知规则编辑（事件→频道→模板）
 *   - G-SEC-12：首次启用需 Approve 确认
 */
import { onMounted, ref, computed, watch } from "vue";
import { ElMessageBox } from "element-plus";
import { useI18n } from "@/lib/i18n";
import {
  imState,
  imConfig,
  loadIMConfig,
  saveIMConfig,
  approveIM,
  sendTestMessage,
} from "@/stores/im";
import type { IMProvider, IMProviderType, NotificationRule, IMConfig } from "@/stores/im";

const { t } = useI18n();

onMounted(async () => {
  await loadIMConfig();
});

// 本地编辑副本
const draftProviders = ref<IMProvider[]>([]);
const draftRules = ref<NotificationRule[]>([]);
const draftApproved = ref(false);
const editingProvider = ref<IMProvider | null>(null);
const showProviderDialog = ref(false);

function syncDraft(): void {
  const cfg = imConfig.value;
  // Backend view omits plaintext webhook/token; seed placeholders when configured.
  draftProviders.value = cfg.providers.map((p) => ({
    type: p.type,
    name: p.name,
    webhookUrl: p.webhookConfigured ? "(configured — edit to overwrite)" : "",
    botToken: p.botTokenConfigured ? "(configured — edit to overwrite)" : "",
    channelId: p.channelId ?? "",
    enabled: p.enabled,
    mentionTrigger: p.mentionTrigger ?? "",
  }));
  draftRules.value = (cfg.notificationRules ?? []).map((r) => ({ ...r }));
  draftApproved.value = cfg.approved;
}

const configVersion = computed(() => imConfig.value);
watch(configVersion, syncDraft, { immediate: true });

const providerTypes: { value: IMProviderType; label: string }[] = [
  { value: "slack", label: "Slack" },
  { value: "discord", label: "Discord" },
  { value: "feishu", label: "飞书" },
  { value: "wechat_work", label: "企业微信" },
];

const eventTypes = [
  { value: "task_completed", labelKey: "imSection.eventTaskCompleted" },
  { value: "error_alert", labelKey: "imSection.eventErrorAlert" },
  { value: "review_result", labelKey: "imSection.eventReviewResult" },
  { value: "daily_report", labelKey: "imSection.eventDailyReport" },
];

const isDirty = computed(() => {
  const cfg = imConfig.value;
  return (
    JSON.stringify(draftProviders.value) !== JSON.stringify(cfg.providers) ||
    JSON.stringify(draftRules.value) !== (JSON.stringify(cfg.notificationRules ?? [])) ||
    draftApproved.value !== cfg.approved
  );
});

function addProvider(): void {
  editingProvider.value = {
    type: "slack",
    name: "",
    webhookUrl: "",
    botToken: "",
    channelId: "",
    enabled: true,
    mentionTrigger: "",
  };
  showProviderDialog.value = true;
}

function editProvider(p: IMProvider): void {
  editingProvider.value = { ...p };
  showProviderDialog.value = true;
}

function removeProvider(idx: number): void {
  draftProviders.value.splice(idx, 1);
}

function saveProvider(): void {
  const ep = editingProvider.value;
  if (!ep || !ep.name) return;
  const idx = draftProviders.value.findIndex((p) => p.name === ep.name);
  if (idx >= 0) {
    draftProviders.value[idx] = { ...ep };
  } else {
    draftProviders.value.push({ ...ep });
  }
  showProviderDialog.value = false;
  editingProvider.value = null;
}

function addRule(): void {
  draftRules.value.push({
    event: "task_completed",
    provider: "",
    channel: "",
    template: "{title}\n{body}",
    enabled: true,
  });
}

function removeRule(idx: number): void {
  draftRules.value.splice(idx, 1);
}

/** G-SEC-12：首次启用需 Approve�?*/
async function handleSave(): Promise<void> {
  if (!imConfig.value.approved && draftApproved.value) {
    try {
      await ElMessageBox.confirm(
        t("imSection.approveConfirm"),
        t("common.confirm"),
        { type: "warning", confirmButtonText: t("common.confirm"), cancelButtonText: t("common.cancel") },
      );
    } catch {
      draftApproved.value = false;
      return;
    }
  }
  const cfg: IMConfig = {
    providers: draftProviders.value,
    notificationRules: draftRules.value,
    approved: draftApproved.value,
  };
  const ok = await saveIMConfig(cfg);
  if (ok && !imConfig.value.approved && draftApproved.value) {
    await approveIM();
  }
}

async function handleApprove(): Promise<void> {
  await approveIM();
}

async function handleTest(p: IMProvider): Promise<void> {
  const ok = await sendTestMessage(p.name, t("imSection.testMessage"));
  if (!ok && imState.error) {
    alert(imState.error);
  } else if (ok) {
    alert(t("imSection.testSuccess"));
  }
}
</script>

<template>
  <section class="settings-section">
    <h2 class="section-title">{{ t("settings.im") }}</h2>
    <p class="section-hint">{{ t("imSection.hint") }}</p>
    <p class="section-warning">
      <strong>{{ t("imSection.warningLabel") }}</strong> {{ t("imSection.warning") }}
    </p>

    <div v-if="imState.error" class="im-error">{{ imState.error }}</div>

    <!-- Approve 开�?-->
    <div class="im-field">
      <label class="im-label">
        <input type="checkbox" v-model="draftApproved" :disabled="imConfig.approved" />
        <span>{{ t("imSection.approved") }}</span>
      </label>
      <span class="im-field-hint">{{ t("imSection.approvedHint") }}</span>
      <el-button
        v-if="!imConfig.approved"
        size="small"
        type="warning"
        @click="handleApprove"
      >{{ t("imSection.approveNow") }}</el-button>
    </div>

    <!-- Provider 列表 -->
    <div class="im-section">
      <h3 class="im-subtitle">{{ t("imSection.providers") }}</h3>
      <el-button size="small" type="primary" @click="addProvider">{{ t("imSection.addProvider") }}</el-button>
    </div>

    <div v-if="draftProviders.length === 0" class="im-empty">
      {{ t("imSection.providersEmpty") }}
    </div>

    <div v-else class="im-provider-list">
      <div v-for="(p, i) in draftProviders" :key="i" class="im-provider-card">
        <div class="im-provider-header">
          <span class="im-provider-type">{{ providerTypes.find((pt) => pt.value === p.type)?.label ?? p.type }}</span>
          <code class="im-provider-name">{{ p.name }}</code>
          <span :class="p.enabled ? 'im-status im-status--on' : 'im-status im-status--off'">
            {{ p.enabled ? t("imSection.enabled") : t("imSection.disabled") }}
          </span>
        </div>
        <div class="im-provider-meta">
          <span v-if="p.channelId">{{ t("imSection.channel") }}: <code>{{ p.channelId }}</code></span>
          <span v-if="p.mentionTrigger">{{ t("imSection.mention") }}: <code>{{ p.mentionTrigger }}</code></span>
        </div>
        <div class="im-provider-actions">
          <el-button size="small" @click="editProvider(p)">{{ t("common.edit") }}</el-button>
          <el-button size="small" type="success" :disabled="!p.enabled || !imConfig.approved" @click="handleTest(p)">{{ t("imSection.test") }}</el-button>
          <el-button size="small" type="danger" @click="removeProvider(i)">{{ t("common.remove") }}</el-button>
        </div>
      </div>
    </div>

    <!-- 通知规则 -->
    <div class="im-section">
      <h3 class="im-subtitle">{{ t("imSection.notificationRules") }}</h3>
      <el-button size="small" @click="addRule">{{ t("imSection.addRule") }}</el-button>
    </div>

    <div v-if="draftRules.length === 0" class="im-empty">
      {{ t("imSection.rulesEmpty") }}
    </div>

    <div v-else class="im-rule-list">
      <div v-for="(r, i) in draftRules" :key="i" class="im-rule-card">
        <select v-model="r.event" class="im-input im-input--select">
          <option v-for="e in eventTypes" :key="e.value" :value="e.value">{{ t(e.labelKey) }}</option>
        </select>
        <input type="text" v-model="r.provider" :placeholder="t('imSection.providerPlaceholder')" class="im-input im-input--rule" />
        <input type="text" v-model="r.channel" :placeholder="t('imSection.channelPlaceholder')" class="im-input im-input--rule" />
        <label class="im-rule-enabled">
          <input type="checkbox" v-model="r.enabled" />
          <span>{{ t("imSection.enabled") }}</span>
        </label>
        <el-button size="small" type="danger" @click="removeRule(i)">{{ t("common.remove") }}</el-button>
        <textarea
          v-model="r.template"
          :placeholder="t('imSection.templatePlaceholder')"
          class="im-input im-input--textarea"
          rows="2"
        ></textarea>
      </div>
    </div>

    <!-- 保存按钮 -->
    <div class="im-actions">
      <el-button type="primary" :disabled="!isDirty" :loading="imState.saving" @click="handleSave">
        {{ t("common.save") }}
      </el-button>
    </div>

    <!-- Provider 编辑对话�?-->
    <div v-if="showProviderDialog && editingProvider" class="im-dialog-overlay" @click.self="showProviderDialog = false">
      <div class="im-dialog" role="dialog" aria-modal="true">
        <h3 class="im-dialog__title">{{ t("imSection.editProvider") }}</h3>
        <div class="im-dialog__field">
          <label class="im-label">{{ t("imSection.providerType") }}</label>
          <select v-model="editingProvider.type" class="im-input im-input--select">
            <option v-for="pt in providerTypes" :key="pt.value" :value="pt.value">{{ pt.label }}</option>
          </select>
        </div>
        <div class="im-dialog__field">
          <label class="im-label">{{ t("imSection.providerName") }}</label>
          <input type="text" v-model="editingProvider.name" class="im-input" />
        </div>
        <div class="im-dialog__field">
          <label class="im-label">{{ t("imSection.webhookUrl") }}</label>
          <input type="text" v-model="editingProvider.webhookUrl" :placeholder="t('imSection.webhookPlaceholder')" class="im-input" />
        </div>
        <div class="im-dialog__field">
          <label class="im-label">{{ t("imSection.botToken") }}</label>
          <input type="password" v-model="editingProvider.botToken" :placeholder="t('imSection.tokenPlaceholder')" class="im-input" />
        </div>
        <div class="im-dialog__field">
          <label class="im-label">{{ t("imSection.channel") }}</label>
          <input type="text" v-model="editingProvider.channelId" class="im-input" />
        </div>
        <div class="im-dialog__field">
          <label class="im-label">{{ t("imSection.mention") }}</label>
          <input type="text" v-model="editingProvider.mentionTrigger" :placeholder="t('imSection.mentionPlaceholder')" class="im-input" />
        </div>
        <div class="im-dialog__field">
          <label class="im-label">
            <input type="checkbox" v-model="editingProvider.enabled" />
            <span>{{ t("imSection.enabled") }}</span>
          </label>
        </div>
        <div class="im-dialog__actions">
          <el-button size="small" @click="showProviderDialog = false">{{ t("common.cancel") }}</el-button>
          <el-button size="small" type="primary" @click="saveProvider">{{ t("common.save") }}</el-button>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.section-hint {
  font-size: 13px;
  color: var(--color-text-secondary);
  margin-bottom: 12px;
  line-height: 1.5;
}

.section-warning {
  font-size: 12px;
  color: var(--color-text-tertiary);
  margin-bottom: 20px;
  padding: 8px 12px;
  background: var(--color-bg-surface-container-low);
  border-radius: var(--radius-sm);
  border-left: 3px solid var(--color-warning, #ff9800);
  line-height: 1.5;
}

.im-error {
  color: var(--color-error, #f44336);
  font-size: 12px;
  margin-bottom: 16px;
}

.im-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 16px;
}

.im-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--color-text-primary);
  font-weight: 500;
}

.im-field-hint {
  font-size: 11px;
  color: var(--color-text-tertiary);
  line-height: 1.4;
}

.im-section {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.im-subtitle {
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.im-empty {
  font-size: 13px;
  color: var(--color-text-tertiary);
  padding: 16px 0;
  text-align: center;
  background: var(--color-bg-surface-container-low);
  border-radius: var(--radius-sm);
  margin-bottom: 16px;
}

.im-provider-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 24px;
}

.im-provider-card {
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.im-provider-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.im-provider-type {
  font-size: 12px;
  font-weight: 600;
  color: var(--color-primary, #2196f3);
}

.im-provider-name {
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--color-text-primary);
  background: var(--color-bg-surface-container);
  padding: 1px 6px;
  border-radius: var(--radius-xs);
}

.im-status {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: var(--radius-xs);
  text-transform: uppercase;
  font-weight: 500;
}

.im-status--on {
  color: var(--color-success, #4caf50);
  background: var(--color-success-container, rgba(76, 175, 80, 0.1));
}

.im-status--off {
  color: var(--color-text-tertiary);
  background: var(--color-bg-surface-container);
}

.im-provider-meta {
  font-size: 12px;
  color: var(--color-text-tertiary);
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.im-provider-meta code {
  font-family: var(--font-mono);
  font-size: 11px;
}

.im-provider-actions {
  display: flex;
  gap: 6px;
}

.im-rule-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 24px;
}

.im-rule-card {
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  padding: 10px 12px;
  display: grid;
  grid-template-columns: 140px 1fr 1fr auto auto;
  gap: 8px;
  align-items: center;
}

.im-rule-enabled {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.im-input {
  font-family: var(--font-mono);
  font-size: 12px;
  padding: 6px 8px;
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-xs);
  background: var(--color-bg-surface);
  color: var(--color-text-primary);
  width: 100%;
  box-sizing: border-box;
}

.im-input--select {
  font-family: var(--font-sans);
}

.im-input--rule {
  width: auto;
}

.im-input--textarea {
  grid-column: 1 / -1;
  resize: vertical;
  min-height: 40px;
}

.im-actions {
  margin-bottom: 24px;
}

.im-dialog-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.im-dialog {
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md, 8px);
  padding: 24px;
  width: 480px;
  max-width: 90vw;
  max-height: 90vh;
  overflow-y: auto;
}

.im-dialog__title {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 16px;
  color: var(--color-text-primary);
}

.im-dialog__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 14px;
}

.im-dialog__actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 16px;
}
</style>
