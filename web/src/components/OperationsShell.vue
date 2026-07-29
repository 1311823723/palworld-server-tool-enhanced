<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { NIcon } from "naive-ui";
import {
  ArchiveOutlined,
  DashboardOutlined,
  MapOutlined,
  PublicRound,
  SupervisedUserCircleRound,
} from "@vicons/material";
import { GameController, Settings } from "@vicons/ionicons5";
import SnapshotStatus from "@/components/SnapshotStatus.vue";

defineProps({
  title: String,
  subtitle: String,
  metadata: { type: Object, default: null },
  loading: Boolean,
});
const emit = defineEmits(["refresh"]);
const route = useRoute();
const router = useRouter();
const isAdmin = ref(false);
const mobileMenuOpen = ref(false);
const connectionIssue = ref(null);

const updateAuth = () => {
  isAdmin.value = Boolean(localStorage.getItem("palworld_token"));
};
const updateConnection = (event) => {
  connectionIssue.value = event.detail?.state === "online" ? null : event.detail;
};
const updateBrowserConnection = () => {
  connectionIssue.value = navigator.onLine
    ? null
    : { state: "offline", message: "设备当前没有网络连接" };
};

onMounted(() => {
  updateAuth();
  window.addEventListener("storage", updateAuth);
  window.addEventListener("pst-auth-changed", updateAuth);
  window.addEventListener("pst-connection-state", updateConnection);
  window.addEventListener("online", updateBrowserConnection);
  window.addEventListener("offline", updateBrowserConnection);
  updateBrowserConnection();
});
onBeforeUnmount(() => {
  window.removeEventListener("storage", updateAuth);
  window.removeEventListener("pst-auth-changed", updateAuth);
  window.removeEventListener("pst-connection-state", updateConnection);
  window.removeEventListener("online", updateBrowserConnection);
  window.removeEventListener("offline", updateBrowserConnection);
});

const navigation = computed(() => [
  { label: "总览", path: "/", icon: DashboardOutlined, public: true },
  { label: "玩家", path: "/players", icon: SupervisedUserCircleRound, public: true },
  { label: "地图", path: "/world-map", icon: MapOutlined, public: true },
  { label: "帕鲁管理", path: "/pal-management", icon: GameController, public: true },
  { label: "据点", path: "/base-camps", icon: PublicRound, public: true },
  { label: "库存", path: "/inventory", icon: ArchiveOutlined, public: false },
  { label: "配种农场", path: "/breeding-farms", icon: GameController, public: false },
  { label: "生产订单", path: "/production-orders", icon: GameController, public: false },
  { label: "服务器运维", path: "/server-operations", icon: GameController, public: false },
  { label: "世界设置", path: "/world-settings", icon: Settings, public: false },
].filter((item) => item.public || isAdmin.value));
const primaryMobileNavigation = computed(() =>
  navigation.value.filter((item) =>
    ["/", "/players", "/world-map", "/pal-management", "/base-camps"].includes(item.path),
  ),
);

const isActive = (path) =>
  path === "/" ? route.path === "/" : route.path === path || route.path.startsWith(`${path}/`);

const goLogin = () => router.push({ path: "/", query: { login: "required" } });
const logout = () => {
  localStorage.removeItem("palworld_token");
  window.dispatchEvent(new Event("pst-auth-changed"));
  router.push("/");
};
</script>

