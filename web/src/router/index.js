import { createRouter, createWebHistory } from "vue-router";
import { canAccessAdminRoute } from "@/utils/enhancedViews";

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: "/",
      name: "home",
      component: () => import("@/views/Home.vue"),
    },
    {
      path: "/work-pals",
      name: "work-pals",
      component: () => import("@/views/WorkPals.vue"),
    },
    {
      path: "/inventory",
      name: "inventory",
      meta: { requiresAdmin: true },
      component: () => import("@/views/Inventory.vue"),
    },
    {
      path: "/world-settings",
      name: "world-settings",
      meta: { requiresAdmin: true },
      component: () => import("@/views/WorldSettings.vue"),
    },
    {
      path: "/breeding-farms",
      name: "breeding-farms",
      meta: { requiresAdmin: true },
      component: () => import("@/views/BreedingFarms.vue"),
    },
  ],
});

router.beforeEach((to) => {
  if (to.meta.requiresAdmin && !canAccessAdminRoute(localStorage)) {
    return { name: "home", query: { login: "required" } };
  }
  return true;
});

export default router;
