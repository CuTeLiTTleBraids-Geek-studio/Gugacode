<script setup lang="ts">
import { useRouter } from "vue-router";
import { FolderOpened, DocumentAdd, Clock, Monitor, Setting, Key, Notebook } from "@element-plus/icons-vue";
import { fileService, projectService } from "@/api/services";
import { openProject } from "@/stores/app";
import { useI18n } from "@/lib/i18n";

const { t } = useI18n();

const router = useRouter();

async function handleOpenProject() {
  const path = await fileService.pickDirectory();
  if (!path) return;
  const project = await projectService.addProject(path);
  openProject(project.name, project.path);
  router.push("/editor");
}

function handleNewProject() {
  router.push("/projects");
}

function handleRecentProjects() {
  router.push("/projects");
}

function handleQuickAction(action: string) {
  switch (action) {
    case "terminal":
      router.push("/editor");
      break;
    case "settings":
      router.push("/settings");
      break;
    case "shortcuts":
      router.push("/settings");
      break;
    case "docs":
      window.open("https://v3.wails.io/", "_blank");
      break;
  }
}
</script>

<template>
  <div class="welcome-page">
    <div class="welcome-inner">
      <!-- Hero -->
      <div class="welcome-hero anim-1">
        <h1 class="welcome-title">{{ t('app.name') }}</h1>
        <p class="welcome-tagline">{{ t("welcome.tagline") }}</p>
      </div>

      <!-- Separator -->
      <div class="welcome-divider anim-2" aria-hidden="true"></div>

      <!-- Primary Actions -->
      <nav class="welcome-actions anim-3" :aria-label="t('welcome.primaryActionsAria')">
        <button type="button" class="action-btn" :aria-label="t('welcome.openProject')" @click="handleOpenProject">
          <el-icon :size="18"><FolderOpened /></el-icon>
          <span>{{ t("welcome.openProject") }}</span>
        </button>

        <span class="action-sep" aria-hidden="true"></span>

        <button type="button" class="action-btn" :aria-label="t('welcome.newProject')" @click="handleNewProject">
          <el-icon :size="18"><DocumentAdd /></el-icon>
          <span>{{ t("welcome.newProject") }}</span>
        </button>

        <span class="action-sep" aria-hidden="true"></span>

        <button type="button" class="action-btn" :aria-label="t('welcome.recentProjects')" @click="handleRecentProjects">
          <el-icon :size="18"><Clock /></el-icon>
          <span>{{ t("welcome.recentProjects") }}</span>
        </button>
      </nav>

      <!-- Quick Actions -->
      <div class="welcome-quick anim-4" :aria-label="t('welcome.quickActionsAria')">
        <button type="button" class="quick-link" :aria-label="t('welcome.openTerminalAria')" @click="handleQuickAction('terminal')">
          <el-icon :size="13"><Monitor /></el-icon>
          <span>{{ t("welcome.terminal") }}</span>
        </button>

        <span class="quick-sep" aria-hidden="true">&middot;</span>

        <button type="button" class="quick-link" :aria-label="t('welcome.settings')" @click="handleQuickAction('settings')">
          <el-icon :size="13"><Setting /></el-icon>
          <span>{{ t("welcome.settings") }}</span>
        </button>

        <span class="quick-sep" aria-hidden="true">&middot;</span>

        <button type="button" class="quick-link" :aria-label="t('welcome.keyboardShortcutsAria')" @click="handleQuickAction('shortcuts')">
          <el-icon :size="13"><Key /></el-icon>
          <span>{{ t("welcome.keys") }}</span>
        </button>

        <span class="quick-sep" aria-hidden="true">&middot;</span>

        <button type="button" class="quick-link" :aria-label="t('welcome.documentationAria')" @click="handleQuickAction('docs')">
          <el-icon :size="13"><Notebook /></el-icon>
          <span>{{ t("welcome.docs") }}</span>
        </button>
      </div>

      <!-- Footer -->
      <div class="welcome-footer anim-5">
        <span>v0.1.0</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* ── Layout ─────────────────────────────────────────────── */

.welcome-page {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  min-height: 100vh;
  padding: 24px;
  background: var(--color-bg-base);
}

.welcome-inner {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 36px;
  max-width: 560px;
  width: 100%;
}

/* ── Hero ───────────────────────────────────────────────── */

