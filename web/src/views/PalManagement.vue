<script setup>
import OperationsShell from "@/components/OperationsShell.vue";
import PlayerList from "@/views/PcHome/component/PlayerList.vue";
</script>

<template>
  <operations-shell title="帕鲁管理" subtitle="先选择玩家，再查看其持有的帕鲁、等级、技能和详情。名称优先使用中文资源。">
    <n-alert type="info" :bordered="false" class="notice">
      当前帕鲁数据来自最近一次存档解析；不会修改游戏存档。
    </n-alert>
    <section class="legacy-surface"><player-list /></section>
  </operations-shell>
</template>

<style scoped>
.notice { margin-bottom: 12px; }
.legacy-surface { min-height: 650px; overflow: hidden; border: 1px solid var(--ops-line); background: var(--ops-panel); }
@media (max-width: 760px) { .legacy-surface { min-height: 560px; margin: 0 -12px; border-right: 0; border-left: 0; } }
</style>
