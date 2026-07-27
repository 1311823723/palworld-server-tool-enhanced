<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useMessage } from "naive-ui";
import { useRoute } from "vue-router";
import dayjs from "dayjs";
import ApiService from "@/service/api";
import OperationsShell from "@/components/OperationsShell.vue";
import ServerProcessCard from "@/components/ServerProcessCard.vue";
import FirstRunSetup from "@/components/FirstRunSetup.vue";
import ConfigManager from "@/components/ConfigManager.vue";
import RconManager from "@/components/RconManager.vue";
import BackupManager from "@/components/BackupManager.vue";
import BroadcastComposer from "@/components/BroadcastComposer.vue";
import ShutdownDialog from "@/components/ShutdownDialog.vue";
import WhitelistManager from "@/components/WhitelistManager.vue";

const route = useRoute();
const api = new ApiService();
const message = useMessage();
const isAdmin = ref(Boolean(localStorage.getItem("palworld_token")));
const initialized = ref(false);
const configReady = ref(false);
const loading = ref(false);
const showLogin = ref(route.query.login === "required");
const showConfig = ref(false);
const showRcon = ref(false);
const showBackup = ref(false);
const showBroadcast = ref(false);
const showShutdown = ref(false);
const showWhitelist = ref(false);
const password = ref("");
const serverInfo = ref({});
const serverMetrics = ref({});
const players = ref([]);
const onlinePlayers = ref([]);
const bases = ref([]);
const toolInfo = ref({});
const metadata = ref({ warnings: [] });
let refreshTimer;

const syncAuth = () => {
  isAdmin.value = Boolean(localStorage.getItem("palworld_token"));
};
const asArray = (value) => (Array.isArray(value) ? value : []);
const errorText = (data, fallback) => data?.error || fallback;

const onlineCount = computed(() => Number(serverMetrics.value.current_player_num ?? onlinePlayers.value.length ?? 0));
const maxPlayers = computed(() => serverMetrics.value.max_player_num ?? "—");
const attentionCount = computed(() => bases.value.reduce((sum, base) => sum + Number(base.hungry_pal_count || 0) + Number(base.low_sanity_pal_count || 0) + Number(base.sick_pal_count || 0) + Number(base.down_pal_count || 0), 0));
const uptimeText = computed(() => {
  const seconds = Number(serverMetrics.value.uptime || 0);
  if (!seconds) return "—";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return days ? `${days} 天 ${hours} 小时` : `${hours} 小时 ${minutes} 分钟`;
});
const serverStatus = computed(() => serverInfo.value?.name ? "运行中" : "等待连接");
const formatTime = (value) => value ? dayjs(value).format("MM月DD日 HH:mm") : "—";

async function loadData() {
  loading.value = true;
  const [server, metrics, playerList, onlineList, baseList, tool] = await Promise.all([
    api.getServerInfo(),
    api.getServerMetrics(),
    api.getPlayerList({ order_by: "last_online", desc: true }),
    api.getOnlinePlayerList(),
    api.getBaseCamps(),
    api.getServerToolInfo(),
  ]);
  loading.value = false;
  if (server.statusCode.value === 200) serverInfo.value = server.data.value || {};
  if (metrics.statusCode.value === 200) serverMetrics.value = metrics.data.value || {};
  if (playerList.statusCode.value === 200) players.value = asArray(playerList.data.value);
  if (onlineList.statusCode.value === 200) onlinePlayers.value = asArray(onlineList.data.value);
  if (baseList.statusCode.value === 200) {
    bases.value = asArray(baseList.data.value?.items);
    metadata.value = baseList.data.value?.metadata || metadata.value;
  }
  if (tool.statusCode.value === 200) toolInfo.value = tool.data.value || {};
  if (server.statusCode.value >= 500 || baseList.statusCode.value >= 500) message.error(errorText(server.data.value, "服务器数据暂时无法读取"));
}

async function loadConfigStatus() {
  const { data, statusCode } = await api.getConfigStatus();
  initialized.value = statusCode.value === 200 && data.value?.initialized === true;
  configReady.value = true;
}

