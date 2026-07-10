import { describe, it, expect, vi, beforeEach } from "vitest";
import { mount } from "@vue/test-utils";
import { nextTick } from "vue";
import PluginViewIframe from "./PluginViewIframe.vue";
import type { PluginManifest } from "@/types";
import type { RpcHandler } from "@/lib/pluginSandbox";

// Mock the global window.postMessage / addEventListener so tests can
// simulate iframe messages.
const messageListeners = new Set<(e: MessageEvent) => void>();
const originalAddEventListener = window.addEventListener;
const originalRemoveEventListener = window.removeEventListener;

beforeEach(() => {
  messageListeners.clear();
  vi.spyOn(window, "addEventListener").mockImplementation((type, listener) => {
    if (type === "message") {
      messageListeners.add(listener as (e: MessageEvent) => void);
    } else {
      originalAddEventListener.call(window, type, listener);
    }
  });
  vi.spyOn(window, "removeEventListener").mockImplementation((type, listener) => {
    if (type === "message") {
      messageListeners.delete(listener as (e: MessageEvent) => void);
    } else {
      originalRemoveEventListener.call(window, type, listener);
    }
  });
});

function makeManifest(overrides?: Partial<PluginManifest>): PluginManifest {
  return {
    name: "test-plugin",
    version: "1.0.0",
    main: "main.js",
    permissions: [],
    ...overrides,
  };
}

/** Simulate the iframe sending a message to the host. */
function sendMessageFromIframe(
  data: unknown,
  source: MessageEventSource | null = null,
  origin = window.location.origin,
) {
  const event = new MessageEvent("message", { data, origin, source });
  for (const listener of messageListeners) {
    listener(event);
  }
}

