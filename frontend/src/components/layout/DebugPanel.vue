<script setup lang="ts">
/**
 * prompt-11/12: DAP panel — stack, locals, watches, restart, conditions.
 */
import { onMounted, ref } from "vue";
import {
  debugState,
  refreshDebugStatus,
  launchDebugPackage,
  stopDebugSession,
  restartDebugSession,
  debugContinue,
  debugStepOver,
  debugStepIn,
  debugStepOut,
  selectDebugFrame,
  refreshStackAndLocals,
  addWatch,
  removeWatch,
  evaluateExpression,
  setBreakpointCondition,
  launchWithConfig,
  loadLaunchConfigs,
} from "@/stores/debug";
import { openFileFromPath } from "@/stores/editor";
import { appState } from "@/stores/app";

const condFile = ref("");
const condLine = ref(1);
const condExpr = ref("");

onMounted(() => {
  loadLaunchConfigs();
  void refreshDebugStatus();
});

async function jumpFrame(file: string, line: number, frameId: number) {
  if (frameId) await selectDebugFrame(frameId);
  if (file) {
    try {
      await openFileFromPath(file);
      appState.cursorLine = line || 1;
      appState.cursorColumn = 1;
      appState.editorJumpSeq = (appState.editorJumpSeq || 0) + 1;
    } catch {
      /* notified */
    }
  }
}

async function onAddWatch() {
  const e = debugState.watchInput.trim();
  if (!e) return;
  await addWatch(e);
  debugState.watchInput = "";
}

async function onEvaluate() {
  const e = debugState.evaluateInput.trim();
  if (!e) return;
  await evaluateExpression(e);
}

async function onSetCondition() {
  if (!condFile.value || !condLine.value) return;
  await setBreakpointCondition(condFile.value, condLine.value, condExpr.value);
}

function editCondition(b: { file: string; line: number; condition?: string }) {
  condFile.value = b.file;
  condLine.value = b.line;
  condExpr.value = b.condition || "";
}
</script>

