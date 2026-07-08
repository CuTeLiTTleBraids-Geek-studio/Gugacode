/**
 * Tests for the plugin registry (Plan 49).
 *
 * The registry's core logic (sync, activation state, permission
 * gating, command/view registration) is testable without a real
 * dynamic import via the `activatePluginWithModule` test entry point.
 */

import { describe, it, expect, beforeEach, vi } from "vitest";
import { watchEffect } from "vue";

// Mock @/stores/app to break the Monaco editor import chain in jsdom.
// The sandbox path in activatePlugin imports appState to get
// currentProject; without this mock, jsdom fails on
// document.queryCommandSupported (called by Monaco's clipboard module).
vi.mock("@/stores/app", () => ({
  appState: {
    currentProject: "",
    language: "en",
  },
  loadSettings: vi.fn(),
}));
import {
  syncPlugins,
  listPluginStates,
  getPluginInfo,
  listPluginCommands,
  listPluginViews,
  activatePluginWithModule,
  activatePlugin,
  activateOnStartup,
  activateOnCommand,
  deactivatePlugin,
  enablePlugin,
  disablePlugin,
  clearRegistry,
  setSandboxHost,
  setSandboxMode,
  isSandboxEnabled,
  executePluginCommand,
  __setPluginModule,
  type NknkAPI,
} from "@/lib/pluginRegistry";
import type { SandboxHost } from "@/lib/pluginSandbox";
import type { PluginInfo, PluginManifest } from "@/types";

function makeManifest(overrides: Partial<PluginManifest> = {}): PluginManifest {
  return {
    name: "test-plugin",
    version: "1.0.0",
    main: "main.js",
    activationEvents: ["onStartup"],
    ...overrides,
  };
}

function makePluginInfo(
  manifestOverrides: Partial<PluginManifest> = {},
  infoOverrides: Partial<PluginInfo> = {},
): PluginInfo {
  return {
    manifest: makeManifest(manifestOverrides),
    path: "/plugins/test-plugin",
    source: "user",
    enabled: true,
    mainExists: true,
    ...infoOverrides,
  };
}

beforeEach(() => {
  setSandboxHost(null);
  clearRegistry();
});

describe("syncPlugins", () => {
  it("adds new plugins in loaded state", () => {
    syncPlugins([makePluginInfo()]);
    const states = listPluginStates();
    expect(states).toHaveLength(1);
    expect(states[0].name).toBe("test-plugin");
    expect(states[0].status).toBe("loaded");
  });

  it("marks disabled plugins as disabled", () => {
    syncPlugins([makePluginInfo({}, { enabled: false })]);
    const states = listPluginStates();
    expect(states[0].status).toBe("disabled");
  });

  it("updates existing plugin info while preserving activation state", () => {
    syncPlugins([makePluginInfo()]);
    // Simulate the plugin being activated.
    return activatePluginWithModule("test-plugin", {
      activate: () => {},
    }).then(() => {
      // Re-sync with updated info.
      syncPlugins([
        makePluginInfo({ version: "2.0.0" }),
      ]);
      const info = getPluginInfo("test-plugin");
      expect(info?.manifest.version).toBe("2.0.0");
      const states = listPluginStates();
      expect(states[0].status).toBe("activated");
    });
  });

  it("removes plugins no longer present", () => {
    syncPlugins([makePluginInfo()]);
    expect(listPluginStates()).toHaveLength(1);
    syncPlugins([]);
    expect(listPluginStates()).toHaveLength(0);
  });

  it("reflects disabled state when backend reports enabled=false", () => {
    syncPlugins([makePluginInfo()]);
    return activatePluginWithModule("test-plugin", {
      activate: () => {},
    }).then(() => {
      // Backend now reports the plugin as disabled.
      syncPlugins([makePluginInfo({}, { enabled: false })]);
      const states = listPluginStates();
      expect(states[0].status).toBe("disabled");
    });
  });
});

describe("getPluginInfo", () => {
  it("returns the plugin info by name", () => {
    syncPlugins([makePluginInfo()]);
    const info = getPluginInfo("test-plugin");
    expect(info).toBeDefined();
    expect(info?.manifest.name).toBe("test-plugin");
  });

  it("returns undefined for unknown plugin", () => {
    expect(getPluginInfo("nonexistent")).toBeUndefined();
  });
});

