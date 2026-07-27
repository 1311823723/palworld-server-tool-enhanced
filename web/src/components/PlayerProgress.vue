<script setup>
import { computed, onMounted, ref } from "vue";
import dayjs from "dayjs";
import ApiService from "@/service/api";
import { useMessage } from "naive-ui";

const api = new ApiService();
const message = useMessage();
const loading = ref(false);
const query = ref("");
const payload = ref({ items: [], capabilities: {}, parsed_at: null });

const items = computed(() => {
  const keyword = query.value.trim().toLocaleLowerCase("zh-CN");
  if (!keyword) return payload.value.items || [];
  return (payload.value.items || []).filter((item) =>
    `${item.nickname} ${item.player_uid}`.toLocaleLowerCase("zh-CN").includes(keyword),
  );
});
const formatDuration = (seconds) => {
  const value = Number(seconds || 0);
  const days = Math.floor(value / 86400);
  const hours = Math.floor((value % 86400) / 3600);
  const minutes = Math.floor((value % 3600) / 60);
  return days ? `${days} 天 ${hours} 小时` : `${hours} 小时 ${minutes} 分钟`;
};
const valueOrUnknown = (value, suffix = "") =>
  value === null || value === undefined ? "无法解析" : `${Number(value).toLocaleString()}${suffix}`;

async function load() {
  loading.value = true;
  const { data, statusCode } = await api.getPlayerProgress();
  loading.value = false;
  if (statusCode.value !== 200) {
    message.error(data.value?.error || "玩家进度读取失败");
    return;
  }
  payload.value = data.value || payload.value;
}

onMounted(load);
</script>

<template>
  <section class="progress-panel">
    <header class="progress-toolbar">
      <div>
        <span>玩家成长快照</span>
        <strong>{{ items.length }} 名玩家</strong>
        <small>解析于 {{ payload.parsed_at ? dayjs(payload.parsed_at).format("YYYY-MM-DD HH:mm:ss") : "尚未同步" }}</small>
      </div>
      <n-input v-model:value="query" clearable placeholder="搜索玩家名称或 UID" class="search" />
      <n-button :loading="loading" secondary @click="load">刷新进度</n-button>
    </header>

    <n-alert v-if="Object.values(payload.capabilities || {}).every((value) => !value)" type="warning" :bordered="false" class="notice">
      当前玩家存档版本尚未提供可验证的探索进度。等级、帕鲁数量和在线时长仍可正常查看；未知字段不会显示为 0。
    </n-alert>

    <div v-if="items.length" class="progress-list">
      <article v-for="item in items" :key="item.player_uid" class="progress-row">
        <div class="identity">
          <span class="avatar">{{ (item.nickname || "?").slice(0, 1) }}</span>
          <div>
            <strong>{{ item.nickname || "未命名玩家" }} <b>Lv.{{ item.level }}</b></strong>
            <small>{{ item.is_online ? "当前在线" : "当前离线" }} · UID {{ item.player_uid }}</small>
          </div>
        </div>
        <dl>
          <div><dt>帕鲁</dt><dd>{{ item.pal_count }}</dd><small>图鉴 {{ valueOrUnknown(item.progress?.discovered_pals) }} · 捕获 {{ valueOrUnknown(item.progress?.captured_pals) }}</small></div>
          <div><dt>探索</dt><dd>{{ valueOrUnknown(item.progress?.fast_travel_points, " 处传送") }}</dd><small>{{ valueOrUnknown(item.progress?.explored_areas, " 个区域") }}</small></div>
          <div><dt>头目进度</dt><dd>{{ valueOrUnknown(item.progress?.field_bosses) }} / {{ valueOrUnknown(item.progress?.tower_bosses) }}</dd><small>野外头目 / 高塔 · 地下城 {{ valueOrUnknown(item.progress?.dungeons) }}</small></div>
          <div><dt>累计在线</dt><dd>{{ formatDuration(item.total_online_seconds) }}</dd><small>本次 {{ formatDuration(item.current_session_seconds) }}</small></div>
        </dl>
        <div class="detail-strip">
          <span>科技点 <b>{{ valueOrUnknown(item.progress?.technology_points) }}</b></span>
          <span>古代科技点 <b>{{ valueOrUnknown(item.progress?.ancient_technology_points) }}</b></span>
          <span>已解锁配方 <b>{{ valueOrUnknown(item.progress?.recipes) }}</b></span>
          <span>油田通关 <b>{{ valueOrUnknown(item.progress?.oil_rig_clears) }}</b></span>
        </div>
      </article>
    </div>
    <n-empty v-else-if="!loading" description="尚未解析到玩家进度" class="empty" />
    <n-skeleton v-if="loading && !items.length" text :repeat="8" />
  </section>
</template>

<style scoped>
.progress-panel { display: grid; gap: 12px; }
.progress-toolbar { position: sticky; top: 0; z-index: 3; display: grid; grid-template-columns: minmax(220px, 1fr) minmax(220px, 360px) auto; align-items: center; gap: 12px; padding: 14px; border: 1px solid var(--ops-line); background: color-mix(in srgb, var(--ops-panel) 94%, transparent); backdrop-filter: blur(16px); }
.progress-toolbar div { display: grid; grid-template-columns: auto auto; justify-content: start; align-items: baseline; gap: 6px 12px; }
.progress-toolbar span { color: var(--ops-muted); font-size: 12px; }
.progress-toolbar strong { font: 700 18px/1 ui-monospace, monospace; }
.progress-toolbar small { grid-column: 1 / -1; color: var(--ops-muted); font-size: 11px; }
.notice { margin: 0; }
.progress-list { display: grid; gap: 10px; }
.progress-row { padding: 16px; border: 1px solid var(--ops-line); background: var(--ops-panel); transition: border-color .2s, transform .2s; }
.progress-row:hover { border-color: rgba(23,141,121,.32); transform: translateY(-1px); }
.identity { display: flex; align-items: center; gap: 11px; }
.avatar { display: grid; place-items: center; width: 42px; height: 42px; border-radius: 10px; color: var(--ops-accent); background: var(--ops-accent-soft); font-weight: 750; }
.identity strong, .identity small { display: block; }
.identity strong { font-size: 15px; }
.identity strong b { margin-left: 6px; color: var(--ops-accent); font: 650 12px/1 ui-monospace, monospace; }
.identity small, dt, dl small { margin-top: 4px; color: var(--ops-muted); font-size: 11px; }
dl { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; margin: 14px 0 0; }
dl div { min-width: 0; padding: 11px; background: color-mix(in srgb, var(--ops-accent-soft) 58%, transparent); }
dt { margin: 0; }
dd { margin: 5px 0 0; overflow: hidden; text-overflow: ellipsis; color: var(--ops-text); font: 650 15px/1.2 ui-monospace, monospace; white-space: nowrap; }
.detail-strip { display: flex; flex-wrap: wrap; gap: 7px 18px; margin-top: 10px; padding-top: 10px; border-top: 1px solid var(--ops-line); color: var(--ops-muted); font-size: 11px; }
.detail-strip b { color: var(--ops-text); font-variant-numeric: tabular-nums; }
.empty { padding: 70px 0; }
@media (max-width: 760px) {
  .progress-toolbar { position: static; grid-template-columns: 1fr auto; }
  .search { grid-column: 1 / -1; grid-row: 2; }
  dl { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
@media (max-width: 430px) {
  .progress-row { padding: 13px; }
  dl { gap: 6px; }
  dl div { padding: 9px; }
  dd { font-size: 13px; }
}
</style>
