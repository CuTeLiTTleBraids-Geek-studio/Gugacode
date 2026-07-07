<script setup lang="ts">
import { computed, onErrorCaptured } from "vue";
import { useRoute } from "vue-router";
import MainLayout from "@/components/layout/MainLayout.vue";
import { notifyError } from "@/lib/notifications";
import { pushOutput } from "@/stores/output";

const route = useRoute();
const hideLayout = computed(() => route.meta.hideLayout === true);

// N-117 / Proposal AD: Error boundary. Catches errors thrown by child
// components' render, setup, lifecycle, and event handlers. Without this,
// a render error in any router view crashes the entire app (Vue unmounts
// the component tree). Returning false stops the error from propagating
// to app.config.errorHandler, but we still log it there via pushOutput —
// so we return true to let the global handler also see it.
onErrorCaptured((err, _instance, info) => {
  const msg = err instanceof Error ? err.message : String(err);
  console.error("[App onErrorCaptured]", err, info);
  pushOutput("ide", "error", `View error (${info}): ${msg}`);
  try {
    notifyError(`${msg}`, "View error");
  } catch {
    // notification may fail during early startup
  }
  // Return false to prevent the error from propagating further up (which
  // would crash the app). The error is already logged + notified above.
  return false;
});
</script>

<template>
  <component :is="hideLayout ? 'div' : MainLayout">
    <router-view v-slot="{ Component }">
      <transition name="page-fade" mode="out-in">
        <component :is="Component" :key="route.path" />
      </transition>
    </router-view>
  </component>
</template>

<style scoped>
.page-fade-enter-active,
.page-fade-leave-active {
  transition: opacity 180ms cubic-bezier(0.4, 0, 0.2, 1);
}
.page-fade-enter-from,
.page-fade-leave-to {
  opacity: 0;
}
</style>
