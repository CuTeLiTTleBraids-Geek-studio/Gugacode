<script setup lang="ts">
// G-VSC-01: VS Code extension marketplace panel.
//
// Surfaces the Open VSX Registry search/browse/install flow in the "extensions"
// activity tab. The panel has three regions:
//   1. A security warning banner reminding the user that installs are
//      disabled-by-default and SHA-256 verified (G-SEC-12 req. 2 & 3).
//   2. A search bar + results list (or the detail view for a selected hit).
//   3. An installed-extensions list with enable/disable + uninstall controls.
//
// The panel is self-contained: it calls marketplaceService directly and can be
// mounted anywhere (the SidePanel "extensions" tab sub-view, or a route).
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { ArrowLeft, Delete, Download, Loading, Search } from "@element-plus/icons-vue";
import { marketplaceService } from "@/api/services";
import { errorMessage } from "@/lib/errors";
import { requestEnableExtension } from "@/stores/extensionSecurity";
import { useI18n } from "@/lib/i18n";
import type {
  ExtensionDetail,
  ExtensionSearchResult,
  InstalledExtension,
} from "@/types";

const { t } = useI18n();

// --- search state ---
const query = ref("");
const results = ref<ExtensionSearchResult[]>([]);
const searching = ref(false);
const hasSearched = ref(false);

// --- detail state ---
const detail = ref<ExtensionDetail | null>(null);
const loadingDetail = ref(false);

// --- installed state ---
const installed = ref<InstalledExtension[]>([]);
const loadingInstalled = ref(false);

// --- install-in-progress tracking (keyed "publisher.name") ---
const installing = ref<Set<string>>(new Set());
function installKey(publisher: string, name: string): string {
  return `${publisher}.${name}`;
}
function isInstalling(publisher: string, name: string): boolean {
  return installing.value.has(installKey(publisher, name));
}

// The currently visible list region. "search" shows search results (or the
// empty prompt before a search); "installed" shows the installed list. The
// detail view overlays whichever list is active when an extension is opened.
const view = ref<"search" | "installed">("search");

const installedIds = computed(
  () => new Set(installed.value.map((e) => installKey(e.publisher, e.name))),
);

function isInstalled(publisher: string, name: string): boolean {
  return installedIds.value.has(installKey(publisher, name));
}

// --- search ---
async function runSearch(): Promise<void> {
  const q = query.value.trim();
  if (!q) {
    results.value = [];
    hasSearched.value = false;
    return;
  }
  searching.value = true;
  hasSearched.value = true;
  try {
    results.value = await marketplaceService.searchExtensions(q, 1, 30);
  } catch (e: unknown) {
    ElMessage.error(errorMessage(e));
    results.value = [];
  } finally {
    searching.value = false;
  }
}

// --- detail ---
async function openDetail(hit: ExtensionSearchResult): Promise<void> {
  loadingDetail.value = true;
  detail.value = null;
  try {
    detail.value = await marketplaceService.getExtensionDetail(hit.publisher, hit.name);
  } catch (e: unknown) {
    ElMessage.error(errorMessage(e));
  } finally {
    loadingDetail.value = false;
  }
}

function closeDetail(): void {
  detail.value = null;
}

// --- install / uninstall ---
async function install(publisher: string, name: string, version: string): Promise<void> {
  const key = installKey(publisher, name);
  if (installing.value.has(key)) return;
  installing.value.add(key);
  try {
    // Downloads the VSIX, verifies SHA-256 (G-SEC-12 req. 3), extracts with
    // path-traversal protection, and records the install as disabled-by-default
    // (G-SEC-12 req. 2). A hash mismatch aborts here with an error.
    await marketplaceService.downloadAndInstallExtension(publisher, name, version);
    ElMessage.success(t("marketplace.installSuccess", { id: key }));
    await refreshInstalled();
  } catch (e: unknown) {
    ElMessage.error(errorMessage(e));
  } finally {
    installing.value.delete(key);
  }
}

