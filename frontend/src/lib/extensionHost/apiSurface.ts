/**
 * Extension API surface restriction (G-SEC-12 requirement 4).
 *
 * This module defines which VS Code-compatible APIs are exposed to which
 * security levels (Trusted / Reviewed / Restricted). It is the
 * compatibility layer that translates VS Code extension API calls into
 * the gugacode permission-gated nknk.* surface.
 *
 * Rules (G-SEC-12 req. 4):
 *   - Trusted: read-only APIs — fs.read, languages.register*Provider,
 *     commands.registerCommand.
 *   - Reviewed: adds fs.write (with path validation),
 *     window.showInformationMessage.
 *   - Restricted: adds shell.execute (with confirmation) and network
 *     (with per-request approval).
 *
 * Dangerous commands always require confirmation regardless of level:
 *   - workbench.action.terminal.sendSequence
 *   - _workbench.*
 *   - workbench.action.files.save
 *
 * The API surface also enforces resource isolation (G-SEC-12 req. 5):
 * extensions never receive appState or window.go bindings directly.
 */

import type {
  ExtensionSecurityLevel,
  ExtensionPermission,
} from "@/stores/extensionSecurity";

// ---------------------------------------------------------------------------
// API → permission mapping
//
// Each VS Code-compatible API method maps to the permission it requires.
// Methods not in this map are unavailable to extensions (deny-by-default).
// ---------------------------------------------------------------------------

/**
 * The set of VS Code-compatible API methods exposed to extensions. Each
 * entry records the required permission and the minimum security level
 * that can call it.
 */
export interface ApiMethodSpec {
  /** The permission the extension must have declared. */
  permission: ExtensionPermission | null;
  /** The minimum security level required (trusted < reviewed < restricted). */
  minLevel: ExtensionSecurityLevel;
  /** Whether the method requires interactive confirmation before running. */
  requiresConfirmation?: boolean;
  /** Human-readable label for the confirmation dialog. */
  confirmLabel?: string;
}

const LEVEL_RANK: Record<ExtensionSecurityLevel, number> = {
  trusted: 0,
  reviewed: 1,
  restricted: 2,
};

/**
 * The canonical API surface map. An extension at level L may call method M
 * iff LEVEL_RANK[L] >= LEVEL_RANK[API[M].minLevel] AND the extension
 * declared API[M].permission (when non-null).
 *
 * Methods with requiresConfirmation=true always prompt the user, even for
 * Trusted extensions — this is the "dangerous commands require
 * confirmation" rule.
 */
export const API_SURFACE: Record<string, ApiMethodSpec> = {
  // --- Trusted (read-only) ---
  "fs.readFile": {
    permission: "fs.read",
    minLevel: "trusted",
  },
  "fs.readdir": {
    permission: "fs.read",
    minLevel: "trusted",
  },
  "languages.registerCompletionItemProvider": {
    permission: null, // registration is always allowed
    minLevel: "trusted",
  },
  "languages.registerHoverProvider": {
    permission: null,
    minLevel: "trusted",
  },
  "languages.registerDefinitionProvider": {
    permission: null,
    minLevel: "trusted",
  },
  "commands.registerCommand": {
    permission: null,
    minLevel: "trusted",
  },
  "window.showInformationMessage": {
    permission: "ui.notifications",
    minLevel: "trusted",
  },
  "window.showWarningMessage": {
    permission: "ui.notifications",
    minLevel: "trusted",
  },
  "window.showErrorMessage": {
    permission: "ui.notifications",
    minLevel: "trusted",
  },

  // --- Reviewed (file write + restricted terminal) ---
  "fs.writeFile": {
    permission: "fs.write",
    minLevel: "reviewed",
  },
  "fs.deleteFile": {
    permission: "fs.write",
    minLevel: "reviewed",
  },
  "fs.createDirectory": {
    permission: "fs.write",
    minLevel: "reviewed",
  },
  "workspace.applyEdit": {
    permission: "fs.write",
    minLevel: "reviewed",
  },
  "window.createWebviewPanel": {
    permission: "ui.webview",
    minLevel: "reviewed",
  },

  // --- Restricted (network + unrestricted shell) ---
  "shell.execute": {
    permission: "shell.execute",
    minLevel: "restricted",
    requiresConfirmation: true,
    confirmLabel: "Execute shell command",
  },
  "network.request": {
    permission: "network",
    minLevel: "restricted",
    requiresConfirmation: true,
    confirmLabel: "Make network request",
  },
  "child_process.exec": {
    permission: "shell.execute",
    minLevel: "restricted",
    requiresConfirmation: true,
    confirmLabel: "Execute child process",
  },
};

// ---------------------------------------------------------------------------
// Dangerous commands — always require confirmation regardless of level.
// These are VS Code built-in commands that can cause side effects beyond
// the extension's declared permissions. G-SEC-12 req. 4.
// ---------------------------------------------------------------------------

/**
 * Commands that always require user confirmation before execution,
 * regardless of the calling extension's security level. Matching is
 * prefix-based for wildcard entries (e.g. "_workbench.*").
 */
export const DANGEROUS_COMMANDS: readonly string[] = [
  "workbench.action.terminal.sendSequence",
  "workbench.action.files.save",
  "_workbench.*", // prefix match — all internal workbench commands
];

