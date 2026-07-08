import { describe, it, expect, beforeEach, vi } from "vitest";

// N-47: Capture Events.On callbacks so tests can simulate event emission.
const eventCallbacks = new Map<string, (event: any) => void>();

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: vi.fn((name: string, cb: (event: any) => void) => {
      eventCallbacks.set(name, cb);
    }),
  },
}));

vi.mock("@/api/services", () => ({
  terminalService: {
    start: vi.fn().mockResolvedValue(undefined),
    write: vi.fn().mockResolvedValue(undefined),
    kill: vi.fn().mockResolvedValue(undefined),
    resize: vi.fn().mockResolvedValue(undefined),
    isRunning: vi.fn().mockReturnValue(false),
    startSession: vi.fn().mockResolvedValue(undefined),
    killSession: vi.fn().mockResolvedValue(undefined),
    writeSession: vi.fn().mockResolvedValue(undefined),
    resizeSession: vi.fn().mockResolvedValue(undefined),
    isSessionRunning: vi.fn().mockReturnValue(false),
    listSessions: vi.fn().mockReturnValue([]),
  },
}));

import {
  terminalState,
  createSession,
  writeToSession,
  killSession,
  resizeSession,
  setActiveSession,
  clearSessionOutput,
  onSessionExit,
  runCommandInSession,
} from "./terminal";

describe("terminal store (multi-session)", () => {
  beforeEach(() => {
    // Reset state
    Object.keys(terminalState.sessions).forEach((id) =>
      delete terminalState.sessions[id],
    );
    terminalState.sessionOrder = [];
    terminalState.activeSessionId = null;
  });

  it("creates a session", async () => {
    const id = await createSession("/some/path");
    expect(id).toBeTruthy();
    expect(terminalState.sessions[id]).toBeDefined();
    expect(terminalState.sessions[id].running).toBe(true);
    expect(terminalState.activeSessionId).toBe(id);
  });

  it("writes to a session", async () => {
    const id = await createSession("/path");
    await writeToSession(id, "ls\n");
    // No throw = pass
  });

  it("kills a session", async () => {
    const id = await createSession("/path");
    await killSession(id);
    expect(terminalState.sessions[id]).toBeUndefined();
    expect(terminalState.activeSessionId).toBeNull();
  });

  it("switches active session", async () => {
    const id1 = await createSession("/path1");
    const id2 = await createSession("/path2");
    expect(terminalState.activeSessionId).toBe(id2);
    setActiveSession(id1);
    expect(terminalState.activeSessionId).toBe(id1);
  });

  it("resizes a session", async () => {
    const id = await createSession("/path");
    await resizeSession(id, 120, 40);
    expect(terminalState.sessions[id].cols).toBe(120);
    expect(terminalState.sessions[id].rows).toBe(40);
  });

  it("clears session output", async () => {
    const id = await createSession("/path");
    terminalState.sessions[id].output = "hello";
    clearSessionOutput(id);
    expect(terminalState.sessions[id].output).toBe("");
  });

  it("maintains session order", async () => {
    const id1 = await createSession("/path1");
    const id2 = await createSession("/path2");
    const id3 = await createSession("/path3");
    expect(terminalState.sessionOrder).toEqual([id1, id2, id3]);
  });

  it("switches active session after kill", async () => {
    const id1 = await createSession("/path1");
    const id2 = await createSession("/path2");
    expect(terminalState.activeSessionId).toBe(id2);
    await killSession(id2);
    expect(terminalState.activeSessionId).toBe(id1);
  });

  // --- N-47: PTY exit detection ---

  it("N-47: onSessionExit returns an unsubscribe function", async () => {
    const id = await createSession("/path");
    const off = onSessionExit(id, () => {});
    expect(typeof off).toBe("function");
    off();
  });

  it("N-47: terminal:exited event sets session.running to false", async () => {
    const id = await createSession("/path");
    expect(terminalState.sessions[id].running).toBe(true);
    // Simulate the backend emitting terminal:exited for this session.
    const cb = eventCallbacks.get("terminal:exited");
    expect(cb).toBeDefined();
    cb!({ data: { sessionId: id, err: "EOF" } });
    expect(terminalState.sessions[id].running).toBe(false);
  });

  it("N-47: terminal:exited event notifies onSessionExit listeners", async () => {
    const id = await createSession("/path");
    let exited = false;
    onSessionExit(id, () => {
      exited = true;
    });
    const cb = eventCallbacks.get("terminal:exited");
    cb!({ data: { sessionId: id } });
    expect(exited).toBe(true);
  });

  it("N-47: runCommandInSession returns -1 when PTY exits mid-command", async () => {
    const id = await createSession("/path");
    // Start the command (returns a promise that resolves on sentinel or exit)
    const promise = runCommandInSession(id, "long-running-cmd", 10000);
    // Simulate PTY exit before the sentinel arrives.
    const exitCb = eventCallbacks.get("terminal:exited");
    exitCb!({ data: { sessionId: id } });
    const exitCode = await promise;
    expect(exitCode).toBe(-1);
  });

  it("N-47: terminal:exited with unknown sessionId does not throw", async () => {
    const cb = eventCallbacks.get("terminal:exited");
    expect(() => cb!({ data: { sessionId: "nonexistent" } })).not.toThrow();
  });

  it("N-47: terminal:exited with missing sessionId does not throw", async () => {
    const cb = eventCallbacks.get("terminal:exited");
    expect(() => cb!({ data: {} })).not.toThrow();
  });
});