async function uninstall(ext: InstalledExtension): Promise<void> {
  try {
    await marketplaceService.uninstallExtension(ext.publisher, ext.name);
    ElMessage.success(t("marketplace.uninstallSuccess", { id: installKey(ext.publisher, ext.name) }));
    await refreshInstalled();
  } catch (e: unknown) {
    ElMessage.error(errorMessage(e));
  }
}

async function toggleEnabled(ext: InstalledExtension, enabled: boolean): Promise<void> {
  if (enabled) {
    // G-SEC-12: route enable requests through the security service so
    // Restricted extensions go through the permission dialog.
    const extensionId = `${ext.publisher}.${ext.name}`;
    const ok = await requestEnableExtension(extensionId);
    if (!ok) {
      ext.enabled = false;
      return;
    }
    ext.enabled = true;
  } else {
    try {
      await marketplaceService.setExtensionEnabled(ext.publisher, ext.name, false);
      ext.enabled = false;
    } catch (e: unknown) {
      ElMessage.error(errorMessage(e));
      ext.enabled = true;
    }
  }
}

async function refreshInstalled(): Promise<void> {
  loadingInstalled.value = true;
  try {
    installed.value = await marketplaceService.listInstalledExtensions();
  } catch (e: unknown) {
    ElMessage.error(errorMessage(e));
    installed.value = [];
  } finally {
    loadingInstalled.value = false;
  }
}

function formatCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

onMounted(() => {
  refreshInstalled();
});
</script>

