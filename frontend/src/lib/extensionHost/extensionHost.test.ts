/**
 * Tests for the G-VSC-02 Extension Host.
 *
 * Covers:
 *   - Extension activation / deactivation lifecycle
 *   - Disposable tracking and cleanup on deactivate
 *   - Permission classification (Trusted / Reviewed / Restricted)
 *   - Restricted extensions disabled by default
 *   - Dangerous command confirmation (G-SEC-12)
 *   - workspace.fs bridging to FileService with permission gating
 *   - Monaco language-provider bridging
 *   - Webview panel creation with sandbox="allow-scripts" (G-SEC-05)
 */

import { describe, it, expect, beforeEach, vi } from "vitest";

// Mock @/stores/app to break the Monaco editor import chain in jsdom
// (mirrors pluginRegistry.test.ts). appState.currentProject is the
// workspace root used to resolve relative URIs from extensions.
vi.mock("@/stores/app", () => ({
  appState: {
    currentProject: "/test/project",
    language: "en",
  },
}));

// Mock @/api/services so workspace.fs bridges to a controllable fake
// instead of the real Wails FileService bindings (which are absent in
// jsdom).
const readFileMock = vi.fn<(path: string) => Promise<string>>();
const writeFileMock = vi.fn<(path: string, content: string) => Promise<void>>();
vi.mock("@/api/services", () => ({
  fileService: {
    readFile: (path: string) => readFileMock(path),
    writeFile: (path: string, content: string) => writeFileMock(path, content),
  },
}));

import {
  ExtensionHost,
  type ExtensionDescriptor,
  type ExtensionModule,
} from "@/lib/extensionHost/extensionHost";
import {
  classifyExtension,
  hasPermission,
  clearPermissionRegistry,
  registerExtensionPermissions,
} from "@/lib/extensionHost/permissions";
import type { VscodeAPI } from "@/lib/extensionHost/vscodeApi";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeDescriptor(
  overrides: Partial<ExtensionDescriptor> = {},
): ExtensionDescriptor {
  return {
    id: "test.ext",
    mainPath: "/exts/test/main.js",
    permissions: [],
    ...overrides,
  };
}

beforeEach(() => {
  readFileMock.mockReset();
  writeFileMock.mockReset();
  clearPermissionRegistry();
});

// ---------------------------------------------------------------------------
// Permission classification
// ---------------------------------------------------------------------------

describe("classifyExtension", () => {
  it("classifies an extension with no permissions as Trusted", () => {
    expect(classifyExtension([])).toBe("Trusted");
  });

  it("classifies an extension with only fs.read as Trusted", () => {
    expect(classifyExtension(["fs.read"])).toBe("Trusted");
  });

  it("classifies safe permissions (clipboard, ui.notifications, ui.webview) as Trusted", () => {
    expect(
      classifyExtension(["clipboard", "ui.notifications", "ui.webview"]),
    ).toBe("Trusted");
  });

  it("classifies fs.write as Reviewed", () => {
    expect(classifyExtension(["fs.write"])).toBe("Reviewed");
  });

  it("classifies shell.execute as Reviewed", () => {
    expect(classifyExtension(["shell.execute"])).toBe("Reviewed");
  });

  it("classifies network as Restricted", () => {
    expect(classifyExtension(["network"])).toBe("Restricted");
  });

  it("classifies a mix of safe + Reviewed as Reviewed", () => {
    expect(classifyExtension(["fs.read", "fs.write"])).toBe("Reviewed");
  });

  it("classifies a mix containing network as Restricted (highest risk wins)", () => {
    expect(classifyExtension(["fs.read", "fs.write", "network"])).toBe(
      "Restricted",
    );
  });

  it("classifies shell.execute + network as Restricted", () => {
    expect(classifyExtension(["shell.execute", "network"])).toBe("Restricted");
  });
});

