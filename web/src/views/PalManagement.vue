<script setup>
import OperationsShell from "@/components/OperationsShell.vue";
import PlayerList from "@/views/PcHome/component/PlayerList.vue";
</script>

<template>
  <operations-shell title="帕鲁管理" subtitle="选择玩家，查看他持有的帕鲁、等级和技能。">
    <section class="legacy-surface"><player-list /></section>
  </operations-shell>
</template>

<style scoped>
.legacy-surface { min-height: 650px; overflow: hidden; border: 1px solid var(--ops-line); background: var(--ops-panel); }
@media (max-width: 760px) { .legacy-surface { min-height: 560px; margin: 0 -12px; border-right: 0; border-left: 0; } }
</style>
