import { createRouter, createWebHistory } from "vue-router";
import { canAccessAdminRoute } from "@/utils/enhancedViews";

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  scrollBehavior() {
    return { top: 0, left: 0 };
  },
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
      path: "/players",
      name: "players",
      component: () => import("@/views/Players.vue"),
    },
    {
      path: "/world-map",
      name: "map",
      component: () => import("@/views/Players.vue"),
    },
    {
      path: "/pal-management",
      name: "pal-management",
      component: () => import("@/views/PalManagement.vue"),
    },
    {
      path: "/base-camps",
      name: "base-camps",
      component: () => import("@/views/BaseCamps.vue"),
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
    {
      path: "/server-operations",
      name: "server-operations",
      meta: { requiresAdmin: true },
      component: () => import("@/views/ServerOperations.vue"),
    },
    {
      path: "/production-orders",
      name: "production-orders",
      meta: { requiresAdmin: true },
      component: () => import("@/views/ProductionOrders.vue"),
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
