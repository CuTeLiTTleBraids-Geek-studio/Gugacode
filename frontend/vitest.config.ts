import { defineConfig } from "vitest/config";
import vue from "@vitejs/plugin-vue";
import { resolve } from "path";

export default defineConfig({
  plugins: [vue()],
  test: {
    environment: "jsdom",
    globals: true,
    // N-130: coverage configuration. Run with `npm run test:coverage`.
    // Reports go to frontend/coverage/. v8 provider requires no extra deps.
    coverage: {
      provider: "v8",
      reporter: ["text", "html", "lcov"],
      reportsDirectory: "coverage",
      // Exclude non-source files from coverage to keep reports focused.
      exclude: [
        "node_modules/**",
        "dist/**",
        "bindings/**",
        ".bindings-tmp-*/**",
        "src/**/*.test.ts",
        "src/**/*.spec.ts",
        "src/**/*.d.ts",
        "src/main.ts",
        "src/vite-env.d.ts",
        "vite.config.ts",
        "vitest.config.ts",
        "eslint.config.js",
      ],
      // Thresholds enforced in CI to prevent coverage regression. Current
      // baseline reflects the existing codebase; raise as tests improve.
      thresholds: {
        statements: 50,
        branches: 50,
        functions: 50,
        lines: 50,
      },
    },
  },
  resolve: {
    alias: {
      "@": resolve(__dirname, "src"),
      // prompt-5 Task D / BUG-M3: monaco-editor cannot resolve fully under
      // vitest/jsdom; stub it so suites that transitively import
      // monaco-themes (e.g. ExtensionPermissionDialog via app store) load.
      "monaco-editor": resolve(__dirname, "src/test-stubs/monaco-editor.ts"),
    },
  },
});