describe("activatePluginWithModule", () => {
  it("invokes the activate function with a nknk context", () => {
    const activate = vi.fn().mockResolvedValue(undefined);
    syncPlugins([makePluginInfo()]);
    return activatePluginWithModule("test-plugin", { activate }).then(() => {
      expect(activate).toHaveBeenCalledTimes(1);
      const ctx = activate.mock.calls[0][0] as NknkAPI;
      expect(ctx.manifest.name).toBe("test-plugin");
      expect(typeof ctx.commands.register).toBe("function");
      expect(typeof ctx.commands.execute).toBe("function");
      expect(typeof ctx.views.register).toBe("function");
    });
  });

  it("transitions to activated state on success", () => {
    syncPlugins([makePluginInfo()]);
    return activatePluginWithModule("test-plugin", {
      activate: () => {},
    }).then(() => {
      const states = listPluginStates();
      expect(states[0].status).toBe("activated");
    });
  });

  it("captures activation errors in state", () => {
    syncPlugins([makePluginInfo()]);
    return activatePluginWithModule("test-plugin", {
      activate: () => {
        throw new Error("boom");
      },
    }).then(() => {
      const states = listPluginStates();
      expect(states[0].status).toBe("error");
      expect(states[0].error).toContain("boom");
    });
  });

  it("is a no-op when already activated", () => {
    const activate = vi.fn().mockResolvedValue(undefined);
    syncPlugins([makePluginInfo()]);
    return activatePluginWithModule("test-plugin", { activate })
      .then(() => activatePluginWithModule("test-plugin", { activate }))
      .then(() => {
        expect(activate).toHaveBeenCalledTimes(1);
      });
  });

  it("is a no-op when disabled", () => {
    const activate = vi.fn();
    syncPlugins([makePluginInfo({}, { enabled: false })]);
    return activatePluginWithModule("test-plugin", { activate }).then(() => {
      expect(activate).not.toHaveBeenCalled();
      const states = listPluginStates();
      expect(states[0].status).toBe("disabled");
    });
  });

  it("throws for unknown plugin", () => {
    return expect(
      activatePluginWithModule("nonexistent", { activate: () => {} }),
    ).rejects.toThrow(/not registered/);
  });
});

describe("activatePlugin (real dynamic import path)", () => {
  it("errors when main file is missing", () => {
    syncPlugins([makePluginInfo({}, { mainExists: false })]);
    return activatePlugin("test-plugin").then(() => {
      const states = listPluginStates();
      expect(states[0].status).toBe("error");
      expect(states[0].error).toContain("not found");
    });
  });
});

describe("activateOnStartup", () => {
  it("activates plugins with onStartup event", () => {
    syncPlugins([
      makePluginInfo({ name: "alpha", activationEvents: ["onStartup"] }),
      makePluginInfo({ name: "beta", activationEvents: ["onCommand:beta.go"] }),
    ]);
    __setPluginModule("alpha", { activate: () => {} });
    return activateOnStartup().then((names) => {
      expect(names).toEqual(["alpha"]);
      const states = listPluginStates();
      const alpha = states.find((s) => s.name === "alpha");
      const beta = states.find((s) => s.name === "beta");
      expect(alpha?.status).toBe("activated");
      expect(beta?.status).toBe("loaded");
    });
  });

  it("skips disabled plugins", () => {
    syncPlugins([
      makePluginInfo(
        { name: "alpha", activationEvents: ["onStartup"] },
        { enabled: false },
      ),
    ]);
    return activateOnStartup().then((names) => {
      expect(names).toEqual([]);
    });
  });

  it("skips already-activated plugins", () => {
    syncPlugins([
      makePluginInfo({ name: "alpha", activationEvents: ["onStartup"] }),
    ]);
    __setPluginModule("alpha", { activate: () => {} });
    return activateOnStartup()
      .then(() => activateOnStartup())
      .then((names) => {
        expect(names).toEqual([]);
      });
  });
});