describe("hasPermission", () => {
  it("returns true when the extension declared the permission", () => {
    registerExtensionPermissions("alpha", ["fs.read", "fs.write"]);
    expect(hasPermission("alpha", "fs.read")).toBe(true);
    expect(hasPermission("alpha", "fs.write")).toBe(true);
  });

  it("returns false when the extension did not declare the permission", () => {
    registerExtensionPermissions("alpha", ["fs.read"]);
    expect(hasPermission("alpha", "fs.write")).toBe(false);
  });

  it("returns false for an unknown extension", () => {
    expect(hasPermission("unknown", "fs.read")).toBe(false);
  });

  it("returns false after the extension permissions are unregistered", () => {
    registerExtensionPermissions("alpha", ["fs.read"]);
    expect(hasPermission("alpha", "fs.read")).toBe(true);
    clearPermissionRegistry();
    expect(hasPermission("alpha", "fs.read")).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Extension activation / deactivation
// ---------------------------------------------------------------------------

describe("ExtensionHost activation", () => {
  it("activates an extension and invokes its activate() with the vscode API", async () => {
    const host = new ExtensionHost();
    const activate = vi.fn<(api: VscodeAPI) => Promise<void>>();
    const module: ExtensionModule = { activate };
    const desc = makeDescriptor({ id: "alpha" });

    await host.activateWithModule(desc, module);

    expect(activate).toHaveBeenCalledTimes(1);
    const api = activate.mock.calls[0][0];
    expect(api.languages).toBeDefined();
    expect(api.commands).toBeDefined();
    expect(api.workspace).toBeDefined();
    expect(api.window).toBeDefined();
    expect(host.isActive("alpha")).toBe(true);
  });

  it("records the security level on activation", async () => {
    const host = new ExtensionHost();
    await host.activateWithModule(
      makeDescriptor({ id: "alpha", permissions: ["fs.write"] }),
      { activate: () => {} },
    );
    expect(host.getSecurityLevel("alpha")).toBe("Reviewed");
  });

  it("throws when activating an unknown extension via activate()", async () => {
    const host = new ExtensionHost({
      loadModule: vi.fn(),
    });
    await expect(host.activate(makeDescriptor({ id: "ghost" }))).rejects.toThrow(
      /loadModule|failed|not found|main/i,
    );
  });

  it("is a no-op when activating an already-active extension", async () => {
    const host = new ExtensionHost();
    const activate = vi.fn(() => Promise.resolve());
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), { activate });
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), { activate });
    expect(activate).toHaveBeenCalledTimes(1);
  });

  it("captures activation errors and does not mark the extension active", async () => {
    const host = new ExtensionHost();
    await expect(
      host.activateWithModule(makeDescriptor({ id: "alpha" }), {
        activate: () => {
          throw new Error("boom");
        },
      }),
    ).rejects.toThrow(/boom/);
    expect(host.isActive("alpha")).toBe(false);
  });
});