<template>
  <div class="debug-panel">
    <div class="debug-panel__toolbar">
      <button type="button" class="debug-panel__btn" :disabled="debugState.busy" @click="launchDebugPackage">
        ▶ Start
      </button>
      <button type="button" class="debug-panel__btn" :disabled="!debugState.running" @click="restartDebugSession">
        Restart
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
      <span v-if="debugState.stopped" class="debug-panel__paused">
        · paused: <strong>{{ debugState.stopReason || "stopped" }}</strong>
      </span>
      <span v-if="debugState.mode" class="debug-panel__mode"> · {{ debugState.mode }}</span>
    </div>

    <div class="debug-panel__configs" v-if="debugState.launchConfigs.length">
      <label>Launch:</label>
      <select
        class="debug-panel__select"
        :value="debugState.activeConfigName"
        @change="
          (e) => {
            const name = (e.target as HTMLSelectElement).value;
            const cfg = debugState.launchConfigs.find((c) => c.name === name);
            if (cfg) void launchWithConfig({ ...cfg, dir: cfg.dir || appState.currentProject || '' });
          }
        "
      >
        <option value="" disabled>Select config…</option>
        <option v-for="c in debugState.launchConfigs" :key="c.name" :value="c.name">{{ c.name }}</option>
      </select>
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

        <h4 class="debug-panel__sub">Watch</h4>
        <div class="debug-panel__row">
          <input v-model="debugState.watchInput" class="debug-panel__input" placeholder="expression" @keydown.enter="onAddWatch" />
          <button type="button" class="debug-panel__btn" @click="onAddWatch">+</button>
        </div>
        <ul v-if="debugState.watches.length" class="debug-panel__list">
          <li v-for="(v, i) in debugState.watches" :key="i" class="debug-panel__item">
            <span class="debug-panel__var">{{ v.name }}</span> = {{ v.value }}
            <button type="button" class="debug-panel__x" @click="removeWatch(v.name)">×</button>
          </li>
        </ul>
        <h4 class="debug-panel__sub">Evaluate</h4>
        <div class="debug-panel__row">
          <input v-model="debugState.evaluateInput" class="debug-panel__input" placeholder="evaluate…" @keydown.enter="onEvaluate" />
          <button type="button" class="debug-panel__btn" @click="onEvaluate">Eval</button>
        </div>
        <div v-if="debugState.evaluateResult" class="debug-panel__val">{{ debugState.evaluateResult }}</div>
      </section>

      <section class="debug-panel__section">
        <h4>Breakpoints ({{ debugState.breakpoints.length }})</h4>
        <ul v-if="debugState.breakpoints.length" class="debug-panel__list">
          <li
            v-for="(b, i) in debugState.breakpoints"
            :key="i"
            class="debug-panel__item"
            :class="{ 'debug-panel__item--unverified': !b.verified && debugState.running }"
            @click="jumpFrame(b.file, b.line, 0)"
          >
            <span class="debug-panel__bp-dot" :class="b.verified || !debugState.running ? 'is-ok' : 'is-warn'" />
            {{ b.file.split(/[\\/]/).pop() }}:{{ b.line }}
            <span v-if="b.condition" class="debug-panel__cond"> if {{ b.condition }}</span>
            <span v-if="!b.verified && debugState.running" class="debug-panel__unverified" :title="b.message || 'unverified'">
              ⚠ unverified
            </span>
            <button type="button" class="debug-panel__link" @click.stop="editCondition(b)">cond</button>
          </li>
        </ul>
        <p v-else class="debug-panel__empty">F9 or glyph margin to set.</p>
        <div class="debug-panel__cond-form" v-if="condFile">
          <div class="debug-panel__loc">{{ condFile }}:{{ condLine }}</div>
          <input v-model="condExpr" class="debug-panel__input" placeholder="condition e.g. x > 0" />
          <button type="button" class="debug-panel__btn" @click="onSetCondition">Set condition</button>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.debug-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 140px;
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
  opacity: 0.9;
  border-bottom: 1px solid var(--color-border, #333);
}
.debug-panel__paused {
  color: #e3b341;
}
.debug-panel__mode {
  opacity: 0.7;
}
.debug-panel__configs {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  border-bottom: 1px solid var(--color-border, #333);
}
.debug-panel__select {
  flex: 1;
  background: #1e1e1e;
  color: inherit;
  border: 1px solid #444;
  border-radius: 4px;
  padding: 2px 6px;
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
.debug-panel__section h4,
.debug-panel__sub {
  margin: 0 0 6px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.7;
}
.debug-panel__sub {
  margin-top: 10px;
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
  position: relative;
}
.debug-panel__item:hover {
  background: rgba(255, 255, 255, 0.06);
}
.debug-panel__item--unverified {
  opacity: 0.85;
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
  font-size: 10px;
}
.debug-panel__bp-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 4px;
  vertical-align: middle;
}
.debug-panel__bp-dot.is-ok {
  background: #f85149;
}
.debug-panel__bp-dot.is-warn {
  background: transparent;
  border: 1.5px solid #e3b341;
}
.debug-panel__cond {
  color: #a5d6ff;
  font-size: 10px;
  margin-left: 4px;
}
.debug-panel__row {
  display: flex;
  gap: 4px;
  margin-bottom: 4px;
}
.debug-panel__input {
  flex: 1;
  min-width: 0;
  background: #1e1e1e;
  border: 1px solid #444;
  color: inherit;
  border-radius: 3px;
  padding: 2px 6px;
  font-size: 11px;
}
.debug-panel__x,
.debug-panel__link {
  background: none;
  border: none;
  color: #8b949e;
  cursor: pointer;
  font-size: 11px;
  margin-left: 4px;
}
.debug-panel__cond-form {
  margin-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
</style>
