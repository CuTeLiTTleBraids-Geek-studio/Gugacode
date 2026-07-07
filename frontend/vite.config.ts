import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import wails from "@wailsio/runtime/plugins/vite";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "path";

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  resolve: {
    alias: {
      "@": resolve(__dirname, "src"),
    },
  },
  plugins: [vue(), tailwindcss(), wails("./bindings")],
  build: {
    rollupOptions: {
      output: {
        // N-145: Split large vendor dependencies into separate chunks
        // to improve caching and reduce the main bundle size.
        // Vite 8 (Rolldown) requires manualChunks as a function.
        manualChunks(id: string): string | undefined {
          if (id.includes("node_modules")) {
            if (id.includes("monaco-editor") || id.includes("@guolao/vue-monaco-editor")) {
              return "vendor-monaco";
            }
            if (id.includes("element-plus") || id.includes("@element-plus/icons-vue")) {
              return "vendor-element";
            }
            if (id.includes("@xterm")) {
              return "vendor-terminal";
            }
            if (id.includes("marked") || id.includes("dompurify") || id.includes("highlight.js")) {
              return "vendor-markdown";
            }
            if (id.includes("vue-router") || /[\\/]node_modules[\\/]vue[\\/]/.test(id)) {
              return "vendor-vue";
            }
          }
          return undefined;
        },
      },
    },
  },
});