describe("ExtensionHost deactivation", () => {
  it("calls deactivate() and disposes all tracked disposables", async () => {
    const host = new ExtensionHost();
    const disposed: string[] = [];
    const deactivate = vi.fn(() => Promise.resolve());

    await host.activateWithModule(makeDescriptor({ id: "alpha" }), {
      activate: (api: VscodeAPI) => {
        api.commands.registerCommand("alpha.cmd", () => undefined);
        // Register a custom disposable to verify cleanup ordering.
        api.languages.registerCompletionItemProvider(
          { language: "go" },
          { provideCompletionItems: () => ({ items: [] }) },
        );
        // Track a manual disposable.
        const tracked = host.trackDisposable("alpha", {
          dispose: () => {
            disposed.push("manual");
          },
        });
        void tracked;
      },
      deactivate,
    });

    await host.deactivate("alpha");

    expect(deactivate).toHaveBeenCalledTimes(1);
    expect(host.isActive("alpha")).toBe(false);
    expect(disposed).toEqual(["manual"]);
  });

  it("is a no-op for an extension that is not active", async () => {
    const host = new ExtensionHost();
    await host.deactivate("alpha");
    expect(host.isActive("alpha")).toBe(false);
  });

  it("unregisters extension permissions on deactivate", async () => {
    const host = new ExtensionHost();
    await host.activateWithModule(
      makeDescriptor({ id: "alpha", permissions: ["fs.read"] }),
      { activate: () => {} },
    );
    expect(hasPermission("alpha", "fs.read")).toBe(true);
    await host.deactivate("alpha");
    expect(hasPermission("alpha", "fs.read")).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Restricted extensions disabled by default
// ---------------------------------------------------------------------------

describe("Restricted extensions are disabled by default", () => {
  it("refuses to activate a Restricted extension without explicit approval", async () => {
    const host = new ExtensionHost();
    const activate = vi.fn(() => Promise.resolve());
    await expect(
      host.activateWithModule(
        makeDescriptor({ id: "net.ext", permissions: ["network"] }),
        { activate },
      ),
    ).rejects.toThrow(/Restricted|disabled|approval/i);
    expect(activate).not.toHaveBeenCalled();
    expect(host.isActive("net.ext")).toBe(false);
  });

  it("activates a Restricted extension when explicitly approved", async () => {
    const host = new ExtensionHost();
    const activate = vi.fn(() => Promise.resolve());
    host.approveExtension("net.ext");
    await host.activateWithModule(
      makeDescriptor({ id: "net.ext", permissions: ["network"] }),
      { activate },
    );
    expect(activate).toHaveBeenCalledTimes(1);
    expect(host.isActive("net.ext")).toBe(true);
  });

  it("activates a Reviewed extension without prior approval (Reviewed needs runtime approval, not activation block)", async () => {
    // Reviewed extensions are allowed to activate; the runtime approval
    // gate applies to the privileged operations themselves (e.g. fs.write
    // requires the fs.write permission, which is already declared).
    const host = new ExtensionHost();
    const activate = vi.fn(() => Promise.resolve());
    await host.activateWithModule(
      makeDescriptor({ id: "write.ext", permissions: ["fs.write"] }),
      { activate },
    );
    expect(activate).toHaveBeenCalledTimes(1);
    expect(host.getSecurityLevel("write.ext")).toBe("Reviewed");
  });

  it("getSecurityLevel returns the classified level for an active extension", async () => {
    const host = new ExtensionHost();
    await host.activateWithModule(
      makeDescriptor({ id: "safe.ext", permissions: ["fs.read"] }),
      { activate: () => {} },
    );
    expect(host.getSecurityLevel("safe.ext")).toBe("Trusted");
  });

  it("getSecurityLevel returns undefined for an inactive extension", () => {
    const host = new ExtensionHost();
    expect(host.getSecurityLevel("never.activated")).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// Dangerous command confirmation (G-SEC-12)
// ---------------------------------------------------------------------------

describe("dangerous commands require confirmation (G-SEC-12)", () => {
  it("rejects workbench.action.terminal.sendSequence without a confirm handler", async () => {
    const host = new ExtensionHost();
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), {
      activate: (api: VscodeAPI) => {
        api.commands.registerCommand("alpha.safe", () => "safe-result");
      },
    });
    await expect(
      host.executeCommand("workbench.action.terminal.sendSequence", { text: "rm -rf /" }),
    ).rejects.toThrow(/confirm|denied|dangerous/i);
  });

  it("calls the confirm handler for workbench.action.terminal.sendSequence", async () => {
    const confirmHandler = vi.fn<(cmd: string, args: unknown[]) => Promise<boolean>>(
      async () => true,
    );
    const host = new ExtensionHost({ confirmHandler });
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), {
      activate: (api: VscodeAPI) => {
        api.commands.registerCommand(
          "workbench.action.terminal.sendSequence",
          () => "ran",
        );
      },
    });
    await host.executeCommand("workbench.action.terminal.sendSequence", "ls");
    expect(confirmHandler).toHaveBeenCalledTimes(1);
    expect(confirmHandler.mock.calls[0][0]).toBe(
      "workbench.action.terminal.sendSequence",
    );
  });

  it("executes the dangerous command when the confirm handler approves", async () => {
    const host = new ExtensionHost({
      confirmHandler: async () => true,
    });
    let executed = false;
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), {
      activate: (api: VscodeAPI) => {
        // The extension registers a command with a dangerous-looking id.
        api.commands.registerCommand(
          "workbench.action.terminal.sendSequence",
          () => {
            executed = true;
            return "sent";
          },
        );
      },
    });
    const result = await host.executeCommand(
      "workbench.action.terminal.sendSequence",
      "ls",
    );
    expect(executed).toBe(true);
    expect(result).toBe("sent");
  });

  it("rejects the dangerous command when the confirm handler denies", async () => {
    const host = new ExtensionHost({
      confirmHandler: async () => false,
    });
    let executed = false;
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), {
      activate: (api: VscodeAPI) => {
        api.commands.registerCommand(
          "workbench.action.terminal.sendSequence",
          () => {
            executed = true;
          },
        );
      },
    });
    await expect(
      host.executeCommand("workbench.action.terminal.sendSequence", "ls"),
    ).rejects.toThrow(/denied|rejected|confirm/i);
    expect(executed).toBe(false);
  });

  it("requires confirmation for _workbench.* commands", async () => {
    const confirmHandler = vi.fn<(cmd: string, args: unknown[]) => Promise<boolean>>(
      async () => true,
    );
    const host = new ExtensionHost({ confirmHandler });
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), {
      activate: (api: VscodeAPI) => {
        api.commands.registerCommand("_workbench.internal", () => "secret");
      },
    });
    await host.executeCommand("_workbench.internal");
    expect(confirmHandler).toHaveBeenCalledTimes(1);
    expect(confirmHandler.mock.calls[0][0]).toBe("_workbench.internal");
  });

  it("does not require confirmation for ordinary commands", async () => {
    const confirmHandler = vi.fn<(cmd: string, args: unknown[]) => Promise<boolean>>();
    const host = new ExtensionHost({ confirmHandler });
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), {
      activate: (api: VscodeAPI) => {
        api.commands.registerCommand("alpha.safe", () => "safe");
      },
    });
    const result = await host.executeCommand("alpha.safe");
    expect(result).toBe("safe");
    expect(confirmHandler).not.toHaveBeenCalled();
  });

  it("vscode.commands.executeCommand routes dangerous commands through confirmation", async () => {
    const confirmHandler = vi.fn<(cmd: string, args: unknown[]) => Promise<boolean>>(
      async () => false,
    );
    const host = new ExtensionHost({ confirmHandler });
    let executed = false;
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), {
      activate: async (api: VscodeAPI) => {
        api.commands.registerCommand(
          "workbench.action.terminal.sendSequence",
          () => {
            executed = true;
          },
        );
        // Extension-initiated execution must also go through the gate.
        await expect(
          api.commands.executeCommand(
            "workbench.action.terminal.sendSequence",
            "rm",
          ),
        ).rejects.toThrow(/denied|rejected|confirm/i);
      },
    });
    expect(executed).toBe(false);
    expect(confirmHandler).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// workspace.fs bridging (G-SEC-12 permission gating)