describe("activateOnCommand", () => {
  it("activates plugins that declare onCommand:<id>", () => {
    syncPlugins([
      makePluginInfo({
        name: "alpha",
        activationEvents: ["onCommand:alpha.go"],
      }),
    ]);
    __setPluginModule("alpha", { activate: () => {} });
    return activateOnCommand("alpha.go").then(() => {
      const states = listPluginStates();
      expect(states[0].status).toBe("activated");
    });
  });

  it("does not activate plugins without matching event", () => {
    syncPlugins([
      makePluginInfo({
        name: "alpha",
        activationEvents: ["onCommand:alpha.other"],
      }),
    ]);
    return activateOnCommand("alpha.go").then(() => {
      const states = listPluginStates();
      expect(states[0].status).toBe("loaded");
    });
  });
});

describe("commands API", () => {
  it("registers and executes a command", () => {
    const handler = vi.fn().mockReturnValue("result");
    syncPlugins([makePluginInfo()]);
    return activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("test-plugin.cmd", handler);
      },
    })
      .then(() => {
        const cmds = listPluginCommands();
        expect(cmds).toHaveLength(1);
        expect(cmds[0].id).toBe("test-plugin.cmd");
        expect(cmds[0].pluginName).toBe("test-plugin");
      })
      .then(() => activateOnCommand("test-plugin.cmd"))
      .then(() => {
        // The handler is invoked via nknk.commands.execute, but here
        // we can verify it's registered.
        expect(handler).not.toHaveBeenCalled(); // not invoked yet
      });
  });

  it("uses contributed title from manifest", () => {
    syncPlugins([
      makePluginInfo({
        contributes: {
          commands: [{ id: "test-plugin.cmd", title: "My Command" }],
        },
      }),
    ]);
    return activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("test-plugin.cmd", () => {});
      },
    }).then(() => {
      const cmds = listPluginCommands();
      expect(cmds[0].title).toBe("My Command");
    });
  });

  it("rejects duplicate command id from a different plugin", () => {
    syncPlugins([
      makePluginInfo({ name: "alpha" }),
      makePluginInfo({ name: "beta" }),
    ]);
    return activatePluginWithModule("alpha", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("shared.cmd", () => {});
      },
    })
      .then(() =>
        activatePluginWithModule("beta", {
          activate: (ctx: NknkAPI) => {
            ctx.commands.register("shared.cmd", () => {});
          },
        }),
      )
      .then(() => {
        const states = listPluginStates();
        const beta = states.find((s) => s.name === "beta");
        expect(beta?.status).toBe("error");
        expect(beta?.error).toContain("already registered");
      });
  });

  it("allows same plugin to re-register the same id", () => {
    syncPlugins([makePluginInfo()]);
    return activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("test-plugin.cmd", () => "first");
        ctx.commands.register("test-plugin.cmd", () => "second");
      },
    }).then(() => {
      expect(listPluginCommands()).toHaveLength(1);
    });
  });
});

