import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@/api/services", () => ({
  searchService: {
    search: vi.fn().mockResolvedValue([
      {
        path: "a.txt",
        matches: [
          { line: 1, column: 1, preview: "hello world" },
          { line: 3, column: 1, preview: "hello again" },
        ],
      },
      {
        path: "b.ts",
        matches: [{ line: 5, column: 3, preview: "  hello there" }],
      },
    ]),
  },
}));

import { searchState, runSearch, clearSearch } from "./search";

describe("search store", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    searchState.query = "";
    searchState.results = [];
    searchState.loading = false;
    searchState.error = null;
    searchState.ignoreCase = false;
  });

  it("starts with empty state", () => {
    expect(searchState.query).toBe("");
    expect(searchState.results).toHaveLength(0);
    expect(searchState.loading).toBe(false);
  });

  it("runSearch populates results", async () => {
    await runSearch("/repo", "hello");
    expect(searchState.results).toHaveLength(2);
    expect(searchState.results[0].path).toBe("a.txt");
    expect(searchState.results[0].matches).toHaveLength(2);
    expect(searchState.loading).toBe(false);
  });

  it("runSearch does nothing with empty query", async () => {
    await runSearch("/repo", "");
    expect(searchState.results).toHaveLength(0);
    const { searchService } = await import("@/api/services");
    expect(searchService.search).not.toHaveBeenCalled();
  });

  it("toggle ignoreCase is reflected in state", async () => {
    searchState.ignoreCase = true;
    await runSearch("/repo", "Hello");
    const { searchService } = await import("@/api/services");
    expect(searchService.search).toHaveBeenCalledWith("/repo", "Hello", true);
  });

  it("clearSearch resets state", () => {
    searchState.query = "foo";
    searchState.results = [{ path: "x", matches: [] }];
    clearSearch();
    expect(searchState.query).toBe("");
    expect(searchState.results).toHaveLength(0);
  });

  it("stores error on failure", async () => {
    const { searchService } = await import("@/api/services");
    (searchService.search as any).mockRejectedValueOnce(new Error("bad regex"));
    await runSearch("/repo", "[invalid");
    expect(searchState.error).toBe("bad regex");
    expect(searchState.loading).toBe(false);
  });
});