// ---------------------------------------------------------------------------

describe("workspace.fs bridges to FileService", () => {
  it("readFile delegates to fileService.readFile and returns a Uint8Array", async () => {
    readFileMock.mockResolvedValue("hello world");
    const host = new ExtensionHost();
    let result: Uint8Array | undefined;
    await host.activateWithModule(
      makeDescriptor({ id: "reader", permissions: ["fs.read"] }),
      {
        activate: async (api: VscodeAPI) => {
          result = await api.workspace.fs.readFile({
            fsPath: "src/foo.txt",
            scheme: "file",
          });
        },
      },
    );
    expect(readFileMock).toHaveBeenCalledTimes(1);
    // The bridge resolves relative paths against the workspace root.
    expect(readFileMock.mock.calls[0][0]).toBe("/test/project/src/foo.txt");
    expect(result).toBeInstanceOf(Uint8Array);
    expect(new TextDecoder().decode(result!)).toBe("hello world");
  });

  it("readFile throws without fs.read permission", async () => {
    const host = new ExtensionHost();
    await host.activateWithModule(makeDescriptor({ id: "reader" }), {
      activate: async (api: VscodeAPI) => {
        await expect(
          api.workspace.fs.readFile({ fsPath: "foo.txt", scheme: "file" }),
        ).rejects.toThrow(/fs\.read|permission/i);
      },
    });
    expect(readFileMock).not.toHaveBeenCalled();
  });

  it("writeFile delegates to fileService.writeFile with fs.write permission", async () => {
    writeFileMock.mockResolvedValue(undefined);
    const host = new ExtensionHost();
    await host.activateWithModule(
      makeDescriptor({ id: "writer", permissions: ["fs.write"] }),
      {
        activate: async (api: VscodeAPI) => {
          await api.workspace.fs.writeFile(
            { fsPath: "out.txt", scheme: "file" },
            new TextEncoder().encode("content"),
          );
        },
      },
    );
    expect(writeFileMock).toHaveBeenCalledTimes(1);
    expect(writeFileMock.mock.calls[0][0]).toBe("/test/project/out.txt");
    expect(writeFileMock.mock.calls[0][1]).toBe("content");
  });

  it("writeFile throws without fs.write permission", async () => {
    const host = new ExtensionHost();
    await host.activateWithModule(
      makeDescriptor({ id: "writer", permissions: ["fs.read"] }),
      {
        activate: async (api: VscodeAPI) => {
          await expect(
            api.workspace.fs.writeFile(
              { fsPath: "out.txt", scheme: "file" },
              new TextEncoder().encode("x"),
            ),
          ).rejects.toThrow(/fs\.write|permission/i);
        },
      },
    );
    expect(writeFileMock).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Monaco language-provider bridging
// ---------------------------------------------------------------------------

describe("Monaco language-provider bridging", () => {
  it("registerCompletionItemProvider delegates to monaco.languages.registerCompletionItemProvider", async () => {
    const registerCompletion = vi.fn(() => ({ dispose: vi.fn() }));
    const monaco = {
      languages: {
        registerCompletionItemProvider: registerCompletion,
        registerHoverProvider: vi.fn(() => ({ dispose: vi.fn() })),
      },
    };
    const host = new ExtensionHost({ monaco });
    const provider = {
      provideCompletionItems: vi.fn(() => ({ items: [] as { label: string }[] })),
    };
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), {
      activate: (api: VscodeAPI) => {
        api.languages.registerCompletionItemProvider(
          { language: "go" },
          provider,
        );
      },
    });
    expect(registerCompletion).toHaveBeenCalledTimes(1);
    // Monaco selector is derived from the vscode DocumentSelector.
    const call0 = registerCompletion.mock.calls[0] as unknown as [string, unknown];
    expect(call0[0]).toBe("go");
  });

  it("registerHoverProvider delegates to monaco.languages.registerHoverProvider", async () => {
    const registerHover = vi.fn(() => ({ dispose: vi.fn() }));
    const monaco = {
      languages: {
        registerCompletionItemProvider: vi.fn(() => ({ dispose: vi.fn() })),
        registerHoverProvider: registerHover,
      },
    };
    const host = new ExtensionHost({ monaco });
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), {
      activate: (api: VscodeAPI) => {
        api.languages.registerHoverProvider(
          { language: "typescript" },
          { provideHover: () => null },
        );
      },
    });
    expect(registerHover).toHaveBeenCalledTimes(1);
    const call0 = registerHover.mock.calls[0] as unknown as [string, unknown];
    expect(call0[0]).toBe("typescript");
  });
});

