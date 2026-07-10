import { reactive } from "vue";
import { searchService } from "@/api/services";
import { errorMessage } from "@/lib/errors";
import type { SearchResult } from "@/types";

export interface SearchState {
  query: string;
  ignoreCase: boolean;
  results: SearchResult[];
  loading: boolean;
  error: string | null;
  /** prompt-7 Task L: true when backend returned more hits than UI cap. */
  truncated: boolean;
}

/** Cap UI-held search results to keep the sidebar responsive (prompt-7 Task L). */
export const MAX_SEARCH_UI_RESULTS = 500;

export const searchState = reactive<SearchState>({
  query: "",
  ignoreCase: false,
  results: [],
  loading: false,
  error: null,
  truncated: false,
});

let debounceTimer: ReturnType<typeof setTimeout> | null = null;

export async function runSearch(root: string, query: string): Promise<void> {
  if (!query.trim()) {
    searchState.results = [];
    searchState.query = query;
    searchState.truncated = false;
    return;
  }
  searchState.query = query;
  searchState.loading = true;
  searchState.error = null;
  try {
    const results = await searchService.search(root, query, searchState.ignoreCase);
    if (results.length > MAX_SEARCH_UI_RESULTS) {
      searchState.results = results.slice(0, MAX_SEARCH_UI_RESULTS);
      searchState.truncated = true;
    } else {
      searchState.results = results;
      searchState.truncated = false;
    }
  } catch (e: unknown) {
    searchState.error = errorMessage(e);
    searchState.results = [];
    searchState.truncated = false;
  } finally {
    searchState.loading = false;
  }
}

export function debouncedSearch(root: string, query: string, delay = 300): void {
  if (debounceTimer) clearTimeout(debounceTimer);
  debounceTimer = setTimeout(() => {
    runSearch(root, query);
  }, delay);
}

export function clearSearch(): void {
  searchState.query = "";
  searchState.results = [];
  searchState.error = null;
  searchState.loading = false;
  searchState.truncated = false;
}

export async function replaceInFile(repoPath: string, filePath: string, pattern: string, replacement: string, caseSensitive: boolean) {
  const fullPath = repoPath + "/" + filePath;
  return searchService.replace(fullPath, pattern, replacement, caseSensitive);
}

export async function replaceAll(repoPath: string, pattern: string, replacement: string, caseSensitive: boolean) {
  let total = 0;
  for (const result of searchState.results) {
    const r = await replaceInFile(repoPath, result.path, pattern, replacement, caseSensitive);
    total += r.replacements;
  }
  return total;
}