.welcome-hero {
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}

.welcome-title {
  font-size: 48px;
  font-weight: 300;
  letter-spacing: -0.03em;
  line-height: 1.1;
  margin: 0;
  color: var(--color-primary);
}

.welcome-tagline {
  font-size: 11px;
  font-weight: 500;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--color-text-tertiary);
  margin: 0;
}

/* ── Divider ───────────────────────────────────────────── */

.welcome-divider {
  width: 48px;
  height: 1px;
  background: var(--color-outline-variant);
}

/* ── Primary Actions ────────────────────────────────────── */

.welcome-actions {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  flex-wrap: wrap;
}

.action-btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 12px 20px;
  border: none;
  border-radius: var(--radius-md, 12px);
  background: transparent;
  color: var(--color-text-primary);
  font-family: var(--font-sans);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: background var(--transition-fast),
              box-shadow var(--transition-fast);
  text-decoration: none;
  position: relative;
}

.action-btn:hover {
  background: var(--color-bg-surface-container);
  box-shadow: var(--shadow-1);
}

.action-btn:active {
  transform: scale(0.98);
}

.action-btn:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 4px;
  border-radius: var(--radius-md, 12px);
}

.action-sep {
  width: 3px;
  height: 3px;
  border-radius: 50%;
  background: var(--color-text-tertiary);
  opacity: 0.5;
  flex-shrink: 0;
}

/* ── Quick Actions ───────────────────────────────────────── */

.welcome-quick {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 2px;
  flex-wrap: wrap;
}

.quick-link {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 8px 12px;
  border: none;
  border-radius: var(--radius-sm, 8px);
  background: transparent;
  color: var(--color-text-secondary);
  font-family: var(--font-sans);
  font-size: 12px;
  font-weight: 400;
  cursor: pointer;
  transition: color var(--transition-fast),
              background var(--transition-fast);
  text-decoration: none;
}

.quick-link:hover {
  color: var(--color-primary);
  background: var(--color-primary-container);
}

.quick-link:active {
  opacity: 0.8;
}

.quick-link:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
  border-radius: var(--radius-sm, 8px);
}

.quick-sep {
  color: var(--color-text-tertiary);
  font-size: 14px;
  opacity: 0.4;
  user-select: none;
  line-height: 1;
}

/* ── Footer ──────────────────────────────────────────────── */

.welcome-footer {
  font-size: 11px;
  color: var(--color-text-disabled);
  letter-spacing: 0.02em;
}

/* ── Staggered Entrance Animation ───────────────────────── */

@keyframes fadeUp {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.anim-1 {
  animation: fadeUp var(--duration-medium2, 500ms) var(--ease-emphasized-decelerate, cubic-bezier(0.2, 0, 0, 1)) 0ms both;
}

.anim-2 {
  animation: fadeUp var(--duration-medium2, 500ms) var(--ease-emphasized-decelerate, cubic-bezier(0.2, 0, 0, 1)) 50ms both;
}

.anim-3 {
  animation: fadeUp var(--duration-medium2, 500ms) var(--ease-emphasized-decelerate, cubic-bezier(0.2, 0, 0, 1)) 100ms both;
}

.anim-4 {
  animation: fadeUp var(--duration-medium2, 500ms) var(--ease-emphasized-decelerate, cubic-bezier(0.2, 0, 0, 1)) 150ms both;
}

.anim-5 {
  animation: fadeUp var(--duration-medium2, 500ms) var(--ease-emphasized-decelerate, cubic-bezier(0.2, 0, 0, 1)) 200ms both;
}

/* ── Reduced Motion ──────────────────────────────────────── */

@media (prefers-reduced-motion: reduce) {
  .anim-1,
  .anim-2,
  .anim-3,
  .anim-4,
  .anim-5 {
    animation: none;
    opacity: 1;
    transform: none;
  }
}

/* ── Responsive ─────────────────────────────────────────── */

@media (max-width: 480px) {
  .welcome-title {
    font-size: 32px;
  }

  .welcome-inner {
    gap: 28px;
  }

  .welcome-actions {
    flex-direction: column;
    gap: 2px;
  }

  .action-sep {
    width: auto;
    height: auto;
    width: 1px;
    height: 16px;
    border-radius: 0;
    background: var(--color-outline-variant);
    opacity: 0.3;
  }
}
</style>
