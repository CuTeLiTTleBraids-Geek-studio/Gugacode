import { createRouter, createWebHashHistory, type RouteRecordRaw } from "vue-router";
import { translate } from "@/lib/i18n";
import { appState } from "@/stores/app";

const routes: RouteRecordRaw[] = [
  {
    path: "/",
    redirect: "/welcome",
  },
  {
    path: "/welcome",
    name: "Welcome",
    component: () => import("@/views/WelcomeView.vue"),
    meta: { title: "Welcome", hideLayout: true },
  },
  {
    path: "/editor",
    name: "Editor",
    component: () => import("@/views/EditorView.vue"),
    meta: { title: "Editor" },
  },
  {
    path: "/settings",
    name: "Settings",
    component: () => import("@/views/SettingsView.vue"),
    meta: { title: "Settings" },
  },
  {
    path: "/projects",
    name: "Projects",
    component: () => import("@/views/ProjectsView.vue"),
    meta: { title: "Projects" },
  },
  {
    path: "/plugins",
    name: "Plugins",
    component: () => import("@/views/PluginsView.vue"),
    meta: { title: "Plugins" },
  },
  {
    // Plan 11 Task 1 — AI 助手独立全屏页面。复用主布局（hideLayout: false）
    // 以保留顶栏与 ActivityBar；三栏布局由 AiAssistantView 自行管理。
    path: "/ai",
    name: "AiAssistant",
    component: () => import("@/views/AiAssistantView.vue"),
    meta: { title: "AI Assistant" },
  },
  {
    // prompt-4 Task 2 — OS 级独立 AI 伴侣窗口根视图。
    // hideLayout: true，不复用主布局；自带活动栏 + 顶栏 + 消息流 + 输入区。
    path: "/ai-window",
    name: "AiWindow",
    component: () => import("@/views/AiWindowView.vue"),
    meta: { title: "gugacode AI", hideLayout: true },
  },
];

const router = createRouter({
  history: createWebHashHistory(),
  routes,
});

router.beforeEach((to, _from, next) => {
  // Plan 11 Task 1 Step 8 — 未配置 AI Provider 时引导去设置页。
  // 判断标准：既无主 key（aiApiKeyConfigured），又无 multi-provider 配置。
  if (to.path === "/ai" || to.path === "/ai-window") {
    // /ai-window is an OS-level companion window: still allow mounting so the
    // user can configure from within, but do not hard-redirect away (no shared
    // navigation chrome). Only redirect the in-app /ai page.
    if (to.path === "/ai") {
      const configured = appState.aiApiKeyConfigured || appState.aiProviderConfigs.length > 0;
      if (!configured) {
        next("/settings");
        return;
      }
    }
  }
  const title = to.meta.title as string | undefined;
  if (title) {
    document.title = `${title} — ${translate("app.name")}`;
  }
  next();
});

export default router;
