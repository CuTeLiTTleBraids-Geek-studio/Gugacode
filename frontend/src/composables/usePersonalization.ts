// Plan 11 Task 15 Step 4-8 — personalization runtime applier.
//
// Watches appState.personalization and projects the config onto CSS
// custom properties on :root. Image fields hold relative asset paths
// (e.g. "assets/abc.png") stored by the backend; we fetch the bytes via
// settingsService.readPersonalizationAsset and materialize them as blob
// URLs so they can be used in CSS background-image.
//
// CSS variables emitted (consumed in main.css):
//   --personalization-editor-bg, --personalization-editor-bg-opacity,
//   --personalization-editor-bg-blur,
//   --personalization-chat-bg, --personalization-chat-bg-opacity,
//   --personalization-chat-bg-blur,
//   --personalization-user-avatar, --personalization-ai-avatar,
//   --personalization-font-family, --personalization-font-size,
//   --personalization-bubble-style, --personalization-bubble-opacity,
//   --personalization-message-spacing
import { watch } from "vue";
import { appState } from "@/stores/app";
import { settingsService } from "@/api/services";
import type { PersonalizationConfig } from "@/types";

// Track active blob URLs so we can revoke them when replaced, avoiding
// memory leaks across repeated image uploads.
const blobUrls = new Map<string, string>();

/**
 * Resolve a relative asset path to a blob URL. Returns "" when relPath is
 * empty or the read fails (best-effort — a missing image must never break
 * the UI). Cached so repeated reads of the same path are free.
 */
async function resolveAssetUrl(relPath: string): Promise<string> {
  if (!relPath) return "";
  const cached = blobUrls.get(relPath);
  if (cached) return cached;
  try {
    const bytes = await settingsService.readPersonalizationAsset(relPath);
    // Copy into a fresh ArrayBuffer-backed view so BlobPart accepts it under
    // strict DOM lib typings (Uint8Array<ArrayBufferLike> is rejected).
    const copy = new Uint8Array(bytes.byteLength);
    copy.set(bytes);
    const blob = new Blob([copy], { type: "image/*" });
    const url = URL.createObjectURL(blob);
    blobUrls.set(relPath, url);
    return url;
  } catch {
    return "";
  }
}

/** Revoke a previously cached blob URL for a path (if any). */
function revokeAsset(relPath: string): void {
  const url = blobUrls.get(relPath);
  if (url) {
    URL.revokeObjectURL(url);
    blobUrls.delete(relPath);
  }
}

function setVar(name: string, value: string): void {
  if (value) {
    document.documentElement.style.setProperty(name, value);
  } else {
    document.documentElement.style.removeProperty(name);
  }
}

/**
 * Apply the non-image fields synchronously (opacity/blur/font/bubble).
 * Image fields are resolved asynchronously and applied when ready.
 */
export function applyPersonalization(): void {
  const p: PersonalizationConfig = appState.personalization;
  setVar("--personalization-editor-bg-opacity", String(p.codeEditorBgOpacity ?? 0));
  setVar("--personalization-editor-bg-blur", `${p.codeEditorBgBlur ?? 0}px`);
  setVar("--personalization-chat-bg-opacity", String(p.chatBgOpacity ?? 0));
  setVar("--personalization-chat-bg-blur", `${p.chatBgBlur ?? 0}px`);
  setVar("--personalization-font-family", p.fontFamily ?? "");
  setVar("--personalization-font-size", p.fontSize ? `${p.fontSize}px` : "");
  setVar("--personalization-bubble-style", p.bubbleStyle ?? "rounded");
  setVar("--personalization-bubble-opacity", String(p.bubbleOpacity ?? 1));
  setVar("--personalization-message-spacing", `${p.messageSpacing ?? 12}px`);

  // Avatars are applied directly as URL strings (consumed by <img>:src
  // bindings in components, not just CSS), so resolve them too.
  void resolveAssetUrl(p.userAvatar ?? "").then((url) =>
    setVar("--personalization-user-avatar", url),
  );
  void resolveAssetUrl(p.aiAvatar ?? "").then((url) =>
    setVar("--personalization-ai-avatar", url),
  );

  // Background images: revoke previous, resolve new.
  if (p.codeEditorBgImage) {
    document.documentElement.setAttribute("data-editor-bg", "");
    void resolveAssetUrl(p.codeEditorBgImage).then((url) =>
      setVar("--personalization-editor-bg", url ? `url("${url}")` : ""),
    );
  } else {
    document.documentElement.removeAttribute("data-editor-bg");
    setVar("--personalization-editor-bg", "");
  }
  if (p.chatBgImage) {
    document.documentElement.setAttribute("data-chat-bg", "");
    void resolveAssetUrl(p.chatBgImage).then((url) =>
      setVar("--personalization-chat-bg", url ? `url("${url}")` : ""),
    );
  } else {
    document.documentElement.removeAttribute("data-chat-bg");
    setVar("--personalization-chat-bg", "");
  }
}

/**
 * Initialize personalization: apply once and watch for changes. Call once
 * at app startup (after loadSettings hydrates appState.personalization).
 */
export function initPersonalization(): void {
  applyPersonalization();
  watch(
    () => ({ ...appState.personalization }),
    () => applyPersonalization(),
    { deep: true },
  );
}

/** Revoke all cached blob URLs. Call on teardown / hot-reload. */
export function teardownPersonalization(): void {
  for (const url of blobUrls.values()) {
    URL.revokeObjectURL(url);
  }
  blobUrls.clear();
}

export { revokeAsset };