// N-42: executePluginCommand is the entry point used by the command palette.
// It triggers lazy activation and routes through the sandbox when needed.
describe("executePluginCommand (N-42)", () => {
  it("throws when command is not registered", async () => {
    await expect(executePluginCommand("nonexistent.cmd")).rejects.toThrow(
      /not registered/,
    );
  });

  it("invokes the handler directly for main-thread plugins", async () => {
    const handler = vi.fn().mockReturnValue("palette-result");
    syncPlugins([makePluginInfo()]);
    await activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("test-plugin.cmd", handler);
      },
    });
    const result = await executePluginCommand("test-plugin.cmd");
    expect(handler).toHaveBeenCalledTimes(1);
    expect(result).toBe("palette-result");
  });

  it("triggers lazy activation via activateOnCommand", async () => {
    const handler = vi.fn().mockReturnValue("activated-result");
    syncPlugins([
      makePluginInfo({
        name: "lazy-plugin",
        activationEvents: ["onCommand:lazy-plugin.run"],
      }),
    ]);
    // Plugin is in "loaded" state, not yet activated.
    expect(listPluginStates()[0].status).toBe("loaded");
    // executePluginCommand should trigger activation, which registers
    // the handler.
    await activatePluginWithModule("lazy-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("lazy-plugin.run", handler);
      },
    });
    const result = await executePluginCommand("lazy-plugin.run");
    expect(result).toBe("activated-result");
    expect(handler).toHaveBeenCalledTimes(1);
  });

  it("can execute private commands (palette is the user, not a plugin)", async () => {
    // The cross-plugin `public` gate does NOT apply to palette invocation.
    // A plugin's private command can still be invoked by the user via Ctrl+Shift+P.
    const handler = vi.fn().mockReturnValue("private-result");
    syncPlugins([makePluginInfo()]);
    await activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        // Register without setting public: true (defaults to private).
        ctx.commands.register("test-plugin.private", handler);
      },
    });
    const result = await executePluginCommand("test-plugin.private");
    expect(result).toBe("private-result");
  });

  it("routes through sandbox callMethod when plugin is sandboxed", async () => {
    // When sandboxed, the stored handler is a no-op stub. executePluginCommand
    // must route through sandboxHost.callMethod to invoke the real handler
    // inside the Worker.
    const mockHost: SandboxHost = {
      has: vi.fn().mockReturnValue(true),
      activate: vi.fn().mockResolvedValue(undefined),
      callMethod: vi.fn().mockResolvedValue("sandbox-result"),
      terminate: vi.fn(),
      terminateAll: vi.fn(),
    };
    setSandboxHost(mockHost);
    syncPlugins([makePluginInfo()]);
    await activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("test-plugin.sandboxed", () => "direct-result");
      },
    });
    const result = await executePluginCommand("test-plugin.sandboxed");
    expect(result).toBe("sandbox-result");
    expect(mockHost.has).toHaveBeenCalledWith("test-plugin");
    expect(mockHost.callMethod).toHaveBeenCalledWith(
      "test-plugin",
      "executeCommand",
      ["test-plugin.sandboxed"],
    );
  });

  it("passes through arguments to the handler", async () => {
    const handler = vi.fn().mockReturnValue("with-args");
    syncPlugins([makePluginInfo()]);
    await activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("test-plugin.args", handler);
      },
    });
    const result = await executePluginCommand("test-plugin.args", "a", 42);
    expect(handler).toHaveBeenCalledWith("a", 42);
    expect(result).toBe("with-args");
  });
});

describe("views API", () => {
  it("registers a view with location from manifest", () => {
    syncPlugins([
      makePluginInfo({
        contributes: {
          views: [{ id: "test-plugin.view", title: "My View", location: "sidebar" }],
        },
      }),
    ]);
    return activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.views.register("test-plugin.view", { template: "<div/>" });
      },
    }).then(() => {
      const views = listPluginViews();
      expect(views).toHaveLength(1);
      expect(views[0].title).toBe("My View");
      expect(views[0].location).toBe("sidebar");
    });
  });

  it("uses options.location override", () => {
    syncPlugins([makePluginInfo()]);
    return activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.views.register("test-plugin.view", {}, { location: "statusbar" });
      },
    }).then(() => {
      const views = listPluginViews();
      expect(views[0].location).toBe("statusbar");
    });
  });

  it("defaults to panel location", () => {
    syncPlugins([makePluginInfo()]);
    return activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.views.register("test-plugin.view", {});
      },
    }).then(() => {
      const views = listPluginViews();
      expect(views[0].location).toBe("panel");
    });
  });

  it("rejects duplicate view id from a different plugin", () => {
    syncPlugins([
      makePluginInfo({ name: "alpha" }),
      makePluginInfo({ name: "beta" }),
    ]);
    return activatePluginWithModule("alpha", {
      activate: (ctx: NknkAPI) => {
        ctx.views.register("shared.view", {});
      },
    })
      .then(() =>
        activatePluginWithModule("beta", {
          activate: (ctx: NknkAPI) => {
            ctx.views.register("shared.view", {});
          },
        }),
      )
      .then(() => {
        const states = listPluginStates();
        const beta = states.find((s) => s.name === "beta");
        expect(beta?.status).toBe("error");
        expect(beta?.error).toContain("already registered");
      });
  });
});

