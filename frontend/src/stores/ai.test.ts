import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@/lib/monaco-themes", () => ({
  accentThemes: [],
  applyMonacoTheme: vi.fn(),
  registerAllThemes: vi.fn(),
}));

// Collect event handlers so tests can simulate backend events.
// vi.hoisted ensures this runs before mock factories are evaluated.
const { eventHandlers } = vi.hoisted(() => ({
  eventHandlers: {} as Record<string, ((...args: any[]) => void) | undefined>,
}));

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: vi.fn((event: string, handler: (...args: any[]) => void) => {
      eventHandlers[event] = handler;
      return () => undefined;
    }),
    Emit: vi.fn(),
  },
}));

vi.mock("@/api/services", () => ({
  aiService: {
    setConfig: vi.fn().mockResolvedValue(undefined),
    startStream: vi.fn().mockResolvedValue("stream-test-1"),
    stopStream: vi.fn().mockResolvedValue(undefined),
    send: vi.fn().mockResolvedValue({ Content: "ok", FinishReason: "stop" }),
    getPresetPrompt: vi.fn().mockResolvedValue("Explain this code."),
    getDefaultSystemPrompt: vi.fn().mockResolvedValue("default prompt"),
    listPresets: vi.fn().mockResolvedValue([]),
    generateTitleWithAI: vi.fn().mockResolvedValue("AI generated title"),
  },
  conversationService: {
    save: vi.fn().mockResolvedValue(undefined),
    load: vi.fn().mockResolvedValue({ id: "1", title: "test", created_at: 0, messages: [] }),
    generateId: vi.fn().mockResolvedValue("new-id"),
    generateTitle: vi.fn().mockResolvedValue("test title"),
  },
}));

vi.mock("@/lib/notifications", () => ({
  notify: vi.fn(),
  notifySuccess: vi.fn(),
  notifyError: vi.fn(),
  notifyWarning: vi.fn(),
  notifyInfo: vi.fn(),
}));

import {
  aiState,
  sendMessage,
  stopGeneration,
  attachContext,
  clearContext,
  runAIAction,
  clearMessages,
  setSystemPromptOverride,
  loadConversation,
  parseAIStreamPayload,
  isOwnedStreamEvent,
} from "./ai";