async function handleLogin() {
  const { data, statusCode } = await api.login({ password: password.value });
  if (statusCode.value !== 200 || !data.value?.token) {
    message.error(statusCode.value === 401 ? "管理员密码不正确" : "登录失败，请稍后重试");
    password.value = "";
    return;
  }
  localStorage.setItem("palworld_token", data.value.token);
  window.dispatchEvent(new Event("pst-auth-changed"));
  syncAuth();
  showLogin.value = false;
  password.value = "";
  message.success("已进入管理模式");
  await loadData();
}

const handleInitialized = () => {
  initialized.value = true;
  showConfig.value = true;
};

watch(() => route.query.login, (value) => { showLogin.value = value === "required"; });
onMounted(async () => {
  window.addEventListener("pst-auth-changed", syncAuth);
  await loadConfigStatus();
  if (initialized.value) {
    await loadData();
    refreshTimer = window.setInterval(loadData, 30000);
  }
});
onBeforeUnmount(() => {
  window.removeEventListener("pst-auth-changed", syncAuth);
  window.clearInterval(refreshTimer);
});
</script>

<template>
  <div v-if="configReady">
    <operations-shell title="总览" subtitle="查看服务器运行状态、在线玩家、世界数据与最近一次存档解析结果。" :metadata="metadata" :loading="loading" @refresh="loadData">
      <template #header-actions>
        <n-button v-if="isAdmin" type="primary" secondary @click="showConfig = true">打开配置</n-button>
      </template>

      <section class="server-hero">
        <div class="hero-icon">▤</div>
        <div class="hero-copy">
          <span>PALWORLD SERVER</span>
          <h2>{{ serverInfo.name || "Palworld Dedicated Server" }}</h2>
          <p><i :class="{ online: serverInfo.name }" />{{ serverStatus }} · {{ serverInfo.version || "版本信息等待连接" }}</p>
        </div>
        <div class="hero-meta"><small>PST 版本</small><strong>{{ toolInfo.version || "开发版" }}</strong></div>
      </section>

      <n-grid cols="1 620:3" :x-gap="12" :y-gap="12" class="stat-grid">
        <n-gi><article class="metric-tile"><span class="metric-icon">♙</span><div><small>在线玩家</small><strong>{{ onlineCount }}<em>/ {{ maxPlayers }}</em></strong></div></article></n-gi>
        <n-gi><article class="metric-tile"><span class="metric-icon">◷</span><div><small>服务器运行时间</small><strong class="metric-wide">{{ uptimeText }}</strong></div></article></n-gi>
        <n-gi><article class="metric-tile"><span class="metric-icon warning-icon">♡</span><div><small>需要关注的工作帕鲁</small><strong>{{ attentionCount }}</strong></div></article></n-gi>
      </n-grid>

      <div v-if="isAdmin" class="process-card"><server-process-card :is-admin="isAdmin" /></div>
      <n-card v-else class="visitor-process-card" size="small">
        <div class="visitor-process-copy"><strong>服务器进程状态</strong><span>登录管理员账号后可查看进程、守护和启停控制。</span></div>
        <n-button type="primary" secondary @click="showLogin = true">管理员登录</n-button>
      </n-card>

      <n-grid cols="1 760:2" :x-gap="14" :y-gap="14" class="content-grid">
        <n-gi>
          <section class="panel activity-panel">
            <header><div><span class="section-kicker">LIVE SNAPSHOT</span><h3>当前在线玩家</h3></div><router-link to="/players">查看全部</router-link></header>
            <n-list v-if="onlinePlayers.length" :show-divider="true">
              <n-list-item v-for="player in onlinePlayers.slice(0, 6)" :key="player.player_uid">
                <div class="player-line"><span class="player-avatar">{{ (player.nickname || "?").slice(0, 1) }}</span><div><strong>{{ player.nickname || "未命名玩家" }}</strong><small>Lv.{{ player.level || 0 }} · {{ player.user_id?.split("_")[0] || "未知平台" }}</small></div><span class="online-dot">在线</span></div>
              </n-list-item>
            </n-list>
            <n-empty v-else description="当前没有在线玩家" />
          </section>
        </n-gi>
        <n-gi>
          <section class="panel snapshot-panel">
            <header><div><span class="section-kicker">WORLD DATA</span><h3>世界数据</h3></div><router-link to="/base-camps">据点管理</router-link></header>
            <div class="snapshot-summary"><strong>{{ bases.length }}</strong><span>个据点</span><strong>{{ players.length }}</strong><span>名已记录玩家</span></div>
            <div class="snapshot-row"><span>最近一次存档解析</span><b>{{ formatTime(metadata.snapshot_time) }}</b></div>
            <div class="snapshot-row"><span>下次同步间隔</span><b>每 {{ metadata.sync_interval_seconds || 120 }} 秒</b></div>
            <p class="muted-copy">数据来自最近一次存档解析，并非游戏内实时遥测。</p>
          </section>
        </n-gi>
      </n-grid>

      <section v-if="isAdmin" class="admin-tools panel">
        <header><div><span class="section-kicker">ADMIN TOOLS</span><h3>管理工具</h3></div><span class="muted-copy">危险操作仍受管理员 JWT 保护</span></header>
        <div class="tool-grid">
          <n-button secondary @click="showBroadcast = true">游戏内广播</n-button>
          <n-button secondary @click="showShutdown = true">平滑关服</n-button>
          <n-button secondary tag="a" href="/server-operations?tab=backup">服务器运维</n-button>
          <n-button secondary tag="a" href="/players?tab=whitelist">白名单</n-button>
          <n-button secondary tag="a" href="/server-operations?tab=rcon">RCON 工具</n-button>
        </div>
      </section>
    </operations-shell>

    <first-run-setup :show="!initialized" @initialized="handleInitialized" />
    <config-manager v-model:show="showConfig" />
    <n-modal v-model:show="showLogin" preset="card" title="管理员登录" style="width:min(420px,92vw)">
      <p class="login-help">登录后才能查看进程状态并执行服务器控制操作。</p>
      <n-input v-model:value="password" type="password" show-password-on="click" placeholder="输入 PST 管理员密码" autocomplete="current-password" @keyup.enter="handleLogin" />
      <template #footer><n-space justify="end"><n-button @click="showLogin = false">取消</n-button><n-button type="primary" :loading="loading" @click="handleLogin">登录</n-button></n-space></template>
    </n-modal>
    <rcon-manager v-model:show="showRcon" />
    <backup-manager v-model:show="showBackup" />
    <broadcast-composer v-model:show="showBroadcast" />
    <shutdown-dialog v-model:show="showShutdown" />
    <whitelist-manager v-model:show="showWhitelist" :players="players" />
  </div>
  <div v-else class="app-loading"><n-spin size="large" /></div>