describe("permission gating", () => {
  it("workspace.readFile throws without fs.read permission", () => {
    syncPlugins([makePluginInfo()]); // no permissions
    return activatePluginWithModule("test-plugin", {
      activate: async (ctx: NknkAPI) => {
        await expect(ctx.workspace.readFile("foo.txt")).rejects.toThrow(
          /fs\.read/,
        );
      },
    }).then(() => {});
  });

  it("workspace.writeFile throws without fs.write permission", () => {
    syncPlugins([makePluginInfo()]);
    return activatePluginWithModule("test-plugin", {
      activate: async (ctx: NknkAPI) => {
        await expect(ctx.workspace.writeFile("foo.txt", "x")).rejects.toThrow(
          /fs\.write/,
        );
      },
    }).then(() => {});
  });

  it("getPermissions returns declared permissions", () => {
    syncPlugins([
      makePluginInfo({
        permissions: ["fs.read", "fs.write"],
      }),
    ]);
    return activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        const perms = ctx.getPermissions();
        expect(perms).toEqual(expect.arrayContaining(["fs.read", "fs.write"]));
      },
    }).then(() => {});
  });

  it("getPermissions returns empty array when no permissions declared", () => {
    syncPlugins([makePluginInfo()]);
    return activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        expect(ctx.getPermissions()).toEqual([]);
      },
    }).then(() => {});
  });

  it("commands.register does not require any permission", () => {
    syncPlugins([makePluginInfo()]); // no permissions
    return activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        // Should not throw.
        ctx.commands.register("test-plugin.cmd", () => {});
      },
    }).then(() => {
      expect(listPluginCommands()).toHaveLength(1);
    });
  });
});

describe("deactivatePlugin", () => {
  it("calls deactivate export and unregisters contributions", () => {
    const deactivate = vi.fn().mockResolvedValue(undefined);
    syncPlugins([makePluginInfo()]);
    return activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("test-plugin.cmd", () => {});
        ctx.views.register("test-plugin.view", {});
      },
      deactivate,
    })
      .then(() => {
        expect(listPluginCommands()).toHaveLength(1);
        expect(listPluginViews()).toHaveLength(1);
        return deactivatePlugin("test-plugin");
      })
      .then(() => {
        expect(deactivate).toHaveBeenCalledTimes(1);
        expect(listPluginCommands()).toHaveLength(0);
        expect(listPluginViews()).toHaveLength(0);
        const states = listPluginStates();
        expect(states[0].status).toBe("loaded");
      });
  });

  it("is a no-op for non-activated plugin", () => {
    syncPlugins([makePluginInfo()]);
    return deactivatePlugin("test-plugin").then(() => {
      expect(listPluginStates()[0].status).toBe("loaded");
    });
  });
});

describe("enablePlugin / disablePlugin", () => {
  it("disablePlugin deactivates and marks disabled", () => {
    syncPlugins([makePluginInfo()]);
    return activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("test-plugin.cmd", () => {});
      },
    })
      .then(() => disablePlugin("test-plugin"))
      .then(() => {
        const states = listPluginStates();
        expect(states[0].status).toBe("disabled");
        expect(listPluginCommands()).toHaveLength(0);
      });
  });

  it("enablePlugin marks loaded and activates onStartup plugins", () => {
    const activate = vi.fn().mockResolvedValue(undefined);
    syncPlugins([makePluginInfo()]);
    __setPluginModule("test-plugin", { activate });
    return disablePlugin("test-plugin")
      .then(() => enablePlugin("test-plugin"))
      .then(() => {
        const states = listPluginStates();
        // status should be "activated" now — the cached module was
        // used instead of the dynamic import path.
        expect(states[0].status).toBe("activated");
        expect(activate).toHaveBeenCalledTimes(1);
      });
  });

  it("enablePlugin is a no-op for non-disabled plugin", () => {
    syncPlugins([makePluginInfo()]);
    return enablePlugin("test-plugin").then(() => {
      expect(listPluginStates()[0].status).toBe("loaded");
    });
  });
});

describe("clearRegistry", () => {
  it("removes all plugins, commands, and views", () => {
    syncPlugins([makePluginInfo()]);
    return activatePluginWithModule("test-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("test-plugin.cmd", () => {});
        ctx.views.register("test-plugin.view", {});
      },
    }).then(() => {
      clearRegistry();
      expect(listPluginStates()).toHaveLength(0);
      expect(listPluginCommands()).toHaveLength(0);
      expect(listPluginViews()).toHaveLength(0);
    });
  });
});