describe("ai store", () => {
  beforeEach(() => {
    aiState.messages = [];
    aiState.streaming = false;
    aiState.globalStreamBusy = false;
    aiState.activeStreamId = null;
    aiState.error = null;
    aiState.context = null;
    aiState.currentConversationId = null;
    aiState.currentConversationTitle = null;
    aiState.mentionedFiles = [];
    aiState.currentSystemPromptOverride = null;
  });

  it("sends a message and appends assistant response via events", async () => {
    const promise = sendMessage("hi");
    await promise;
    // prompt-6 Task 2: structured payloads with streamId
    const sid = aiState.activeStreamId || "stream-test-1";
    eventHandlers["ai:chunk"]?.({ data: { streamId: sid, data: "hello" } });
    eventHandlers["ai:chunk"]?.({ data: { streamId: sid, data: " world" } });
    eventHandlers["ai:done"]?.({ data: { streamId: sid, data: "" } });
    eventHandlers["ai:stream-busy"]?.({ data: { streamId: sid, busy: false } });

    expect(aiState.messages.length).toBe(2);
    expect(aiState.messages[0].role).toBe("user");
    expect(aiState.messages[1].role).toBe("assistant");
    expect(aiState.messages[1].content).toBe("hello world");
  });

  it("ignores chunks for a foreign streamId (prompt-6 Task 2)", async () => {
    const promise = sendMessage("hi");
    await promise;
    const sid = aiState.activeStreamId || "stream-test-1";
    eventHandlers["ai:chunk"]?.({ data: { streamId: "other-stream", data: "LEAK" } });
    eventHandlers["ai:chunk"]?.({ data: { streamId: sid, data: "ok" } });
    eventHandlers["ai:done"]?.({ data: { streamId: sid, data: "" } });
    expect(aiState.messages[1].content).toBe("ok");
    expect(aiState.messages[1].content).not.toContain("LEAK");
  });

  it("includes context prefix when attached", async () => {
    attachContext({
      kind: "selection",
      filePath: "/test.ts",
      language: "typescript",
      content: "const x = 1;",
      startLine: 1,
      endLine: 1,
    });
    const promise = sendMessage("explain");
    await promise;
    const sid = aiState.activeStreamId || "stream-test-1";
    eventHandlers["ai:done"]?.({ data: { streamId: sid, data: "" } });

    expect(aiState.messages[0].content).toContain("const x = 1");
    expect(aiState.messages[0].content).toContain("/test.ts");
    expect(aiState.messages[0].content).toContain("explain");
  });

  it("stops generation without clearing globalStreamBusy locally (Task 5)", async () => {
    aiState.streaming = true;
    aiState.globalStreamBusy = true;
    await stopGeneration();
    expect(aiState.streaming).toBe(false);
    // busy only cleared by backend event
    expect(aiState.globalStreamBusy).toBe(true);
    eventHandlers["ai:stream-busy"]?.({ data: { streamId: "x", busy: false } });
    expect(aiState.globalStreamBusy).toBe(false);
  });

  it("handles error event", async () => {
    const promise = sendMessage("hi");
    await promise;
    const sid = aiState.activeStreamId || "stream-test-1";
    eventHandlers["ai:error"]?.({ data: { streamId: sid, data: "network error" } });

    expect(aiState.error).toBe("network error");
    expect(aiState.streaming).toBe(false);
  });

  it("parseAIStreamPayload accepts legacy string chunks", () => {
    const p = parseAIStreamPayload({ data: "token" });
    expect(p.data).toBe("token");
    expect(p.streamId).toBe("");
  });

  it("isOwnedStreamEvent rejects mismatch when activeStreamId set", () => {
    aiState.activeStreamId = "mine";
    expect(isOwnedStreamEvent("mine")).toBe(true);
    expect(isOwnedStreamEvent("theirs")).toBe(false);
    expect(isOwnedStreamEvent("")).toBe(true); // legacy while active
  });

  it("clears context", () => {
    aiState.context = { kind: "file", filePath: "/x", language: "go", content: "x" };
    clearContext();
    expect(aiState.context).toBe(null);
  });

  it("clears messages", () => {
    aiState.messages = [{ role: "user", content: "x" }];
    clearMessages();
    expect(aiState.messages).toHaveLength(0);
  });

  // N-60: clearMessages also resets the system prompt override.
  it("clearMessages resets systemPromptOverride (N-60)", () => {
    aiState.currentSystemPromptOverride = "custom prompt";
    clearMessages();
    expect(aiState.currentSystemPromptOverride).toBeNull();
  });

  // N-60: setSystemPromptOverride sets a non-empty prompt.
  it("setSystemPromptOverride sets non-empty prompt (N-60)", () => {
    setSystemPromptOverride("You are a code reviewer.");
    expect(aiState.currentSystemPromptOverride).toBe("You are a code reviewer.");
  });

  // N-60: setSystemPromptOverride with null resets to null.
  it("setSystemPromptOverride with null resets to null (N-60)", () => {
    aiState.currentSystemPromptOverride = "custom";
    setSystemPromptOverride(null);
    expect(aiState.currentSystemPromptOverride).toBeNull();
  });

  // N-60: setSystemPromptOverride with empty/whitespace resets to null.
  it("setSystemPromptOverride with empty string resets to null (N-60)", () => {
    aiState.currentSystemPromptOverride = "custom";
    setSystemPromptOverride("   ");
    expect(aiState.currentSystemPromptOverride).toBeNull();
  });

  // N-60: loadConversation restores the override from the loaded conversation.
  it("loadConversation restores systemPromptOverride (N-60)", async () => {
    const { conversationService } = await import("@/api/services");
    (conversationService.load as any).mockResolvedValue({
      id: "conv-1",
      title: "test",
      created_at: 0,
      messages: [{ role: "user", content: "hi" }],
      system_prompt_override: "You are a senior engineer.",
    });
    await loadConversation("conv-1");
    expect(aiState.currentSystemPromptOverride).toBe("You are a senior engineer.");
  });

  // N-60: loadConversation with no override field sets null.
  it("loadConversation with no override sets null (N-60)", async () => {
    const { conversationService } = await import("@/api/services");
    (conversationService.load as any).mockResolvedValue({
      id: "conv-2",
      title: "test",
      created_at: 0,
      messages: [],
    });
    await loadConversation("conv-2");
    expect(aiState.currentSystemPromptOverride).toBeNull();
  });

  it("does not send while streaming", async () => {
    aiState.streaming = true;
    const before = aiState.messages.length;
    await sendMessage("hi");
    expect(aiState.messages.length).toBe(before);
  });

  it("runAIAction attaches context and sends", async () => {
    const promise = runAIAction("explain", "func foo() {}", "go", "/main.go");
    eventHandlers["ai:done"]?.();
    await promise;

    expect(aiState.messages.length).toBe(2);
    expect(aiState.messages[0].content).toContain("func foo() {}");
    expect(aiState.messages[0].content).toContain("Explain this code.");
  });
});