<template>
  <div class="operations-shell">
    <a class="skip-link" href="#main-content">跳到主要内容</a>
    <aside class="operations-sidebar" aria-label="管理中心侧栏">
      <router-link to="/" class="brand" aria-label="返回 PST 总览">
        <span class="brand-mark">P</span>
        <span><strong>PST</strong><small>帕鲁服务器管理中心</small></span>
      </router-link>
      <nav aria-label="管理中心导航">
        <router-link
          v-for="item in navigation"
          :key="item.path"
          :to="item.path"
          :class="{ active: isActive(item.path) }"
        >
          <n-icon :size="18"><component :is="item.icon" /></n-icon>
          <span>{{ item.label }}</span>
        </router-link>
      </nav>
    </aside>

    <div class="operations-main">
      <header class="operations-header">
        <div class="mobile-brand-row">
          <n-button class="mobile-menu-button" quaternary circle size="small" aria-label="打开导航菜单" @click="mobileMenuOpen = !mobileMenuOpen">☰</n-button>
          <router-link to="/" class="mobile-brand"><span class="brand-mark">P</span><strong>PST</strong></router-link>
          <span class="mobile-page-title">{{ title }}</span>
          <n-button quaternary circle size="small" aria-label="刷新当前页面" @click="emit('refresh')">↻</n-button>
        </div>
        <div class="title-block">
          <span class="breadcrumb">管理中心 <b>›</b> {{ title }}</span>
          <h1>{{ title }}</h1>
          <p>{{ subtitle }}</p>
        </div>
        <div class="header-actions">
          <slot name="header-actions" />
          <n-button v-if="!isAdmin" size="small" secondary @click="goLogin">管理员登录</n-button>
          <n-button v-else size="small" quaternary @click="logout">退出管理</n-button>
        </div>
      </header>
      <transition name="mobile-menu">
        <div v-if="mobileMenuOpen" class="mobile-menu-layer" @click.self="mobileMenuOpen = false">
          <nav class="mobile-drawer" aria-label="移动端完整导航">
            <div class="mobile-drawer-header"><strong>管理中心</strong><n-button quaternary circle size="small" aria-label="关闭导航菜单" @click="mobileMenuOpen = false">×</n-button></div>
            <router-link v-for="item in navigation" :key="item.path" :to="item.path" :class="{ active: isActive(item.path) }" @click="mobileMenuOpen = false">
              <n-icon :size="18"><component :is="item.icon" /></n-icon><span>{{ item.label }}</span>
            </router-link>
          </nav>
        </div>
      </transition>
      <n-alert
        v-if="connectionIssue"
        type="error"
        class="connection-alert"
        :title="connectionIssue.state === 'timeout' ? 'PST 响应超时' : 'PST 连接中断'"
      >
        {{ connectionIssue.message }}。确认 PST 正在运行后重试。
        <template #action><n-button size="small" secondary @click="emit('refresh')">重试</n-button></template>
      </n-alert>
      <snapshot-status
        v-if="metadata"
        :metadata="metadata"
        :loading="loading"
        class="status-bar"
        @refresh="emit('refresh')"
      />
      <main id="main-content" class="operations-content"><slot /></main>
    </div>

    <nav class="mobile-bottom-nav" aria-label="移动端导航">
      <router-link v-for="item in primaryMobileNavigation" :key="item.path" :to="item.path" :class="{ active: isActive(item.path) }">
        <n-icon :size="20"><component :is="item.icon" /></n-icon>
        <span>{{ item.label }}</span>
      </router-link>
      <button type="button" :class="{ active: mobileMenuOpen }" aria-label="打开更多功能" @click="mobileMenuOpen = true">
        <n-icon :size="20"><Settings /></n-icon><span>更多</span>
      </button>
    </nav>
  </div>
</template>