</template>

<style scoped>
.server-hero { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 16px; padding: 23px 25px; border: 1px solid rgba(23,141,121,.28); border-left: 4px solid var(--ops-accent); background: rgba(255,255,255,.88); box-shadow: 0 16px 32px rgba(37,67,56,.06); }
.hero-icon { display: grid; place-items: center; width: 62px; height: 62px; border: 1px solid rgba(23,141,121,.25); border-radius: 14px; color: var(--ops-accent); background: var(--ops-accent-soft); font-size: 29px; }
.hero-copy > span, .section-kicker { color: var(--ops-accent); font: 700 10px/1.3 ui-monospace, monospace; letter-spacing: .1em; }
.hero-copy h2 { margin: 5px 0 5px; font-size: clamp(22px, 3vw, 31px); letter-spacing: -.04em; }
.hero-copy p { display: flex; align-items: center; gap: 7px; font-size: 13px; }
.hero-copy i { width: 8px; height: 8px; border-radius: 50%; background: #a5b0ab; }
.hero-copy i.online, .online-dot::before { background: #1aa47e; }
.hero-meta { display: grid; gap: 5px; min-width: 88px; text-align: right; }
.hero-meta small, .metric-tile small, .player-line small { color: var(--ops-muted); font-size: 12px; }
.hero-meta strong { font: 650 14px/1.2 ui-monospace, monospace; }
.stat-grid { margin-top: 14px; }
.metric-tile { display: flex; align-items: center; gap: 13px; min-height: 90px; padding: 16px; border: 1px solid var(--ops-line); background: rgba(255,255,255,.88); }
.metric-icon { display: grid; place-items: center; width: 38px; height: 38px; border: 1px solid rgba(23,141,121,.18); border-radius: 10px; color: var(--ops-accent); background: var(--ops-accent-soft); font-size: 22px; }
.warning-icon { color: #ae7040; background: #f7eee5; }
.metric-tile div { min-width: 0; }
.metric-tile small, .metric-tile strong { display: block; }
.metric-tile strong { margin-top: 5px; font: 700 24px/1 ui-monospace, monospace; font-variant-numeric: tabular-nums; }
.metric-tile strong em { color: var(--ops-muted); font-size: 13px; font-style: normal; font-weight: 500; }
.metric-wide { font-size: 18px !important; }
.process-card, .visitor-process-card { margin-top: 14px; }
.visitor-process-card { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.visitor-process-copy strong, .visitor-process-copy span { display: block; }
.visitor-process-copy span, .muted-copy, .login-help { color: var(--ops-muted); font-size: 13px; line-height: 1.55; }
.content-grid { margin-top: 14px; }
.panel { padding: 18px; border: 1px solid var(--ops-line); background: rgba(255,255,255,.9); }
.panel header { display: flex; align-items: flex-start; justify-content: space-between; gap: 14px; margin-bottom: 12px; }
.panel h3 { margin-top: 5px; font-size: 18px; letter-spacing: -.02em; }
.panel header a { color: var(--ops-accent); font-size: 12px; font-weight: 650; }
.player-line { display: flex; align-items: center; gap: 10px; width: 100%; }
.player-avatar { display: grid; place-items: center; width: 32px; height: 32px; border-radius: 9px; color: var(--ops-accent); background: var(--ops-accent-soft); font-weight: 700; }
.player-line div { min-width: 0; flex: 1; }
.player-line strong, .player-line small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.player-line strong { font-size: 14px; }
.online-dot { color: var(--ops-accent); font-size: 12px; }
.online-dot::before { display: inline-block; width: 6px; height: 6px; margin-right: 5px; border-radius: 50%; content: ""; }
.snapshot-summary { display: grid; grid-template-columns: auto 1fr auto 1fr; align-items: baseline; gap: 8px; padding: 13px; background: var(--ops-accent-soft); }
.snapshot-summary strong { font: 700 22px/1 ui-monospace, monospace; }
.snapshot-summary span { color: var(--ops-muted); font-size: 12px; }
.snapshot-row { display: flex; justify-content: space-between; gap: 10px; padding: 13px 0; border-bottom: 1px solid var(--ops-line); font-size: 13px; }
.snapshot-row b { font-variant-numeric: tabular-nums; }
.snapshot-panel .muted-copy { margin-top: 13px; }
.admin-tools { margin-top: 14px; }
.tool-grid { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 9px; }
.login-help { margin-bottom: 15px; }
.app-loading { display: grid; place-items: center; min-height: 100dvh; }
@media (max-width: 760px) { .server-hero { grid-template-columns: auto minmax(0, 1fr); padding: 18px; } .hero-meta { grid-column: 2; text-align: left; } .hero-icon { width: 52px; height: 52px; } .tool-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .panel { padding: 14px; } }
@media (max-width: 430px) { .server-hero { gap: 11px; } .hero-copy h2 { font-size: 20px; } .metric-wide { font-size: 16px !important; } .visitor-process-card { align-items: flex-start; flex-direction: column; } }
</style>