<template>
  <div class="marketplace">
    <!-- Security warning banner (G-SEC-12 req. 2 & 3) -->
    <div class="marketplace__security" role="note">
      <span class="marketplace__security-title">{{ t("marketplace.securityTitle") }}</span>
      <p class="marketplace__security-text">{{ t("marketplace.securityText") }}</p>
    </div>

    <!-- Search bar -->
    <div class="marketplace__search">
      <el-input
        v-model="query"
        :placeholder="t('marketplace.searchPlaceholder')"
        clearable
        :aria-label="t('marketplace.searchPlaceholder')"
        @keyup.enter="runSearch"
      >
        <template #prefix>
          <el-icon><Search /></el-icon>
        </template>
      </el-input>
      <el-button
        type="primary"
        :loading="searching"
        :aria-label="t('marketplace.searchButton')"
        @click="runSearch"
      >
        {{ t("marketplace.searchButton") }}
      </el-button>
    </div>

    <!-- Sub-tab toggle between search results and installed list -->
    <div class="marketplace__tabs" role="tablist">
      <button
        type="button"
        role="tab"
        :aria-selected="view === 'search'"
        class="marketplace__tab"
        :class="{ 'marketplace__tab--active': view === 'search' && !detail }"
        @click="view = 'search'"
      >
        {{ t("marketplace.tabResults") }}
      </button>
      <button
        type="button"
        role="tab"
        :aria-selected="view === 'installed'"
        class="marketplace__tab"
        :class="{ 'marketplace__tab--active': view === 'installed' && !detail }"
        @click="view = 'installed'"
      >
        {{ t("marketplace.tabInstalled") }} ({{ installed.length }})
      </button>
    </div>

    <div class="marketplace__body">
      <!-- Detail view (overlays the active list) -->
      <div v-if="detail || loadingDetail" key="detail" class="marketplace__detail">
        <button type="button" class="marketplace__back" @click="closeDetail">
          <el-icon><ArrowLeft /></el-icon>
          <span>{{ t("marketplace.backToList") }}</span>
        </button>
        <div v-if="loadingDetail" class="marketplace__loading">
          <el-icon class="is-loading"><Loading /></el-icon>
          <span>{{ t("marketplace.loading") }}</span>
        </div>
        <article v-else-if="detail" class="marketplace__detail-content">
          <header class="marketplace__detail-header">
            <div class="marketplace__detail-titles">
              <h3 class="marketplace__detail-name">{{ detail.displayName || detail.name }}</h3>
              <span class="marketplace__detail-id">{{ detail.publisher }}.{{ detail.name }}</span>
            </div>
            <el-button
              v-if="!isInstalled(detail.publisher, detail.name)"
              type="primary"
              :loading="isInstalling(detail.publisher, detail.name)"
              @click="install(detail.publisher, detail.name, detail.version)"
            >
              <el-icon><Download /></el-icon>
              <span>{{ t("marketplace.install") }}</span>
            </el-button>
            <el-tag v-else type="success" size="small">
              {{ t("marketplace.installed") }}
            </el-tag>
          </header>

          <p v-if="detail.description" class="marketplace__detail-desc">{{ detail.description }}</p>

          <dl class="marketplace__meta">
            <div v-if="detail.version" class="marketplace__meta-row">
              <dt>{{ t("marketplace.metaVersion") }}</dt>
              <dd>{{ detail.version }}</dd>
            </div>
            <div v-if="detail.license" class="marketplace__meta-row">
              <dt>{{ t("marketplace.metaLicense") }}</dt>
              <dd>{{ detail.license }}</dd>
            </div>
            <div class="marketplace__meta-row">
              <dt>{{ t("marketplace.metaDownloads") }}</dt>
              <dd>{{ formatCount(detail.downloadCount) }}</dd>
            </div>
            <div v-if="detail.ratingCount > 0" class="marketplace__meta-row">
              <dt>{{ t("marketplace.metaRating") }}</dt>
              <dd>{{ detail.rating.toFixed(1) }} ({{ detail.ratingCount }})</dd>
            </div>
            <div v-if="detail.repository" class="marketplace__meta-row">
              <dt>{{ t("marketplace.metaRepository") }}</dt>
              <dd class="marketplace__link">{{ detail.repository }}</dd>
            </div>
          </dl>

          <div v-if="detail.categories && detail.categories.length" class="marketplace__tags">
            <el-tag v-for="c in detail.categories" :key="c" size="small" type="info">{{ c }}</el-tag>
          </div>

          <div v-if="detail.versions && detail.versions.length" class="marketplace__versions">
            <h4 class="marketplace__versions-title">{{ t("marketplace.versions") }}</h4>
            <ul class="marketplace__versions-list">
              <li v-for="v in detail.versions" :key="v.version" class="marketplace__version-item">
                <span class="marketplace__version-num">{{ v.version }}</span>
                <span v-if="v.date" class="marketplace__version-date">{{ v.date }}</span>
              </li>
            </ul>
          </div>
        </article>
      </div>

      <!-- Search results -->
      <div v-else-if="view === 'search'" key="search" class="marketplace__results">
        <div v-if="searching" class="marketplace__loading">
          <el-icon class="is-loading"><Loading /></el-icon>
          <span>{{ t("marketplace.loading") }}</span>
        </div>
        <div v-else-if="results.length === 0" class="marketplace__empty">
          {{ hasSearched ? t("marketplace.noResults") : t("marketplace.searchPrompt") }}
        </div>
        <ul v-else class="marketplace__list">
          <li
            v-for="r in results"
            :key="r.id"
            class="marketplace__item"
          >
            <button type="button" class="marketplace__item-main" @click="openDetail(r)">
              <div class="marketplace__item-title">
                <span class="marketplace__name">{{ r.displayName || r.name }}</span>
                <span class="marketplace__version">v{{ r.version }}</span>
              </div>
              <p class="marketplace__item-publisher">{{ t("marketplace.by", { author: r.publisher }) }}</p>
              <p v-if="r.description" class="marketplace__item-desc">{{ r.description }}</p>
              <div class="marketplace__item-stats">
                <span v-if="r.downloadCount > 0">{{ t("marketplace.metaDownloads") }}: {{ formatCount(r.downloadCount) }}</span>
                <span v-if="r.ratingCount > 0">{{ r.rating.toFixed(1) }} ★ ({{ r.ratingCount }})</span>
              </div>
            </button>
            <div class="marketplace__item-actions">
              <el-button
                v-if="!isInstalled(r.publisher, r.name)"
                size="small"
                type="primary"
                :loading="isInstalling(r.publisher, r.name)"
                @click="install(r.publisher, r.name, r.version)"
              >
                {{ t("marketplace.install") }}
              </el-button>
              <el-tag v-else type="success" size="small">
                {{ t("marketplace.installed") }}
              </el-tag>
            </div>
          </li>
        </ul>
      </div>

      <!-- Installed extensions -->
      <div v-else key="installed" class="marketplace__installed">
        <div v-if="loadingInstalled" class="marketplace__loading">
          <el-icon class="is-loading"><Loading /></el-icon>
          <span>{{ t("marketplace.loading") }}</span>
        </div>
        <div v-else-if="installed.length === 0" class="marketplace__empty">
          {{ t("marketplace.noInstalled") }}
        </div>
        <ul v-else class="marketplace__list">
          <li
            v-for="ext in installed"
            :key="`${ext.publisher}.${ext.name}`"
            class="marketplace__item"
            :class="{ 'is-disabled': !ext.enabled }"
          >
            <div class="marketplace__item-main">
              <div class="marketplace__item-title">
                <span class="marketplace__name">{{ ext.publisher }}.{{ ext.name }}</span>
                <span class="marketplace__version">v{{ ext.version }}</span>
                <el-tag size="small" :type="ext.enabled ? 'success' : 'info'">
                  {{ ext.enabled ? t("marketplace.enabled") : t("marketplace.disabled") }}
                </el-tag>
              </div>
              <p class="marketplace__item-desc marketplace__item-desc--muted">
                {{ ext.enabled ? t("marketplace.enabledHint") : t("marketplace.disabledHint") }}
              </p>
            </div>
            <div class="marketplace__item-actions">
              <el-switch
                :model-value="ext.enabled"
                :aria-label="t('marketplace.enableDisableAria', { id: `${ext.publisher}.${ext.name}` })"
                @change="(val: boolean) => toggleEnabled(ext, val)"
              />
              <el-button
                size="small"
                type="danger"
                plain
                :aria-label="t('marketplace.uninstallAria', { id: `${ext.publisher}.${ext.name}` })"
                @click="uninstall(ext)"
              >
                <el-icon><Delete /></el-icon>
              </el-button>
            </div>
          </li>
        </ul>
      </div>
    </div>
  </div>
