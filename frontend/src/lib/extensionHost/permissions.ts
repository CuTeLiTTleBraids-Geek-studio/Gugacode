/**
 * G-VSC-02: Permission system for the VS Code Extension Host.
 *
 * VS Code extensions declare their capabilities in `package.json`. The
 * Extension Host maps those declarations to a finite set of permissions
 * and classifies each extension into a security level. The level governs
 * whether the extension can activate without explicit user approval and
 * which privileged operations are gated at runtime.
 *
 * Security levels (G-SEC-12):
 *   - Trusted:    only safe permissions (fs.read, clipboard, ui.*). May
 *                 activate without approval.
 *   - Reviewed:   declares fs.write or shell.execute. May activate, but
 *                 each privileged operation is still permission-gated at
 *                 runtime (e.g. writeFile requires fs.write).
 *   - Restricted: declares network (or any combination that includes a
 *                 Restricted-tier permission). Disabled by default — the
 *                 user must explicitly approve the extension before it
 *                 can activate.
 *
 * The permission registry is module-level so that the `vscode` API shim
 * (which lives in a separate module) can query permissions by extension
 * id without holding a back-reference to the ExtensionHost instance.
 */

/**
 * The finite set of capabilities an extension may declare. These mirror
 * the G-VSC-02 spec and map from the extension's `package.json`
 * `gugacode.permissions` (or VS Code's `contributes`/activation request).
 */
export type ExtensionPermission =
  | "fs.read"
  | "fs.write"
  | "shell.execute"
  | "network"
  | "clipboard"
  | "ui.notifications"
  | "ui.webview";

/**
 * Coarse security tier assigned from the declared permissions. Drives the
 * default-enabled behavior and the approval gate.
 */
export type SecurityLevel = "Trusted" | "Reviewed" | "Restricted";

/**
 * Risk rank per permission. Higher number = higher risk. The classifier
 * takes the max across all declared permissions. Permissions not listed
 * here default to the Trusted tier (rank 0) so unknown permissions fail
 * safe-ish: they don't elevate the level on their own, but they also
 * don't grant any privileged operation (the runtime gates check the
 * exact permission string, not the tier).
 */
const PERMISSION_RANK: Record<ExtensionPermission, number> = {
  // Trusted tier (rank 0–1)
  "fs.read": 0,
  clipboard: 0,
  "ui.notifications": 0,
  "ui.webview": 0,
  // Reviewed tier (rank 2)
  "fs.write": 2,
  "shell.execute": 2,
  // Restricted tier (rank 3)
  network: 3,
};

const REVIEWED_THRESHOLD = 2;
const RESTRICTED_THRESHOLD = 3;

/**
 * Determine the security level from requested permissions. The highest
 * risk permission wins: a single `network` declaration classifies the
 * whole extension as Restricted, even if it also declares `fs.read`.
 *
 * Empty permission list → Trusted (the extension only gets the always-
 * allowed surface like command registration).
 */
export function classifyExtension(
  permissions: ExtensionPermission[],
): SecurityLevel {
  let maxRank = 0;
  for (const perm of permissions) {
    const rank = PERMISSION_RANK[perm] ?? 0;
    if (rank > maxRank) maxRank = rank;
  }
  if (maxRank >= RESTRICTED_THRESHOLD) return "Restricted";
  if (maxRank >= REVIEWED_THRESHOLD) return "Reviewed";
  return "Trusted";
}

// ---------------------------------------------------------------------------
// Permission registry
// ---------------------------------------------------------------------------

/**
 * Module-level registry mapping extension id → declared permissions. The
 * ExtensionHost populates this on activation and clears it on
 * deactivation. The `vscode` API shim queries it via `hasPermission`
 * before dispatching privileged operations.
 */
const extensionPermissions = new Map<string, Set<ExtensionPermission>>();

/**
 * Register the permissions an extension declared. Called by the
 * ExtensionHost during activation. Re-registration overwrites the
 * previous set (idempotent for re-activation).
 */
export function registerExtensionPermissions(
  extensionId: string,
  permissions: ExtensionPermission[],
): void {
  extensionPermissions.set(extensionId, new Set(permissions));
}

/**
 * Remove an extension's permissions from the registry. Called by the
 * ExtensionHost during deactivation so that subsequent `hasPermission`
 * lookups for the extension return false.
 */
export function unregisterExtensionPermissions(extensionId: string): void {
  extensionPermissions.delete(extensionId);
}

/**
 * Check if an extension has a specific permission. Returns false for
 * unknown extensions (fail-closed: no permission record → no access).
 */
export function hasPermission(
  extensionId: string,
  permission: ExtensionPermission,
): boolean {
  const set = extensionPermissions.get(extensionId);
  return set ? set.has(permission) : false;
}

/**
 * Clear the entire permission registry. Used in tests and on a full
 * extension reset.
 */
export function clearPermissionRegistry(): void {
  extensionPermissions.clear();
}
