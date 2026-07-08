import { createRouter, createWebHashHistory, type RouteRecordRaw } from "vue-router";
import { translate } from "@/lib/i18n";

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
];

const router = createRouter({
  history: createWebHashHistory(),
  routes,
});

router.beforeEach((to, _from, next) => {
  const title = to.meta.title as string | undefined;
  if (title) {
    document.title = `${title} — ${translate("app.name")}`;
  }
  next();
});

export default router;