</template>

<style scoped>
.marketplace {
  display: flex;
  flex-direction: column;
  gap: 10px;
  height: 100%;
  overflow: hidden;
  padding: 10px 8px;
}

/* Security warning banner — visually distinct (amber-ish) so the user notices
   the default-disabled + SHA-256 policy before installing anything. */
.marketplace__security {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
  border: 1px solid color-mix(in srgb, var(--color-warning, #e6a23c) 45%, transparent);
  border-radius: var(--radius-md, 8px);
  background: color-mix(in srgb, var(--color-warning, #e6a23c) 10%, transparent);
}

.marketplace__security-title {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--color-warning, #e6a23c);
  letter-spacing: 0.01em;
}

.marketplace__security-text {
  margin: 0;
  font-size: 0.75rem;
  line-height: 1.45;
  color: var(--color-text-secondary, #a0a0a0);
}

.marketplace__search {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}

.marketplace__tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--color-border-subtle, rgba(255, 255, 255, 0.08));
  flex-shrink: 0;
}

.marketplace__tab {
  padding: 6px 10px;
  border: none;
  background: transparent;
  color: var(--color-text-tertiary, #707070);
  font-size: 0.8125rem;
  font-weight: 500;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: color var(--transition-fast, 150ms) ease, border-color var(--transition-fast, 150ms) ease;
}

.marketplace__tab:hover {
  color: var(--color-text-secondary, #a0a0a0);
}

.marketplace__tab--active {
  color: var(--chrome-text-active, var(--color-primary, #4285f4));
  border-bottom-color: var(--chrome-text-active, var(--color-primary, #4285f4));
}

.marketplace__body {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
}

.marketplace__loading,
.marketplace__empty {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 24px 12px;
  font-size: 0.8125rem;
  color: var(--color-text-tertiary, #707070);
  text-align: center;
}

.marketplace__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.marketplace__item {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid var(--color-border-default, #2a2a2c);
  border-radius: var(--radius-md, 8px);
  background: var(--color-bg-surface-container-low, #161616);
  transition: border-color var(--transition-fast, 150ms) ease;
}

.marketplace__item:hover {
  border-color: var(--color-primary, #a0c4ff);
}

.marketplace__item.is-disabled {
  opacity: 0.7;
}

.marketplace__item-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
  text-align: left;
  background: transparent;
  border: none;
  padding: 0;
  color: inherit;
  cursor: pointer;
  font: inherit;
}

.marketplace__item-title {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.marketplace__name {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--color-text-primary, #f0f0f0);
}

.marketplace__version {
  font-family: var(--font-mono);
  font-size: 0.75rem;
  color: var(--color-text-secondary, #a0a0a0);
}

.marketplace__item-publisher,
.marketplace__item-desc {
  margin: 0;
  font-size: 0.8125rem;
  color: var(--color-text-secondary, #a0a0a0);
  line-height: 1.45;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.marketplace__item-desc--muted {
  font-style: italic;
}

.marketplace__item-stats {
  display: flex;
  gap: 10px;
  font-size: 0.7rem;
  color: var(--color-text-tertiary, #707070);
}

.marketplace__item-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

/* --- detail view --- */
.marketplace__detail {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 4px 4px 12px;
}

.marketplace__back {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  align-self: flex-start;
  padding: 4px 8px;
  border: none;
  background: transparent;
  color: var(--color-text-secondary, #a0a0a0);
  font-size: 0.8125rem;
  cursor: pointer;
  border-radius: var(--radius-sm, 6px);
  transition: background-color var(--transition-fast, 150ms) ease;
}

.marketplace__back:hover {
  background-color: var(--chrome-hover-bg, rgba(255, 255, 255, 0.06));
}

.marketplace__detail-content {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.marketplace__detail-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
}

.marketplace__detail-titles {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.marketplace__detail-name {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
  color: var(--color-text-primary, #f0f0f0);
}

.marketplace__detail-id {
  font-family: var(--font-mono);
  font-size: 0.75rem;
  color: var(--color-text-tertiary, #707070);
}

.marketplace__detail-desc {
  margin: 0;
  font-size: 0.8125rem;
  color: var(--color-text-secondary, #a0a0a0);
  line-height: 1.5;
}

.marketplace__meta {
  margin: 0;
  display: grid;
  grid-template-columns: 1fr;
  gap: 4px;
}

.marketplace__meta-row {
  display: flex;
  gap: 8px;
  font-size: 0.8125rem;
}

.marketplace__meta-row dt {
  min-width: 90px;
  color: var(--color-text-tertiary, #707070);
  font-weight: 500;
}

.marketplace__meta-row dd {
  margin: 0;
  color: var(--color-text-secondary, #a0a0a0);
  word-break: break-all;
}

.marketplace__link {
  color: var(--color-primary, #4285f4);
}

.marketplace__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.marketplace__versions-title {
  margin: 4px 0 4px;
  font-size: 0.8125rem;
  font-weight: 600;
  color: var(--color-text-secondary, #a0a0a0);
}

.marketplace__versions-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.marketplace__version-item {
  display: flex;
  justify-content: space-between;
  font-size: 0.75rem;
  color: var(--color-text-tertiary, #707070);
  padding: 2px 0;
}

.marketplace__version-num {
  font-family: var(--font-mono);
}

@media (prefers-reduced-motion: reduce) {
  .marketplace__item,
  .marketplace__tab,
  .marketplace__back {
    transition: none;
  }
}
</style>
