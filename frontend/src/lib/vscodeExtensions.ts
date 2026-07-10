/**
 * G-VSC-04: VS Code extension registry — frontend counterpart to a future
 * Extension Host bridge.
 *
 * The IDE hosts two coexisting extension systems:
 *   1. gugacode native plugins (pluginRegistry.ts) — permission-gated,
 *      sandboxed, higher priority.
 *   2. VS Code extensions — run in the Extension Host process, broader
 *      capabilities, supplementary.
 *
 * This module is the integration point the Extension Host bridge calls into.
 * It registers VS Code extension commands and installed-extension metadata so
 * that `unifiedCommands.ts` can aggregate them into one command palette and
 * `PluginManagementPanel.vue` can render a unified management UI.
 *
 * Reactivity: mirrors pluginRegistry's pattern — a version ref is bumped on
 * every mutation so Vue computeds that call listVscodeExtensionCommands() /
 * listVscodeExtensions() re-evaluate. Until a real Extension Host is wired up,
 * the registry stays empty; the aggregation code handles that gracefully.
 */

import { ref } from "vue";
import type {
  VscodeExtensionCommand,
  VscodeExtensionInfo,
  VscodeExtensionSecurityLevel,
} from "@/types";

// ---------------------------------------------------------------------------
// Registry state
// ---------------------------------------------------------------------------

const extensions = new Map<string, VscodeExtensionInfo>();
const commands = new Map<string, VscodeExtensionCommand>();

// G-VSC-04: Reactive version counters bumped on every mutation so Vue
// computeds that consume the registry re-evaluate (same pattern as
// pluginRegistry's commandsVersion / viewsVersion).
const extensionsVersion = ref(0);
const commandsVersion = ref(0);

// ---------------------------------------------------------------------------
// Extension metadata
// ---------------------------------------------------------------------------

/**
 * Register (or upsert) an installed VS Code extension. Called by the
 * Extension Host bridge after it discovers installed extensions. Re-registering
 * an existing id updates its metadata in place.
 */
export function registerVscodeExtension(info: VscodeExtensionInfo): void {
  extensions.set(info.id, { ...info });
  extensionsVersion.value++;
}

/**
 * Remove a VS Code extension from the registry. Also unregisters any commands
 * owned by it. Called when an extension is uninstalled.
 */
export function unregisterVscodeExtension(id: string): void {
  let changed = extensions.delete(id);
  for (const [cmdId, cmd] of Array.from(commands.entries())) {
    if (cmd.extensionId === id) {
      commands.delete(cmdId);
      changed = true;
    }
  }
  if (changed) {
    extensionsVersion.value++;
    commandsVersion.value++;
  }
}

/**
 * List all known VS Code extensions. Reads extensionsVersion to establish a
 * reactive dependency.
 */
export function listVscodeExtensions(): VscodeExtensionInfo[] {
  void extensionsVersion.value; // track for reactivity
  return Array.from(extensions.values());
}

/**
 * Enable or disable a VS Code extension. Persists only in-memory; the
 * Extension Host bridge is responsible for actually (de)activating it.
 */
export function setVscodeExtensionEnabled(id: string, enabled: boolean): void {
  const ext = extensions.get(id);
  if (!ext) return;
  ext.enabled = enabled;
  extensionsVersion.value++;
}

/** Look up a single extension by id. */
export function getVscodeExtension(id: string): VscodeExtensionInfo | undefined {
  return extensions.get(id);
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

/**
 * Register a command contributed by a VS Code extension. The Extension Host
 * bridge calls this for each command an extension declares. Re-registering the
 * same id by the same extension is idempotent; a different extension id wins
 * the slot only if the previous owner was unregistered.
 */
export function registerVscodeExtensionCommand(
  cmd: VscodeExtensionCommand,
): void {
  const existing = commands.get(cmd.id);
  if (existing && existing.extensionId !== cmd.extensionId) {
    throw new Error(
      `VS Code command "${cmd.id}" is already registered by extension "${existing.extensionId}"`,
    );
  }
  commands.set(cmd.id, { ...cmd });
  commandsVersion.value++;
}

/** Remove a single VS Code extension command by id. */
export function unregisterVscodeExtensionCommand(id: string): void {
  if (commands.delete(id)) {
    commandsVersion.value++;
  }
}

/**
 * List all commands contributed by VS Code extensions. Reads commandsVersion
 * to establish a reactive dependency.
 */
export function listVscodeExtensionCommands(): VscodeExtensionCommand[] {
  void commandsVersion.value; // track for reactivity
  return Array.from(commands.values());
}

/**
 * Execute a VS Code extension command by id. The handler registered via
 * registerVscodeExtensionCommand is invoked directly; the Extension Host
 * bridge is responsible for ensuring the owning extension is active.
 */
export async function executeVscodeExtensionCommand(
  id: string,
  ...args: unknown[]
): Promise<unknown> {
  const cmd = commands.get(id);
  if (!cmd) {
    throw new Error(`VS Code extension command "${id}" is not registered`);
  }
  return cmd.handler(...args);
}

// ---------------------------------------------------------------------------
// Helpers / test utilities
// ---------------------------------------------------------------------------

/**
 * Map a VscodeExtensionSecurityLevel to a user-facing badge label key. Used by
 * the management panel to render the security-level tag. The level vocabulary
 * mirrors the G-VSC-03 extensionSecurity store (trusted / reviewed / restricted).
 */
export function securityLevelLabel(
  level: VscodeExtensionSecurityLevel,
): "trusted" | "reviewed" | "restricted" {
  return level;
}

/** Clear the entire registry. Used in tests and on full project switch. */
export function clearVscodeExtensions(): void {
  extensions.clear();
  commands.clear();
  extensionsVersion.value++;
  commandsVersion.value++;
}