/**
 * Check if a command ID is in the dangerous-commands list. Wildcard
 * entries (ending in ".*") match by prefix.
 */
export function isDangerousCommand(commandId: string): boolean {
  for (const pattern of DANGEROUS_COMMANDS) {
    if (pattern.endsWith(".*")) {
      const prefix = pattern.slice(0, -1); // keep the dot
      if (commandId.startsWith(prefix)) return true;
    } else if (commandId === pattern) {
      return true;
    }
  }
  return false;
}

// ---------------------------------------------------------------------------
// Access checks
// ---------------------------------------------------------------------------

/**
 * Result of an API access check. `allowed` is false when the extension
 * lacks the required permission or security level. `requiresConfirmation`
 * is true for dangerous/restricted operations that need a user prompt.
 */
export interface ApiAccessResult {
  allowed: boolean;
  reason?: string;
  requiresConfirmation: boolean;
  confirmLabel?: string;
}

/**
 * Check if an extension at the given level with the given declared
 * permissions may call an API method. Returns an ApiAccessResult.
 *
 * G-SEC-12 req. 4: the compatibility layer only exposes read-only +
 * restricted write APIs. Dangerous commands require confirmation.
 */
export function checkApiAccess(
  method: string,
  level: ExtensionSecurityLevel,
  declaredPermissions: ExtensionPermission[],
): ApiAccessResult {
  const spec = API_SURFACE[method];
  if (!spec) {
    // Deny-by-default: unknown methods are not exposed.
    return {
      allowed: false,
      reason: `API method "${method}" is not exposed to extensions.`,
      requiresConfirmation: false,
    };
  }

  // Level gate: the extension must be at or above the method's min level.
  if (LEVEL_RANK[level] < LEVEL_RANK[spec.minLevel]) {
    return {
      allowed: false,
      reason: `API method "${method}" requires security level "${spec.minLevel}" or higher (extension is "${level}").`,
      requiresConfirmation: false,
    };
  }

  // Permission gate: the extension must have declared the required permission.
  if (spec.permission !== null) {
    if (!declaredPermissions.includes(spec.permission)) {
      return {
        allowed: false,
        reason: `API method "${method}" requires permission "${spec.permission}" which the extension did not declare.`,
        requiresConfirmation: false,
      };
    }
  }

  return {
    allowed: true,
    requiresConfirmation: spec.requiresConfirmation === true,
    confirmLabel: spec.confirmLabel,
  };
}

/**
 * Check if a command execution should require confirmation. Returns true
 * for dangerous commands (G-SEC-12 req. 4) and for shell/network methods
 * in the API surface.
 */
export function shouldConfirmCommand(
  commandId: string,
  level: ExtensionSecurityLevel,
): boolean {
  // Dangerous commands always require confirmation.
  if (isDangerousCommand(commandId)) return true;
  // Shell/network API methods require confirmation (already encoded in
  // API_SURFACE, but command IDs may differ from API method names).
  if (level === "restricted") {
    // Restricted extensions touching shell/network always confirm.
    if (commandId.startsWith("shell.") || commandId.startsWith("network.")) {
      return true;
    }
  }
  return false;
}

// ---------------------------------------------------------------------------
// Exposed API namespaces per level
//
// Used by the extension host to build the `vscode`-compatible API object
// passed to an extension's activate() function. Only the namespaces listed
// here are present on the object; absent namespaces are `undefined` so
// accessing them throws naturally.
// ---------------------------------------------------------------------------

/**
 * The list of API namespace keys exposed at each security level. Lower
 * levels are subsets of higher levels.
 */
export const EXPOSED_NAMESPACES: Record<ExtensionSecurityLevel, readonly string[]> = {
  trusted: [
    "commands",
    "languages",
    "window", // only show*Message + register*Provider
    "workspace", // only readFile / readdir
  ],
  reviewed: [
    "commands",
    "languages",
    "window",
    "workspace", // adds writeFile / applyEdit
  ],
  restricted: [
    "commands",
    "languages",
    "window",
    "workspace",
    "shell", // adds execute (with confirmation)
    "network", // adds request (with per-request approval)
  ],
};

/**
 * Returns the list of API method names an extension at the given level
 * (with the given declared permissions) may call. Used by the host to
 * build the gated proxy and by the permission dialog to list exactly
 * which APIs will be available.
 */
export function allowedMethodsFor(
  level: ExtensionSecurityLevel,
  declaredPermissions: ExtensionPermission[],
): string[] {
  const out: string[] = [];
  for (const method of Object.keys(API_SURFACE)) {
    const result = checkApiAccess(method, level, declaredPermissions);
    if (result.allowed) out.push(method);
  }
  return out;
}

/**
 * Build a human-readable summary of the API surface for an extension at
 * the given level. Used by the permission dialog's "Requested permissions"
 * list when the raw permission strings are too terse.
 */
export function apiSurfaceSummary(level: ExtensionSecurityLevel): string {
  switch (level) {
    case "trusted":
      return "Read-only: read files, register language providers and commands, show notifications.";
    case "reviewed":
      return "Read + write: create/modify files, apply edits, create webview panels.";
    case "restricted":
      return "Read + write + network/shell: execute commands and make network requests (with confirmation).";
    default:
      return "Unknown access level.";
  }
}
