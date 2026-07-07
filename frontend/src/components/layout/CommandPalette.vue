<script setup lang="ts">
import { ref, computed, watch, nextTick } from "vue";
import type { Command } from "@/types";
import { useI18n } from "@/lib/i18n";

const props = defineProps<{
  visible: boolean;
  commands: Command[];
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "run", command: Command): void;
}>();

const { t } = useI18n();

const query = ref("");
const selectedIndex = ref(0);
const inputRef = ref<HTMLInputElement | null>(null);
const dialogRef = ref<HTMLElement | null>(null);
// N-126: remember the element that had focus before opening so we can
// restore it when the dialog closes.
let previouslyFocused: HTMLElement | null = null;

const filtered = computed(() => {
  const q = query.value.toLowerCase().trim();
  if (!q) return props.commands;
  return props.commands.filter((c) =>
    c.label.toLowerCase().includes(q),
  );
});

watch(
  () => props.visible,
  (v) => {
    if (v) {
      // N-126: capture the trigger element so we can restore focus on close.
      previouslyFocused = document.activeElement as HTMLElement | null;
      query.value = "";
      selectedIndex.value = 0;
      nextTick(() => inputRef.value?.focus());
    } else {
      // N-126: restore focus to the trigger element on close.
      previouslyFocused?.focus?.();
      previouslyFocused = null;
    }
  },
);

watch(filtered, () => {
  selectedIndex.value = 0;
});

function handleKeydown(e: KeyboardEvent) {
  if (e.key === "ArrowDown") {
    e.preventDefault();
    selectedIndex.value = Math.min(selectedIndex.value + 1, filtered.value.length - 1);
  } else if (e.key === "ArrowUp") {
    e.preventDefault();
    selectedIndex.value = Math.max(selectedIndex.value - 1, 0);
  } else if (e.key === "Enter") {
    e.preventDefault();
    const cmd = filtered.value[selectedIndex.value];
    if (cmd) emit("run", cmd);
  } else if (e.key === "Escape") {
    emit("close");
  }
}

// N-126: focus trap — when Tab/Shift+Tab is pressed inside the dialog,
// cycle focus among the dialog's focusable elements instead of letting
// it escape to the underlying UI.
function handleTab(e: KeyboardEvent) {
  const root = dialogRef.value;
  if (!root) return;
  const focusable = Array.from(
    root.querySelectorAll<HTMLElement>(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((el) => !el.hasAttribute("disabled") && el.offsetParent !== null);
  if (focusable.length === 0) return;
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (e.shiftKey) {
    if (document.activeElement === first || document.activeElement === root) {
      e.preventDefault();
      last.focus();
    }
  } else {
    if (document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }
}
</script>

<template>
  <transition name="cmd-fade">
    <div v-if="visible" class="command-palette-overlay" @click="emit('close')">
      <div
        ref="dialogRef"
        class="command-palette"
        role="dialog"
        aria-modal="true"
        :aria-label="t('commandPalette.title')"
        tabindex="-1"
        @click.stop
        @keydown.tab="handleTab"
      >
        <input
          ref="inputRef"
          v-model="query"
          class="command-palette__input"
          :placeholder="t('commandPalette.placeholder')"
          :aria-label="t('commandPalette.inputAria')"
          role="combobox"
          aria-expanded="true"
          :aria-activedescendant="selectedIndex >= 0 && filtered[selectedIndex] ? `cmd-item-${selectedIndex}` : undefined"
          @keydown="handleKeydown"
        />
        <div class="command-palette__list" role="listbox" :aria-label="t('commandPalette.title')">
          <div v-if="filtered.length === 0" class="command-palette__empty">
            {{ t('commandPalette.noMatches') }}
          </div>
          <button
            v-for="(cmd, i) in filtered"
            :id="`cmd-item-${i}`"
            :key="cmd.id"
            type="button"
            class="command-palette__item"
            :class="{ 'command-palette__item--active': i === selectedIndex }"
            role="option"
            :aria-selected="i === selectedIndex"
            @click="emit('run', cmd)"
            @mouseenter="selectedIndex = i"
          >
            <span class="command-palette__label">{{ cmd.label }}</span>
            <span v-if="cmd.shortcut" class="command-palette__shortcut">{{ cmd.shortcut }}</span>
          </button>
        </div>
      </div>
    </div>
  </transition>
</template>

<style scoped>
.command-palette-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: rgba(0, 0, 0, 0.4);
  z-index: 1000;
  display: flex;
  justify-content: center;
  align-items: flex-start;
  padding-top: 80px;
}

.command-palette {
  width: 520px;
  max-width: 90vw;
  background-color: var(--color-bg-surface);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md, 12px);
  overflow: hidden;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3);
}

.command-palette__input {
  width: 100%;
  padding: 12px 16px;
  font-size: 14px;
  font-family: var(--font-sans);
  color: var(--color-text-primary);
  background-color: transparent;
  border: none;
  border-bottom: 1px solid var(--color-border-subtle);
  outline: none;
}

.command-palette__input::placeholder {
  color: var(--color-text-tertiary);
}

.command-palette__list {
  max-height: 320px;
  overflow-y: auto;
  padding: 4px;
}

.command-palette__empty {
  padding: 16px;
  font-size: 12px;
  color: var(--color-text-tertiary);
  text-align: center;
}

.command-palette__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 8px 12px;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm, 8px);
  cursor: pointer;
  text-align: left;
  color: var(--color-text-primary);
  font-size: 13px;
  transition: background-color 80ms ease;
}

.command-palette__item--active {
  background-color: color-mix(in srgb, var(--color-primary) 12%, transparent);
}

.command-palette__shortcut {
  font-size: 11px;
  color: var(--color-text-tertiary);
  font-family: var(--font-mono);
}

.cmd-fade-enter-active,
.cmd-fade-leave-active {
  transition: opacity 120ms ease;
}

.cmd-fade-enter-from,
.cmd-fade-leave-to {
  opacity: 0;
}
</style>
