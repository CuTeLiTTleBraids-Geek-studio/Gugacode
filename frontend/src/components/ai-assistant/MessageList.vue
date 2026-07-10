<script setup lang="ts">
// Plan 11 Task 1 — 中间消息列表骨架。复用 aiState（与嵌入式 AiChatPanel
// 共享同一 reactive 实例，切换不丢失）。Markdown 渲染走 MarkdownContent
// SFC + renderMarkdown（内部 DOMPurify 净化），禁 v-html（G-SEC-11）。
// Task 2/3 充实滚动/流式/工具调用展示。
// Task 15 Step 7: 气泡样式由 personalization.bubbleStyle 驱动（rounded/sharp/bubble）。
import { computed } from "vue";
import { useI18n } from "@/lib/i18n";
import { aiState } from "@/stores/ai";
import { appState } from "@/stores/app";
import { renderMarkdown } from "@/lib/markdown";
import MarkdownContent from "@/components/common/MarkdownContent.vue";

const { t } = useI18n();

const bubbleClass = computed(() => {
  const style = appState.personalization.bubbleStyle ?? "rounded";
  return `ai-msg--${style}`;
});
</script>

<template>
  <div class="ai-msg-list">
    <div v-if="aiState.messages.length === 0" class="ai-msg-list__empty">
      {{ t("aiAssistant.emptyHint") }}
    </div>
    <div
      v-for="(msg, i) in aiState.messages"
      :key="i"
      class="ai-msg"
      :class="[`ai-msg--${msg.role}`, bubbleClass]"
    >
      <div class="ai-msg__role">{{ msg.role }}</div>
      <MarkdownContent
        v-if="msg.role !== 'user'"
        class="ai-msg__body markdown-body"
        :html="renderMarkdown(msg.content)"
      />
      <div v-else class="ai-msg__body">{{ msg.content }}</div>
    </div>
  </div>
</template>

<style scoped>
.ai-msg-list {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.ai-msg-list__empty {
  margin: auto;
  color: var(--color-text-secondary, #888);
  font-size: 13px;
}
.ai-msg {
  padding: 10px 12px;
  border-radius: 8px;
  max-width: 80%;
}
.ai-msg--user {
  align-self: flex-end;
  background: var(--color-accent, #3b82f6);
  color: #fff;
}
.ai-msg--assistant {
  align-self: flex-start;
  background: var(--color-bg-elevated, #252525);
}
.ai-msg__role {
  font-size: 11px;
  opacity: 0.7;
  margin-bottom: 4px;
  text-transform: uppercase;
}
</style>