describe("PluginViewIframe (N-36 / Proposal G)", () => {
  it("renders an iframe with the correct src", () => {
    const manifest = makeManifest();
    const rpcHandler: RpcHandler = vi.fn();
    const wrapper = mount(PluginViewIframe, {
      props: {
        pluginName: "test-plugin",
        viewId: "my-view",
        title: "My View",
        manifest,
        rpcHandler,
      },
    });
    const iframe = wrapper.find("iframe");
    expect(iframe.exists()).toBe(true);
    expect(iframe.attributes("src")).toContain("/_plugins/test-plugin/view.html");
    expect(iframe.attributes("src")).toContain("viewId=my-view");
    expect(iframe.attributes("sandbox")).toBe("allow-scripts");
    expect(iframe.attributes("title")).toBe("My View");
  });

  it("sends nknk:init when iframe reports ready", async () => {
    const manifest = makeManifest();
    const rpcHandler: RpcHandler = vi.fn();
    const wrapper = mount(PluginViewIframe, {
      props: {
        pluginName: "test-plugin",
        viewId: "my-view",
        title: "My View",
        manifest,
        rpcHandler,
      },
    });

    // Mock the iframe's contentWindow.postMessage
    const postMessageSpy = vi.fn();
    const iframe = wrapper.find("iframe").element as HTMLIFrameElement;
    const mockContentWindow = { postMessage: postMessageSpy };
    Object.defineProperty(iframe, "contentWindow", {
      value: mockContentWindow,
      configurable: true,
    });

    // Simulate iframe ready
    sendMessageFromIframe(
      { type: "nknk:ready" },
      mockContentWindow as unknown as MessageEventSource,
    );

    await nextTick();

    expect(postMessageSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "nknk:init",
        viewId: "my-view",
      }),
      window.location.origin,
    );
    const initMsg = postMessageSpy.mock.calls[0][0];
    expect(initMsg.manifest.name).toBe("test-plugin");
  });

  it("routes rpc-request to the rpcHandler and sends response", async () => {
    const manifest = makeManifest({ permissions: ["fs.read"] });
    const rpcHandler: RpcHandler = vi.fn().mockResolvedValue("file contents");
    const wrapper = mount(PluginViewIframe, {
      props: {
        pluginName: "test-plugin",
        viewId: "my-view",
        title: "My View",
        manifest,
        rpcHandler,
      },
    });

    const postMessageSpy = vi.fn();
    const iframe = wrapper.find("iframe").element as HTMLIFrameElement;
    const mockContentWindow = { postMessage: postMessageSpy };
    Object.defineProperty(iframe, "contentWindow", {
      value: mockContentWindow,
      configurable: true,
    });

    sendMessageFromIframe(
      {
        type: "nknk:rpc-request",
        id: 42,
        method: "workspace.readFile",
        args: ["src/main.ts"],
      },
      mockContentWindow as unknown as MessageEventSource,
    );

    // Wait for async handler
    await new Promise((r) => setTimeout(r, 10));

    expect(rpcHandler).toHaveBeenCalledWith(
      "test-plugin",
      manifest,
      "workspace.readFile",
      ["src/main.ts"],
    );
    // Response sent back
    expect(postMessageSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "nknk:rpc-response",
        id: 42,
        result: "file contents",
      }),
      window.location.origin,
    );
  });

  it("rejects rpc-request when permission not declared", async () => {
    const manifest = makeManifest({ permissions: [] }); // no fs.read
    const rpcHandler: RpcHandler = vi.fn();
    const wrapper = mount(PluginViewIframe, {
      props: {
        pluginName: "test-plugin",
        viewId: "my-view",
        title: "My View",
        manifest,
        rpcHandler,
      },
    });

    const postMessageSpy = vi.fn();
    const iframe = wrapper.find("iframe").element as HTMLIFrameElement;
    const mockContentWindow = { postMessage: postMessageSpy };
    Object.defineProperty(iframe, "contentWindow", {
      value: mockContentWindow,
      configurable: true,
    });

    sendMessageFromIframe(
      {
        type: "nknk:rpc-request",
        id: 1,
        method: "workspace.readFile",
        args: ["file.ts"],
      },
      mockContentWindow as unknown as MessageEventSource,
    );

    await new Promise((r) => setTimeout(r, 10));

    // Handler should NOT have been called (permission denied)
    expect(rpcHandler).not.toHaveBeenCalled();
    // Error response sent
    expect(postMessageSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "nknk:rpc-response",
        id: 1,
        error: expect.stringContaining("fs.read"),
      }),
      window.location.origin,
    );
  });

  it("sends error response when rpcHandler throws", async () => {
    const manifest = makeManifest({ permissions: ["fs.read"] });
    const rpcHandler: RpcHandler = vi.fn().mockRejectedValue(new Error("File not found"));
    const wrapper = mount(PluginViewIframe, {
      props: {
        pluginName: "test-plugin",
        viewId: "my-view",
        title: "My View",
        manifest,
        rpcHandler,
      },
    });

    const postMessageSpy = vi.fn();
    const iframe = wrapper.find("iframe").element as HTMLIFrameElement;
    const mockContentWindow = { postMessage: postMessageSpy };
    Object.defineProperty(iframe, "contentWindow", {
      value: mockContentWindow,
      configurable: true,
    });

    sendMessageFromIframe(
      {
        type: "nknk:rpc-request",
        id: 7,
        method: "workspace.readFile",
        args: ["missing.ts"],
      },
      mockContentWindow as unknown as MessageEventSource,
    );

    await new Promise((r) => setTimeout(r, 10));

    expect(postMessageSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "nknk:rpc-response",
        id: 7,
        error: "File not found",
      }),
      window.location.origin,
    );
  });

  it("ignores messages from disallowed sources", async () => {
    const manifest = makeManifest();
    const rpcHandler: RpcHandler = vi.fn();
    const wrapper = mount(PluginViewIframe, {
      props: {
        pluginName: "test-plugin",
        viewId: "my-view",
        title: "My View",
        manifest,
        rpcHandler,
      },
    });

    const postMessageSpy = vi.fn();
    const iframe = wrapper.find("iframe").element as HTMLIFrameElement;
    const mockContentWindow = { postMessage: postMessageSpy };
    Object.defineProperty(iframe, "contentWindow", {
      value: mockContentWindow,
      configurable: true,
    });

    // Send from a foreign source (not the iframe's contentWindow).
    // A spoofed "null" origin must NOT bypass the source check.
    const foreignSource = { postMessage: vi.fn() } as unknown as MessageEventSource;
    sendMessageFromIframe(
      { type: "nknk:rpc-request", id: 1, method: "workspace.readFile", args: [] },
      foreignSource,
      "null",
    );

    // Also send with no source at all (e.g. a spoofed message).
    sendMessageFromIframe(
      { type: "nknk:rpc-request", id: 2, method: "workspace.readFile", args: [] },
      null,
      "null",
    );

    await new Promise((r) => setTimeout(r, 10));

    expect(rpcHandler).not.toHaveBeenCalled();
    expect(postMessageSpy).not.toHaveBeenCalled();
  });

  it("logs nknk:log messages to console", async () => {
    const manifest = makeManifest();
    const rpcHandler: RpcHandler = vi.fn();
    const wrapper = mount(PluginViewIframe, {
      props: {
        pluginName: "test-plugin",
        viewId: "my-view",
        title: "My View",
        manifest,
        rpcHandler,
      },
    });

    const iframe = wrapper.find("iframe").element as HTMLIFrameElement;
    const mockContentWindow = { postMessage: vi.fn() };
    Object.defineProperty(iframe, "contentWindow", {
      value: mockContentWindow,
      configurable: true,
    });

    const infoSpy = vi.spyOn(console, "info").mockImplementation(() => {});
    sendMessageFromIframe(
      { type: "nknk:log", level: "info", message: "Hello from iframe" },
      mockContentWindow as unknown as MessageEventSource,
    );

    await nextTick();

    expect(infoSpy).toHaveBeenCalledWith(
      expect.stringContaining("plugin view: test-plugin/my-view"),
      "Hello from iframe",
    );
    infoSpy.mockRestore();
  });
});
