<script setup lang="ts">
/**
 * prompt-11 11-A: in-IDE debug panel — stack, locals, continue/step.
 */
import { onMounted, onBeforeUnmount } from "vue";
import {
  debugState,
  refreshDebugStatus,
  launchDebugPackage,
  stopDebugSession,
  debugContinue,
  debugStepOver,
  debugStepIn,
  debugStepOut,
  selectDebugFrame,
  refreshStackAndLocals,
} from "@/stores/debug";
import { openFileFromPath } from "@/stores/editor";
import { appState } from "@/stores/app";

onMounted(() => {
  void refreshDebugStatus();
});

async function jumpFrame(file: string, line: number, frameId: number) {
  await selectDebugFrame(frameId);
  if (file) {
    try {
      await openFileFromPath(file);
      appState.cursorLine = line || 1;
      appState.cursorColumn = 1;
      appState.editorJumpSeq = (appState.editorJumpSeq || 0) + 1;
    } catch {
      /* notify already */
    }
  }
}
</script>

<template>
  <div class="debug-panel">
    <div class="debug-panel__toolbar">
      <button type="button" class="debug-panel__btn" :disabled="debugState.busy" @click="launchDebugPackage">
        ▶ Start
      </button>
      <button type="button" class="debug-panel__btn" :disabled="!debugState.running || !debugState.stopped" @click="debugContinue">
        Continue
      </button>
      <button type="button" class="debug-panel__btn" :disabled="!debugState.running || !debugState.stopped" @click="debugStepOver">
        Step Over
      </button>
      <button type="button" class="debug-panel__btn" :disabled="!debugState.running || !debugState.stopped" @click="debugStepIn">
        Step In
      </button>
      <button type="button" class="debug-panel__btn" :disabled="!debugState.running || !debugState.stopped" @click="debugStepOut">
        Step Out
      </button>
      <button type="button" class="debug-panel__btn" :disabled="!debugState.running" @click="stopDebugSession">
        Stop
      </button>
      <button type="button" class="debug-panel__btn" :disabled="!debugState.running" @click="refreshStackAndLocals">
        Refresh
      </button>
    </div>
    <div class="debug-panel__status">
      {{ debugState.message || (debugState.available ? "Delve ready" : "Delve not found") }}
      <span v-if="debugState.stopped" class="debug-panel__paused"> · paused ({{ debugState.stopReason }})</span>
    </div>
    <div class="debug-panel__cols">
      <section class="debug-panel__section">
        <h4>Call stack</h4>
        <ul v-if="debugState.stack.length" class="debug-panel__list">
          <li
            v-for="f in debugState.stack"
            :key="f.id"
            class="debug-panel__item"
            @click="jumpFrame(f.file, f.line, f.id)"
          >
            <div class="debug-panel__name">{{ f.name }}</div>
            <div class="debug-panel__loc">{{ f.file }}:{{ f.line }}</div>
          </li>
        </ul>
        <p v-else class="debug-panel__empty">No frames — hit a breakpoint or step.</p>
      </section>
      <section class="debug-panel__section">
        <h4>Locals</h4>
        <ul v-if="debugState.locals.length" class="debug-panel__list">
          <li v-for="(v, i) in debugState.locals" :key="i" class="debug-panel__item">
            <span class="debug-panel__var">{{ v.name }}</span>
            <span class="debug-panel__type" v-if="v.type">: {{ v.type }}</span>
            <div class="debug-panel__val">{{ v.value }}</div>
          </li>
        </ul>
        <p v-else class="debug-panel__empty">No locals.</p>
      </section>
      <section class="debug-panel__section">
        <h4>Breakpoints ({{ debugState.breakpoints.length }})</h4>
        <ul v-if="debugState.breakpoints.length" class="debug-panel__list">
          <li
            v-for="(b, i) in debugState.breakpoints"
            :key="i"
            class="debug-panel__item"
            @click="jumpFrame(b.file, b.line, 0)"
          >
            {{ b.file }}:{{ b.line }}
            <span v-if="!b.verified" class="debug-panel__unverified">?</span>
          </li>
        </ul>
        <p v-else class="debug-panel__empty">F9 or click glyph margin to set.</p>
      </section>
    </div>
  </div>
</template>

<style scoped>
.debug-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 120px;
  font-size: 12px;
  color: var(--color-text-secondary, #ccc);
}
.debug-panel__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding: 6px 8px;
  border-bottom: 1px solid var(--color-border, #333);
}
.debug-panel__btn {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 4px;
  border: 1px solid var(--color-border, #444);
  background: var(--color-bg-elevated, #2a2a2c);
  color: inherit;
  cursor: pointer;
}
.debug-panel__btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.debug-panel__status {
  padding: 4px 8px;
  opacity: 0.85;
  border-bottom: 1px solid var(--color-border, #333);
}
.debug-panel__paused {
  color: #e3b341;
}
.debug-panel__cols {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 0;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
.debug-panel__section {
  overflow: auto;
  padding: 6px 8px;
  border-right: 1px solid var(--color-border, #333);
}
.debug-panel__section h4 {
  margin: 0 0 6px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.7;
}
.debug-panel__list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.debug-panel__item {
  padding: 4px 2px;
  cursor: pointer;
  border-radius: 3px;
}
.debug-panel__item:hover {
  background: rgba(255, 255, 255, 0.06);
}
.debug-panel__name {
  font-weight: 500;
  color: var(--color-text-primary, #eee);
}
.debug-panel__loc,
.debug-panel__val {
  opacity: 0.75;
  word-break: break-all;
}
.debug-panel__var {
  color: #79c0ff;
}
.debug-panel__type {
  opacity: 0.6;
}
.debug-panel__empty {
  opacity: 0.5;
  margin: 0;
}
.debug-panel__unverified {
  color: #e3b341;
  margin-left: 4px;
}
</style>
