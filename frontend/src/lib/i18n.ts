import { computed } from "vue";
import { appState } from "@/stores/app";

/**
 * Minimal i18n framework (N-12).
 *
 * Why not vue-i18n? The project is a desktop IDE with a fixed, finite set
 * of UI strings. vue-i18n would add a heavy dependency and a global plugin
 * that complicates testing. A purpose-built module with the same reactive
 * contract (read appState.language inside template-rendered functions) is
 * simpler, type-safe, and ~80 lines.
 *
 * Reactivity: `t()` reads `appState.language` in its body, so any template
 * that calls `t('foo')` re-renders when the language changes. `useI18n()`
 * additionally exposes a `locale` computed for components that need to
 * branch on the current language.
 */

export type Locale = "en" | "zh" | "ja";

export type MessageDict = Record<string, string>;

import en from "./locales/en";
import zh from "./locales/zh";
import ja from "./locales/ja";

const dictionaries: Record<Locale, MessageDict> = { en, zh, ja };

/**
 * Get the active locale, falling back to "en" for unknown values.
 */
export function getCurrentLocale(): Locale {
  const lang = appState.language;
  if (lang === "zh" || lang === "ja") return lang;
  return "en";
}

/**
 * Look up a translation key in the current locale's dictionary, falling
 * back to English, then to the key itself (so missing keys are visible
 * but never crash the UI). Supports `{name}` placeholder interpolation.
 *
 * Reading `appState.language` (via getCurrentLocale) inside this function
 * is what makes template calls reactive — Vue tracks the read during
 * render and re-renders on change.
 */
export function translate(
  key: string,
  params?: Record<string, string | number>,
): string {
  const locale = getCurrentLocale();
  const dict = dictionaries[locale] || en;
  let value = dict[key] ?? en[key] ?? key;
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      // Escape regex metacharacters in the placeholder name (defensive —
      // keys are static strings in practice).
      const safe = k.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      value = value.replace(new RegExp(`\\{${safe}\\}`, "g"), String(v));
    }
  }
  return value;
}

/**
 * Composable for components that need a reactive `t` function or the
 * current `locale` as a computed ref.
 *
 * Usage in <script setup>:
 *   const { t, locale } = useI18n();
 *   // In template: {{ t('activity.explorer') }}
 */
export function useI18n() {
  const t = (key: string, params?: Record<string, string | number>) =>
    translate(key, params);
  const locale = computed(() => getCurrentLocale());
  return { t, locale };
}

/**
 * Test-only helper: register an additional locale dictionary at runtime.
 * Used by the i18n tests to verify fallback behavior with a fake locale.
 * Not exported through the public surface; tests import this directly.
 */
export function __setLocaleDictionary(locale: Locale, dict: MessageDict): void {
  dictionaries[locale] = dict;
}

/**
 * Test-only helper: reset a locale dictionary to its original value.
 * Implemented by re-assigning the built-in dictionary reference.
 */
export function __resetLocaleDictionary(locale: Locale): void {
  if (locale === "en") dictionaries.en = en;
  else if (locale === "zh") dictionaries.zh = zh;
  else if (locale === "ja") dictionaries.ja = ja;
}

/**
 * Test-only helper: get a read-only reference to a locale's dictionary.
 * Used by the i18n completeness test to verify key parity across locales
 * (Proposal AI — catches missing/truncated translations automatically).
 */
export function __getLocaleDictionary(locale: Locale): MessageDict {
  return dictionaries[locale];
}