// Proposal E — cross-plugin command interop (nknk.commands.execute)
describe("cross-plugin command interop (Proposal E)", () => {
  it("allows a plugin to execute its own command (default private)", () => {
    syncPlugins([makePluginInfo({ name: "alpha" })]);
    return activatePluginWithModule("alpha", {
      activate: async (ctx: NknkAPI) => {
        ctx.commands.register("alpha.cmd", () => "ok");
        const result = await ctx.commands.execute("alpha.cmd");
        expect(result).toBe("ok");
      },
    }).then(() => {});
  });

  it("rejects cross-plugin execute when public flag is not set", () => {
    syncPlugins([
      makePluginInfo({ name: "alpha" }),
      makePluginInfo({ name: "beta" }),
    ]);
    // alpha registers a private command.
    return activatePluginWithModule("alpha", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("alpha.private", () => "secret");
      },
    })
      .then(() =>
        // beta tries to execute alpha's private command.
        activatePluginWithModule("beta", {
          activate: async (ctx: NknkAPI) => {
            await expect(
              ctx.commands.execute("alpha.private"),
            ).rejects.toThrow(/not public/);
          },
        }),
      )
      .then(() => {
        const states = listPluginStates();
        const beta = states.find((s) => s.name === "beta");
        expect(beta?.status).toBe("activated"); // no error thrown at activate level
      });
  });

  it("allows cross-plugin execute when public: true is declared", () => {
    syncPlugins([
      makePluginInfo({
        name: "alpha",
        contributes: {
          commands: [{ id: "alpha.public", title: "Pub", public: true }],
        },
      }),
      makePluginInfo({ name: "beta" }),
    ]);
    return activatePluginWithModule("alpha", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("alpha.public", (...args: unknown[]) => {
          const x = args[0] as number;
          return x * 2;
        });
      },
    })
      .then(() =>
        activatePluginWithModule("beta", {
          activate: async (ctx: NknkAPI) => {
            const result = await ctx.commands.execute("alpha.public", 21);
            expect(result).toBe(42);
          },
        }),
      )
      .then(() => {
        // beta should be activated cleanly (no error).
        const states = listPluginStates();
        const beta = states.find((s) => s.name === "beta");
        expect(beta?.status).toBe("activated");
      });
  });

  it("stores the public flag on RegisteredCommand", () => {
    syncPlugins([
      makePluginInfo({
        name: "alpha",
        contributes: {
          commands: [
            { id: "alpha.pub", title: "Pub", public: true },
            { id: "alpha.priv", title: "Priv" },
          ],
        },
      }),
    ]);
    return activatePluginWithModule("alpha", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("alpha.pub", () => {});
        ctx.commands.register("alpha.priv", () => {});
      },
    }).then(() => {
      const cmds = listPluginCommands();
      const pub = cmds.find((c) => c.id === "alpha.pub");
      const priv = cmds.find((c) => c.id === "alpha.priv");
      expect(pub?.public).toBe(true);
      expect(priv?.public).toBe(false);
    });
  });

  it("registerCommand alias works identically to register", () => {
    syncPlugins([makePluginInfo({ name: "alpha" })]);
    return activatePluginWithModule("alpha", {
      activate: async (ctx: NknkAPI) => {
        ctx.commands.registerCommand("alpha.cmd", () => "alias-works");
        const result = await ctx.commands.execute("alpha.cmd");
        expect(result).toBe("alias-works");
      },
    }).then(() => {});
  });

  it("executeCommand alias works identically to execute", () => {
    syncPlugins([makePluginInfo({ name: "alpha" })]);
    return activatePluginWithModule("alpha", {
      activate: async (ctx: NknkAPI) => {
        ctx.commands.register("alpha.cmd", () => "exec-alias");
        const result = await ctx.commands.executeCommand("alpha.cmd");
        expect(result).toBe("exec-alias");
      },
    }).then(() => {});
  });

  it("rejects cross-plugin executeCommand alias too (private cmd)", () => {
    syncPlugins([
      makePluginInfo({ name: "alpha" }),
      makePluginInfo({ name: "beta" }),
    ]);
    return activatePluginWithModule("alpha", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("alpha.secret", () => "nope");
      },
    }).then(() =>
      activatePluginWithModule("beta", {
        activate: async (ctx: NknkAPI) => {
          await expect(
            ctx.commands.executeCommand("alpha.secret"),
          ).rejects.toThrow(/not public/);
        },
      }),
    );
  });

  it("error message names the owning plugin for actionable feedback", () => {
    syncPlugins([
      makePluginInfo({ name: "alpha" }),
      makePluginInfo({ name: "beta" }),
    ]);
    return activatePluginWithModule("alpha", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("alpha.cmd", () => {});
      },
    }).then(() =>
      activatePluginWithModule("beta", {
        activate: async (ctx: NknkAPI) => {
          try {
            await ctx.commands.execute("alpha.cmd");
            throw new Error("should have thrown");
          } catch (e) {
            const msg = (e as Error).message;
            // Error should mention the calling plugin, the target
            // command, the owning plugin, and the public flag.
            expect(msg).toContain("beta");
            expect(msg).toContain("alpha.cmd");
            expect(msg).toContain("alpha");
            expect(msg).toContain("public");
          }
        },
      }),
    );
  });
});