// ---------------------------------------------------------------------------
// Webview panel creation (G-SEC-05 sandbox)
// ---------------------------------------------------------------------------

describe("window.createWebviewPanel", () => {
  it("creates a panel and tracks it for disposal", async () => {
    const host = new ExtensionHost();
    let panel: ReturnType<VscodeAPI["window"]["createWebviewPanel"]> | undefined;
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), {
      activate: (api: VscodeAPI) => {
        panel = api.window.createWebviewPanel(
          "alpha.preview",
          "Preview",
          {},
          {},
        );
      },
    });
    expect(panel).toBeDefined();
    expect(panel!.webview).toBeDefined();
    // Setting HTML should not throw.
    panel!.webview.html = "<p>hello</p>";
    expect(panel!.webview.html).toBe("<p>hello</p>");
    // The underlying iframe must use sandbox="allow-scripts" (G-SEC-05).
    const iframe = panel!.webview._iframe as HTMLIFrameElement | undefined;
    expect(iframe).toBeDefined();
    expect(iframe!.getAttribute("sandbox")).toBe("allow-scripts");
  });

  it("disposes the panel (removes the iframe) on deactivate", async () => {
    const host = new ExtensionHost();
    let panel: ReturnType<VscodeAPI["window"]["createWebviewPanel"]> | undefined;
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), {
      activate: (api: VscodeAPI) => {
        panel = api.window.createWebviewPanel("alpha.preview", "Preview", {}, {});
      },
    });
    const iframe = panel!.webview._iframe as HTMLIFrameElement;
    expect(document.body.contains(iframe)).toBe(true);
    await host.deactivate("alpha");
    expect(document.body.contains(iframe)).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Command registration & host-level executeCommand
// ---------------------------------------------------------------------------

describe("commands registration and execution", () => {
  it("registerCommand registers a command that executeCommand can invoke", async () => {
    const host = new ExtensionHost();
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), {
      activate: (api: VscodeAPI) => {
        api.commands.registerCommand("alpha.greet", (...args: unknown[]) => {
          const name = String(args[0] ?? "");
          return `hi ${name}`;
        });
      },
    });
    const result = await host.executeCommand("alpha.greet", "world");
    expect(result).toBe("hi world");
  });

  it("executeCommand throws for an unknown command", async () => {
    const host = new ExtensionHost();
    await expect(host.executeCommand("nope")).rejects.toThrow(/not registered/i);
  });

  it("registered commands are disposed on deactivate", async () => {
    const host = new ExtensionHost();
    await host.activateWithModule(makeDescriptor({ id: "alpha" }), {
      activate: (api: VscodeAPI) => {
        api.commands.registerCommand("alpha.cmd", () => undefined);
      },
    });
    await host.deactivate("alpha");
    await expect(host.executeCommand("alpha.cmd")).rejects.toThrow(/not registered/i);
  });
});
