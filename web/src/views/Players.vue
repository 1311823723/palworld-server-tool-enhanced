<script setup>
import { computed, ref } from "vue";
import OperationsShell from "@/components/OperationsShell.vue";
import PlayerList from "@/views/PcHome/component/PlayerList.vue";
import GuildList from "@/views/PcHome/component/GuildList.vue";
import MapView from "@/views/PcHome/component/MapView.vue";

const activeView = ref("players");
const title = computed(() => ({ players: "玩家", guilds: "公会", map: "地图" }[activeView.value]));
</script>

<template>
  <operations-shell :title="title" subtitle="查看玩家、公会和世界地图。点击玩家可以继续查看个人帕鲁与管理操作。">
    <n-tabs v-model:value="activeView" type="line" animated class="view-tabs">
      <n-tab-pane name="players" tab="玩家" />
      <n-tab-pane name="guilds" tab="公会" />
      <n-tab-pane name="map" tab="地图" />
    </n-tabs>
    <section class="legacy-surface">
      <player-list v-if="activeView === 'players'" />
      <guild-list v-else-if="activeView === 'guilds'" />
      <map-view v-else />
    </section>
  </operations-shell>
</template>

<style scoped>
.view-tabs { margin-bottom: 12px; }
.legacy-surface { min-height: 650px; overflow: hidden; border: 1px solid var(--ops-line); background: var(--ops-panel); }
@media (max-width: 760px) { .legacy-surface { min-height: 560px; margin: 0 -12px; border-right: 0; border-left: 0; } }
</style>