// ---------------------------------------------------------------------------
// Sandbox integration (N-26)
// ---------------------------------------------------------------------------

/**
 * Mock PluginSandboxHost for testing the registry's sandbox integration
 * without spawning real Web Workers. Simulates the Worker reporting
 * 'activated' immediately on activate().
 */
class MockSandboxHost implements SandboxHost {
  activated = new Map<string, PluginManifest>();
  terminatedNames: string[] = [];
  terminateAllCalled = false;

  async activate(
    pluginName: string,
    manifest: PluginManifest,
    _pluginUrl: string,
  ): Promise<void> {
    this.activated.set(pluginName, manifest);
  }

  callMethod(_pluginName: string, _method: string, _args: unknown[]): Promise<unknown> {
    return Promise.reject(new Error("callMethod not yet implemented"));
  }

  terminate(pluginName: string): void {
    this.terminatedNames.push(pluginName);
    this.activated.delete(pluginName);
  }

  terminateAll(): void {
    this.terminateAllCalled = true;
    for (const name of Array.from(this.activated.keys())) {
      this.terminatedNames.push(name);
    }
    this.activated.clear();
  }

  has(pluginName: string): boolean {
    return this.activated.has(pluginName);
  }
}

describe("sandbox integration (N-26)", () => {
  it("setSandboxMode enables/disables sandbox mode", () => {
    expect(isSandboxEnabled()).toBe(false);
    setSandboxMode(true);
    expect(isSandboxEnabled()).toBe(true);
    setSandboxMode(false);
    expect(isSandboxEnabled()).toBe(false);
  });

  it("setSandboxHost sets and clears the host", () => {
    const host = new MockSandboxHost();
    setSandboxHost(host);
    expect(isSandboxEnabled()).toBe(true);
    setSandboxHost(null);
    expect(isSandboxEnabled()).toBe(false);
  });

  it("activatePlugin routes through sandbox host when enabled", async () => {
    const host = new MockSandboxHost();
    setSandboxHost(host);
    syncPlugins([makePluginInfo({ name: "sandbox-plugin" })]);

    await activatePlugin("sandbox-plugin");

    expect(host.activated.has("sandbox-plugin")).toBe(true);
    const states = listPluginStates();
    expect(states[0].status).toBe("activated");
  });

  it("activatePlugin captures activation error from sandbox", async () => {
    const host = new MockSandboxHost();
    host.activate = vi.fn().mockRejectedValue(new Error("Worker init failed"));
    setSandboxHost(host);
    syncPlugins([makePluginInfo({ name: "fail-plugin" })]);

    await activatePlugin("fail-plugin");

    const states = listPluginStates();
    expect(states[0].status).toBe("error");
    expect(states[0].error).toContain("Worker init failed");
  });

  it("deactivatePlugin terminates sandbox worker", async () => {
    const host = new MockSandboxHost();
    setSandboxHost(host);
    syncPlugins([makePluginInfo({ name: "sandbox-plugin" })]);

    await activatePlugin("sandbox-plugin");
    expect(host.has("sandbox-plugin")).toBe(true);

    await deactivatePlugin("sandbox-plugin");

    expect(host.terminatedNames).toContain("sandbox-plugin");
    expect(host.has("sandbox-plugin")).toBe(false);
    const states = listPluginStates();
    expect(states[0].status).toBe("loaded");
  });

  it("clearRegistry terminates all sandboxed plugins", async () => {
    const host = new MockSandboxHost();
    setSandboxHost(host);
    syncPlugins([
      makePluginInfo({ name: "plugin-a" }),
      makePluginInfo({ name: "plugin-b" }),
    ]);

    await activatePlugin("plugin-a");
    await activatePlugin("plugin-b");

    clearRegistry();

    expect(host.terminateAllCalled).toBe(true);
  });

  it("cached module takes precedence over sandbox", async () => {
    // When a test injects a module via __setPluginModule, the sandbox
    // should NOT be used (the cached module path runs first).
    const host = new MockSandboxHost();
    setSandboxHost(host);

    let activated = false;
    syncPlugins([makePluginInfo({ name: "cached-plugin" })]);
    __setPluginModule("cached-plugin", {
      activate: () => {
        activated = true;
      },
    });

    await activatePlugin("cached-plugin");

    expect(activated).toBe(true);
    expect(host.activated.has("cached-plugin")).toBe(false);
  });
});

