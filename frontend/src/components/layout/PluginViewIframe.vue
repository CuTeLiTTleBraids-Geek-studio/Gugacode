<script setup lang="ts">
// N-36 (prompt-5.md): Plugin view iframe wrapper.
//
// Sandboxed plugins register views with `nknk.views.register(id, null, options)`
// because Vue components can't cross the Worker boundary. This component
// renders the plugin's `view.html` in an iframe and establishes a
// bidirectional postMessage channel for the plugin to call host APIs.
//
// Proposal G (prompt-5.md): iframe communication protocol.
//
// The iframe receives an `nknk` proxy via `window.parent.postMessage`.
// The host validates permissions using the same METHOD_PERMISSIONS map
// as the Worker sandbox (N-26), so the security model is consistent.
//
// Message protocol (host ↔ iframe):
//   Host → Iframe:
//     { type: "nknk:init", manifest, viewId }
//     { type: "nknk:rpc-response", id, result?, error? }
//     { type: "nknk:event", event, data }
//   Iframe → Host:
//     { type: "nknk:ready" }
//     { type: "nknk:rpc-request", id, method, args }
//     { type: "nknk:log", level, message }
//
// Security:
//   - iframe has `sandbox="allow-scripts"` (no allow-same-origin, no forms,
//     no top navigation, no popup, no pointer lock). Without allow-same-origin
//     the iframe gets an opaque origin and cannot remove its own sandbox or
//     reach parent.window.go bindings directly.
//   - Source check on every message: only messages whose `event.source`
//     matches the iframe's `contentWindow` are accepted. Origin-string checks
//     are unsafe here because sandboxed iframes without allow-same-origin
//     emit `origin: "null"`, which can be spoofed.
//   - Permission check on every RPC: METHOD_PERMISSIONS gate.
//   - With only `allow-scripts`, the iframe cannot access the parent's DOM,
//     localStorage, cookies, or parent.window bindings — only via the
//     postMessage RPC bridge.
import { ref, onMounted, onUnmounted, computed } from "vue";
import type { PluginManifest } from "@/types";
import { errorMessage } from "@/lib/errors";
import { hasPermissionForMethod, type RpcHandler, type RpcMethod } from "@/lib/pluginSandbox";

const props = defineProps<{
  pluginName: string;
  viewId: string;
  title: string;
  manifest: PluginManifest;
  rpcHandler: RpcHandler;
}>();

// Build the iframe src. The plugin's view.html is served by the
// backend's /_plugins/<name>/ asset handler.
const iframeSrc = computed(() => {
  const root = (window as unknown as { __NKNK_PROJECT_ROOT__?: string }).__NKNK_PROJECT_ROOT__ ?? "";
  const q = root ? `?projectRoot=${encodeURIComponent(root)}&` : "?";
  return `/_plugins/${encodeURIComponent(props.pluginName)}/view.html${q}viewId=${encodeURIComponent(props.viewId)}`;
});

const iframeEl = ref<HTMLIFrameElement | null>(null);
const isReady = ref(false);
const lastError = ref<string | null>(null);

// Pending RPC calls waiting for the iframe to respond.
interface PendingCall {
  resolve: (value: unknown) => void;
  reject: (error: Error) => void;
}
const pendingCalls = new Map<number, PendingCall>();
let nextCallId = 1;

// Validate message source — only messages from the iframe's own
// contentWindow are accepted. Using `event.source` is safer than
// checking `event.origin`, because a sandboxed iframe without
// allow-same-origin emits `origin: "null"` which any document can spoof.
function isAllowedOrigin(event: MessageEvent): boolean {
  return event.source === iframeEl.value?.contentWindow;
}

async function handleRpcRequest(id: number, method: RpcMethod, args: unknown[]): Promise<void> {
  try {
    // Permission check — same as Worker sandbox (N-26).
    if (!hasPermissionForMethod(props.manifest, method)) {
      const perm = method.startsWith("workspace.read") ? "fs.read" :
        method.startsWith("workspace.write") ? "fs.write" : "unknown";
      throw new Error(
        `Permission denied: plugin "${props.pluginName}" did not declare permission "${perm}" required for method "${method}"`,
      );
    }
    const result = await props.rpcHandler(props.pluginName, props.manifest, method, args);
    sendToIframe({ type: "nknk:rpc-response", id, result });
  } catch (e: unknown) {
    sendToIframe({ type: "nknk:rpc-response", id, error: errorMessage(e) });
  }
}

function sendToIframe(msg: unknown): void {
  const el = iframeEl.value;
  if (!el?.contentWindow) return;
  el.contentWindow.postMessage(msg, window.location.origin);
}

function onMessage(event: MessageEvent): void {
  if (!isAllowedOrigin(event)) return;
  const data = event.data;
  if (!data || typeof data !== "object") return;
  const msg = data as { type?: string };

  switch (msg.type) {
    case "nknk:ready":
      // Iframe is ready — send init with manifest and viewId.
      isReady.value = true;
      sendToIframe({
        type: "nknk:init",
        manifest: props.manifest,
        viewId: props.viewId,
      });
      break;

    case "nknk:rpc-request": {
      const req = data as { id: number; method: RpcMethod; args: unknown[] };
      void handleRpcRequest(req.id, req.method, req.args ?? []);
      break;
    }

    case "nknk:rpc-response": {
      // This case is for responses to host→iframe calls (if we ever
      // need them). Currently the host only responds to iframe requests.
      const resp = data as { id: number; result?: unknown; error?: string };
      const pending = pendingCalls.get(resp.id);
      if (pending) {
        pendingCalls.delete(resp.id);
        if (resp.error) {
          pending.reject(new Error(resp.error));
        } else {
          pending.resolve(resp.result);
        }
      }
      break;
    }

    case "nknk:log": {
      const log = data as { level: "info" | "warn" | "error"; message: string };
      console[log.level](`[plugin view: ${props.pluginName}/${props.viewId}]`, log.message);
      break;
    }
  }
}

onMounted(() => {
  window.addEventListener("message", onMessage);
});

onUnmounted(() => {
  window.removeEventListener("message", onMessage);
  // Reject any pending calls so callers don't hang.
  for (const [, pending] of pendingCalls) {
    pending.reject(new Error("Plugin view iframe unmounted"));
  }
  pendingCalls.clear();
});

// Public API: call a method on the iframe (host → iframe direction).
// Currently unused but available for future host-initiated actions
// like "view:refresh" or "view:setState".
function callIframeMethod(method: string, args: unknown[]): Promise<unknown> {
  const id = nextCallId++;
  return new Promise((resolve, reject) => {
    pendingCalls.set(id, { resolve, reject });
    sendToIframe({ type: "nknk:rpc-call", id, method, args });
  });
}

defineExpose({ callIframeMethod, isReady, lastError });
</script>

<template>
  <div class="plugin-view-iframe">
    <div v-if="lastError" class="plugin-view-iframe__error">
      {{ lastError }}
    </div>
    <iframe
      ref="iframeEl"
      :src="iframeSrc"
      :title="title"
      sandbox="allow-scripts"
      class="plugin-view-iframe__el"
    />
  </div>
</template>

<style scoped>
.plugin-view-iframe {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.plugin-view-iframe__el {
  flex: 1;
  border: none;
  width: 100%;
  height: 100%;
  background: var(--color-bg, #fff);
}

.plugin-view-iframe__error {
  padding: 8px 12px;
  background: var(--color-error-bg, rgba(244, 67, 54, 0.1));
  color: var(--color-error, #ff6b6b);
  font-size: 12px;
  font-family: var(--font-mono, monospace);
}
</style>
