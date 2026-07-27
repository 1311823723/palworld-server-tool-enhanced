<script setup>
import { computed, ref, watch } from "vue";
import { useRoute } from "vue-router";
import OperationsShell from "@/components/OperationsShell.vue";
import PlayerList from "@/views/PcHome/component/PlayerList.vue";
import GuildList from "@/views/PcHome/component/GuildList.vue";
import MapView from "@/views/PcHome/component/MapView.vue";
import PlayerProgress from "@/components/PlayerProgress.vue";
import WhitelistPanel from "@/components/WhitelistPanel.vue";
import playerToGuildStore from "@/stores/model/playerToGuild";

const route = useRoute();
const isAdmin = computed(() => Boolean(localStorage.getItem("palworld_token")));
const activeView = ref(route.path === "/world-map" ? "map" : (route.query.tab || "players"));
const navigationStore = playerToGuildStore();
const title = computed(() => ({ players: "玩家", progress: "玩家进度", guilds: "公会", whitelist: "白名单", map: "地图" }[activeView.value]));

watch(
  () => navigationStore.getUpdateStatus(),
  (view) => {
    if (["players", "guilds", "map"].includes(view)) activeView.value = view;
  },
);

watch(
  () => route.path,
  (path) => {
    if (path === "/world-map") activeView.value = "map";
  },
);
</script>

<template>
  <operations-shell :title="title" subtitle="查看玩家、公会和世界地图。点击玩家可以继续查看个人帕鲁与管理操作。">
    <n-tabs v-model:value="activeView" type="line" animated class="view-tabs">
      <n-tab-pane name="players" tab="玩家" />
      <n-tab-pane v-if="isAdmin" name="progress" tab="成长进度" />
      <n-tab-pane name="guilds" tab="公会" />
      <n-tab-pane v-if="isAdmin" name="whitelist" tab="白名单" />
      <n-tab-pane name="map" tab="地图" />
    </n-tabs>
    <section class="legacy-surface" :class="{ 'map-surface': activeView === 'map' }">
      <player-list v-if="activeView === 'players'" />
      <player-progress v-else-if="activeView === 'progress'" />
      <guild-list v-else-if="activeView === 'guilds'" />
      <whitelist-panel v-else-if="activeView === 'whitelist'" />
      <map-view v-else />
    </section>
  </operations-shell>
</template>

<style scoped>
.view-tabs { margin-bottom: 12px; }
.legacy-surface { min-height: 650px; overflow: hidden; border: 1px solid var(--ops-line); border-radius: 14px; background: var(--ops-panel); box-shadow: 0 12px 28px rgba(37, 67, 56, .045); }
.map-surface { height: clamp(600px, calc(100dvh - 230px), 920px); min-height: 600px; }
@media (max-width: 760px) {
  .legacy-surface { min-height: 560px; margin: 0 -12px; border-right: 0; border-left: 0; border-radius: 0; }
  .map-surface { height: clamp(520px, calc(100dvh - 205px), 760px); min-height: 520px; }
}
</style>
