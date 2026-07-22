<script setup>
import { computed, ref, watch } from "vue";
import { useRoute } from "vue-router";
import OperationsShell from "@/components/OperationsShell.vue";
import PlayerList from "@/views/PcHome/component/PlayerList.vue";
import GuildList from "@/views/PcHome/component/GuildList.vue";
import MapView from "@/views/PcHome/component/MapView.vue";
import playerToGuildStore from "@/stores/model/playerToGuild";

const route = useRoute();
const activeView = ref(route.path === "/world-map" ? "map" : "players");
const navigationStore = playerToGuildStore();
const title = computed(() => ({ players: "玩家", guilds: "公会", map: "地图" }[activeView.value]));

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
      <n-tab-pane name="guilds" tab="公会" />
      <n-tab-pane name="map" tab="地图" />
    </n-tabs>
    <section class="legacy-surface" :class="{ 'map-surface': activeView === 'map' }">
      <player-list v-if="activeView === 'players'" />
      <guild-list v-else-if="activeView === 'guilds'" />
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