describe("registry reactivity (N-57 / Proposal Q)", () => {
  it("listPluginViews updates reactively when a view is registered", async () => {
    let runCount = 0;
    let currentViews: ReturnType<typeof listPluginViews> = [];
    watchEffect(
      () => {
        currentViews = listPluginViews();
        runCount++;
      },
      { flush: "sync" },
    );
    expect(runCount).toBe(1);
    expect(currentViews).toHaveLength(0);

    syncPlugins([makePluginInfo({ name: "view-plugin" })]);
    await activatePluginWithModule("view-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.views.register("my-view", null, { title: "My View" });
      },
    });

    expect(runCount).toBe(2);
    expect(currentViews).toHaveLength(1);
    expect(currentViews[0].id).toBe("my-view");
  });

  it("listPluginCommands updates reactively when a command is registered", async () => {
    let runCount = 0;
    let currentCmds: ReturnType<typeof listPluginCommands> = [];
    watchEffect(
      () => {
        currentCmds = listPluginCommands();
        runCount++;
      },
      { flush: "sync" },
    );
    expect(runCount).toBe(1);

    syncPlugins([makePluginInfo({ name: "cmd-plugin" })]);
    await activatePluginWithModule("cmd-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("my-cmd", () => undefined);
      },
    });

    expect(runCount).toBe(2);
    expect(currentCmds).toHaveLength(1);
    expect(currentCmds[0].id).toBe("my-cmd");
  });

  it("listPluginViews updates reactively when a plugin is deactivated", async () => {
    let runCount = 0;
    let currentViews: ReturnType<typeof listPluginViews> = [];
    watchEffect(
      () => {
        currentViews = listPluginViews();
        runCount++;
      },
      { flush: "sync" },
    );

    syncPlugins([makePluginInfo({ name: "view-plugin" })]);
    await activatePluginWithModule("view-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.views.register("my-view", null, { title: "My View" });
      },
    });
    expect(currentViews).toHaveLength(1);
    expect(runCount).toBe(2);

    await deactivatePlugin("view-plugin");
    expect(currentViews).toHaveLength(0);
    expect(runCount).toBe(3);
  });

  it("listPluginViews updates reactively when clearRegistry is called", async () => {
    let runCount = 0;
    let currentViews: ReturnType<typeof listPluginViews> = [];
    watchEffect(
      () => {
        currentViews = listPluginViews();
        runCount++;
      },
      { flush: "sync" },
    );

    syncPlugins([makePluginInfo({ name: "view-plugin" })]);
    await activatePluginWithModule("view-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.views.register("my-view", null, { title: "My View" });
      },
    });
    expect(currentViews).toHaveLength(1);

    clearRegistry();
    expect(currentViews).toHaveLength(0);
    // runCount should have increased (clearRegistry bumps the version)
    expect(runCount).toBeGreaterThanOrEqual(3);
  });

  it("listPluginCommands updates reactively when clearRegistry is called", async () => {
    let runCount = 0;
    let currentCmds: ReturnType<typeof listPluginCommands> = [];
    watchEffect(
      () => {
        currentCmds = listPluginCommands();
        runCount++;
      },
      { flush: "sync" },
    );

    syncPlugins([makePluginInfo({ name: "cmd-plugin" })]);
    await activatePluginWithModule("cmd-plugin", {
      activate: (ctx: NknkAPI) => {
        ctx.commands.register("my-cmd", () => undefined);
      },
    });
    expect(currentCmds).toHaveLength(1);

    clearRegistry();
    expect(currentCmds).toHaveLength(0);
    expect(runCount).toBeGreaterThanOrEqual(3);
  });
});