<style scoped>
.operations-shell {
  --ops-bg: #eef4f1;
  --ops-panel: #fff;
  --ops-text: #17221d;
  --ops-muted: #6c7b74;
  --ops-line: rgba(38, 74, 61, .15);
  --ops-accent: #178d79;
  --ops-accent-soft: #e3f2ec;
  min-height: 100dvh;
  color: var(--ops-text);
  background-color: var(--ops-bg);
  background-image: linear-gradient(rgba(23, 141, 121, .045) 1px, transparent 1px), linear-gradient(90deg, rgba(23, 141, 121, .045) 1px, transparent 1px);
  background-size: 28px 28px;
}
.skip-link { position: fixed; top: 8px; left: 8px; z-index: 100; padding: 8px 12px; color: #fff; background: var(--ops-accent); transform: translateY(-150%); transition: transform .18s; }
.skip-link:focus { transform: translateY(0); }
.operations-sidebar { position: fixed; inset: 0 auto 0 0; z-index: 20; display: flex; flex-direction: column; width: 228px; padding: 22px 14px 18px; border-right: 1px solid var(--ops-line); background: rgba(255,255,255,.92); backdrop-filter: blur(18px); overflow-y: auto; overscroll-behavior: contain; }
.brand, .mobile-brand { display: flex; align-items: center; gap: 11px; color: inherit; }
.brand { padding: 4px 8px 25px; }
.brand-mark { display: grid; place-items: center; flex: 0 0 auto; width: 38px; height: 38px; border: 1px solid rgba(23,141,121,.25); border-radius: 11px; color: #fff; background: var(--ops-accent); font: 700 20px/1 ui-monospace, monospace; box-shadow: 0 8px 18px rgba(23,141,121,.18); }
.brand strong, .brand small { display: block; }
.brand strong { font-size: 16px; letter-spacing: -.02em; }
.brand small { margin-top: 3px; color: var(--ops-muted); font-size: 11px; }
nav { display: grid; gap: 5px; }
nav a { display: flex; align-items: center; gap: 10px; min-height: 42px; padding: 0 12px; border-radius: 9px; color: var(--ops-muted); font-size: 14px; font-weight: 550; transition: color .2s, background .2s, transform .2s; }
nav a:hover { color: var(--ops-text); background: rgba(23,141,121,.08); transform: translateX(2px); }
nav a:active { transform: translateX(2px) scale(.98); }
nav a:focus-visible { outline: 2px solid var(--ops-accent); outline-offset: 2px; }
nav a.active { color: var(--ops-accent); background: var(--ops-accent-soft); font-weight: 650; }
.sidebar-note { margin-top: auto; padding: 13px; border-radius: 10px; background: var(--ops-accent-soft); }
.sidebar-note span, .sidebar-note strong, .sidebar-note small { display: block; }
.sidebar-note span { color: var(--ops-muted); font-size: 10px; letter-spacing: .08em; }
.sidebar-note strong { margin-top: 5px; font-size: 13px; }
.sidebar-note small { margin-top: 4px; color: var(--ops-muted); font-size: 11px; line-height: 1.45; }
.operations-main { min-height: 100dvh; margin-left: 228px; }
.operations-header { display: flex; align-items: flex-end; justify-content: space-between; gap: 24px; max-width: 1420px; margin: 0 auto; padding: 35px clamp(20px, 4vw, 52px) 18px; }
.title-block { min-width: 0; }
.breadcrumb { display: inline-flex; align-items: center; gap: 8px; color: var(--ops-muted); font-size: 12px; font-weight: 600; }
.breadcrumb b { color: var(--ops-accent); font-size: 18px; font-weight: 400; }
h1 { margin: 8px 0 6px; font-size: clamp(28px, 3vw, 42px); line-height: 1.04; letter-spacing: -.045em; text-wrap: balance; }
p { max-width: 65ch; margin: 0; color: var(--ops-muted); font-size: 14px; line-height: 1.65; text-wrap: pretty; }
.header-actions { display: flex; align-items: center; gap: 8px; flex: 0 0 auto; }
.status-bar { max-width: calc(1420px - clamp(40px, 8vw, 104px)); margin: 0 auto; }
.connection-alert { max-width: calc(1420px - clamp(40px, 8vw, 104px)); margin: 0 auto 12px; }
.operations-content { max-width: 1420px; margin: 0 auto; padding: 18px clamp(20px, 4vw, 52px) 64px; }
.mobile-brand-row, .mobile-bottom-nav { display: none; }
:global(.operations-shell .n-card) { border-color: var(--ops-line); border-radius: 13px; box-shadow: 0 12px 28px rgba(37, 67, 56, .045); }
:global(.operations-shell .n-button) { transition: transform .18s, box-shadow .18s, background-color .18s; }
:global(.operations-shell .n-button:active) { transform: scale(.98); }
@media (prefers-color-scheme: dark) {
  .operations-shell { --ops-bg: #101915; --ops-panel: #18221d; --ops-text: #edf5f0; --ops-muted: #9eafa6; --ops-line: rgba(205,232,220,.12); --ops-accent-soft: rgba(61,146,121,.16); background-image: linear-gradient(rgba(110,180,157,.04) 1px, transparent 1px), linear-gradient(90deg, rgba(110,180,157,.04) 1px, transparent 1px); }
  .operations-sidebar { background: rgba(24,34,29,.94); }
}
@media (max-width: 860px) {
  .operations-sidebar { position: sticky; inset: 0 0 auto; width: 100%; height: auto; padding: 0; border-right: 0; border-bottom: 1px solid var(--ops-line); }
  .operations-sidebar > .brand, .operations-sidebar > nav, .sidebar-note { display: none; }
  .operations-main { margin-left: 0; }
  .operations-header { display: block; padding: 0 14px 12px; }
  .mobile-brand-row { display: grid; grid-template-columns: auto auto minmax(0, 1fr) auto; align-items: center; gap: 9px; min-height: 58px; border-bottom: 1px solid var(--ops-line); }
  .mobile-menu-button { color: var(--ops-text); font-size: 20px; }
  .mobile-brand .brand-mark { width: 30px; height: 30px; border-radius: 9px; font-size: 16px; }
  .mobile-brand strong { font-size: 16px; }
  .mobile-page-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: center; font-size: 15px; font-weight: 650; }
  .title-block { padding: 16px 2px 0; }
  .breadcrumb { font-size: 11px; }
  h1 { display: none; }
  p { font-size: 13px; line-height: 1.5; }
  .header-actions { justify-content: flex-end; margin-top: 12px; }
  .status-bar { margin: 0 14px; }
  .connection-alert { margin: 0 14px 10px; }
  .operations-content { padding: 14px 12px 82px; }
  .mobile-bottom-nav { position: fixed; inset: auto 0 0; z-index: 30; display: flex; align-items: stretch; gap: 2px; min-height: 68px; padding: 6px max(4px, env(safe-area-inset-left)) max(6px, env(safe-area-inset-bottom)); border-top: 1px solid var(--ops-line); background: rgba(255,255,255,.96); backdrop-filter: blur(18px); overflow-x: auto; }
  .mobile-bottom-nav a, .mobile-bottom-nav button { display: flex; flex: 1 0 54px; flex-direction: column; justify-content: center; align-items: center; gap: 3px; min-height: 52px; padding: 3px 4px; border: 0; border-radius: 8px; color: var(--ops-muted); background: transparent; font: inherit; font-size: 10px; white-space: nowrap; }
  .mobile-bottom-nav a.active, .mobile-bottom-nav button.active { color: var(--ops-accent); background: var(--ops-accent-soft); }
  .mobile-bottom-nav a:focus-visible, .mobile-bottom-nav button:focus-visible { outline: 2px solid var(--ops-accent); outline-offset: -2px; }
  .mobile-menu-layer { position: fixed; inset: 0; z-index: 40; background: rgba(20,33,27,.24); }
  .mobile-drawer { position: absolute; inset: 0 auto 0 0; display: flex; flex-direction: column; gap: 5px; width: min(78vw, 300px); padding: 15px 12px; border-right: 1px solid var(--ops-line); background: var(--ops-panel); box-shadow: 14px 0 32px rgba(22,52,41,.18); overflow-y: auto; overscroll-behavior: contain; -webkit-overflow-scrolling: touch; }
  .mobile-drawer-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 42px; padding: 0 8px 10px; border-bottom: 1px solid var(--ops-line); }
  .mobile-drawer a { display: flex; align-items: center; gap: 11px; min-height: 42px; padding: 0 12px; border-radius: 9px; color: var(--ops-muted); font-size: 14px; }
  .mobile-drawer a.active { color: var(--ops-accent); background: var(--ops-accent-soft); font-weight: 650; }
  .mobile-menu-enter-active, .mobile-menu-leave-active { transition: opacity .18s ease; }
  .mobile-menu-enter-active .mobile-drawer, .mobile-menu-leave-active .mobile-drawer { transition: transform .2s ease; }
  .mobile-menu-enter-from, .mobile-menu-leave-to { opacity: 0; }
  .mobile-menu-enter-from .mobile-drawer, .mobile-menu-leave-to .mobile-drawer { transform: translateX(-100%); }
}
</style>
